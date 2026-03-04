package swapmath

import (
	"math/big"
	"sync"

	"github.com/defistate/defistate/protocols/uniswap-v3/math/sqrtpricemath"
)

var (
	// feeDenominator is the denominator for fee calculations, representing 100% or 1,000,000 ppm.
	feeDenominator = big.NewInt(1_000_000)
	// one is a pre-computed big.Int for the value 1.
	one = big.NewInt(1)
)

// SwapMath holds reusable big.Int objects for all calculations to avoid memory allocations.
// Instances are managed by a sync.Pool for safe concurrent use.
type SwapMath struct {
	// --- Return Values ---
	sqrtRatioNextX96 *big.Int
	amountIn         *big.Int
	amountOut        *big.Int
	feeAmount        *big.Int

	// --- Temporary Internal Values ---
	// These are used for intermediate calculations within a single computeSwapStep call.
	amountRemainingLessFee *big.Int
	amountRemainingAbs     *big.Int
	tempValue              *big.Int
	product                *big.Int
	rem                    *big.Int
}

// swapMathPool manages a pool of SwapMath objects.
var swapMathPool = sync.Pool{
	New: func() any {
		return &SwapMath{
			sqrtRatioNextX96:       new(big.Int),
			amountIn:               new(big.Int),
			amountOut:              new(big.Int),
			feeAmount:              new(big.Int),
			amountRemainingLessFee: new(big.Int),
			amountRemainingAbs:     new(big.Int),
			tempValue:              new(big.Int),
			product:                new(big.Int),
			rem:                    new(big.Int),
		}
	},
}

// ComputeSwapStep calculates the result of a swap within a single tick range.
// It determines the next price, the amounts swapped, and the fee taken.
func ComputeSwapStep(
	// destination pointers
	sqrtRatioNextX96 *big.Int,
	amountIn *big.Int,
	amountOut *big.Int,
	feeAmount *big.Int,

	sqrtRatioCurrentX96 *big.Int,
	sqrtRatioTargetX96 *big.Int,
	liquidity *big.Int,
	amountRemaining *big.Int,
	feePips *big.Int,

) (
	err error,
) {
	// Borrow a SwapMath object from the pool.
	s := swapMathPool.Get().(*SwapMath)
	defer swapMathPool.Put(s)

	// Call the internal, allocation-free implementation.
	err = s.computeSwapStep(sqrtRatioCurrentX96, sqrtRatioTargetX96, liquidity, amountRemaining, feePips)
	if err != nil {
		return err
	}

	// Create new big.Ints for the return values, setting them from the results in the struct.
	// This ensures the caller gets clean copies and the pooled objects can be safely reused.
	sqrtRatioNextX96.Set(s.sqrtRatioNextX96)
	amountIn.Set(s.amountIn)
	amountOut.Set(s.amountOut)
	feeAmount.Set(s.feeAmount)

	return
}

// computeSwapStep is the internal, allocation-free implementation.
// It is a 1:1 replica of the logic in SwapMath.sol.
func (s *SwapMath) computeSwapStep(
	sqrtRatioCurrentX96, sqrtRatioTargetX96, liquidity, amountRemaining, feePips *big.Int,
) (err error) {
	zeroForOne := sqrtRatioCurrentX96.Cmp(sqrtRatioTargetX96) >= 0
	exactIn := amountRemaining.Sign() >= 0

	// Reset temporary fields to ensure no stale data from previous uses.
	s.amountIn.SetInt64(0)
	s.amountOut.SetInt64(0)
	s.feeAmount.SetInt64(0)

	if exactIn {
		// --- Logic for an exact-input swap ---
		s.tempValue.Sub(feeDenominator, feePips)
		s.mulDiv(s.amountRemainingLessFee, amountRemaining, s.tempValue, feeDenominator)

		if zeroForOne {
			err = sqrtpricemath.GetAmount0Delta(s.amountIn, sqrtRatioTargetX96, sqrtRatioCurrentX96, liquidity, true)
			if err != nil {
				return err
			}
		} else {
			sqrtpricemath.GetAmount1Delta(s.amountIn, sqrtRatioCurrentX96, sqrtRatioTargetX96, liquidity, true)

		}

		if s.amountRemainingLessFee.Cmp(s.amountIn) >= 0 {
			s.sqrtRatioNextX96.Set(sqrtRatioTargetX96)
		} else {
			err = sqrtpricemath.GetNextSqrtPriceFromInput(s.sqrtRatioNextX96, sqrtRatioCurrentX96, liquidity, s.amountRemainingLessFee, zeroForOne)
			if err != nil {
				return err
			}
		}
	} else {
		// --- Logic for an exact-output swap ---
		s.amountRemainingAbs.Neg(amountRemaining)

		if zeroForOne {
			sqrtpricemath.GetAmount1Delta(s.amountOut, sqrtRatioTargetX96, sqrtRatioCurrentX96, liquidity, false)
		} else {
			err = sqrtpricemath.GetAmount0Delta(s.amountOut, sqrtRatioCurrentX96, sqrtRatioTargetX96, liquidity, false)
			if err != nil {
				return err
			}
		}

		if s.amountRemainingAbs.Cmp(s.amountOut) >= 0 {
			s.sqrtRatioNextX96.Set(sqrtRatioTargetX96)
		} else {
			err = sqrtpricemath.GetNextSqrtPriceFromOutput(s.sqrtRatioNextX96, sqrtRatioCurrentX96, liquidity, s.amountRemainingAbs, zeroForOne)
			if err != nil {
				return err
			}
		}
	}

	max := sqrtRatioTargetX96.Cmp(s.sqrtRatioNextX96) == 0

	// --- Recalculate amounts based on the actual price movement ---
	if zeroForOne {
		if !(max && exactIn) {
			err = sqrtpricemath.GetAmount0Delta(s.amountIn, s.sqrtRatioNextX96, sqrtRatioCurrentX96, liquidity, true)
			if err != nil {
				return err
			}
		}
		if !(max && !exactIn) {
			sqrtpricemath.GetAmount1Delta(s.amountOut, s.sqrtRatioNextX96, sqrtRatioCurrentX96, liquidity, false)
		}
	} else {
		if !(max && exactIn) {
			sqrtpricemath.GetAmount1Delta(s.amountIn, sqrtRatioCurrentX96, s.sqrtRatioNextX96, liquidity, true)
		}
		if !(max && !exactIn) {
			err = sqrtpricemath.GetAmount0Delta(s.amountOut, sqrtRatioCurrentX96, s.sqrtRatioNextX96, liquidity, false)
			if err != nil {
				return err
			}
		}
	}

	// --- Final Adjustments ---
	if !exactIn && s.amountOut.Cmp(s.amountRemainingAbs) > 0 {
		s.amountOut.Set(s.amountRemainingAbs)
	}

	if exactIn && s.sqrtRatioNextX96.Cmp(sqrtRatioTargetX96) != 0 {
		// If we didn't reach the target, the fee is the leftover input amount.
		s.feeAmount.Sub(amountRemaining, s.amountIn)
	} else {
		// Otherwise, calculate the fee based on the actual amountIn.
		s.tempValue.Sub(feeDenominator, feePips)
		s.mulDivRoundingUp(s.feeAmount, s.amountIn, feePips, s.tempValue)
	}

	return nil
}

// --- Optimized Helper Methods ---

// mulDiv writes (a * b) / c into dest.
func (s *SwapMath) mulDiv(dest, a, b, c *big.Int) {
	s.product.Mul(a, b)
	dest.Div(s.product, c)
}

// mulDivRoundingUp writes ceil((a * b) / c) into dest.
func (s *SwapMath) mulDivRoundingUp(dest, a, b, c *big.Int) {
	s.product.Mul(a, b)
	dest.Div(s.product, c)
	if s.rem.Rem(s.product, c).Sign() > 0 {
		dest.Add(dest, one)
	}
}
