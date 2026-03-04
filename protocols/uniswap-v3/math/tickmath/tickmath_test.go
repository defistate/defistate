package tickmath

import (
	"crypto/rand"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper to create a big.Int from a string for tests.
func fromString(s string) *big.Int {
	n, _ := new(big.Int).SetString(s, 10)
	return n
}

// encodePriceSqrt is a Go equivalent of the ethers.js helper for testing.
func encodePriceSqrt(reserve1, reserve0 *big.Int) *big.Int {
	num := new(big.Int).Mul(reserve1, new(big.Int).Lsh(big.NewInt(1), 192))
	ratio := new(big.Int).Div(num, reserve0)
	return new(big.Int).Sqrt(ratio)
}

func TestGetSqrtRatioAtTick(t *testing.T) {

	t.Run("throws for too low", func(t *testing.T) {
		temp := new(big.Int)
		err := GetSqrtRatioAtTick(temp, MIN_TICK-1)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrTickOutOfBounds)
	})

	t.Run("throws for too high", func(t *testing.T) {
		temp := new(big.Int)
		err := GetSqrtRatioAtTick(temp, MAX_TICK+1)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrTickOutOfBounds)
	})

	t.Run("min tick", func(t *testing.T) {
		sqrtP := new(big.Int)
		err := GetSqrtRatioAtTick(sqrtP, MIN_TICK)
		require.NoError(t, err)
		assert.Zero(t, fromString("4295128739").Cmp(sqrtP))
	})

	t.Run("max tick", func(t *testing.T) {
		sqrtP := new(big.Int)
		err := GetSqrtRatioAtTick(sqrtP, MAX_TICK)
		require.NoError(t, err)
		assert.Zero(t, fromString("1461446703485210103287273052203988822378723970342").Cmp(sqrtP))
	})
}

func TestGetTickAtSqrtRatio(t *testing.T) {
	t.Run("throws for too low", func(t *testing.T) {
		_, err := GetTickAtSqrtRatio(new(big.Int).Sub(MIN_SQRT_RATIO, big.NewInt(1)))
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrSqrtPriceOutOfBounds)
	})

	t.Run("throws for too high", func(t *testing.T) {
		_, err := GetTickAtSqrtRatio(MAX_SQRT_RATIO)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrSqrtPriceOutOfBounds)
	})

	t.Run("ratio of min tick", func(t *testing.T) {
		tick, err := GetTickAtSqrtRatio(MIN_SQRT_RATIO)
		require.NoError(t, err)
		assert.Equal(t, MIN_TICK, tick)
	})

	t.Run("ratio closest to max tick", func(t *testing.T) {
		tick, err := GetTickAtSqrtRatio(new(big.Int).Sub(MAX_SQRT_RATIO, big.NewInt(1)))
		require.NoError(t, err)
		assert.Equal(t, MAX_TICK-1, tick)
	})

	// Table-driven test for various ratios
	ratios := []struct {
		name  string
		ratio *big.Int
	}{
		{"MIN_SQRT_RATIO", MIN_SQRT_RATIO},
		{"1e12:1", encodePriceSqrt(new(big.Int).Exp(big.NewInt(10), big.NewInt(12), nil), big.NewInt(1))},
		{"1e6:1", encodePriceSqrt(new(big.Int).Exp(big.NewInt(10), big.NewInt(6), nil), big.NewInt(1))},
		{"1:64", encodePriceSqrt(big.NewInt(1), big.NewInt(64))},
		{"1:8", encodePriceSqrt(big.NewInt(1), big.NewInt(8))},
		{"1:2", encodePriceSqrt(big.NewInt(1), big.NewInt(2))},
		{"1:1", encodePriceSqrt(big.NewInt(1), big.NewInt(1))},
		{"2:1", encodePriceSqrt(big.NewInt(2), big.NewInt(1))},
		{"8:1", encodePriceSqrt(big.NewInt(8), big.NewInt(1))},
		{"64:1", encodePriceSqrt(big.NewInt(64), big.NewInt(1))},
		{"1:1e6", encodePriceSqrt(big.NewInt(1), new(big.Int).Exp(big.NewInt(10), big.NewInt(6), nil))},
		{"1:1e12", encodePriceSqrt(big.NewInt(1), new(big.Int).Exp(big.NewInt(10), big.NewInt(12), nil))},
		{"MAX_SQRT_RATIO-1", new(big.Int).Sub(MAX_SQRT_RATIO, big.NewInt(1))},
	}

	for _, tc := range ratios {
		t.Run(tc.name, func(t *testing.T) {
			tick, err := GetTickAtSqrtRatio(tc.ratio)
			require.NoError(t, err)
			ratioOfTick := new(big.Int)
			err = GetSqrtRatioAtTick(ratioOfTick, tick)
			require.NoError(t, err)
			ratioOfTickPlusOne := new(big.Int)
			err = GetSqrtRatioAtTick(ratioOfTickPlusOne, tick+1)
			require.NoError(t, err)

			// Invariant: ratioOfTick <= ratio < ratioOfTickPlusOne
			assert.True(t, tc.ratio.Cmp(ratioOfTick) >= 0)
			assert.True(t, tc.ratio.Cmp(ratioOfTickPlusOne) < 0)
		})
	}
}

// TestInvariants checks that GetTickAtSqrtRatio is the inverse of GetSqrtRatioAtTick.
func TestInvariants_InverseFunctions(t *testing.T) {
	for i := 0; i < 1000; i++ {
		// Generate a random tick within the valid range.
		tickRange := big.NewInt(int64(MAX_TICK - MIN_TICK))
		randomOffset, _ := rand.Int(rand.Reader, tickRange)
		tick := MIN_TICK + randomOffset.Int64()
		sqrtP := new(big.Int)
		err := GetSqrtRatioAtTick(sqrtP, tick)
		require.NoError(t, err)

		tickCalculated, err := GetTickAtSqrtRatio(sqrtP)
		require.NoError(t, err)

		// The calculated tick should be equal to the original tick.
		assert.Equal(t, tick, tickCalculated, "tick %d -> sqrtP %s -> tick %d", tick, sqrtP.String(), tickCalculated)
	}
}
