package uniswapv3

import (
	"math/big"
	"testing"

	"github.com/defistate/defistate/protocols/uniswap-v3/ticks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper to find a pool by ID in a slice for testing assertions.
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
	// Note: newTestPool signature: (id, liquidity, sqrtPrice, tick, fee, ticks)
	tick1 := ticks.TickInfo{Index: 10, LiquidityNet: big.NewInt(100), LiquidityGross: big.NewInt(100)}
	tick2 := ticks.TickInfo{Index: 20, LiquidityNet: big.NewInt(200), LiquidityGross: big.NewInt(200)}

	pool1Old := newTestPool(1, 1000, 5000, 100, 3000, []ticks.TickInfo{tick1})
	pool2Old := newTestPool(2, 2000, 6000, 200, 500, []ticks.TickInfo{tick2})
	pool3Old := newTestPool(3, 3000, 7000, 300, 10000, nil)

	initialState := []PoolView{pool1Old, pool2Old, pool3Old}

	t.Run("should handle only additions", func(t *testing.T) {
		pool4New := newTestPool(4, 4000, 8000, 400, 3000, nil)
		diff := UniswapV3SystemDiff{
			Additions: []PoolView{pool4New},
		}

		newState, err := Patcher(initialState, diff)
		require.NoError(t, err)

		assert.Len(t, newState, 4)
		newPool := findPoolByID(newState, 4)
		require.NotNil(t, newPool)
		assert.Equal(t, uint64(3000), newPool.Fee)
		assert.Equal(t, int64(4000), newPool.Liquidity.Int64())
	})

	t.Run("should handle only deletions", func(t *testing.T) {
		diff := UniswapV3SystemDiff{
			Deletions: []uint64{2},
		}

		newState, err := Patcher(initialState, diff)
		require.NoError(t, err)

		assert.Len(t, newState, 2)
		assert.Nil(t, findPoolByID(newState, 2))
	})

	t.Run("should handle only updates including Fee", func(t *testing.T) {
		// Update pool1: Liquidity, Price, Tick, AND Fee changed
		pool1Updated := newTestPool(1, 1001, 5005, 101, 500, []ticks.TickInfo{tick1})
		diff := UniswapV3SystemDiff{
			Updates: []PoolView{pool1Updated},
		}

		newState, err := Patcher(initialState, diff)
		require.NoError(t, err)

		assert.Len(t, newState, 3)
		updatedPool := findPoolByID(newState, 1)
		require.NotNil(t, updatedPool)
		assert.Equal(t, uint64(500), updatedPool.Fee)
		assert.Equal(t, int64(1001), updatedPool.Liquidity.Int64())
		assert.Equal(t, int64(5005), updatedPool.SqrtPriceX96.Int64())
	})

	t.Run("should verify deep copy on update", func(t *testing.T) {
		localInitialState := []PoolView{newTestPool(1, 1000, 5000, 100, 3000, []ticks.TickInfo{tick1})}

		pool1Updated := newTestPool(1, 1001, 5005, 101, 3000, []ticks.TickInfo{tick1})
		diff := UniswapV3SystemDiff{
			Updates: []PoolView{pool1Updated},
		}

		newState, err := Patcher(localInitialState, diff)
		require.NoError(t, err)
		require.Len(t, newState, 1)

		// Modify original state to ensure isolation
		localInitialState[0].Liquidity.SetInt64(9999)
		localInitialState[0].Ticks[0].LiquidityNet.SetInt64(9999)

		updatedPool := findPoolByID(newState, 1)
		require.NotNil(t, updatedPool)
		assert.Equal(t, int64(1001), updatedPool.Liquidity.Int64(), "Should be isolated")
		assert.Equal(t, int64(100), updatedPool.Ticks[0].LiquidityNet.Int64(), "Ticks should be isolated")
	})

	t.Run("should handle a mix of operations", func(t *testing.T) {
		pool4New := newTestPool(4, 4000, 8000, 400, 3000, nil)
		pool2Updated := newTestPool(2, 2002, 6006, 202, 100, []ticks.TickInfo{tick2})
		diff := UniswapV3SystemDiff{
			Additions: []PoolView{pool4New},
			Updates:   []PoolView{pool2Updated},
			Deletions: []uint64{3},
		}

		newState, err := Patcher(initialState, diff)
		require.NoError(t, err)

		assert.Len(t, newState, 3)
		assert.NotNil(t, findPoolByID(newState, 4))

		updatedPool := findPoolByID(newState, 2)
		require.NotNil(t, updatedPool)
		assert.Equal(t, uint64(100), updatedPool.Fee)

		assert.Nil(t, findPoolByID(newState, 3))
	})

	t.Run("should handle an empty diff", func(t *testing.T) {
		diff := UniswapV3SystemDiff{}
		newState, err := Patcher(initialState, diff)
		require.NoError(t, err)
		assert.ElementsMatch(t, initialState, newState)
	})
}
