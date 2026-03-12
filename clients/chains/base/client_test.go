package base

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

// --- Mocks and Call Tracking ---

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
type mockIndexedUniswapV3 struct{ clients.IndexedUniswapV3 }
type mockIndexedUniswapV2 struct{ clients.IndexedUniswapV2 }

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

func TestClient_BaseLifecycle(t *testing.T) {
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

	t.Run("Full Pipeline Validation withrequire.Eventually", func(t *testing.T) {
		cfg, mStream, tracker := setup()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		client, err := NewClient(ctx, cfg)
		require.NoError(t, err)

		// Synchronize the test with the background loop
		var mu sync.Mutex
		handlerCalled := false
		var finalBlockNumber int64

		err = client.OnNewBlock(func(ctx context.Context, state *State) error {
			mu.Lock()
			defer mu.Unlock()
			handlerCalled = true
			finalBlockNumber = state.Block.Number.Int64()
			return nil
		})
		require.NoError(t, err)

		// Send a complex multi-protocol block
		mStream.stateCh <- &engine.State{
			Block: engine.BlockSummary{Number: big.NewInt(888)},
			Protocols: map[engine.ProtocolID]engine.ProtocolState{
				"tokens":                {Schema: tokenregistry.TokenProtocolSchema, Data: []tokenregistry.TokenView{}},
				"registry":              {Schema: poolregistry.PoolProtocolSchema, Data: poolregistry.PoolRegistryView{}},
				"graph":                 {Schema: poolregistry.TokenPoolProtocolSchema, Data: &poolregistry.TokenPoolsRegistryView{}},
				UniswapV2ProtocolID:     {Schema: uniswapv2.UniswapV2ProtocolSchema, Data: []uniswapv2.PoolView{}},
				UniswapV3ProtocolID:     {Schema: uniswapv3.UniswapV3ProtocolSchema, Data: []uniswapv3.PoolView{}},
				PancakeswapV3ProtocolID: {Schema: uniswapv3.UniswapV3ProtocolSchema, Data: []uniswapv3.PoolView{}},
			},
		}

		// Verify callback execution using polling instead of sleep
		require.Eventually(t, func() bool {
			mu.Lock()
			defer mu.Unlock()
			return handlerCalled && finalBlockNumber == 888
		}, 2*time.Second, 10*time.Millisecond, "OnNewBlock handler failed to execute with correct data")

		// Verify Pull pattern visibility
		latest := client.State()
		require.NotNil(t, latest)
		assert.Equal(t, int64(888), latest.Block.Number.Int64())

		// Verify Indexer Parallelism (confirming errgroup worked)
		assert.True(t, tracker.wasCalled("token"), "Token indexer was skipped")
		assert.True(t, tracker.wasCalled("registry"), "Registry indexer was skipped")
		assert.True(t, tracker.wasCalled(string(UniswapV2ProtocolID)), "Uniswap V2 indexer was skipped")
		assert.True(t, tracker.wasCalled(string(UniswapV3ProtocolID)), "Uniswap V3 indexer was skipped")
		assert.True(t, tracker.wasCalled(string(PancakeswapV3ProtocolID)), "Pancake V3 indexer was skipped")
	})

	t.Run("OnNewBlock Registration Guard", func(t *testing.T) {
		cfg, _, _ := setup()
		client, _ := NewClient(context.Background(), cfg)

		// First registration succeeds
		err := client.OnNewBlock(func(ctx context.Context, s *State) error { return nil })
		assert.NoError(t, err)

		// Second registration fails
		err2 := client.OnNewBlock(func(ctx context.Context, s *State) error { return nil })
		assert.Error(t, err2)
		assert.Contains(t, err2.Error(), "already registered")
	})

	t.Run("Graceful Context Shutdown", func(t *testing.T) {
		cfg, mStream, _ := setup()
		ctx, cancel := context.WithCancel(context.Background())
		client, _ := NewClient(ctx, cfg)

		cancel() // Signal loop to exit

		// Ensure the atomic pointer remains stable or nil after shutdown
		mStream.stateCh <- &engine.State{Block: engine.BlockSummary{Number: big.NewInt(999)}}

		// Wait a small buffer to ensure the loop had time to see the cancel signal
		time.Sleep(50 * time.Millisecond)
		assert.Nil(t, client.State(), "Client should have stopped processing before block 999")
	})
}
