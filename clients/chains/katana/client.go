package katana

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
	uniswapv3 "github.com/defistate/defistate/protocols/uniswap-v3"
	jsonrpcclient "github.com/defistate/defistate/streams/jsonrpc/client"
	stateops "github.com/defistate/defistate/streams/jsonrpc/stateops/chains/katana"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"golang.org/x/sync/errgroup"

	poolregistryindexer "github.com/defistate/defistate/clients/indexers/poolregistry"
	tokenregistryindexer "github.com/defistate/defistate/clients/indexers/token"
	uniswapv2indexer "github.com/defistate/defistate/clients/indexers/uniswapv2"
	uniswapv3indexer "github.com/defistate/defistate/clients/indexers/uniswapv3"
)

// State is the high-level, indexed and graphed representation of the Katana DeFi state.
type State struct {
	Tokens           clients.IndexedTokenSystem
	Pools            clients.IndexedPoolRegistry
	SushiV3          clients.IndexedUniswapV3
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

// Config defines the required dependencies for the Katana client.
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

	SushiV3ProtocolID = engine.ProtocolID("sushiswap-v3-katana-mainnet")
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

	// Pull Pattern: High-performance atomic pointer for current state snapshots.
	latestState atomic.Pointer[State]

	// Push Pattern: Thread-safe callback registry for block events.
	handlerMutex sync.RWMutex
	onNewBlock   func(context.Context, *State) error
	isRegistered bool
}

// DialJSONRPCStream initializes the network connection and background processing loop.
func DialJSONRPCStream(
	ctx context.Context,
	url string,
	logger *slog.Logger,
	prometheusRegistry prometheus.Registerer,
	opts ...Option,
) (*Client, error) {

	stateOps, err := stateops.NewStateOps(logger.With("component", "state-ops"), prometheusRegistry)
	if err != nil {
		return nil, fmt.Errorf("failed to create state ops: %w", err)
	}

	client, err := jsonrpcclient.NewClient(ctx, jsonrpcclient.Config{
		URL:              url,
		Logger:           logger.With("component", "json-rpc-client"),
		BufferSize:       100,
		StatePatcher:     stateOps.Patch,
		StateDecoder:     stateOps.DecodeStateJSON,
		StateDiffDecoder: stateOps.DecodeStateDiffJSON,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to dial katana defistate stream: %w", err)
	}

	cfg := &Config{
		Client: client,
		Logger: logger,
		Indexers: IndexerConfig{
			Token:        tokenregistryindexer.New(),
			PoolRegistry: poolregistryindexer.New(),
			UniswapV2:    uniswapv2indexer.New(),
			UniswapV3:    uniswapv3indexer.New(),
		},
		MetricsRegisterer: prometheusRegistry,
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
		rawStateProcessed: factory.NewCounter(prometheus.CounterOpts{Name: "defistate_katana_processed_total"}),
		processingDuration: factory.NewHistogramVec(prometheus.HistogramOpts{
			Name: "defistate_katana_processing_duration_seconds", Buckets: prometheus.DefBuckets,
		}, []string{"stage"}),
	}

	client := &Client{stream: cfg.Client, logger: cfg.Logger, indexers: cfg.Indexers, metrics: m}
	go client.loop(ctx)
	return client, nil
}

// State provides instantaneous, thread-safe access to the most recently indexed Katana state.
func (p *Client) State() *State {
	return p.latestState.Load()
}

// OnNewBlock registers a strategy function to execute immediately upon block arrival.
func (p *Client) OnNewBlock(handler func(context.Context, *State) error) error {
	p.handlerMutex.Lock()
	defer p.handlerMutex.Unlock()

	if p.isRegistered {
		return errors.New("OnNewBlock handler is already registered for Katana")
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
				p.logger.Error("Upstream Katana stream closed unexpectedly")
				return
			}

			processed, err := p.processState(ctx, rawState)
			if err != nil {
				p.logger.Error("Katana state processing failed", "block", rawState.Block.Number, "err", err)
				continue
			}

			// Atomic update for Pull pattern users.
			p.latestState.Store(processed)

			// Read-locked execution for Push pattern users.
			p.handlerMutex.RLock()
			currentHandler := p.onNewBlock
			p.handlerMutex.RUnlock()

			if currentHandler != nil {
				if err := currentHandler(ctx, processed); err != nil {
					p.logger.Error("Katana OnNewBlock callback error", "block", processed.Block.Number, "err", err)
				}
			}

		case err := <-p.stream.Err():
			p.logger.Error("Katana client fatal stream error", "err", err)
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
	SushiV3       []uniswapv3.PoolView
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
		indexedSushiV3 clients.IndexedUniswapV3
	)

	g, _ := errgroup.WithContext(ctx)
	g.Go(func() error {
		var err error
		indexedSushiV3, err = p.indexers.UniswapV3.Index(SushiV3ProtocolID, buckets.SushiV3, indexedTokenSystem, indexedPoolRegistry)
		return err
	})

	if err := g.Wait(); err != nil {
		return nil, fmt.Errorf("Katana protocol indexing failed: %w", err)
	}

	indexingDuration := time.Since(start)
	p.metrics.processingDuration.WithLabelValues("indexing").Observe(indexingDuration.Seconds())

	protocolIDToSchema := make(map[engine.ProtocolID]engine.ProtocolSchema)
	for id, proto := range rawState.Protocols {
		protocolIDToSchema[id] = proto.Schema
	}

	state := &State{
		Tokens:           indexedTokenSystem,
		Pools:            indexedPoolRegistry,
		SushiV3:          indexedSushiV3,
		ProtocolResolver: clients.NewProtocolResolver(protocolIDToSchema, indexedPoolRegistry),
		Graph:            buckets.graph,
		Block:            rawState.Block,
		Timestamp:        uint64(time.Now().UnixNano()),
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
		case uniswapv3.UniswapV3ProtocolSchema:
			if protocolID == SushiV3ProtocolID {
				buckets.SushiV3 = protocol.Data.([]uniswapv3.PoolView)
			}
		}
	}
	if buckets.tokenregistry == nil || buckets.poolregistry == nil || buckets.graph == nil {
		return nil, fmt.Errorf("incomplete foundation data in Katana block %d", rawState.Block.Number)
	}
	return buckets, nil
}

// Option allows for fine-grained configuration during client creation.
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
