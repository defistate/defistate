package uniswapv3

import (
	"math/big"
	"testing"

	uniswapv3ticks "github.com/defistate/defistate/protocols/uniswap-v3/ticks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper function updated to include Fee as uint64
func newTestPool(id uint64, liquidity, sqrtPrice, tick int64, fee uint64, ticks []uniswapv3ticks.TickInfo) PoolView {
	return PoolView{
		PoolViewMinimal: PoolViewMinimal{
			ID:           id,
			Liquidity:    big.NewInt(liquidity),
			SqrtPriceX96: big.NewInt(sqrtPrice),
			Tick:         tick,
			Fee:          fee, // uint64
		},
		Ticks: ticks,
	}
}

func TestDiffer(t *testing.T) {
	// --- Base Data for Tests ---
	tick1 := uniswapv3ticks.TickInfo{Index: 10, LiquidityNet: big.NewInt(100)}
	tick2 := uniswapv3ticks.TickInfo{Index: 20, LiquidityNet: big.NewInt(200)}

	// Fees: 3000 (0.3%), 500 (0.05%)
	pool1Old := newTestPool(1, 1000, 5000, 100, 3000, []uniswapv3ticks.TickInfo{tick1})
	pool2Old := newTestPool(2, 2000, 6000, 200, 3000, []uniswapv3ticks.TickInfo{tick2})

	t.Run("should identify updates when Fee changes", func(t *testing.T) {
		// Only the fee changes, triggering a diff update
		pool1UpdatedFee := newTestPool(1, 1000, 5000, 100, 500, []uniswapv3ticks.TickInfo{tick1})

		oldState := []PoolView{pool1Old}
		newState := []PoolView{pool1UpdatedFee}

		diff := Differ(oldState, newState)

		require.NotNil(t, diff)
		assert.Len(t, diff.Updates, 1, "A change in the uint64 Fee field must trigger an update")
		assert.Equal(t, pool1UpdatedFee.ID, diff.Updates[0].ID)
		assert.Equal(t, uint64(500), diff.Updates[0].Fee)
	})

	t.Run("should identify updates when core fields and ticks change", func(t *testing.T) {
		// Change Tick and Liquidity
		pool1Updated := newTestPool(1, 1001, 5000, 101, 3000, []uniswapv3ticks.TickInfo{tick1})

		oldState := []PoolView{pool1Old}
		newState := []PoolView{pool1Updated}

		diff := Differ(oldState, newState)

		assert.Len(t, diff.Updates, 1)
		assert.Equal(t, int64(101), diff.Updates[0].Tick)
	})

	t.Run("should identify deletions correctly", func(t *testing.T) {
		oldState := []PoolView{pool1Old, pool2Old}
		newState := []PoolView{pool1Old}

		diff := Differ(oldState, newState)

		assert.Len(t, diff.Deletions, 1)
		assert.Equal(t, pool2Old.ID, diff.Deletions[0])
	})

	t.Run("should produce an empty diff when states are identical", func(t *testing.T) {
		oldState := []PoolView{pool1Old}
		newState := []PoolView{pool1Old}

		diff := Differ(oldState, newState)

		assert.True(t, diff.IsEmpty())
	})
}
