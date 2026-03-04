package engine

import (
	"context"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --------------------------------------------------------------------------------
// --- Mock Implementations ---
// --------------------------------------------------------------------------------

// --- Mock Protocol (Block-Unaware) ---
// Generic mock for static data sources.
type MockProtocol struct {
	mu     sync.Mutex
	id     ProtocolID
	data   any
	err    error
	schema ProtocolSchema
}

func NewMockProtocol(id ProtocolID) *MockProtocol {
	return &MockProtocol{
		id:     id,
		schema: "mock/unaware@v1",
		data:   map[string]string{"type": "static_registry"},
	}
}

func (m *MockProtocol) View() (any, ProtocolSchema, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return nil, "", m.err
	}
	return m.data, m.schema, nil
}

func (m *MockProtocol) Meta() ProtocolMeta {
	return ProtocolMeta{Name: ProtocolName(m.id), Tags: []string{"unaware"}}
}

func (m *MockProtocol) Schema() ProtocolSchema {
	return m.schema
}

func (m *MockProtocol) SetData(data any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data = data
}

// --- Mock BlockSynchronizedProtocol (Block-Aware) ---
// Generic mock for dynamic data sources that track blocks.
type MockBlockSynchronizedProtocol struct {
	*MockProtocol // Embed to reuse View/Meta/Data logic
	syncedBlock   uint64
}

func NewMockBlockSynchronizedProtocol(id ProtocolID) *MockBlockSynchronizedProtocol {
	return &MockBlockSynchronizedProtocol{
		MockProtocol: NewMockProtocol(id),
		syncedBlock:  0,
	}
}

func (m *MockBlockSynchronizedProtocol) LastUpdatedAtBlock() uint64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.syncedBlock
}

func (m *MockBlockSynchronizedProtocol) SetSyncedBlock(b uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.syncedBlock = b
}

// noOpLogger suppresses logging during tests
type noOpLogger struct{}

func (l *noOpLogger) Debug(msg string, args ...any) {}
func (l *noOpLogger) Info(msg string, args ...any)  {}
func (l *noOpLogger) Warn(msg string, args ...any)  {}
func (l *noOpLogger) Error(msg string, args ...any) {}

// --------------------------------------------------------------------------------
// --- Test Helper ---
// --------------------------------------------------------------------------------

func newTestConfig(blockEventer chan *types.Block) *Config {
	return &Config{
		ChainID:                    big.NewInt(1),
		NewBlockEventer:            blockEventer,
		PollSyncInterval:           5 * time.Millisecond,
		MaxWaitUntilSync:           100 * time.Millisecond,
		BlockQueueSize:             10,
		Registry:                   prometheus.NewRegistry(),
		Logger:                     &noOpLogger{},
		Protocols:                  make(map[ProtocolID]Protocol),
		BlockSynchronizedProtocols: make(map[ProtocolID]BlockSynchronizedProtocol),
	}
}

// --------------------------------------------------------------------------------
// --- Tests ---
// --------------------------------------------------------------------------------

func TestEngine_HappyPath_MixedProtocols(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	blockEventer := make(chan *types.Block, 10)
	cfg := newTestConfig(blockEventer)

	// 1. Setup Block-Unaware Protocol (e.g., Token Registry)
	staticID := ProtocolID("token-registry")
	staticProto := NewMockProtocol(staticID)
	staticProto.SetData(map[string]string{"tokens": "100"})
	cfg.Protocols[staticID] = staticProto

	// 2. Setup BlockSynchronizedProtocol (e.g., Uniswap)
	dynamicID := ProtocolID("uniswap-v2")
	dynamicProto := NewMockBlockSynchronizedProtocol(dynamicID)
	dynamicProto.SetData(map[string]string{"reserves": "1000"})
	cfg.BlockSynchronizedProtocols[dynamicID] = dynamicProto

	engine, err := NewEngine(ctx, cfg)
	require.NoError(t, err)

	sub, unsub := engine.Subscribe()
	defer unsub()

	// --- Action ---
	// Sync the dynamic protocol to target block 100
	dynamicProto.SetSyncedBlock(100)

	block := types.NewBlock(&types.Header{Number: big.NewInt(100)}, nil, nil, nil)
	blockEventer <- block

	// --- Verification ---
	select {
	case view := <-sub.C():
		require.NotNil(t, view)
		assert.Equal(t, uint64(100), view.Block.Number.Uint64())

		// Check BlockSynchronizedProtocol Result
		dynRes, ok := view.Protocols[dynamicID]
		require.True(t, ok)
		assert.Empty(t, dynRes.Error)
		require.NotNil(t, dynRes.SyncedBlockNumber, "Synchronized protocols must return a SyncedBlockNumber")
		assert.Equal(t, uint64(100), *dynRes.SyncedBlockNumber)
		assert.Equal(t, "1000", dynRes.Data.(map[string]string)["reserves"])

		// Check Unaware Protocol Result
		statRes, ok := view.Protocols[staticID]
		require.True(t, ok)
		assert.Empty(t, statRes.Error)
		assert.Nil(t, statRes.SyncedBlockNumber, "Unaware protocols should have nil SyncedBlockNumber")
		assert.Equal(t, "100", statRes.Data.(map[string]string)["tokens"])

	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for view event")
	}

	assert.Eventually(t, func() bool {
		return engine.LastProcessedBlock() == 100
	}, 100*time.Millisecond, 10*time.Millisecond)
}

func TestEngine_SyncTimeout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	blockEventer := make(chan *types.Block, 10)
	cfg := newTestConfig(blockEventer)
	cfg.MaxWaitUntilSync = 50 * time.Millisecond // Fast timeout

	// 1. BlockSynchronizedProtocol (Stuck at block 99)
	stuckID := ProtocolID("stuck-proto")
	stuckProto := NewMockBlockSynchronizedProtocol(stuckID)
	stuckProto.SetSyncedBlock(99)
	cfg.BlockSynchronizedProtocols[stuckID] = stuckProto

	// 2. Unaware Protocol (Should still return data!)
	staticID := ProtocolID("always-ready")
	staticProto := NewMockProtocol(staticID)
	cfg.Protocols[staticID] = staticProto

	engine, err := NewEngine(ctx, cfg)
	require.NoError(t, err)

	sub, unsub := engine.Subscribe()
	defer unsub()

	// --- Action ---
	block := types.NewBlock(&types.Header{Number: big.NewInt(100)}, nil, nil, nil)
	blockEventer <- block

	// --- Verification ---
	select {
	case view := <-sub.C():
		require.NotNil(t, view)
		assert.Equal(t, uint64(100), view.Block.Number.Uint64())

		// Verify Stuck Protocol -> Error
		stuckRes := view.Protocols[stuckID]
		assert.NotEmpty(t, stuckRes.Error)
		assert.Contains(t, stuckRes.Error, "out of sync")
		assert.Nil(t, stuckRes.Data)

		// Verify Static Protocol -> Success
		// This proves that a syncing failure in one part of the engine
		// does not prevent static data from being delivered.
		staticRes := view.Protocols[staticID]
		assert.Empty(t, staticRes.Error)
		assert.NotNil(t, staticRes.Data)

	case <-time.After(200 * time.Millisecond):
		t.Fatal("timed out waiting for view event")
	}
}

func TestEngine_SubscriptionLifecycle(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	blockEventer := make(chan *types.Block, 10)
	cfg := newTestConfig(blockEventer)
	engine, err := NewEngine(ctx, cfg)
	require.NoError(t, err)

	// Subscribe 2 clients
	sub1, unsub1 := engine.Subscribe()
	sub2, unsub2 := engine.Subscribe()

	assert.Eventually(t, func() bool {
		engine.subscribersMu.Lock()
		defer engine.subscribersMu.Unlock()
		return len(engine.subscribers) == 2
	}, time.Second, 10*time.Millisecond, "Should have 2 subscribers")

	// Broadcast a block to verify both receive it
	blockEventer <- types.NewBlock(&types.Header{Number: big.NewInt(1)}, nil, nil, nil)
	<-sub1.C()
	<-sub2.C()

	// Unsubscribe 1
	unsub1()
	assert.Eventually(t, func() bool {
		engine.subscribersMu.Lock()
		defer engine.subscribersMu.Unlock()
		return len(engine.subscribers) == 1
	}, time.Second, 10*time.Millisecond, "Should have 1 subscriber")

	// Ensure sub1 channel is closed
	_, ok := <-sub1.C()
	assert.False(t, ok, "Channel for subscriber 1 should be closed")

	// Unsubscribe 2
	unsub2()
	assert.Eventually(t, func() bool {
		engine.subscribersMu.Lock()
		defer engine.subscribersMu.Unlock()
		return len(engine.subscribers) == 0
	}, time.Second, 10*time.Millisecond, "Should have 0 subscribers")
}

func TestEngine_InitialViewOnSubscribe(t *testing.T) {
	t.Run("should receive initial view if cache is populated", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		blockEventer := make(chan *types.Block, 10)
		cfg := newTestConfig(blockEventer)

		// Use an unaware protocol for simplicity
		pID := ProtocolID("simple")
		cfg.Protocols[pID] = NewMockProtocol(pID)

		engine, err := NewEngine(ctx, cfg)
		require.NoError(t, err)

		// 1. Process block 100 to fill cache
		block100 := types.NewBlock(&types.Header{Number: big.NewInt(100)}, nil, nil, nil)
		blockEventer <- block100

		// Wait for processing
		assert.Eventually(t, func() bool {
			return engine.LastProcessedBlock() == 100
		}, 200*time.Millisecond, 5*time.Millisecond, "Engine failed to process block 100")

		// 2. New Subscriber
		sub, unsub := engine.Subscribe()
		defer unsub()

		// 3. Expect cached view
		select {
		case view := <-sub.C():
			assert.Equal(t, uint64(100), view.Block.Number.Uint64())
			_, ok := view.Protocols[pID]
			assert.True(t, ok)
		case <-time.After(100 * time.Millisecond):
			t.Fatal("Did not receive cached initial view")
		}
	})

	t.Run("should not receive initial view if cache is empty", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		blockEventer := make(chan *types.Block, 10)
		cfg := newTestConfig(blockEventer)
		engine, err := NewEngine(ctx, cfg)
		require.NoError(t, err)

		sub, unsub := engine.Subscribe()
		defer unsub()

		// Verify nothing is received immediately
		select {
		case view := <-sub.C():
			t.Fatalf("Unexpected initial view: %d", view.Block.Number.Uint64())
		case <-time.After(50 * time.Millisecond):
			// Success
		}
	})
}

func TestEngine_RapidBlocks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	blockEventer := make(chan *types.Block, 20)
	cfg := newTestConfig(blockEventer)

	protoID := ProtocolID("rapid-proto")
	mockProto := NewMockBlockSynchronizedProtocol(protoID) // Must be block-synchronized for this test
	cfg.BlockSynchronizedProtocols[protoID] = mockProto

	engine, err := NewEngine(ctx, cfg)
	require.NoError(t, err)

	sub, unsub := engine.Subscribe()
	defer unsub()

	// Feed 15 blocks
	for i := 1; i <= 15; i++ {
		blockEventer <- types.NewBlock(&types.Header{Number: big.NewInt(int64(i))}, nil, nil, nil)
	}

	// Consume 15 blocks
	var lastView *State
	for i := 1; i <= 15; i++ {
		// Update mock sync so engine doesn't timeout
		mockProto.SetSyncedBlock(uint64(i))

		select {
		case view := <-sub.C():
			lastView = view
		case <-time.After(200 * time.Millisecond):
			t.Fatalf("Timeout waiting for block %d", i)
		}
	}

	require.NotNil(t, lastView)
	assert.Equal(t, uint64(15), lastView.Block.Number.Uint64())
	assert.Equal(t, uint64(15), engine.LastProcessedBlock())
}

func TestEngine_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	blockEventer := make(chan *types.Block, 1)
	cfg := newTestConfig(blockEventer)
	engine, err := NewEngine(ctx, cfg)
	require.NoError(t, err)

	sub, unsub := engine.Subscribe()
	defer unsub()

	cancel() // Trigger shutdown

	select {
	case <-sub.Done():
		// Success
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Subscription Done channel not closed on context cancel")
	}
}

func TestEngine_ConfigValidation_Duplicates(t *testing.T) {
	// Test that we cannot register the same ID in both maps
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	blockEventer := make(chan *types.Block, 1)
	cfg := newTestConfig(blockEventer)

	dupID := ProtocolID("duplicate-id")

	// Create generic protocols for this test
	cfg.Protocols[dupID] = NewMockProtocol(dupID)
	cfg.BlockSynchronizedProtocols[dupID] = NewMockBlockSynchronizedProtocol(dupID)

	_, err := NewEngine(ctx, cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "registered in both")
}
