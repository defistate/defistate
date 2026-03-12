package celo

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

type mockTokenIndexer struct{ t *callTracker }

func (m *mockTokenIndexer) Index(_ []tokenregistry.TokenView) clients.IndexedTokenSystem {
	m.t.track("token")
	return &mockIndexedToken{}
}

type mockPoolRegistryIndexer struct{ t *callTracker }

func (m *mockPoolRegistryIndexer) Index(_ poolregistry.PoolRegistryView) clients.IndexedPoolRegistry {
	m.t.track("registry")
	return &mockIndexedPoolRegistry{}
}

type mockV2Indexer struct{ t *callTracker }

func (m *mockV2Indexer) Index(protocolID engine.ProtocolID, _ []uniswapv2.PoolView, _ clients.IndexedTokenSystem, _ clients.IndexedPoolRegistry) (clients.IndexedUniswapV2, error) {
	m.t.track(string(protocolID))
	return &mockIndexedUniswapV2{}, nil
}

type mockV3Indexer struct{ t *callTracker }

func (m *mockV3Indexer) Index(protocolID engine.ProtocolID, _ []uniswapv3.PoolView, _ clients.IndexedTokenSystem, _ clients.IndexedPoolRegistry) (clients.IndexedUniswapV3, error) {
	m.t.track(string(protocolID))
	return &mockIndexedUniswapV3{}, nil
}

type mockStream struct {
	stateCh chan *engine.State
	errCh   chan error
}

func (m *mockStream) State() <-chan *engine.State { return m.stateCh }
func (m *mockStream) Err() <-chan error           { return m.errCh }

// --- Test Suite ---

func TestClient_CeloLifecycle(t *testing.T) {
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
				Token:        &mockTokenIndexer{t: tracker},
				PoolRegistry: &mockPoolRegistryIndexer{t: tracker},
				UniswapV2:    &mockV2Indexer{t: tracker},
				UniswapV3:    &mockV3Indexer{t: tracker},
			},
			MetricsRegisterer: prometheus.NewRegistry(),
		}
		return cfg, stream, tracker
	}

	t.Run("Concurrent Pipeline and Callback Validation", func(t *testing.T) {
		cfg, mStream, tracker := setup()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		client, err := NewClient(ctx, cfg)
		require.NoError(t, err)

		// Verification variables for the callback
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

		// Inject complex raw state
		mStream.stateCh <- &engine.State{
			Block: engine.BlockSummary{Number: big.NewInt(42)},
			Protocols: map[engine.ProtocolID]engine.ProtocolState{
				"tokens":              {Schema: tokenregistry.TokenProtocolSchema, Data: []tokenregistry.TokenView{{ID: 1}}},
				"registry":            {Schema: poolregistry.PoolProtocolSchema, Data: poolregistry.PoolRegistryView{}},
				"graph":               {Schema: poolregistry.TokenPoolProtocolSchema, Data: &poolregistry.TokenPoolsRegistryView{}},
				UniswapV2ProtocolID:   {Schema: uniswapv2.UniswapV2ProtocolSchema, Data: []uniswapv2.PoolView{}},
				UniswapV3ProtocolID:   {Schema: uniswapv3.UniswapV3ProtocolSchema, Data: []uniswapv3.PoolView{}},
				AerodromeV3ProtocolID: {Schema: uniswapv3.UniswapV3ProtocolSchema, Data: []uniswapv3.PoolView{}},
			},
		}

		// 1. Assert handler execution via polling
		require.Eventually(t, func() bool {
			mu.Lock()
			defer mu.Unlock()
			return handlerCalled && capturedBlock == 42
		}, 2*time.Second, 10*time.Millisecond, "OnNewBlock handler was not called")

		// 2. Assert Pull Pattern: State() provides the latest snapshot
		latest := client.State()
		require.NotNil(t, latest)
		assert.Equal(t, int64(42), latest.Block.Number.Int64())

		// 3. Assert Indexer Concurrency: verify all specific protocol indexers were hit
		assert.True(t, tracker.wasCalled("token"))
		assert.True(t, tracker.wasCalled("registry"))
		assert.True(t, tracker.wasCalled(string(UniswapV2ProtocolID)))
		assert.True(t, tracker.wasCalled(string(UniswapV3ProtocolID)))
		assert.True(t, tracker.wasCalled(string(AerodromeV3ProtocolID)))
	})

	t.Run("Registration Guard Enforcement", func(t *testing.T) {
		cfg, _, _ := setup()
		client, _ := NewClient(context.Background(), cfg)

		// First registration succeeds
		err := client.OnNewBlock(func(ctx context.Context, s *State) error { return nil })
		assert.NoError(t, err)

		// Second registration fails explicitly
		err2 := client.OnNewBlock(func(ctx context.Context, s *State) error { return nil })
		assert.Error(t, err2)
		assert.Contains(t, err2.Error(), "already registered")
	})

	t.Run("Graceful Context Shutdown", func(t *testing.T) {
		cfg, mStream, _ := setup()
		ctx, cancel := context.WithCancel(context.Background())
		client, _ := NewClient(ctx, cfg)

		cancel() // Shut down
		time.Sleep(50 * time.Millisecond)

		// Send data post-shutdown
		mStream.stateCh <- &engine.State{
			Block: engine.BlockSummary{Number: big.NewInt(100)},
			Protocols: map[engine.ProtocolID]engine.ProtocolState{
				"tokens":   {Schema: tokenregistry.TokenProtocolSchema, Data: []tokenregistry.TokenView{}},
				"registry": {Schema: poolregistry.PoolProtocolSchema, Data: poolregistry.PoolRegistryView{}},
				"graph":    {Schema: poolregistry.TokenPoolProtocolSchema, Data: &poolregistry.TokenPoolsRegistryView{}},
			},
		}

		// Ensure nothing was processed
		time.Sleep(50 * time.Millisecond)
		assert.Nil(t, client.State(), "Client should have ignored block 100 after shutdown")
	})
}
