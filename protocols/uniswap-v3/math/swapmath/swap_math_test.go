package swapmath

import (
	"crypto/rand"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Helper to create a random big.Int up to a given bit length.
func newRandInt(bits int) *big.Int {
	max := new(big.Int).Lsh(big.NewInt(1), uint(bits))
	n, err := rand.Int(rand.Reader, max)
	if err != nil {
		panic(err)
	}
	return n
}

// TestComputeSwapStep_Invariants simulates fuzz testing by running the function
// on a large number of random inputs and verifying its mathematical properties.
func TestComputeSwapStep_Invariants(t *testing.T) {
	for i := 0; i < 1000; i++ {
		// Generate random inputs, respecting the constraints from the Solidity test.
		sqrtPriceRaw := newRandInt(160)
		sqrtPriceTargetRaw := newRandInt(160)
		liquidity := newRandInt(128)
		amountRemaining := newRandInt(256)
		// Make amountRemaining negative 50% of the time.
		if i%2 == 1 {
			amountRemaining.Neg(amountRemaining)
		}
		feePips := newRandInt(20) // Corresponds to up to 1,048,576 ppm, covering all valid fee tiers.

		// require(sqrtPriceRaw > 0);
		if sqrtPriceRaw.Sign() == 0 {
			sqrtPriceRaw.SetInt64(1)
		}
		// require(sqrtPriceTargetRaw > 0);
		if sqrtPriceTargetRaw.Sign() == 0 {
			sqrtPriceTargetRaw.SetInt64(1)
		}
		// require(feePips > 0);
		if feePips.Sign() == 0 {
			feePips.SetInt64(1)
		}
		// require(feePips < 1e6);
		if feePips.Cmp(feeDenominator) >= 0 {
			feePips.Set(new(big.Int).Sub(feeDenominator, big.NewInt(1)))
		}

		sqrtQ, amountIn, amountOut, feeAmount := new(big.Int), new(big.Int), new(big.Int), new(big.Int)
		// Call the function, skipping cases that are expected to error (e.g., underflow/overflow).
		err := ComputeSwapStep(
			sqrtQ, amountIn, amountOut, feeAmount,
			sqrtPriceRaw,
			sqrtPriceTargetRaw,
			liquidity,
			amountRemaining,
			feePips,
		)
		if err != nil {
			continue
		}

		// assert(amountIn <= type(uint256).max - feeAmount);
		// This is implicitly true in Go's big.Int, but we check that the sum doesn't overflow 256 bits.
		sumIn := new(big.Int).Add(amountIn, feeAmount)
		assert.True(t, sumIn.BitLen() <= 256)

		if amountRemaining.Sign() < 0 {
			// assert(amountOut <= uint256(-amountRemaining));
			assert.True(t, amountOut.Cmp(new(big.Int).Neg(amountRemaining)) <= 0)
		} else {
			// assert(amountIn + feeAmount <= uint256(amountRemaining));
			assert.True(t, sumIn.Cmp(amountRemaining) <= 0)
		}

		if sqrtPriceRaw.Cmp(sqrtPriceTargetRaw) == 0 {
			assert.Zero(t, amountIn.Sign())
			assert.Zero(t, amountOut.Sign())
			assert.Zero(t, feeAmount.Sign())
			assert.Zero(t, sqrtQ.Cmp(sqrtPriceTargetRaw))
		}

		// didn't reach price target, entire amount must be consumed
		if sqrtQ.Cmp(sqrtPriceTargetRaw) != 0 {
			if amountRemaining.Sign() < 0 {
				assert.Zero(t, amountOut.Cmp(new(big.Int).Neg(amountRemaining)))
			} else {
				assert.Zero(t, sumIn.Cmp(amountRemaining))
			}
		}

		// next price is between price and price target
		if sqrtPriceTargetRaw.Cmp(sqrtPriceRaw) <= 0 {
			assert.True(t, sqrtQ.Cmp(sqrtPriceRaw) <= 0)
			assert.True(t, sqrtQ.Cmp(sqrtPriceTargetRaw) >= 0)
		} else {
			assert.True(t, sqrtQ.Cmp(sqrtPriceRaw) >= 0)
			assert.True(t, sqrtQ.Cmp(sqrtPriceTargetRaw) <= 0)
		}
	}
}
