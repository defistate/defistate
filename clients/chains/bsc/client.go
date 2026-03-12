package bsc

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	clients "github.com/defistate/defistate/clients"
	"github.com/defistate/defistate/engine"
	tokenregistry "github.com/defistate/defistate/protocols/erc20-token-system"
	poolregistry "github.com/defistate/defistate/protocols/pool-registry"
	uniswapv2 "github.com/defistate/defistate/protocols/uniswap-v2"
	uniswapv3 "github.com/defistate/defistate/protocols/uniswap-v3"
	jsonrpcclient "github.com/defistate/defistate/streams/jsonrpc/client"
	stateops "github.com/defistate/defistate/streams/jsonrpc/stateops/chains/bsc"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"golang.org/x/sync/errgroup"

	poolregistryindexer "github.com/defistate/defistate/clients/indexers/poolregistry"
	tokenregistryindexer "github.com/defistate/defistate/clients/indexers/token"
	uniswapv2indexer "github.com/defistate/defistate/clients/indexers/uniswapv2"
	uniswapv3indexer "github.com/defistate/defistate/clients/indexers/uniswapv3"
)

// State is the high-level, indexed and graphed representation of the DeFi state.
type State struct {
	Tokens           clients.IndexedTokenSystem
	Pools            clients.IndexedPoolRegistry
	UniswapV2        clients.IndexedUniswapV2
	UniswapV3        clients.IndexedUniswapV3
	PancakeswapV2    clients.IndexedUniswapV2
	PancakeswapV3    clients.IndexedUniswapV3
	ProtocolResolver *clients.ProtocolResolver
	Graph            *poolregistry.TokenPoolsRegistryView
	Block            engine.BlockSummary
	Timestamp        uint64
}

// IndexerConfig groups all protocol indexers for structured initialization.
type IndexerConfig struct {
	Token        clients.TokenIndexer
	PoolRegistry clients.PoolRegistryIndexer
	UniswapV2    clients.UniswapV2Indexer
	UniswapV3    clients.UniswapV3Indexer
}

// Config defines the required dependencies for the BSC client.
type Config struct {
	Client            clients.Client
	Logger            clients.Logger
	Indexers          IndexerConfig
	MetricsRegisterer prometheus.Registerer
}

var (
	ErrClientRequired              = errors.New("config validation failed: Client is required")
	ErrLoggerRequired              = errors.New("config validation failed: Logger is required")
	ErrTokenIndexerRequired        = errors.New("config validation failed: TokenIndexer is required")
	ErrPoolRegistryIndexerRequired = errors.New("config validation failed: PoolRegistryIndexer is required")
	ErrUniswapV2IndexerRequired    = errors.New("config validation failed: UniswapV2Indexer is required")
	ErrUniswapV3IndexerRequired    = errors.New("config validation failed: UniswapV3Indexer is required")
	ErrMetricsRegistererRequired   = errors.New("config validation failed: MetricsRegisterer is required")
)

const (
	UniswapV2ProtocolID     = engine.ProtocolID("uniswap-v2-bsc-mainnet")
	UniswapV3ProtocolID     = engine.ProtocolID("uniswap-v3-bsc-mainnet")
	PancakeswapV2ProtocolID = engine.ProtocolID("pancakeswap-v2-bsc-mainnet")
	PancakeswapV3ProtocolID = engine.ProtocolID("pancakeswap-v3-bsc-mainnet")
)

func (i *IndexerConfig) Validate() error {
	if i.Token == nil {
		return ErrTokenIndexerRequired
	}
	if i.PoolRegistry == nil {
		return ErrPoolRegistryIndexerRequired
	}
	if i.UniswapV2 == nil {
		return ErrUniswapV2IndexerRequired
	}
	if i.UniswapV3 == nil {
		return ErrUniswapV3IndexerRequired
	}
	return nil
}

func (c *Config) Validate() error {
	if c.Client == nil {
		return ErrClientRequired
	}
	if c.Logger == nil {
		return ErrLoggerRequired
	}
	if c.MetricsRegisterer == nil {
		return ErrMetricsRegistererRequired
	}
	return c.Indexers.Validate()
}

type metrics struct {
	rawStateProcessed  prometheus.Counter
	processingDuration *prometheus.HistogramVec
}

type Client struct {
	stream   clients.Client
	logger   clients.Logger
	indexers IndexerConfig
	metrics  *metrics

	// State primitives
	latestState atomic.Pointer[State]

	// Hook management
	handlerMutex sync.RWMutex
	onNewBlock   func(context.Context, *State) error
	isRegistered bool
}

func DialJSONRPCStream(ctx context.Context, url string, logger *slog.Logger, reg prometheus.Registerer, opts ...Option) (*Client, error) {
	stateOps, err := stateops.NewStateOps(logger.With("component", "state-ops"), reg)
	if err != nil {
		return nil, fmt.Errorf("failed to create state ops: %w", err)
	}

	client, err := jsonrpcclient.NewClient(ctx, jsonrpcclient.Config{
		URL: url, Logger: logger.With("component", "json-rpc-client"), BufferSize: 100,
		StatePatcher: stateOps.Patch, StateDecoder: stateOps.DecodeStateJSON, StateDiffDecoder: stateOps.DecodeStateDiffJSON,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to dial defistate stream: %w", err)
	}

	cfg := &Config{
		Client: client, Logger: logger, MetricsRegisterer: reg,
		Indexers: IndexerConfig{
			Token: tokenregistryindexer.New(), PoolRegistry: poolregistryindexer.New(),
			UniswapV2: uniswapv2indexer.New(), UniswapV3: uniswapv3indexer.New(),
		},
	}

	instance, err := NewClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	for _, opt := range opts {
		opt.apply(instance)
	}
	return instance, nil
}

func NewClient(ctx context.Context, cfg *Config) (*Client, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	factory := promauto.With(cfg.MetricsRegisterer)
	m := &metrics{
		rawStateProcessed: factory.NewCounter(prometheus.CounterOpts{Name: "client_rawstate_processed_total"}),
		processingDuration: factory.NewHistogramVec(prometheus.HistogramOpts{
			Name: "client_processing_duration_seconds", Buckets: prometheus.DefBuckets,
		}, []string{"stage"}),
	}
	client := &Client{stream: cfg.Client, logger: cfg.Logger, indexers: cfg.Indexers, metrics: m}
	go client.loop(ctx)
	return client, nil
}

// State provides instantaneous access to the most recently indexed DeFi state.
func (p *Client) State() *State {
	return p.latestState.Load()
}

// OnNewBlock registers the strategy hook. Returns error if already set.
func (p *Client) OnNewBlock(handler func(context.Context, *State) error) error {
	p.handlerMutex.Lock()
	defer p.handlerMutex.Unlock()
	if p.isRegistered {
		return errors.New("OnNewBlock handler is already registered")
	}
	p.onNewBlock = handler
	p.isRegistered = true
	return nil
}

func (p *Client) loop(ctx context.Context) {
	for {
		select {
		case rawState, ok := <-p.stream.State():
			if !ok {
				p.logger.Error("Upstream stream closed unexpectedly")
				return
			}
			processed, err := p.processState(ctx, rawState)
			if err != nil {
				p.logger.Error("State processing failed", "block", rawState.Block.Number, "err", err)
				continue
			}
			p.latestState.Store(processed)

			p.handlerMutex.RLock()
			currentHandler := p.onNewBlock
			p.handlerMutex.RUnlock()

			if currentHandler != nil {
				if err := currentHandler(ctx, processed); err != nil {
					p.logger.Error("OnNewBlock callback error", "block", processed.Block.Number, "err", err)
				}
			}
		case err := <-p.stream.Err():
			p.logger.Error("Client encountered fatal stream error", "err", err)
			return
		case <-ctx.Done():
			return
		}
	}
}

type dataBuckets struct {
	tokenregistry []tokenregistry.TokenView
	poolregistry  *poolregistry.PoolRegistryView
	graph         *poolregistry.TokenPoolsRegistryView
	uniswapv2     []uniswapv2.PoolView
	uniswapv3     []uniswapv3.PoolView
	pancakeswapv2 []uniswapv2.PoolView
	pancakeswapv3 []uniswapv3.PoolView
}

func (p *Client) processState(ctx context.Context, rawState *engine.State) (*State, error) {
	p.metrics.rawStateProcessed.Inc()
	start := time.Now()

	buckets, err := p.extractBuckets(rawState)
	if err != nil {
		return nil, err
	}

	indexedTokenSystem := p.indexers.Token.Index(buckets.tokenregistry)
	indexedPoolRegistry := p.indexers.PoolRegistry.Index(*buckets.poolregistry)

	var (
		indexedUniswapV2     clients.IndexedUniswapV2
		indexedUniswapV3     clients.IndexedUniswapV3
		indexedPancakeswapV2 clients.IndexedUniswapV2
		indexedPancakeswapV3 clients.IndexedUniswapV3
	)

	g, _ := errgroup.WithContext(ctx)
	g.Go(func() error {
		var err error
		indexedUniswapV2, err = p.indexers.UniswapV2.Index(UniswapV2ProtocolID, buckets.uniswapv2, indexedTokenSystem, indexedPoolRegistry)
		return err
	})
	g.Go(func() error {
		var err error
		indexedPancakeswapV2, err = p.indexers.UniswapV2.Index(PancakeswapV2ProtocolID, buckets.pancakeswapv2, indexedTokenSystem, indexedPoolRegistry)
		return err
	})
	g.Go(func() error {
		var err error
		indexedUniswapV3, err = p.indexers.UniswapV3.Index(UniswapV3ProtocolID, buckets.uniswapv3, indexedTokenSystem, indexedPoolRegistry)
		return err
	})
	g.Go(func() error {
		var err error
		indexedPancakeswapV3, err = p.indexers.UniswapV3.Index(PancakeswapV3ProtocolID, buckets.pancakeswapv3, indexedTokenSystem, indexedPoolRegistry)
		return err
	})

	if err := g.Wait(); err != nil {
		return nil, fmt.Errorf("parallel indexing failed: %w", err)
	}

	indexingDuration := time.Since(start)
	p.metrics.processingDuration.WithLabelValues("indexing").Observe(indexingDuration.Seconds())

	protocolIDToSchema := make(map[engine.ProtocolID]engine.ProtocolSchema)
	for id, proto := range rawState.Protocols {
		protocolIDToSchema[id] = proto.Schema
	}

	state := &State{
		Tokens: indexedTokenSystem, Pools: indexedPoolRegistry,
		UniswapV2: indexedUniswapV2, UniswapV3: indexedUniswapV3,
		PancakeswapV2: indexedPancakeswapV2, PancakeswapV3: indexedPancakeswapV3,
		ProtocolResolver: clients.NewProtocolResolver(protocolIDToSchema, indexedPoolRegistry),
		Graph:            buckets.graph, Block: rawState.Block, Timestamp: uint64(time.Now().UnixNano()),
	}

	p.metrics.processingDuration.WithLabelValues("total").Observe(time.Since(start).Seconds())
	return state, nil
}

func (p *Client) extractBuckets(rawState *engine.State) (*dataBuckets, error) {
	buckets := &dataBuckets{}
	for protocolID, protocol := range rawState.Protocols {
		switch protocol.Schema {
		case tokenregistry.TokenProtocolSchema:
			buckets.tokenregistry = protocol.Data.([]tokenregistry.TokenView)
		case poolregistry.PoolProtocolSchema:
			view := protocol.Data.(poolregistry.PoolRegistryView)
			buckets.poolregistry = &view
		case poolregistry.TokenPoolProtocolSchema:
			buckets.graph = protocol.Data.(*poolregistry.TokenPoolsRegistryView)
		case uniswapv2.UniswapV2ProtocolSchema:
			switch protocolID {
			case UniswapV2ProtocolID:
				buckets.uniswapv2 = protocol.Data.([]uniswapv2.PoolView)
			case PancakeswapV2ProtocolID:
				buckets.pancakeswapv2 = protocol.Data.([]uniswapv2.PoolView)
			}
		case uniswapv3.UniswapV3ProtocolSchema:
			switch protocolID {
			case UniswapV3ProtocolID:
				buckets.uniswapv3 = protocol.Data.([]uniswapv3.PoolView)
			case PancakeswapV3ProtocolID:
				buckets.pancakeswapv3 = protocol.Data.([]uniswapv3.PoolView)
			}
		}
	}
	if buckets.tokenregistry == nil || buckets.poolregistry == nil || buckets.graph == nil {
		return nil, fmt.Errorf("incomplete foundation data in block %d", rawState.Block.Number)
	}
	return buckets, nil
}

type Option interface{ apply(*Client) }
type funcOption func(*Client)

func (f funcOption) apply(p *Client) { f(p) }

func WithTokenIndexer(i clients.TokenIndexer) Option {
	return funcOption(func(p *Client) { p.indexers.Token = i })
}
func WithPoolRegistryIndexer(i clients.PoolRegistryIndexer) Option {
	return funcOption(func(p *Client) { p.indexers.PoolRegistry = i })
}
func WithUniswapV2Indexer(i clients.UniswapV2Indexer) Option {
	return funcOption(func(p *Client) { p.indexers.UniswapV2 = i })
}
func WithUniswapV3Indexer(i clients.UniswapV3Indexer) Option {
	return funcOption(func(p *Client) { p.indexers.UniswapV3 = i })
}
