package sqrtpricemath

import (
	"errors"
	"math/big"
	"sync"
)

var (
	// Q96 is the UQ64.96 fixed-point number representing 1.
	Q96 = new(big.Int).Lsh(big.NewInt(1), 96)
	// Resolution is the number of bits in the Q96 format.
	Resolution = uint(96)

	ErrLiquidityZero = errors.New("liquidity must be greater than zero")
	ErrSqrtPriceZero = errors.New("sqrt price must be greater than zero")

	// one is a pre-computed big.Int for the value 1.
	one = big.NewInt(1)
)

// SqrtPriceMath holds reusable big.Int objects to avoid memory allocations.
// Instances are managed by a sync.Pool for safe concurrent use.
type SqrtPriceMath struct {
	// Reusable temporary variables for calculations
	product     *big.Int
	numerator1  *big.Int
	numerator2  *big.Int
	denominator *big.Int
	quotient    *big.Int
	term        *big.Int
	rem         *big.Int // Dedicated field for remainder calculations
}

// pool manages a pool of SqrtPriceMath objects.
var pool = sync.Pool{
	New: func() any {
		return &SqrtPriceMath{
			product:     new(big.Int),
			numerator1:  new(big.Int),
			numerator2:  new(big.Int),
			denominator: new(big.Int),
			quotient:    new(big.Int),
			term:        new(big.Int),
			rem:         new(big.Int),
		}
	},
}

// --- Zero-Allocation Helper Methods (Internal) ---

// mulDiv writes (a * b) / c into dest.
func (s *SqrtPriceMath) mulDiv(dest, a, b, c *big.Int) {
	s.product.Mul(a, b)
	dest.Div(s.product, c)
}

// mulDivRoundingUp writes ceil((a * b) / c) into dest.
func (s *SqrtPriceMath) mulDivRoundingUp(dest, a, b, c *big.Int) {
	s.product.Mul(a, b)
	dest.Div(s.product, c)
	if s.rem.Rem(s.product, c).Sign() > 0 {
		dest.Add(dest, one)
	}
}

// divRoundingUp writes ceil(a / b) into dest.
func (s *SqrtPriceMath) divRoundingUp(dest, a, b *big.Int) {
	dest.Div(a, b)
	if s.rem.Rem(a, b).Sign() > 0 {
		dest.Add(dest, one)
	}
}

// --- Public API with Destination-Passing ---

// GetNextSqrtPriceFromAmount0RoundingUp calculates the next sqrt price given a delta of token0.
func GetNextSqrtPriceFromAmount0RoundingUp(dest, sqrtPX96, liquidity, amount *big.Int, add bool) error {
	s := pool.Get().(*SqrtPriceMath)
	defer pool.Put(s)
	return s.getNextSqrtPriceFromAmount0RoundingUp(dest, sqrtPX96, liquidity, amount, add)
}

// GetNextSqrtPriceFromAmount1RoundingDown calculates the next sqrt price given a delta of token1.
func GetNextSqrtPriceFromAmount1RoundingDown(dest, sqrtPX96, liquidity, amount *big.Int, add bool) error {
	s := pool.Get().(*SqrtPriceMath)
	defer pool.Put(s)
	return s.getNextSqrtPriceFromAmount1RoundingDown(dest, sqrtPX96, liquidity, amount, add)
}

// GetNextSqrtPriceFromInput calculates the next sqrt price given an input amount.
func GetNextSqrtPriceFromInput(dest, sqrtPX96, liquidity, amountIn *big.Int, zeroForOne bool) error {
	if sqrtPX96.Sign() <= 0 {
		return ErrSqrtPriceZero
	}
	if liquidity.Sign() <= 0 {
		return ErrLiquidityZero
	}

	if zeroForOne {
		return GetNextSqrtPriceFromAmount0RoundingUp(dest, sqrtPX96, liquidity, amountIn, true)
	}
	return GetNextSqrtPriceFromAmount1RoundingDown(dest, sqrtPX96, liquidity, amountIn, true)
}

// GetNextSqrtPriceFromOutput calculates the next sqrt price given an output amount.
func GetNextSqrtPriceFromOutput(dest, sqrtPX96, liquidity, amountOut *big.Int, zeroForOne bool) error {
	if sqrtPX96.Sign() <= 0 {
		return ErrSqrtPriceZero
	}
	if liquidity.Sign() <= 0 {
		return ErrLiquidityZero
	}

	if zeroForOne {
		return GetNextSqrtPriceFromAmount1RoundingDown(dest, sqrtPX96, liquidity, amountOut, false)
	}
	return GetNextSqrtPriceFromAmount0RoundingUp(dest, sqrtPX96, liquidity, amountOut, false)
}

// GetAmount0Delta calculates the amount0 delta between two prices.
func GetAmount0Delta(dest, sqrtRatioAX96, sqrtRatioBX96, liquidity *big.Int, roundUp bool) error {
	s := pool.Get().(*SqrtPriceMath)
	defer pool.Put(s)
	return s.getAmount0Delta(dest, sqrtRatioAX96, sqrtRatioBX96, liquidity, roundUp)
}

// GetAmount1Delta calculates the amount1 delta between two prices.
func GetAmount1Delta(dest, sqrtRatioAX96, sqrtRatioBX96, liquidity *big.Int, roundUp bool) {
	s := pool.Get().(*SqrtPriceMath)
	defer pool.Put(s)
	s.getAmount1Delta(dest, sqrtRatioAX96, sqrtRatioBX96, liquidity, roundUp)
}

// --- Internal Implementations (using destination-passing for performance) ---

func (s *SqrtPriceMath) getNextSqrtPriceFromAmount0RoundingUp(dest, sqrtPX96, liquidity, amount *big.Int, add bool) error {
	if amount.Sign() == 0 {
		dest.Set(sqrtPX96)
		return nil
	}

	s.numerator1.Lsh(liquidity, Resolution)

	if add {
		s.product.Mul(amount, sqrtPX96)
		if s.quotient.Div(s.product, amount).Cmp(sqrtPX96) == 0 {
			s.denominator.Add(s.numerator1, s.product)
			if s.denominator.Cmp(s.numerator1) >= 0 {
				s.mulDivRoundingUp(dest, s.numerator1, sqrtPX96, s.denominator)
				return nil
			}
		}
		s.denominator.Div(s.numerator1, sqrtPX96)
		s.denominator.Add(s.denominator, amount)
		s.divRoundingUp(dest, s.numerator1, s.denominator)
		return nil
	} else {
		s.product.Mul(amount, sqrtPX96)
		if s.quotient.Div(s.product, amount).Cmp(sqrtPX96) != 0 || s.numerator1.Cmp(s.product) <= 0 {
			return errors.New("product overflow or denominator underflow")
		}
		s.denominator.Sub(s.numerator1, s.product)
		s.mulDivRoundingUp(dest, s.numerator1, sqrtPX96, s.denominator)
		return nil
	}
}

func (s *SqrtPriceMath) getNextSqrtPriceFromAmount1RoundingDown(dest, sqrtPX96, liquidity, amount *big.Int, add bool) error {
	if add {
		s.mulDiv(s.quotient, amount, Q96, liquidity)
		dest.Add(sqrtPX96, s.quotient)
		return nil
	} else {
		s.mulDivRoundingUp(s.quotient, amount, Q96, liquidity)
		if sqrtPX96.Cmp(s.quotient) <= 0 {
			return errors.New("sqrtPX96 must be greater than quotient")
		}
		dest.Sub(sqrtPX96, s.quotient)
		return nil
	}
}

func (s *SqrtPriceMath) getAmount0Delta(dest, sqrtRatioAX96, sqrtRatioBX96, liquidity *big.Int, roundUp bool) error {
	if sqrtRatioAX96.Cmp(sqrtRatioBX96) > 0 {
		sqrtRatioAX96, sqrtRatioBX96 = sqrtRatioBX96, sqrtRatioAX96
	}
	if sqrtRatioAX96.Sign() <= 0 {
		return ErrSqrtPriceZero
	}

	s.numerator1.Lsh(liquidity, Resolution)
	s.numerator2.Sub(sqrtRatioBX96, sqrtRatioAX96)

	if roundUp {
		s.mulDivRoundingUp(s.term, s.numerator1, s.numerator2, sqrtRatioBX96)
		s.divRoundingUp(dest, s.term, sqrtRatioAX96)
	} else {
		s.mulDiv(s.term, s.numerator1, s.numerator2, sqrtRatioBX96)
		dest.Div(s.term, sqrtRatioAX96)
	}
	return nil
}

func (s *SqrtPriceMath) getAmount1Delta(dest, sqrtRatioAX96, sqrtRatioBX96, liquidity *big.Int, roundUp bool) {
	if sqrtRatioAX96.Cmp(sqrtRatioBX96) > 0 {
		sqrtRatioAX96, sqrtRatioBX96 = sqrtRatioBX96, sqrtRatioAX96
	}

	s.numerator1.Sub(sqrtRatioBX96, sqrtRatioAX96)
	if roundUp {
		s.mulDivRoundingUp(dest, liquidity, s.numerator1, Q96)
	} else {
		s.mulDiv(dest, liquidity, s.numerator1, Q96)
	}
}
