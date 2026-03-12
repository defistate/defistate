package pulsechain

import (
	"context"
	"io"
	"log/slog"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/defistate/defistate/clients"
	"github.com/defistate/defistate/engine"
	tokenregistry "github.com/defistate/defistate/protocols/erc20-token-system"
	poolregistry "github.com/defistate/defistate/protocols/pool-registry"
	uniswapv2 "github.com/defistate/defistate/protocols/uniswap-v2"
	uniswapv3 "github.com/defistate/defistate/protocols/uniswap-v3"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Localized Mocks & Trackers ---

type callTracker struct {
	mu     sync.Mutex
	called map[string]bool
}

func (c *callTracker) track(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.called[name] = true
}

func (c *callTracker) wasCalled(name string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.called[name]
}

type mockIndexedToken struct{ clients.IndexedTokenSystem }
type mockIndexedPoolRegistry struct{ clients.IndexedPoolRegistry }
type mockIndexedUniswapV2 struct{ clients.IndexedUniswapV2 }
type mockIndexedUniswapV3 struct{ clients.IndexedUniswapV3 }

type mockTokenIndexer struct{ tracker *callTracker }

func (m *mockTokenIndexer) Index(_ []tokenregistry.TokenView) clients.IndexedTokenSystem {
	m.tracker.track("token")
	return &mockIndexedToken{}
}

type mockPoolRegistryIndexer struct{ tracker *callTracker }

func (m *mockPoolRegistryIndexer) Index(_ poolregistry.PoolRegistryView) clients.IndexedPoolRegistry {
	m.tracker.track("registry")
	return &mockIndexedPoolRegistry{}
}

type mockV2Indexer struct{ tracker *callTracker }

func (m *mockV2Indexer) Index(id engine.ProtocolID, _ []uniswapv2.PoolView, _ clients.IndexedTokenSystem, _ clients.IndexedPoolRegistry) (clients.IndexedUniswapV2, error) {
	m.tracker.track(string(id))
	return &mockIndexedUniswapV2{}, nil
}

type mockV3Indexer struct{ tracker *callTracker }

func (m *mockV3Indexer) Index(id engine.ProtocolID, _ []uniswapv3.PoolView, _ clients.IndexedTokenSystem, _ clients.IndexedPoolRegistry) (clients.IndexedUniswapV3, error) {
	m.tracker.track(string(id))
	return &mockIndexedUniswapV3{}, nil
}

type mockStream struct {
	stateCh chan *engine.State
	errCh   chan error
}

func (m *mockStream) State() <-chan *engine.State { return m.stateCh }
func (m *mockStream) Err() <-chan error           { return m.errCh }

// --- Test Suite ---

func TestClient_PulseChainLifecycle(t *testing.T) {
	setup := func() (*Config, *mockStream, *callTracker) {
		tracker := &callTracker{called: make(map[string]bool)}
		stream := &mockStream{
			stateCh: make(chan *engine.State, 1),
			errCh:   make(chan error, 1),
		}

		cfg := &Config{
			Client: stream,
			Logger: slog.New(slog.NewJSONHandler(io.Discard, nil)),
			Indexers: IndexerConfig{
				Token:        &mockTokenIndexer{tracker: tracker},
				PoolRegistry: &mockPoolRegistryIndexer{tracker: tracker},
				UniswapV2:    &mockV2Indexer{tracker: tracker},
				UniswapV3:    &mockV3Indexer{tracker: tracker},
			},
			MetricsRegisterer: prometheus.NewRegistry(),
		}
		return cfg, stream, tracker
	}

	t.Run("Active Pipeline and Parallel Indexer Validation", func(t *testing.T) {
		cfg, mStream, tracker := setup()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		client, err := NewClient(ctx, cfg)
		require.NoError(t, err)

		var mu sync.Mutex
		handlerCalled := false
		var capturedBlock int64

		err = client.OnNewBlock(func(ctx context.Context, state *State) error {
			mu.Lock()
			defer mu.Unlock()
			handlerCalled = true
			capturedBlock = state.Block.Number.Int64()
			return nil
		})
		require.NoError(t, err)

		// Inject PulseChain raw state with 4 DEX protocols
		mStream.stateCh <- &engine.State{
			Block: engine.BlockSummary{Number: big.NewInt(42)},
			Protocols: map[engine.ProtocolID]engine.ProtocolState{
				"tokens":                {Schema: tokenregistry.TokenProtocolSchema, Data: []tokenregistry.TokenView{{ID: 1}}},
				"registry":              {Schema: poolregistry.PoolProtocolSchema, Data: poolregistry.PoolRegistryView{}},
				"graph":                 {Schema: poolregistry.TokenPoolProtocolSchema, Data: &poolregistry.TokenPoolsRegistryView{}},
				UniswapV2ProtocolID:     {Schema: uniswapv2.UniswapV2ProtocolSchema, Data: []uniswapv2.PoolView{}},
				PulseXV2ProtocolID:      {Schema: uniswapv2.UniswapV2ProtocolSchema, Data: []uniswapv2.PoolView{}},
				UniswapV3ProtocolID:     {Schema: uniswapv3.UniswapV3ProtocolSchema, Data: []uniswapv3.PoolView{}},
				PancakeswapV3ProtocolID: {Schema: uniswapv3.UniswapV3ProtocolSchema, Data: []uniswapv3.PoolView{}},
			},
		}

		// 1. Assert synchronous handler execution via polling
		require.Eventually(t, func() bool {
			mu.Lock()
			defer mu.Unlock()
			return handlerCalled && capturedBlock == 42
		}, 2*time.Second, 10*time.Millisecond, "PulseChain OnNewBlock was not called")

		// 2. Assert Pull Pattern: latest state retrieval
		latest := client.State()
		require.NotNil(t, latest)
		assert.Equal(t, int64(42), latest.Block.Number.Int64())

		// 3. Assert Parallel Indexer Coverage
		assert.True(t, tracker.wasCalled("token"), "Token indexer missed")
		assert.True(t, tracker.wasCalled("registry"), "Pool registry missed")
		assert.True(t, tracker.wasCalled(string(UniswapV2ProtocolID)), "Uniswap V2 missed")
		assert.True(t, tracker.wasCalled(string(PulseXV2ProtocolID)), "PulseX V2 missed")
		assert.True(t, tracker.wasCalled(string(UniswapV3ProtocolID)), "Uniswap V3 missed")
		assert.True(t, tracker.wasCalled(string(PancakeswapV3ProtocolID)), "Pancake V3 missed")
	})

	t.Run("PulseChain Registration Guard", func(t *testing.T) {
		cfg, _, _ := setup()
		client, _ := NewClient(context.Background(), cfg)

		// First registration
		require.NoError(t, client.OnNewBlock(func(ctx context.Context, s *State) error { return nil }))

		// Second registration must fail
		err := client.OnNewBlock(func(ctx context.Context, s *State) error { return nil })
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already registered")
	})

	t.Run("Shutdown Integrity", func(t *testing.T) {
		cfg, mStream, _ := setup()
		ctx, cancel := context.WithCancel(context.Background())
		client, _ := NewClient(ctx, cfg)

		cancel()
		time.Sleep(50 * time.Millisecond) // Buffer for loop exit

		mStream.stateCh <- &engine.State{Block: engine.BlockSummary{Number: big.NewInt(100)}}

		time.Sleep(50 * time.Millisecond)
		assert.Nil(t, client.State(), "PulseChain client should have stopped processing")
	})
}
