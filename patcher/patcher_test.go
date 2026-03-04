package patcher

import (
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/defistate/defistate/differ"
	"github.com/defistate/defistate/engine"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --------------------------------------------------------------------------------
// --- Mocks ---
// --------------------------------------------------------------------------------

// mockIntPatcher is a simple generic patcher for testing.
// It treats the State as an Integer and the Diff as an addition.
// This proves the engine can carry values and update them without knowing what they are.
func mockIntPatcher(old any, diff any) (any, error) {
	val := 0
	if old != nil {
		val = old.(int)
	}
	delta, ok := diff.(int)
	if !ok {
		return nil, errors.New("diff is not int")
	}
	return val + delta, nil
}

// --------------------------------------------------------------------------------
// --- Helpers ---
// --------------------------------------------------------------------------------

func makeState(blockNum uint64, protocols map[engine.ProtocolID]engine.ProtocolState) *engine.State {
	bNum := big.NewInt(int64(blockNum))
	return &engine.State{
		Block: engine.BlockSummary{
			Number: bNum,
			Hash:   common.BigToHash(bNum),
		},
		Timestamp: uint64(time.Now().UnixNano()),
		Protocols: protocols,
	}
}

// --------------------------------------------------------------------------------
// --- Main Test Suite ---
// --------------------------------------------------------------------------------

func TestStatePatcher_HappyPath(t *testing.T) {
	// 1. Setup Config
	// We register our generic integer patcher against a test schema.
	schema := engine.ProtocolSchema("mock/int@v1")
	cfg := &StatePatcherConfig{
		Patchers: map[engine.ProtocolSchema]PatcherFunc{
			schema: mockIntPatcher,
		},
	}
	patcher, err := NewStatePatcher(cfg)
	require.NoError(t, err)

	// 2. Setup Data
	// "uniswap_v2" -> Value 10
	// "uniswap_v3" -> Value 50
	p1 := engine.ProtocolID("uniswap_v2_mainnet")
	p2 := engine.ProtocolID("uniswap_v3_mainnet")

	oldState := makeState(100, map[engine.ProtocolID]engine.ProtocolState{
		p1: {Schema: schema, Data: 10},
		p2: {Schema: schema, Data: 50},
	})

	// 3. Create Diff
	// "uniswap_v2" -> Add 5  (Update)
	// "uniswap_v3" -> Missing (No Change)
	// "pancake_v3" -> Add 100 (New Protocol)
	p3 := engine.ProtocolID("pancakeswap_v3_bsc")

	diff := &differ.StateDiff{
		FromBlock: 100,
		ToBlock: engine.BlockSummary{
			Number: big.NewInt(101),
		},
		Protocols: map[engine.ProtocolID]differ.ProtocolDiff{
			p1: {Schema: schema, Data: 5},
			p3: {Schema: schema, Data: 100},
		},
	}

	// 4. Execute Patch
	newState, err := patcher.Patch(oldState, diff)
	require.NoError(t, err)

	// 5. Verify Results
	assert.Equal(t, uint64(101), newState.Block.Number.Uint64())

	// Verify P1 (Update: 10 + 5 = 15)
	res1, ok := newState.Protocols[p1]
	require.True(t, ok)
	assert.Equal(t, 15, res1.Data.(int))

	// Verify P2 (Deep Copy / Persistence: 50)
	res2, ok := newState.Protocols[p2]
	require.True(t, ok)
	assert.Equal(t, 50, res2.Data.(int))

	// Verify P3 (New Creation: 0 + 100 = 100)
	res3, ok := newState.Protocols[p3]
	require.True(t, ok)
	assert.Equal(t, 100, res3.Data.(int))
}

func TestStatePatcher_BlockMismatch(t *testing.T) {
	patcher, _ := NewStatePatcher(&StatePatcherConfig{})

	oldState := makeState(100, nil)
	diff := &differ.StateDiff{FromBlock: 99} // Mismatch!

	_, err := patcher.Patch(oldState, diff)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mismatch fromBlock")
}

func TestStatePatcher_MissingPatcher(t *testing.T) {
	// Setup patcher with NO registered functions
	patcher, _ := NewStatePatcher(&StatePatcherConfig{
		Patchers: map[engine.ProtocolSchema]PatcherFunc{},
	})

	schema := engine.ProtocolSchema("unknown")
	oldState := makeState(100, nil)
	diff := &differ.StateDiff{
		FromBlock: 100,
		Protocols: map[engine.ProtocolID]differ.ProtocolDiff{
			"p1": {Schema: schema, Data: 1},
		},
	}

	_, err := patcher.Patch(oldState, diff)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no patcher registered")
}

func TestStatePatcher_SchemaMismatch(t *testing.T) {
	// Register schema B
	schemaA := engine.ProtocolSchema("A")
	schemaB := engine.ProtocolSchema("B")
	cfg := &StatePatcherConfig{
		Patchers: map[engine.ProtocolSchema]PatcherFunc{
			schemaB: mockIntPatcher,
		},
	}
	patcher, _ := NewStatePatcher(cfg)

	pID := engine.ProtocolID("p1")

	// Old state has Schema A
	oldState := makeState(100, map[engine.ProtocolID]engine.ProtocolState{
		pID: {Schema: schemaA, Data: 1},
	})

	// Diff attempts to update it using Schema B
	diff := &differ.StateDiff{
		FromBlock: 100,
		Protocols: map[engine.ProtocolID]differ.ProtocolDiff{
			pID: {Schema: schemaB, Data: 1},
		},
	}

	_, err := patcher.Patch(oldState, diff)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "schema mismatch")
}
