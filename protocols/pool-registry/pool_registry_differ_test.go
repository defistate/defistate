package poolregistry

import (
	"testing"

	"github.com/defistate/defistate/engine"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPoolRegistryDiffer(t *testing.T) {
	// --- Base Data ---
	key1 := AddressToPoolKey(common.HexToAddress("0x1"))
	key2 := AddressToPoolKey(common.HexToAddress("0x2"))

	protoUni := engine.ProtocolID("uniswap")
	protoCurve := engine.ProtocolID("curve")
	protoSushi := engine.ProtocolID("sushi")

	// Helper to create a view easily
	makeView := func(pools []PoolView, protos map[uint16]engine.ProtocolID) PoolRegistryView {
		if protos == nil {
			protos = make(map[uint16]engine.ProtocolID)
		}
		return PoolRegistryView{Pools: pools, Protocols: protos}
	}

	t.Run("Should identify Pool Additions", func(t *testing.T) {
		pool1 := PoolView{ID: 1, Key: key1, Protocol: 0}
		pool2 := PoolView{ID: 2, Key: key2, Protocol: 0} // New pool, same protocol

		oldState := makeView([]PoolView{pool1}, map[uint16]engine.ProtocolID{0: protoUni})
		newState := makeView([]PoolView{pool1, pool2}, map[uint16]engine.ProtocolID{0: protoUni})

		diff := Differ(oldState, newState)

		require.False(t, diff.IsEmpty())
		assert.Len(t, diff.PoolAdditions, 1)
		assert.Equal(t, pool2, diff.PoolAdditions[0])

		assert.Empty(t, diff.PoolDeletions)
		assert.Empty(t, diff.ProtocolAdditions) // Protocol 0 already existed
	})

	t.Run("Should identify Pool and Protocol Additions", func(t *testing.T) {
		pool1 := PoolView{ID: 1, Key: key1, Protocol: 0}
		pool2 := PoolView{ID: 2, Key: key2, Protocol: 1} // New pool, NEW protocol

		oldState := makeView([]PoolView{pool1}, map[uint16]engine.ProtocolID{0: protoUni})
		newState := makeView(
			[]PoolView{pool1, pool2},
			map[uint16]engine.ProtocolID{0: protoUni, 1: protoCurve},
		)

		diff := Differ(oldState, newState)

		// Check Pool Addition
		assert.Len(t, diff.PoolAdditions, 1)
		assert.Equal(t, pool2, diff.PoolAdditions[0])

		// Check Protocol Addition
		assert.Len(t, diff.ProtocolAdditions, 1)
		assert.Equal(t, protoCurve, diff.ProtocolAdditions[1])
	})

	t.Run("Should identify Deletions", func(t *testing.T) {
		pool1 := PoolView{ID: 1, Key: key1, Protocol: 0}

		oldState := makeView([]PoolView{pool1}, map[uint16]engine.ProtocolID{0: protoUni})
		newState := makeView([]PoolView{}, map[uint16]engine.ProtocolID{0: protoUni})

		diff := Differ(oldState, newState)

		assert.Len(t, diff.PoolDeletions, 1)
		assert.Equal(t, uint64(1), diff.PoolDeletions[0])

		assert.Empty(t, diff.PoolAdditions)
		assert.Empty(t, diff.ProtocolDeletions) // Protocol map wasn't cleaned up
	})

	t.Run("Should identify Protocol Deletions", func(t *testing.T) {
		// Old state has an unused protocol that gets cleaned up in new state
		oldState := makeView([]PoolView{}, map[uint16]engine.ProtocolID{0: protoUni})
		newState := makeView([]PoolView{}, map[uint16]engine.ProtocolID{})

		diff := Differ(oldState, newState)

		assert.Len(t, diff.ProtocolDeletions, 1)
		assert.Equal(t, uint16(0), diff.ProtocolDeletions[0])
	})

	t.Run("Should handle Mixed Changes", func(t *testing.T) {
		// Scenario:
		// 1. Pool 1 (Uniswap) is deleted.
		// 2. Pool 2 (Curve) is added.
		// 3. Protocol 0 (Uniswap) is removed from dictionary (cleanup).
		// 4. Protocol 1 (Curve) is added to dictionary.

		p1 := PoolView{ID: 1, Key: key1, Protocol: 0}
		p2 := PoolView{ID: 2, Key: key2, Protocol: 1}

		oldState := makeView(
			[]PoolView{p1},
			map[uint16]engine.ProtocolID{0: protoUni},
		)
		newState := makeView(
			[]PoolView{p2},
			map[uint16]engine.ProtocolID{1: protoCurve},
		)

		diff := Differ(oldState, newState)

		// Pools
		assert.Len(t, diff.PoolDeletions, 1)
		assert.Contains(t, diff.PoolDeletions, uint64(1))

		assert.Len(t, diff.PoolAdditions, 1)
		assert.Equal(t, p2, diff.PoolAdditions[0])

		// Protocols
		assert.Len(t, diff.ProtocolDeletions, 1)
		assert.Contains(t, diff.ProtocolDeletions, uint16(0))

		assert.Len(t, diff.ProtocolAdditions, 1)
		assert.Equal(t, protoCurve, diff.ProtocolAdditions[1])
	})

	t.Run("Should handle Protocol Updates (Edge Case)", func(t *testing.T) {
		// If ID 0 changes definition from "uniswap" to "sushi" (should rarely happen, but possible)
		p1 := PoolView{ID: 1, Key: key1, Protocol: 0}

		oldState := makeView([]PoolView{p1}, map[uint16]engine.ProtocolID{0: protoUni})
		newState := makeView([]PoolView{p1}, map[uint16]engine.ProtocolID{0: protoSushi})

		diff := Differ(oldState, newState)

		assert.Len(t, diff.ProtocolAdditions, 1)
		assert.Equal(t, protoSushi, diff.ProtocolAdditions[0], "Should verify the new mapping overwrites the old")
	})

	t.Run("Should be empty on No Changes", func(t *testing.T) {
		p1 := PoolView{ID: 1, Key: key1, Protocol: 0}
		state := makeView([]PoolView{p1}, map[uint16]engine.ProtocolID{0: protoUni})

		diff := Differ(state, state)

		assert.True(t, diff.IsEmpty())
		assert.Empty(t, diff.PoolAdditions)
		assert.Empty(t, diff.PoolDeletions)
		assert.Empty(t, diff.ProtocolAdditions)
		assert.Empty(t, diff.ProtocolDeletions)
	})
}
