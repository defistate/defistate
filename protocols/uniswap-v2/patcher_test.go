package uniswapv2

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper to find a pool by ID in a slice, for testing assertions.
func findPoolByID(pools []PoolView, id uint64) *PoolView {
	for i := range pools {
		if pools[i].ID == id {
			return &pools[i]
		}
	}
	return nil
}

func TestPatcher(t *testing.T) {
	// --- Base Data for Tests ---
	pool1Old := PoolView{ID: 1, Reserve0: big.NewInt(1000), Reserve1: big.NewInt(5000)}
	pool2Old := PoolView{ID: 2, Reserve0: big.NewInt(2000), Reserve1: big.NewInt(6000)}
	pool3Old := PoolView{ID: 3, Reserve0: big.NewInt(3000), Reserve1: big.NewInt(7000)}

	initialState := []PoolView{pool1Old, pool2Old, pool3Old}

	t.Run("should handle only additions", func(t *testing.T) {
		pool4New := PoolView{ID: 4, Reserve0: big.NewInt(4000)}
		diff := UniswapV2SystemDiff{
			Additions: []PoolView{pool4New},
		}

		newState, err := Patcher(initialState, diff)
		require.NoError(t, err)

		assert.Len(t, newState, 4, "Should have 4 pools after addition")
		newPool := findPoolByID(newState, 4)
		require.NotNil(t, newPool)
		assert.Equal(t, int64(4000), newPool.Reserve0.Int64())
	})

	t.Run("should handle only deletions", func(t *testing.T) {
		diff := UniswapV2SystemDiff{
			Deletions: []uint64{2}, // Delete pool with ID 2
		}

		newState, err := Patcher(initialState, diff)
		require.NoError(t, err)

		assert.Len(t, newState, 2, "Should have 2 pools after deletion")
		deletedPool := findPoolByID(newState, 2)
		assert.Nil(t, deletedPool, "Pool 2 should be deleted")
		assert.NotNil(t, findPoolByID(newState, 1), "Pool 1 should remain")
	})

	t.Run("should handle only updates", func(t *testing.T) {
		pool1Updated := PoolView{ID: 1, Reserve0: big.NewInt(1001), Reserve1: big.NewInt(5005)} // Reserves changed
		diff := UniswapV2SystemDiff{
			Updates: []PoolView{pool1Updated},
		}

		newState, err := Patcher(initialState, diff)
		require.NoError(t, err)

		assert.Len(t, newState, 3, "Should still have 3 pools after update")
		updatedPool := findPoolByID(newState, 1)
		require.NotNil(t, updatedPool)
		assert.Equal(t, int64(1001), updatedPool.Reserve0.Int64(), "Pool 1 Reserve0 should be updated")
		assert.Equal(t, int64(5005), updatedPool.Reserve1.Int64(), "Pool 1 Reserve1 should be updated")
	})

	t.Run("should verify deep copy on update", func(t *testing.T) {
		// Create a fresh copy of the initial state for this test to avoid modifying the original.
		localInitialState := []PoolView{
			{ID: 1, Reserve0: big.NewInt(1000), Reserve1: big.NewInt(5000)},
		}

		pool1Updated := PoolView{ID: 1, Reserve0: big.NewInt(1001), Reserve1: big.NewInt(5005)}
		diff := UniswapV2SystemDiff{
			Updates: []PoolView{pool1Updated},
		}

		newState, err := Patcher(localInitialState, diff)
		require.NoError(t, err)
		require.Len(t, newState, 1)

		// CRITICAL: Modify a big.Int in the *original* state slice after the patch has been applied.
		localInitialState[0].Reserve0.SetInt64(9999)

		// Verify that the new state was not affected, proving the deep copy worked.
		updatedPool := findPoolByID(newState, 1)
		require.NotNil(t, updatedPool)
		assert.Equal(t, int64(1001), updatedPool.Reserve0.Int64(), "New state should be isolated from changes to the old state")
	})

	t.Run("should handle a mix of operations", func(t *testing.T) {
		// Add pool 4, update pool 2, delete pool 3
		pool4New := PoolView{ID: 4, Reserve0: big.NewInt(4000)}
		pool2Updated := PoolView{ID: 2, Reserve0: big.NewInt(2002)}
		diff := UniswapV2SystemDiff{
			Additions: []PoolView{pool4New},
			Updates:   []PoolView{pool2Updated},
			Deletions: []uint64{3},
		}

		newState, err := Patcher(initialState, diff)
		require.NoError(t, err)

		assert.Len(t, newState, 3, "Final state should have 3 pools")
		// Verify addition
		assert.NotNil(t, findPoolByID(newState, 4))
		// Verify update
		updatedPool := findPoolByID(newState, 2)
		require.NotNil(t, updatedPool)
		assert.Equal(t, int64(2002), updatedPool.Reserve0.Int64())
		// Verify deletion
		assert.Nil(t, findPoolByID(newState, 3))
		// Verify unchanged pool is still present
		assert.NotNil(t, findPoolByID(newState, 1))
	})

	t.Run("should handle an empty diff", func(t *testing.T) {
		diff := UniswapV2SystemDiff{}

		newState, err := Patcher(initialState, diff)
		require.NoError(t, err)

		// Using assert.ElementsMatch is a robust way to compare slices regardless of order.
		assert.ElementsMatch(t, initialState, newState, "State should be unchanged for an empty diff")
	})
}
