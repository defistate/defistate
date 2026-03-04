package celo

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	clients "github.com/defistate/defistate/clients"
	"github.com/defistate/defistate/engine"
	tokenregistry "github.com/defistate/defistate/protocols/erc20-token-system"
	poolregistry "github.com/defistate/defistate/protocols/pool-registry"
	uniswapv2 "github.com/defistate/defistate/protocols/uniswap-v2"
	uniswapv3 "github.com/defistate/defistate/protocols/uniswap-v3"
	jsonrpcclient "github.com/defistate/defistate/streams/jsonrpc/client"
	stateops "github.com/defistate/defistate/streams/jsonrpc/stateops/chains/celo"
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
	IndexedTokenSystem  clients.IndexedTokenSystem
	IndexedPoolRegistry clients.IndexedPoolRegistry
	IndexedUniswapV2    clients.IndexedUniswapV2
	IndexedUniswapV3    clients.IndexedUniswapV3
	ProtocolResolver    *clients.ProtocolResolver
	Graph               *poolregistry.TokenPoolsRegistryView
	Block               engine.BlockSummary
	Timestamp           uint64
}

// IndexerConfig groups all protocol indexers.
type IndexerConfig struct {
	Token        clients.TokenIndexer
	PoolRegistry clients.PoolRegistryIndexer
	UniswapV2    clients.UniswapV2Indexer
	UniswapV3    clients.UniswapV3Indexer
}

// Config now looks incredibly clean.
type Config struct {
	Client            clients.Client
	Logger            clients.Logger
	Indexers          IndexerConfig
	MetricsRegisterer prometheus.Registerer
}

// Exported errors for Processor config validation, allowing for specific error checking in tests.
var (
	ErrClientRequired              = errors.New("config validation failed: Client is required")
	ErrLoggerRequired              = errors.New("config validation failed: Logger is required")
	ErrTokenIndexerRequired        = errors.New("config validation failed: TokenIndexer is required")
	ErrPoolRegistryIndexerRequired = errors.New("config validation failed: PoolRegistryIndexer is required")
	ErrUniswapV2IndexerRequired    = errors.New("config validation failed: UniswapV2Indexer is required")
	ErrUniswapV3IndexerRequired    = errors.New("config validation failed: UniswapV3Indexer is required")
	ErrMetricsRegistererRequired   = errors.New("config validation failed: MetricsRegisterer is required")
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

	// Delegate grouped validation
	if err := c.Indexers.Validate(); err != nil {
		return err
	}

	return nil
}

// metrics holds all the Prometheus collectors for the Processor manager.
type metrics struct {
	rawStateProcessed  prometheus.Counter
	stateUpdateDropped prometheus.Counter
	processingDuration *prometheus.HistogramVec
}

type Client struct {
	stream   clients.Client
	logger   clients.Logger
	indexers IndexerConfig
	metrics  *metrics
	stateCh  chan *State
}

// DialJSONRPCStream establishes the connection and starts the processing loop.
// The returned Client will remain active until the provided ctx is cancelled.
func DialJSONRPCStream(
	ctx context.Context,
	url string,
	logger *slog.Logger,
	prometheusRegistry prometheus.Registerer,
	opts ...Option,
) (*Client, error) {

	stateOps, err := stateops.NewStateOps(
		logger.With("component", "state-ops"),
		prometheusRegistry,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create state ops: %w", err)
	}

	clientCfg := jsonrpcclient.Config{
		URL:              url,
		Logger:           logger.With("component", "json-rpc-client"),
		BufferSize:       100,
		StatePatcher:     stateOps.Patch,
		StateDecoder:     stateOps.DecodeStateJSON,
		StateDiffDecoder: stateOps.DecodeStateDiffJSON,
	}

	client, err := jsonrpcclient.NewClient(ctx, clientCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to dial defistate stream url: %w", err)
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

	return NewClient(ctx, cfg)

}

func NewClient(ctx context.Context, cfg *Config) (*Client, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	// Create a new factory with the provided registry for auto-registration.
	factory := promauto.With(cfg.MetricsRegisterer)

	m := &metrics{
		rawStateProcessed: factory.NewCounter(prometheus.CounterOpts{
			Name: "defistate_views_processed_total",
			Help: "The total number of state views processed.",
		}),
		stateUpdateDropped: factory.NewCounter(prometheus.CounterOpts{
			Name: "defistate_update_signals_dropped_total",
			Help: "The total number of view update signals dropped due to a busy consumer.",
		}),
		processingDuration: factory.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "defistate_processing_duration_seconds",
			Help:    "The duration of each stage in the view processing pipeline.",
			Buckets: prometheus.DefBuckets,
		}, []string{"stage"}),
	}

	state := &Client{
		stream:   cfg.Client,
		logger:   cfg.Logger,
		indexers: cfg.Indexers,
		metrics:  m,
		stateCh:  make(chan *State, 1000),
	}

	go state.loop(ctx)

	return state, nil
}

func (p *Client) State() <-chan *State {
	return p.stateCh
}

// loop processes new raw views, indexes them in parallel, graphs them, and signals consumers.
func (p *Client) loop(ctx context.Context) {

	for {
		select {
		case rawState, ok := <-p.stream.State():
			if !ok {
				p.logger.Error("Upstream state channel closed")
				return
			}

			processed, err := p.processState(ctx, rawState)
			if err != nil {
				p.logger.Error("Failed to process state", "block", rawState.Block.Number, "err", err)
				continue
			}
			select {
			case p.stateCh <- processed:
				p.logger.Debug("sent new state update", "block", rawState.Block.Number)
			default:
				p.metrics.stateUpdateDropped.Inc()
				p.logger.Warn("State buffer full, discarding processed state...", "block", rawState.Block.Number)
			}

		case err := <-p.stream.Err():
			p.logger.Error("DeFi state client encountered a fatal error", "error", err)
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
}

func (p *Client) processState(ctx context.Context, rawState *engine.State) (*State, error) {
	p.metrics.rawStateProcessed.Inc()
	start := time.Now()

	// 1. Extraction: Bucket the raw interface data
	buckets, err := p.extractBuckets(rawState)
	if err != nil {
		return nil, fmt.Errorf("extraction failed: %w", err)
	}

	// 2. Foundation: Index the registries (Sequential as they are dependencies)
	indexedTokenSystem := p.indexers.Token.Index(buckets.tokenregistry)
	indexedPoolRegistry := p.indexers.PoolRegistry.Index(*buckets.poolregistry)

	// 3. Protocols: Index in parallel using errgroup
	var (
		indexedUniswapV2 clients.IndexedUniswapV2
		indexedUniswapV3 clients.IndexedUniswapV3
	)

	g, _ := errgroup.WithContext(ctx)

	g.Go(func() error {
		var err error
		indexedUniswapV2, err = p.indexers.UniswapV2.Index(
			buckets.uniswapv2,
			indexedTokenSystem,
			indexedPoolRegistry,
		)
		return err
	})

	g.Go(func() error {
		var err error
		indexedUniswapV3, err = p.indexers.UniswapV3.Index(
			buckets.uniswapv3,
			indexedTokenSystem,
			indexedPoolRegistry,
		)
		return err
	})

	if err := g.Wait(); err != nil {
		return nil, fmt.Errorf("protocol indexing failed: %w", err)
	}

	// 4. Metrics & Mapping
	indexingDuration := time.Since(start)
	p.metrics.processingDuration.WithLabelValues("indexing").Observe(indexingDuration.Seconds())

	protocolIDToSchema := make(map[engine.ProtocolID]engine.ProtocolSchema)
	for id, proto := range rawState.Protocols {
		protocolIDToSchema[id] = proto.Schema
	}

	// 5. Final State Assembly
	state := &State{
		IndexedTokenSystem:  indexedTokenSystem,
		IndexedPoolRegistry: indexedPoolRegistry,
		IndexedUniswapV2:    indexedUniswapV2,
		IndexedUniswapV3:    indexedUniswapV3,
		ProtocolResolver:    clients.NewProtocolResolver(protocolIDToSchema, indexedPoolRegistry),
		Graph:               buckets.graph,
		Block:               rawState.Block,
		Timestamp:           uint64(time.Now().UnixNano()),
	}

	p.metrics.processingDuration.WithLabelValues("total").Observe(time.Since(start).Seconds())
	p.logger.Info("state processed", "block", rawState.Block.Number, "duration_ms", time.Since(start).Milliseconds())

	return state, nil
}

func (p *Client) extractBuckets(rawState *engine.State) (*dataBuckets, error) {
	b := &dataBuckets{}

	for _, proto := range rawState.Protocols {
		switch proto.Schema {
		case tokenregistry.TokenProtocolSchema:
			b.tokenregistry = proto.Data.([]tokenregistry.TokenView)
		case poolregistry.PoolProtocolSchema:
			d := proto.Data.(poolregistry.PoolRegistryView)
			b.poolregistry = &d
		case poolregistry.TokenPoolProtocolSchema:
			b.graph = proto.Data.(*poolregistry.TokenPoolsRegistryView)
		case uniswapv2.UniswapV2ProtocolSchema:
			b.uniswapv2 = append(b.uniswapv2, proto.Data.([]uniswapv2.PoolView)...)
		case uniswapv3.UniswapV3ProtocolSchema:
			b.uniswapv3 = append(b.uniswapv3, proto.Data.([]uniswapv3.PoolView)...)
		}
	}

	if b.tokenregistry == nil || b.poolregistry == nil || b.graph == nil {
		return nil, fmt.Errorf("missing foundation data in block %d", rawState.Block.Number)
	}

	return b, nil
}

// Option configures the Client.
// The interface method is unexported to prevent external modification after Dial.
type Option interface {
	apply(*Client)
}

type funcOption func(*Client)

func (f funcOption) apply(p *Client) {
	f(p)
}

func newOption(f func(*Client)) Option {
	return funcOption(f)
}
func WithTokenIndexer(indexer clients.TokenIndexer) Option {
	return newOption(func(p *Client) {
		p.indexers.Token = indexer
	})
}

func WithPoolRegistryIndexer(indexer clients.PoolRegistryIndexer) Option {
	return newOption(func(p *Client) {
		p.indexers.PoolRegistry = indexer
	})
}

func WithUniswapV2Indexer(indexer clients.UniswapV2Indexer) Option {
	return newOption(func(p *Client) {
		p.indexers.UniswapV2 = indexer
	})
}

func WithUniswapV3Indexer(indexer clients.UniswapV3Indexer) Option {
	return newOption(func(p *Client) {
		p.indexers.UniswapV3 = indexer
	})
}
