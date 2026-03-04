package sqrtpricemath

import (
	"crypto/rand"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Helper Functions for Invariant Testing ---

// newRandInt generates a random big.Int up to a given number of bits.
func newRandInt(bits int) *big.Int {
	max := new(big.Int).Lsh(big.NewInt(1), uint(bits))
	n, err := rand.Int(rand.Reader, max)
	if err != nil {
		panic(err)
	}
	return n
}

// --- Invariant Tests (Simulating Fuzzing) ---

func TestGetAmount0Delta_Invariants(t *testing.T) {
	for i := 0; i < 1000; i++ {
		sqrtP := newRandInt(160)
		sqrtQ := newRandInt(160)
		liquidity := newRandInt(128)

		if sqrtP.Sign() == 0 {
			sqrtP.SetInt64(1)
		}
		if sqrtQ.Sign() == 0 {
			sqrtQ.SetInt64(1)
		}

		// Pre-allocate destination variables and call the function.
		amount0Down := new(big.Int)
		err := GetAmount0Delta(amount0Down, sqrtP, sqrtQ, liquidity, false)
		require.NoError(t, err)

		amount0Up := new(big.Int)
		err = GetAmount0Delta(amount0Up, sqrtP, sqrtQ, liquidity, true)
		require.NoError(t, err)

		// assert(amount0Down <= amount0Up);
		assert.True(t, amount0Down.Cmp(amount0Up) <= 0)

		// assert(amount0Up - amount0Down < 2);
		diff := new(big.Int).Sub(amount0Up, amount0Down)
		assert.True(t, diff.Cmp(big.NewInt(2)) < 0)
	}
}

func TestGetAmount1Delta_Invariants(t *testing.T) {
	for i := 0; i < 1000; i++ {
		sqrtP := newRandInt(160)
		sqrtQ := newRandInt(160)
		liquidity := newRandInt(128)

		if sqrtP.Sign() == 0 {
			sqrtP.SetInt64(1)
		}
		if sqrtQ.Sign() == 0 {
			sqrtQ.SetInt64(1)
		}

		// Pre-allocate destination variables and call the function.
		amount1Down := new(big.Int)
		GetAmount1Delta(amount1Down, sqrtP, sqrtQ, liquidity, false)

		amount1Up := new(big.Int)
		GetAmount1Delta(amount1Up, sqrtP, sqrtQ, liquidity, true)

		// assert(amount1Down <= amount1Up);
		assert.True(t, amount1Down.Cmp(amount1Up) <= 0)

		// assert(amount1Up - amount1Down < 2);
		diff := new(big.Int).Sub(amount1Up, amount1Down)
		assert.True(t, diff.Cmp(big.NewInt(2)) < 0)
	}
}

func TestGetNextSqrtPriceFromInput_Invariants(t *testing.T) {
	for i := 0; i < 100; i++ { // Reduced iterations due to complexity
		sqrtP := newRandInt(160)
		liquidity := newRandInt(128)
		amountIn := newRandInt(256)
		zeroForOne := i%2 == 0

		if sqrtP.Sign() == 0 {
			sqrtP.SetInt64(1)
		}
		if liquidity.Sign() == 0 {
			liquidity.SetInt64(1)
		}

		// Pre-allocate destination variable and call the function.
		sqrtQ := new(big.Int)
		err := GetNextSqrtPriceFromInput(sqrtQ, sqrtP, liquidity, amountIn, zeroForOne)
		if err != nil {
			continue // Skip cases that are expected to fail (e.g., underflow)
		}

		if zeroForOne {
			// assert(sqrtQ <= sqrtP);
			assert.True(t, sqrtQ.Cmp(sqrtP) <= 0)
			// assert(amountIn >= SqrtPriceMath.getAmount0Delta(sqrtQ, sqrtP, liquidity, true));
			delta := new(big.Int)
			err := GetAmount0Delta(delta, sqrtQ, sqrtP, liquidity, true)
			if err == nil {
				assert.True(t, amountIn.Cmp(delta) >= 0)
			}
		} else {
			// assert(sqrtQ >= sqrtP);
			assert.True(t, sqrtQ.Cmp(sqrtP) >= 0)
			// assert(amountIn >= SqrtPriceMath.getAmount1Delta(sqrtP, sqrtQ, liquidity, true));
			delta := new(big.Int)
			GetAmount1Delta(delta, sqrtP, sqrtQ, liquidity, true)
			assert.True(t, amountIn.Cmp(delta) >= 0)
		}
	}
}
