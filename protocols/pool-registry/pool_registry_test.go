package poolregistry

import (
	"testing"

	"github.com/defistate/defistate/engine"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Test Helpers ---

// testKey creates a PoolKey from a hex string for testing.
func testKey(hexStr string) PoolKey {
	return AddressToPoolKey(common.HexToAddress(hexStr))
}

// findPoolInView is a helper to find a specific pool in a view slice.
func findPoolInView(pools []PoolView, id uint64) *PoolView {
	for i := range pools {
		if pools[i].ID == id {
			return &pools[i]
		}
	}
	return nil
}

func TestPoolRegistry(t *testing.T) {
	key1 := testKey("0x1")
	key2 := testKey("0x2")
	key3 := testKey("0x3")

	protoUniswap := engine.ProtocolID("uniswap-v2")
	protoCurve := engine.ProtocolID("curve")

	t.Run("NewPoolRegistry", func(t *testing.T) {
		r := NewPoolRegistry()
		require.NotNil(t, r, "registry should not be nil")

		view := r.view()
		assert.Len(t, view.Pools, 0, "a new registry should be empty")
		assert.Len(t, view.Protocols, 0, "protocol map should be empty")
	})

	t.Run("AddPool", func(t *testing.T) {
		r := NewPoolRegistry()

		// --- Test Success ---
		id, err := r.addPool(key1, protoUniswap)
		require.NoError(t, err, "adding a new pool should not fail")
		assert.Equal(t, uint64(1), id, "should return the correct new ID")

		// Verify state via view()
		view := r.view()
		require.Len(t, view.Pools, 1)
		require.Len(t, view.Protocols, 1) // 1 unique protocol

		pool := view.Pools[0]
		assert.Equal(t, id, pool.ID)
		assert.Equal(t, key1, pool.Key)

		// Verify Dictionary Encoding
		internalProto := pool.Protocol
		assert.Equal(t, protoUniswap, view.Protocols[internalProto], "internal ID should map back to uniswap")

		// --- Test Duplicate Add (Same Key) ---
		id2, err2 := r.addPool(key1, protoUniswap)
		require.NoError(t, err2, "adding a duplicate pool should not fail")
		assert.Equal(t, id, id2, "should return the existing ID for a duplicate pool")
		assert.Len(t, r.view().Pools, 1, "registry size should not change on duplicate add")

		// --- Test Different Pool, Same Protocol (Should Reuse Internal ID) ---
		id3, err3 := r.addPool(key2, protoUniswap)
		require.NoError(t, err3)

		view2 := r.view()
		require.Len(t, view2.Pools, 2)
		require.Len(t, view2.Protocols, 1, "should still only have 1 unique protocol")

		pool2 := findPoolInView(view2.Pools, id3)
		assert.Equal(t, internalProto, pool2.Protocol, "second pool should share the same internal protocol ID")
	})

	t.Run("BlockList", func(t *testing.T) {
		r := NewPoolRegistry()

		// Add to blocklist
		r.addToBlockList(key1)
		assert.True(t, r.isOnBlockList(key1), "pool should be on blocklist after being added")

		// Attempt to add a blocklisted pool
		_, err := r.addPool(key1, protoUniswap)
		require.Error(t, err, "adding a blocklisted pool should return an error")
		assert.ErrorIs(t, err, ErrPoolBlocked)

		// Remove from blocklist
		r.removeFromBlockList(key1)
		assert.False(t, r.isOnBlockList(key1), "pool should not be on blocklist after being removed")

		// Attempt to add again, should now succeed
		id, err := r.addPool(key1, protoUniswap)
		require.NoError(t, err, "adding a pool after removal from blocklist should succeed")
		assert.Equal(t, uint64(1), id)
	})

	t.Run("DeletePool", func(t *testing.T) {
		t.Run("DeleteMiddle_VerifiesSwapAndPop", func(t *testing.T) {
			r := NewPoolRegistry()
			id1, _ := r.addPool(key1, protoUniswap) // index 0
			id2, _ := r.addPool(key2, protoCurve)   // index 1
			id3, _ := r.addPool(key3, protoUniswap) // index 2
			require.Len(t, r.view().Pools, 3)

			// Delete the middle pool (p2)
			err := r.deletePool(id2)
			require.NoError(t, err)

			// --- Assertions ---
			view := r.view()
			require.Len(t, view.Pools, 2, "registry should have 2 pools after deletion")

			// The last element (id3) should have been swapped into the deleted slot (index 1).
			assert.Nil(t, findPoolInView(view.Pools, id2), "pool with id2 should not be in the view")

			p1 := findPoolInView(view.Pools, id1)
			p3 := findPoolInView(view.Pools, id3)
			require.NotNil(t, p1)
			require.NotNil(t, p3)
		})

		t.Run("DeleteNonExistent", func(t *testing.T) {
			r := NewPoolRegistry()
			err := r.deletePool(999)
			assert.ErrorIs(t, err, ErrPoolNotFound)
		})

		t.Run("ReAddDeletedPool_GetsNewID", func(t *testing.T) {
			r := NewPoolRegistry()
			id1, _ := r.addPool(key1, protoUniswap)
			require.NoError(t, r.deletePool(id1))
			require.Len(t, r.view().Pools, 0)

			// Re-add the same pool key
			newID, err := r.addPool(key1, protoUniswap)
			require.NoError(t, err)

			// HARDENING: The new ID must be different from the original ID.
			assert.NotEqual(t, id1, newID, "re-added pool must get a new, unique ID")
			assert.Equal(t, uint64(2), newID, "the nextID counter should have been incremented")
		})
	})

	t.Run("Getters", func(t *testing.T) {
		r := NewPoolRegistry()
		id1, _ := r.addPool(key1, protoUniswap)

		// Test getID
		retrievedID, err := r.getID(key1)
		require.NoError(t, err)
		assert.Equal(t, id1, retrievedID)
		_, err = r.getID(key2)
		assert.ErrorIs(t, err, ErrPoolNotFound)

		// Test getKey
		retrievedKey, err := r.getKey(id1)
		require.NoError(t, err)
		assert.Equal(t, key1, retrievedKey)
		_, err = r.getKey(999)
		assert.ErrorIs(t, err, ErrPoolNotFound)

		// --- Test getProtocolID ---
		pID, err := r.getProtocolID(id1)
		require.NoError(t, err)
		assert.Equal(t, protoUniswap, pID)

		_, err = r.getProtocolID(999)
		assert.ErrorIs(t, err, ErrPoolNotFound)

		// --- Test getByID ---
		retrievedView, err := r.getByID(id1)
		require.NoError(t, err)
		assert.Equal(t, id1, retrievedView.ID)
		assert.Equal(t, key1, retrievedView.Key)
		assert.True(t, retrievedView.Protocol < 65535)
	})

	t.Run("View_ReturnsCopy", func(t *testing.T) {
		r := NewPoolRegistry()
		r.addPool(key1, protoUniswap)

		view1 := r.view()
		require.Len(t, view1.Pools, 1)

		// Maliciously modify the returned view
		view1.Pools[0].ID = 9999

		// Get a new view and check if the original registry was affected
		view2 := r.view()
		require.Len(t, view2.Pools, 1)
		assert.Equal(t, uint64(1), view2.Pools[0].ID, "modifying a returned view slice should not affect the internal registry data")
	})
}

func TestNewPoolRegistryFromView(t *testing.T) {
	key1 := testKey("0x1")
	key2 := testKey("0x2")
	protoUni := engine.ProtocolID("uniswap")
	protoSushi := engine.ProtocolID("sushi")

	t.Run("Success_FullRestore", func(t *testing.T) {
		// Construct a snapshot manually
		view := PoolRegistryView{
			Pools: []PoolView{
				{ID: 10, Key: key1, Protocol: 0},
				{ID: 5, Key: key2, Protocol: 1}, // Non-sequential IDs
			},
			Protocols: map[uint16]engine.ProtocolID{
				0: protoUni,
				1: protoSushi,
			},
		}

		r, err := NewPoolRegistryFromView(view)
		require.NoError(t, err)
		require.NotNil(t, r)

		// 1. Verify Pools
		assert.Len(t, r.view().Pools, 2)

		p1, err1 := r.getByID(10)
		p2, err2 := r.getByID(5)
		require.NoError(t, err1)
		require.NoError(t, err2)

		assert.Equal(t, key1, p1.Key)
		assert.Equal(t, key2, p2.Key)

		// 2. Verify Protocol Logic
		pID1, _ := r.getProtocolID(10)
		pID2, _ := r.getProtocolID(5)
		assert.Equal(t, protoUni, pID1)
		assert.Equal(t, protoSushi, pID2)

		// 3. Verify nextID is correct (max + 1)
		// Max ID was 10, so next should be 11
		id3, err := r.addPool(testKey("0x3"), protoUni)
		require.NoError(t, err)
		assert.Equal(t, uint64(11), id3)

		// 4. Verify nextProto is correct (max + 1)
		// Used 0 and 1, next should be 2.
		// If we add a NEW protocol, it should get ID 2.
		id4, err := r.addPool(testKey("0x4"), "new-proto")
		require.NoError(t, err)
		p4, _ := r.getByID(id4)
		assert.Equal(t, uint16(2), p4.Protocol)
	})

	t.Run("Failure_MissingProtocolMapping", func(t *testing.T) {
		view := PoolRegistryView{
			Pools: []PoolView{
				{ID: 10, Key: key1, Protocol: 5}, // Protocol 5 is NOT in the map
			},
			Protocols: map[uint16]engine.ProtocolID{
				0: protoUni,
			},
		}
		r, err := NewPoolRegistryFromView(view)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrProtocolMappingNotFound)
		assert.Nil(t, r)
	})

	t.Run("FailureOnDuplicateID", func(t *testing.T) {
		view := PoolRegistryView{
			Pools: []PoolView{
				{ID: 10, Key: key1, Protocol: 0},
				{ID: 10, Key: key2, Protocol: 0}, // Duplicate ID
			},
			Protocols: map[uint16]engine.ProtocolID{0: protoUni},
		}
		r, err := NewPoolRegistryFromView(view)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrDuplicateID)
		assert.Nil(t, r, "registry should be nil on failure")

	})

	t.Run("FailureOnDuplicateKey", func(t *testing.T) {
		view := PoolRegistryView{
			Pools: []PoolView{
				{ID: 10, Key: key1, Protocol: 0},
				{ID: 20, Key: key1, Protocol: 0}, // Duplicate Key
			},
			Protocols: map[uint16]engine.ProtocolID{0: protoUni},
		}
		r, err := NewPoolRegistryFromView(view)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrDuplicateKey)
		assert.Nil(t, r, "registry should be nil on failure")
	})
}
