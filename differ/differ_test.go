package differ

import (
	"errors"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/defistate/defistate/engine"
	"github.com/ethereum/go-ethereum/common"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --------------------------------------------------------------------------------
// --- Mocks & Helpers ---
// --------------------------------------------------------------------------------

// noOpLogger suppresses logging during tests
type noOpLogger struct{}

func (l *noOpLogger) Debug(msg string, args ...any) {}
func (l *noOpLogger) Info(msg string, args ...any)  {}
func (l *noOpLogger) Warn(msg string, args ...any)  {}
func (l *noOpLogger) Error(msg string, args ...any) {}

// mockStringDiffer is a simple differ for testing that expects strings.
// It returns a map describing the change.
func mockStringDiffer(old, new any) (any, error) {
	oldStr, ok1 := old.(string)
	newStr, ok2 := new.(string)
	if !ok1 || !ok2 {
		return nil, errors.New("mockStringDiffer: type mismatch")
	}

	// Logic: Return a diff only if they are different
	if oldStr == newStr {
		return nil, nil // No diff
	}

	return map[string]string{
		"change_type": "update",
		"from":        oldStr,
		"to":          newStr,
	}, nil
}

// makeState is a helper to construct a generic State object for testing
func makeState(blockNum uint64, protocols map[engine.ProtocolID]engine.ProtocolState) *engine.State {
	bNum := big.NewInt(int64(blockNum))
	return &engine.State{
		Block: engine.BlockSummary{
			Number:     bNum,
			Hash:       common.BigToHash(bNum),
			ReceivedAt: time.Now().UnixNano(),
		},
		Timestamp: uint64(time.Now().UnixNano()),
		Protocols: protocols,
	}
}

// --------------------------------------------------------------------------------
// --- Main Test Suite ---
// --------------------------------------------------------------------------------

func TestStateDiffer_HappyPath(t *testing.T) {
	// 1. Setup Configuration
	schema := engine.ProtocolSchema("mock/string@v1")
	cfg := &StateDifferConfig{
		ProtocolDiffers: map[engine.ProtocolSchema]ProtocolDiffer{
			schema: mockStringDiffer,
		},
		Registry: prometheus.NewRegistry(),
		Logger:   &noOpLogger{},
	}
	differ, err := NewStateDiffer(cfg)
	require.NoError(t, err)

	// 2. Prepare Data
	pID := engine.ProtocolID("protocol-1")

	// Old State: Value is "A"
	oldState := makeState(100, map[engine.ProtocolID]engine.ProtocolState{
		pID: {
			Meta:              engine.ProtocolMeta{Name: "Protocol One"},
			SyncedBlockNumber: new(uint64), // pointer to 0
			Schema:            schema,
			Data:              "Value_A",
		},
	})
	*oldState.Protocols[pID].SyncedBlockNumber = 100

	// New State: Value is "B"
	newState := makeState(101, map[engine.ProtocolID]engine.ProtocolState{
		pID: {
			Meta:              engine.ProtocolMeta{Name: "Protocol One"},
			SyncedBlockNumber: new(uint64),
			Schema:            schema,
			Data:              "Value_B",
		},
	})
	*newState.Protocols[pID].SyncedBlockNumber = 101

	// 3. Execute Diff
	diff, err := differ.Diff(oldState, newState)
	require.NoError(t, err)
	require.NotNil(t, diff)

	// 4. Verify Headers
	assert.Equal(t, uint64(100), diff.FromBlock)
	assert.Equal(t, uint64(101), diff.ToBlock.Number.Uint64())

	// 5. Verify Protocol Diff
	pDiff, exists := diff.Protocols[pID]
	require.True(t, exists, "Protocol diff should exist")
	assert.Equal(t, schema, pDiff.Schema)
	assert.Equal(t, uint64(101), *pDiff.SyncedBlockNumber)

	// Check the actual diff payload returned by mockStringDiffer
	dataMap, ok := pDiff.Data.(map[string]string)
	require.True(t, ok)
	assert.Equal(t, "Value_A", dataMap["from"])
	assert.Equal(t, "Value_B", dataMap["to"])
}

func TestStateDiffer_MissingSchema_ShouldError(t *testing.T) {
	// Setup with NO differs registered
	cfg := &StateDifferConfig{
		ProtocolDiffers: map[engine.ProtocolSchema]ProtocolDiffer{},
		Registry:        prometheus.NewRegistry(),
		Logger:          &noOpLogger{},
	}
	differ, err := NewStateDiffer(cfg)
	require.NoError(t, err)

	// Create states with a schema we haven't registered
	pID := engine.ProtocolID("p1")
	unknownSchema := engine.ProtocolSchema("unknown@v1")

	oldState := makeState(100, map[engine.ProtocolID]engine.ProtocolState{
		pID: {Schema: unknownSchema, Data: "A"},
	})
	newState := makeState(101, map[engine.ProtocolID]engine.ProtocolState{
		pID: {Schema: unknownSchema, Data: "B"},
	})

	// Execute
	_, err = differ.Diff(oldState, newState)

	// Verify
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no differ registered for schema")
}

func TestStateDiffer_ProtocolMismatch(t *testing.T) {
	// Test the "ProtocolID does not exist in old state" logic
	schema := engine.ProtocolSchema("mock@v1")
	cfg := &StateDifferConfig{
		ProtocolDiffers: map[engine.ProtocolSchema]ProtocolDiffer{schema: mockStringDiffer},
		Registry:        prometheus.NewRegistry(),
		Logger:          &noOpLogger{},
	}
	differ, _ := NewStateDiffer(cfg)

	// Old state has NOTHING
	oldState := makeState(100, map[engine.ProtocolID]engine.ProtocolState{})

	// New state has Protocol P1
	newState := makeState(101, map[engine.ProtocolID]engine.ProtocolState{
		"p1": {Schema: schema, Data: "A"},
	})

	_, err := differ.Diff(oldState, newState)
	require.Error(t, err)

	// Check logic: We accept either the strict length error OR the missing ID error,
	// depending on how you configured the engine in differ.go
	errStr := err.Error()
	isLengthErr := strings.Contains(errStr, "inconsistent protocol length")
	isMissingErr := strings.Contains(errStr, "does not exist in old state")

	assert.True(t, isLengthErr || isMissingErr, "Unexpected error message: %s", errStr)
}

func TestStateDiffer_InputErrors(t *testing.T) {
	// Test that we reject states containing errors
	cfg := &StateDifferConfig{
		ProtocolDiffers: map[engine.ProtocolSchema]ProtocolDiffer{},
		Registry:        prometheus.NewRegistry(),
		Logger:          &noOpLogger{},
	}
	differ, _ := NewStateDiffer(cfg)

	// Create a state that has an error in one of its protocols
	errorState := makeState(100, map[engine.ProtocolID]engine.ProtocolState{
		"p1": {Error: "DB Connection Failed"},
	})
	validState := makeState(101, map[engine.ProtocolID]engine.ProtocolState{})

	// Check Old has error
	_, err := differ.Diff(errorState, validState)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "received view with error")

	// Check New has error
	_, err = differ.Diff(validState, errorState)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "received view with error")
}
