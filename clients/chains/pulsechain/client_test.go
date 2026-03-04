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

// callTracker helps us verify that our concurrent pipeline actually hits all indexers.
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

// dummyIndexed types satisfy the interface return values for the indexers.
type mockIndexedToken struct{ clients.IndexedTokenSystem }
type mockIndexedPoolRegistry struct{ clients.IndexedPoolRegistry }
type mockIndexedUniswapV2 struct{ clients.IndexedUniswapV2 }
type mockIndexedUniswapV3 struct{ clients.IndexedUniswapV3 }

// Indexer Mock Implementations
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

func (m *mockV2Indexer) Index(_ []uniswapv2.PoolView, _ clients.IndexedTokenSystem, _ clients.IndexedPoolRegistry) (clients.IndexedUniswapV2, error) {
	m.t.track("v2")
	return &mockIndexedUniswapV2{}, nil
}

type mockV3Indexer struct{ t *callTracker }

func (m *mockV3Indexer) Index(_ []uniswapv3.PoolView, _ clients.IndexedTokenSystem, _ clients.IndexedPoolRegistry) (clients.IndexedUniswapV3, error) {
	m.t.track("v3")
	return &mockIndexedUniswapV3{}, nil
}

// mockStream simulates the upstream raw state channel.
type mockStream struct {
	stateCh chan *engine.State
	errCh   chan error
}

func (m *mockStream) State() <-chan *engine.State { return m.stateCh }
func (m *mockStream) Err() <-chan error           { return m.errCh }

// --- Test Suite ---

func TestClient_Pipeline(t *testing.T) {
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

	t.Run("Processing Flow and Concurrency", func(t *testing.T) {
		cfg, mStream, tracker := setup()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		client, err := NewClient(ctx, cfg)
		require.NoError(t, err)

		// Construct a raw state with all required protocol schemas
		rawState := &engine.State{
			Block: engine.BlockSummary{Number: big.NewInt(42)},
			Protocols: map[engine.ProtocolID]engine.ProtocolState{
				"tokens": {
					Schema: tokenregistry.TokenProtocolSchema,
					Data:   []tokenregistry.TokenView{{ID: 1, Symbol: "TEST"}},
				},
				"registry": {
					Schema: poolregistry.PoolProtocolSchema,
					Data:   poolregistry.PoolRegistryView{},
				},
				"graph": {
					Schema: poolregistry.TokenPoolProtocolSchema,
					Data:   &poolregistry.TokenPoolsRegistryView{},
				},
				"univ2": {
					Schema: uniswapv2.UniswapV2ProtocolSchema,
					Data:   []uniswapv2.PoolView{},
				},
				"univ3": {
					Schema: uniswapv3.UniswapV3ProtocolSchema,
					Data:   []uniswapv3.PoolView{},
				},
			},
		}

		// Inject raw data into the processor
		mStream.stateCh <- rawState

		// Assert the state emerges from the output channel fully processed
		select {
		case processed := <-client.State():
			assert.Equal(t, int64(42), processed.Block.Number.Int64())
			assert.NotNil(t, processed.ProtocolResolver)
			assert.NotNil(t, processed.Graph)

			// Verify all indexers were hit (confirming parallel errgroup execution)
			assert.True(t, tracker.wasCalled("token"), "Token Indexer missed")
			assert.True(t, tracker.wasCalled("registry"), "Pool Registry Indexer missed")
			assert.True(t, tracker.wasCalled("v2"), "Uniswap V2 Indexer missed")
			assert.True(t, tracker.wasCalled("v3"), "Uniswap V3 Indexer missed")

		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for processed state")
		}
	})

	t.Run("Configuration Validation", func(t *testing.T) {
		cfg, _, _ := setup()

		// Case 1: Missing Root Client
		badCfg := *cfg
		badCfg.Client = nil
		_, err := NewClient(context.Background(), &badCfg)
		assert.ErrorIs(t, err, ErrClientRequired)

		// Case 2: Missing Nested Indexer
		badCfg2 := *cfg
		badCfg2.Indexers.UniswapV2 = nil
		_, err2 := NewClient(context.Background(), &badCfg2)
		assert.ErrorIs(t, err2, ErrUniswapV2IndexerRequired)
	})

	t.Run("Graceful Shutdown", func(t *testing.T) {
		cfg, mStream, _ := setup()
		ctx, cancel := context.WithCancel(context.Background())

		client, err := NewClient(ctx, cfg)
		require.NoError(t, err)

		cancel() // Shut down the processor
		time.Sleep(50 * time.Millisecond)

		// Try to send a state
		mStream.stateCh <- &engine.State{Block: engine.BlockSummary{Number: big.NewInt(100)}}

		// Channel should not receive the update because the loop should have exited
		select {
		case <-client.State():
			t.Fatal("client should not have processed state after context cancellation")
		case <-time.After(100 * time.Millisecond):
			// Success: loop exited
		}
	})
}
