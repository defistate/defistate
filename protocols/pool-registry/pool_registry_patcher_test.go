package poolregistry

import (
	"testing"

	"github.com/defistate/defistate/engine"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPoolRegistryPatcher(t *testing.T) {
	// --- Base Data ---
	key1 := AddressToPoolKey(common.HexToAddress("0x1"))
	key2 := AddressToPoolKey(common.HexToAddress("0x2"))
	key3 := AddressToPoolKey(common.HexToAddress("0x3"))

	protoUni := engine.ProtocolID("uniswap")
	protoCurve := engine.ProtocolID("curve")
	protoSushi := engine.ProtocolID("sushi")

	// Helper to construct a view easily
	makeView := func(pools []PoolView, protos map[uint16]engine.ProtocolID) PoolRegistryView {
		if protos == nil {
			protos = make(map[uint16]engine.ProtocolID)
		}
		return PoolRegistryView{Pools: pools, Protocols: protos}
	}

	// Helper to find a pool in the resulting slice
	findPool := func(view PoolRegistryView, id uint64) *PoolView {
		for _, p := range view.Pools {
			if p.ID == id {
				v := p // copy loop variable
				return &v
			}
		}
		return nil
	}

	// Initial State: 3 pools, 2 protocols
	pool1 := PoolView{ID: 1, Key: key1, Protocol: 0}
	pool2 := PoolView{ID: 2, Key: key2, Protocol: 1}
	pool3 := PoolView{ID: 3, Key: key3, Protocol: 0}

	initialState := makeView(
		[]PoolView{pool1, pool2, pool3},
		map[uint16]engine.ProtocolID{0: protoUni, 1: protoCurve},
	)

	t.Run("Should handle Pool Additions only", func(t *testing.T) {
		pool4 := PoolView{ID: 4, Key: AddressToPoolKey(common.HexToAddress("0x4")), Protocol: 0}

		diff := PoolRegistryDiff{
			PoolAdditions: []PoolView{pool4},
		}

		newState, err := Patcher(initialState, diff)
		require.NoError(t, err)

		assert.Len(t, newState.Pools, 4)
		assert.NotNil(t, findPool(newState, 4))
		assert.Equal(t, initialState.Protocols, newState.Protocols, "Protocols should remain unchanged")
	})

	t.Run("Should handle Pool and Protocol Additions", func(t *testing.T) {
		// Add Pool 5 which uses a NEW protocol (ID 2: sushi)
		pool5 := PoolView{ID: 5, Key: AddressToPoolKey(common.HexToAddress("0x5")), Protocol: 2}

		diff := PoolRegistryDiff{
			PoolAdditions:     []PoolView{pool5},
			ProtocolAdditions: map[uint16]engine.ProtocolID{2: protoSushi},
		}

		newState, err := Patcher(initialState, diff)
		require.NoError(t, err)

		// Verify Pool
		assert.Len(t, newState.Pools, 4)
		p5 := findPool(newState, 5)
		require.NotNil(t, p5)
		assert.Equal(t, uint16(2), p5.Protocol)

		// Verify Protocol Dictionary
		assert.Len(t, newState.Protocols, 3)
		assert.Equal(t, protoSushi, newState.Protocols[2])
	})

	t.Run("Should handle Pool Deletions only", func(t *testing.T) {
		diff := PoolRegistryDiff{
			PoolDeletions: []uint64{2}, // Delete pool 2
		}

		newState, err := Patcher(initialState, diff)
		require.NoError(t, err)

		assert.Len(t, newState.Pools, 2)
		assert.Nil(t, findPool(newState, 2), "Pool 2 should be deleted")
		assert.NotNil(t, findPool(newState, 1))
	})

	t.Run("Should handle Protocol Deletions", func(t *testing.T) {
		// Delete protocol 1 (curve) - assume no pools use it anymore
		diff := PoolRegistryDiff{
			ProtocolDeletions: []uint16{1},
		}

		newState, err := Patcher(initialState, diff)
		require.NoError(t, err)

		assert.Len(t, newState.Protocols, 1)
		_, exists := newState.Protocols[1]
		assert.False(t, exists, "Protocol 1 should be removed")
	})

	t.Run("Should handle Mixed Operations (Add/Delete Pools & Protocols)", func(t *testing.T) {
		// 1. Delete Pool 2 (Curve user)
		// 2. Add Pool 4 (Sushi user)
		// 3. Delete Protocol 1 (Curve - cleanup)
		// 4. Add Protocol 2 (Sushi)

		pool4 := PoolView{ID: 4, Key: AddressToPoolKey(common.HexToAddress("0x4")), Protocol: 2}

		diff := PoolRegistryDiff{
			PoolDeletions:     []uint64{2},
			PoolAdditions:     []PoolView{pool4},
			ProtocolDeletions: []uint16{1},
			ProtocolAdditions: map[uint16]engine.ProtocolID{2: protoSushi},
		}

		newState, err := Patcher(initialState, diff)
		require.NoError(t, err)

		// Check Pools
		assert.Len(t, newState.Pools, 3) // 3 start - 1 del + 1 add = 3
		assert.Nil(t, findPool(newState, 2))
		assert.NotNil(t, findPool(newState, 4))

		// Check Protocols
		assert.Len(t, newState.Protocols, 2) // 2 start - 1 del + 1 add = 2
		_, hasCurve := newState.Protocols[1]
		assert.False(t, hasCurve)
		assert.Equal(t, protoSushi, newState.Protocols[2])
	})

	t.Run("Should ensure Immutability (Deep Copy)", func(t *testing.T) {
		diff := PoolRegistryDiff{
			ProtocolAdditions: map[uint16]engine.ProtocolID{99: "temp"},
		}

		newState, err := Patcher(initialState, diff)
		require.NoError(t, err)

		// Modifying newState should NOT affect initialState
		_, existsInNew := newState.Protocols[99]
		_, existsInOld := initialState.Protocols[99]

		assert.True(t, existsInNew)
		assert.False(t, existsInOld, "Patcher must deep copy the protocol map")
	})

	t.Run("Should handle Empty Diff", func(t *testing.T) {
		diff := PoolRegistryDiff{}
		newState, err := Patcher(initialState, diff)
		require.NoError(t, err)

		assert.ElementsMatch(t, initialState.Pools, newState.Pools)
		assert.Equal(t, initialState.Protocols, newState.Protocols)
	})

	t.Run("Should handle Empty Initial State", func(t *testing.T) {
		emptyState := makeView([]PoolView{}, nil)
		pool1 := PoolView{ID: 1, Key: key1, Protocol: 0}

		diff := PoolRegistryDiff{
			PoolAdditions:     []PoolView{pool1},
			ProtocolAdditions: map[uint16]engine.ProtocolID{0: protoUni},
		}

		newState, err := Patcher(emptyState, diff)
		require.NoError(t, err)

		assert.Len(t, newState.Pools, 1)
		assert.Len(t, newState.Protocols, 1)
		assert.Equal(t, protoUni, newState.Protocols[0])
	})
}
