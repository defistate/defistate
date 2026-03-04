package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"testing"
	"time"

	differ "github.com/defistate/defistate/differ"
	"github.com/defistate/defistate/engine"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Test Setup: Mock RPC Server ---

type MockStateStreamer struct {
	events chan *SubscriptionEvent
	t      *testing.T
}

func SetupMockStateStreamer(ctx context.Context, t *testing.T, port int, events []*SubscriptionEvent) (<-chan error, error) {
	eventChan := make(chan *SubscriptionEvent, len(events))
	for _, e := range events {
		eventChan <- e
	}
	close(eventChan)

	api := &MockStateStreamer{events: eventChan, t: t}
	server := rpc.NewServer()
	if err := server.RegisterName("defi", api); err != nil {
		return nil, fmt.Errorf("failed to register API: %v", err)
	}

	wsHandler := server.WebsocketHandler([]string{"*"})
	httpServer := &http.Server{Addr: fmt.Sprintf(":%d", port), Handler: wsHandler}

	errChan := make(chan error, 1)
	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- err
		}
	}()
	go func() {
		<-ctx.Done()
		server.Stop()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
	}()

	return errChan, nil
}

func (api *MockStateStreamer) SubscribeStateStream(ctx context.Context) (*rpc.Subscription, error) {
	notifier, supported := rpc.NotifierFromContext(ctx)
	if !supported {
		return nil, rpc.ErrNotificationsUnsupported
	}

	rpcSub := notifier.CreateSubscription()
	go func() {
		for event := range api.events {
			select {
			case <-rpcSub.Err():
				return
			default:
				if err := notifier.Notify(rpcSub.ID, event); err != nil {
					api.t.Logf("Error notifying subscriber: %v", err)
					return
				}
			}
		}
	}()
	return rpcSub, nil
}

// --- Test Helpers & Data Generation ---

var mockDecoder = func(schema engine.ProtocolSchema, data json.RawMessage) (any, error) {
	if len(data) == 0 {
		return map[string]any{}, nil
	}
	var genericMap map[string]any
	err := json.Unmarshal(data, &genericMap)
	return genericMap, err
}

func generateTestEvents(t *testing.T) []*SubscriptionEvent {
	mustMarshal := func(v interface{}) json.RawMessage {
		data, err := json.Marshal(v)
		require.NoError(t, err)
		return data
	}

	pID := engine.ProtocolID("uniswap_v2")
	schema := engine.ProtocolSchema("uniswap-v2@v1")

	// --- Event 1: Full View ---
	fullViewPayload := engine.State{
		Block: engine.BlockSummary{
			Number:     big.NewInt(100),
			ReceivedAt: time.Now().UnixNano(),
		},
		Protocols: map[engine.ProtocolID]engine.ProtocolState{
			pID: {
				Meta:   engine.ProtocolMeta{Name: "Uniswap V2"},
				Schema: schema,
				Data:   map[string]interface{}{"id": 1, "reserve": 1000},
			},
		},
	}
	event1 := &SubscriptionEvent{Type: "full", Payload: mustMarshal(fullViewPayload)}

	// --- Event 2: Diff ---
	// IMPORTANT: We use an anonymous struct to force the JSON tag "protocols"
	// to match the clientStateDiff definition in types.go.
	// If differ.StateDiff uses "protocolDiffs", the standard marshal would fail to be read by client.
	diffStruct := struct {
		FromBlock uint64                                    `json:"fromBlock"`
		ToBlock   engine.BlockSummary                       `json:"toBlock"`
		Timestamp uint64                                    `json:"timestamp"`
		Protocols map[engine.ProtocolID]differ.ProtocolDiff `json:"protocols"` // Tag must match client types.go
	}{
		FromBlock: 100,
		ToBlock: engine.BlockSummary{
			Number:     big.NewInt(101),
			ReceivedAt: time.Now().UnixNano(),
		},
		Timestamp: uint64(time.Now().Unix()),
		Protocols: map[engine.ProtocolID]differ.ProtocolDiff{
			pID: {
				Schema: schema,
				Data:   map[string]interface{}{"id": 1, "reserve": 12345},
			},
		},
	}
	event2 := &SubscriptionEvent{Type: "diff", Payload: mustMarshal(diffStruct)}

	// --- Event 3: Malformed ---
	malformedPayload := json.RawMessage(`{"block":{"number":"not-a-number"}}`)
	event3 := &SubscriptionEvent{Type: "full", Payload: malformedPayload}

	// --- Event 4: Another Full ---
	goodViewPayload2 := engine.State{
		Block: engine.BlockSummary{
			Number:     big.NewInt(2),
			ReceivedAt: time.Now().UnixNano(),
		},
	}
	event4 := &SubscriptionEvent{Type: "full", Payload: mustMarshal(goodViewPayload2)}

	return []*SubscriptionEvent{event1, event2, event3, event4}
}

// --- Tests ---

var noopStatePatcher = func(prevView *engine.State, diff *differ.StateDiff) (*engine.State, error) {
	// FIX: Guard against nil pointer if test data is malformed
	block := diff.ToBlock
	if block.Number == nil {
		block.Number = big.NewInt(0)
	}
	return &engine.State{
		Block:     block,
		Protocols: map[engine.ProtocolID]engine.ProtocolState{},
	}, nil
}

func TestClient_SuccessfulSubscription(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	testEvents := generateTestEvents(t)
	_, err := SetupMockStateStreamer(ctx, t, 9988, testEvents[:1])
	require.NoError(t, err)

	client, err := NewClient(ctx, Config{
		URL:              "ws://localhost:9988",
		Logger:           slog.New(slog.NewTextHandler(io.Discard, nil)),
		BufferSize:       10,
		StatePatcher:     noopStatePatcher,
		StateDecoder:     mockDecoder,
		StateDiffDecoder: mockDecoder,
	})
	require.NoError(t, err)

	select {
	case view := <-client.State():
		assert.Equal(t, int64(100), view.Block.Number.Int64())
		protocolData, ok := view.Protocols["uniswap_v2"]
		require.True(t, ok, "Protocol data should exist")
		dataMap := protocolData.Data.(map[string]any)
		assert.Equal(t, float64(1), dataMap["id"])
	case <-time.After(2 * time.Second):
		t.Fatal("Test timed out waiting for state view")
	}
}

func TestClient_DiffReconstruction(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	testEvents := generateTestEvents(t)
	_, err := SetupMockStateStreamer(ctx, t, 9987, testEvents[:2])
	require.NoError(t, err)

	constructorCalled := false

	mockConstructor := func(prevView *engine.State, diff *differ.StateDiff) (*engine.State, error) {
		constructorCalled = true
		require.NotNil(t, prevView)
		require.NotNil(t, diff)

		// FIX: Guard against potential nil Block.Number
		if prevView.Block.Number != nil {
			assert.Equal(t, uint64(100), prevView.Block.Number.Uint64())
		}
		assert.Equal(t, uint64(100), diff.FromBlock)
		if diff.ToBlock.Number != nil {
			assert.Equal(t, uint64(101), diff.ToBlock.Number.Uint64())
		}

		pDiff, ok := diff.Protocols["uniswap_v2"]
		require.True(t, ok)
		dataMap := pDiff.Data.(map[string]any)
		assert.Equal(t, float64(12345), dataMap["reserve"])

		return &engine.State{
			Block: diff.ToBlock,
			// FIX: Initialize the map to prevent nil-map issues downstream
			Protocols: make(map[engine.ProtocolID]engine.ProtocolState),
		}, nil
	}

	client, err := NewClient(ctx, Config{
		URL:              "ws://localhost:9987",
		Logger:           slog.New(slog.NewTextHandler(io.Discard, nil)),
		BufferSize:       10,
		StatePatcher:     mockConstructor,
		StateDecoder:     mockDecoder,
		StateDiffDecoder: mockDecoder,
	})
	require.NoError(t, err)

	select {
	case view1 := <-client.State():
		assert.Equal(t, int64(100), view1.Block.Number.Int64())
	case <-time.After(2 * time.Second):
		t.Fatal("Test timed out waiting for initial full view")
	}

	select {
	case view2 := <-client.State():
		assert.Equal(t, int64(101), view2.Block.Number.Int64())
	case <-time.After(2 * time.Second):
		t.Fatal("Test timed out waiting for reconstructed diff view")
	}

	assert.True(t, constructorCalled, "The injected diff constructor should have been called")
}

func TestClient_DropsMalformedMessage(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	testEvents := generateTestEvents(t)
	_, err := SetupMockStateStreamer(ctx, t, 9989, append(testEvents[0:1], testEvents[2:4]...))
	require.NoError(t, err)

	client, err := NewClient(ctx, Config{
		URL:              "ws://localhost:9989",
		Logger:           slog.New(slog.NewTextHandler(io.Discard, nil)),
		BufferSize:       10,
		StatePatcher:     noopStatePatcher,
		StateDecoder:     mockDecoder,
		StateDiffDecoder: mockDecoder,
	})
	require.NoError(t, err)

	receivedCount := 0
	expectedBlocks := map[int64]bool{100: false, 2: false}

	for i := 0; i < 2; i++ {
		select {
		case view := <-client.State():
			receivedCount++
			if view.Block.Number != nil {
				expectedBlocks[view.Block.Number.Int64()] = true
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("Test timed out waiting for view %d", i+1)
		}
	}
	assert.Equal(t, 2, receivedCount)
	assert.True(t, expectedBlocks[100])
	assert.True(t, expectedBlocks[2])
}

func TestClient_Reconnection(t *testing.T) {
	const testPort = 9990
	clientCtx, clientCancel := context.WithCancel(context.Background())
	defer clientCancel()

	client, err := NewClient(clientCtx, Config{
		URL:              fmt.Sprintf("ws://localhost:%d", testPort),
		Logger:           slog.New(slog.NewTextHandler(io.Discard, nil)),
		BufferSize:       10,
		StatePatcher:     noopStatePatcher,
		StateDecoder:     mockDecoder,
		StateDiffDecoder: mockDecoder,
	})
	require.NoError(t, err)

	server1Ctx, server1Cancel := context.WithCancel(clientCtx)
	// IMPORTANT: Provide full JSON structure so block number is parsed correctly
	event1 := []*SubscriptionEvent{{Type: "full", Payload: json.RawMessage(`{"block":{"number":1}}`)}}
	_, err = SetupMockStateStreamer(server1Ctx, t, testPort, event1)
	require.NoError(t, err)

	select {
	case view := <-client.State():
		assert.Equal(t, int64(1), view.Block.Number.Int64())
	case <-time.After(2 * time.Second):
		t.Fatal("Timed out waiting for first message")
	}

	server1Cancel()
	time.Sleep(100 * time.Millisecond)

	server2Ctx, server2Cancel := context.WithCancel(clientCtx)
	defer server2Cancel()
	event2 := []*SubscriptionEvent{{Type: "full", Payload: json.RawMessage(`{"block":{"number":2}}`)}}
	_, err = SetupMockStateStreamer(server2Ctx, t, testPort, event2)
	require.NoError(t, err)

	select {
	case view := <-client.State():
		assert.Equal(t, int64(2), view.Block.Number.Int64())
	case <-time.After(5 * time.Second):
		t.Fatal("Timed out waiting for client to reconnect")
	}
}

// --- StreamProcessor Tests ---

func TestStreamProcessor_FullAndDiffFlow(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Use a patcher that actually updates the block number so we can verify diffs
	statePatcher := func(prev *engine.State, diff *differ.StateDiff) (*engine.State, error) {
		return &engine.State{
			Block:     diff.ToBlock,   // Update block info
			Protocols: prev.Protocols, // Keep protocols (simplification)
		}, nil
	}

	sp := NewStreamProcessor(logger, 10, statePatcher, mockDecoder, mockDecoder)

	events := generateTestEvents(t)
	// Event 0: Full (Block 100)
	// Event 1: Diff (100->101)

	// 1. Process Full State
	fullEventBytes, err := json.Marshal(events[0])
	require.NoError(t, err)

	err = sp.ProcessMessage(fullEventBytes)
	require.NoError(t, err)

	select {
	case state := <-sp.State():
		assert.Equal(t, int64(100), state.Block.Number.Int64())
	case <-time.After(time.Second):
		t.Fatal("Timeout waiting for full state")
	}

	// 2. Process Diff
	diffEventBytes, err := json.Marshal(events[1])
	require.NoError(t, err)

	err = sp.ProcessMessage(diffEventBytes)
	require.NoError(t, err)

	select {
	case state := <-sp.State():
		assert.Equal(t, int64(101), state.Block.Number.Int64())
	case <-time.After(time.Second):
		t.Fatal("Timeout waiting for diff state")
	}
}

func TestStreamProcessor_ValidationErrors(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	sp := NewStreamProcessor(logger, 10, noopStatePatcher, mockDecoder, mockDecoder)

	events := generateTestEvents(t)
	// Event 1 is Diff (100->101)

	// 1. Diff before Full
	diffEventBytes, _ := json.Marshal(events[1])
	err := sp.ProcessMessage(diffEventBytes)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "received diff before full state")

	// 2. Malformed JSON
	err = sp.ProcessMessage([]byte(`{not-json}`))
	require.Error(t, err)
}

func TestStreamProcessor_OutOfOrderDiff(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	sp := NewStreamProcessor(logger, 10, noopStatePatcher, mockDecoder, mockDecoder)

	events := generateTestEvents(t)
	// Process Full first
	fullEventBytes, _ := json.Marshal(events[0]) // Block 100
	require.NoError(t, sp.ProcessMessage(fullEventBytes))
	<-sp.State() // Drain

	// Create Gap Diff (105 -> 106)
	gapStruct := struct {
		FromBlock uint64                                    `json:"fromBlock"`
		ToBlock   engine.BlockSummary                       `json:"toBlock"`
		Timestamp uint64                                    `json:"timestamp"`
		Protocols map[engine.ProtocolID]differ.ProtocolDiff `json:"protocols"`
	}{
		FromBlock: 105,
		ToBlock:   engine.BlockSummary{Number: big.NewInt(106)},
		Timestamp: uint64(time.Now().Unix()),
		Protocols: map[engine.ProtocolID]differ.ProtocolDiff{},
	}
	payload, _ := json.Marshal(gapStruct)
	gapEvent := &SubscriptionEvent{Type: "diff", Payload: payload}
	gapBytes, _ := json.Marshal(gapEvent)

	// Should not error, but logger warn and not emit state
	err := sp.ProcessMessage(gapBytes)
	require.NoError(t, err)

	select {
	case <-sp.State():
		t.Fatal("Should not emit state for out-of-order diff")
	default:
		// OK
	}
}
