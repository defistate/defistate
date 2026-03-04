package uniswapv2

import (
	"errors"
	"fmt"
	"math/big"
	"sync"

	uniswapv2 "github.com/defistate/defistate/protocols/uniswap-v2"
)

var (
	// basisPointDivisor is a constant representing 100% in basis points (10000).
	basisPointDivisor = big.NewInt(10000)

	ten     = big.NewInt(10)
	hundred = big.NewInt(100)

	// precomputed 10^dec for typical ERC20 decimals (0..18)
	precomputedScales [19]*big.Int

	bigIntPool = sync.Pool{
		New: func() any {
			return new(big.Int)
		},
	}

	// ErrInvalidAmount is returned when an input/output amount is nil or negative.
	ErrInvalidAmount = errors.New("amount must be non-nil and non-negative")
	// ErrNilAmount is returned when a nil pointer is passed for an amount.
	ErrNilAmount = errors.New("nil pointer passed as amount")
	// ErrTokenMismatch is returned when the specified input/output tokens do not match the pool's tokens.
	ErrTokenMismatch = errors.New("token mismatch")
	// ErrInvalidState is returned for internal calculation errors, like division by zero.
	ErrInvalidState = errors.New("invalid internal state")
	// ErrInsufficientLiquidity is returned when an amountOut is requested that is greater than or equal to the available reserve.
	ErrInsufficientLiquidity = errors.New("insufficient liquidity for swap")
)

func init() {
	// fill precomputedScales[0..18]
	precomputedScales[0] = big.NewInt(1)
	for i := 1; i < len(precomputedScales); i++ {
		precomputedScales[i] = new(big.Int).Mul(precomputedScales[i-1], ten)
	}
}

// getBig grabs a *big.Int from the pool and zeros it.
func getBig() *big.Int {
	b := bigIntPool.Get().(*big.Int)
	b.SetUint64(0)
	return b
}

// putBig returns a *big.Int to the pool.
func putBig(b *big.Int) {
	if b != nil {
		bigIntPool.Put(b)
	}
}

// GetScaledDecimal returns 10^dec. It returns a *big.Int that MUST NOT be modified.
// If dec <= 18 we return the precomputed immutable value.
// If dec > 18 we compute it on the fly.
func GetScaledDecimal(dec uint8) *big.Int {
	if int(dec) < len(precomputedScales) {
		return precomputedScales[dec] // safe to return as read-only
	}

	// rare path: compute on the fly
	// this one is allocated fresh because it's an uncommon case
	return new(big.Int).Exp(ten, big.NewInt(int64(dec)), nil)
}

// Calculator holds reusable big.Int objects to avoid memory allocations during calculations.
// Instances of this struct are NOT safe for concurrent use by themselves.
// They are intended to be managed by the sync.Pool below.
type Calculator struct {
	// Reusable objects for GetAmountOut
	feeMultiplier   *big.Int
	amountInWithFee *big.Int
	numerator       *big.Int
	denominator     *big.Int

	// Reusable objects for GetAmountIn
	numeratorIn   *big.Int
	denominatorIn *big.Int

	// Reusable objects for SimulateSwap
	newReserve0 *big.Int
	newReserve1 *big.Int
}

// calculatorPool manages a pool of Calculator objects, allowing for safe concurrent use
// and drastically reducing memory allocations.
var calculatorPool = sync.Pool{
	New: func() any {
		// This function is called when a goroutine needs a Calculator and none are available.
		return &Calculator{
			feeMultiplier:   new(big.Int),
			amountInWithFee: new(big.Int),
			numerator:       new(big.Int),
			denominator:     new(big.Int),
			numeratorIn:     new(big.Int),
			denominatorIn:   new(big.Int),
			newReserve0:     new(big.Int),
			newReserve1:     new(big.Int),
		}
	},
}

// GetAmountOut calculates the output amount for a swap, optimized to reduce allocations.
func GetAmountOut(
	amountIn *big.Int,
	tokenIn uint64,
	tokenOut uint64,
	pool uniswapv2.PoolView,
) (*big.Int, error) {
	calc := calculatorPool.Get().(*Calculator)
	defer calculatorPool.Put(calc)
	return calc.getAmountOut(amountIn, tokenIn, tokenOut, pool)
}

// GetAmountIn calculates the required input amount for a desired output, optimized to reduce allocations.
func GetAmountIn(
	amountOut *big.Int,
	tokenIn uint64,
	tokenOut uint64,
	pool uniswapv2.PoolView,
) (*big.Int, error) {
	calc := calculatorPool.Get().(*Calculator)
	defer calculatorPool.Put(calc)
	return calc.getAmountIn(amountOut, tokenIn, tokenOut, pool)
}

// SimulateSwap calculates the result of a swap, optimized to reduce allocations.
func SimulateSwap(
	amountIn *big.Int,
	tokenInID uint64,
	tokenOutID uint64,
	pool uniswapv2.PoolView,
) (*big.Int, uniswapv2.PoolView, error) {
	calc := calculatorPool.Get().(*Calculator)
	defer calculatorPool.Put(calc)
	return calc.simulateSwap(amountIn, tokenInID, tokenOutID, pool)
}

// getAmountOut is the internal calculation method that uses the pre-allocated fields.
func (c *Calculator) getAmountOut(
	amountIn *big.Int,
	tokenIn uint64,
	tokenOut uint64,
	pool uniswapv2.PoolView,
) (*big.Int, error) {
	if amountIn == nil {
		return nil, ErrNilAmount
	}
	if amountIn.Sign() < 0 {
		return nil, ErrInvalidAmount
	}

	reserveIn, reserveOut, err := GetReserves(tokenIn, tokenOut, pool)
	if err != nil {
		return nil, err
	}

	if reserveIn.Sign() <= 0 || reserveOut.Sign() <= 0 {
		return new(big.Int), nil
	}

	c.feeMultiplier.Sub(basisPointDivisor, big.NewInt(int64(pool.FeeBps)))
	c.amountInWithFee.Mul(amountIn, c.feeMultiplier)
	c.numerator.Mul(reserveOut, c.amountInWithFee)
	c.denominator.Mul(reserveIn, basisPointDivisor)
	c.denominator.Add(c.denominator, c.amountInWithFee)

	if c.denominator.Sign() == 0 {
		return nil, fmt.Errorf("%w: pool denominator is zero", ErrInvalidState)
	}

	return new(big.Int).Div(c.numerator, c.denominator), nil
}

// getAmountIn is the internal calculation method for finding the required input for a desired output.
func (c *Calculator) getAmountIn(
	amountOut *big.Int,
	tokenIn uint64,
	tokenOut uint64,
	pool uniswapv2.PoolView,
) (*big.Int, error) {
	if amountOut == nil {
		return nil, ErrNilAmount
	}
	if amountOut.Sign() < 0 {
		return nil, ErrInvalidAmount
	}

	reserveIn, reserveOut, err := GetReserves(tokenIn, tokenOut, pool)
	if err != nil {
		return nil, err
	}

	if reserveIn.Sign() <= 0 || reserveOut.Sign() <= 0 || amountOut.Cmp(reserveOut) >= 0 {
		return nil, fmt.Errorf("%w: requested amountOut (%s) is >= reserveOut (%s)", ErrInsufficientLiquidity, amountOut.String(), reserveOut.String())
	}

	c.numeratorIn.Mul(reserveIn, amountOut)
	c.numeratorIn.Mul(c.numeratorIn, basisPointDivisor)

	c.feeMultiplier.Sub(basisPointDivisor, big.NewInt(int64(pool.FeeBps)))
	c.denominatorIn.Sub(reserveOut, amountOut)
	c.denominatorIn.Mul(c.denominatorIn, c.feeMultiplier)

	if c.denominatorIn.Sign() == 0 {
		return nil, fmt.Errorf("%w: pool denominator is zero", ErrInvalidState)
	}

	// amountIn = (reserveIn * amountOut * 10000) / ((reserveOut - amountOut) * 9970) + 1
	amountIn := new(big.Int).Div(c.numeratorIn, c.denominatorIn)
	return amountIn.Add(amountIn, big.NewInt(1)), nil
}

// simulateSwap is the internal calculation method that uses pre-allocated fields.
func (c *Calculator) simulateSwap(
	amountIn *big.Int,
	tokenInID uint64,
	tokenOutID uint64,
	pool uniswapv2.PoolView,
) (*big.Int, uniswapv2.PoolView, error) {
	amountOut, err := c.getAmountOut(amountIn, tokenInID, tokenOutID, pool)
	if err != nil {
		return nil, uniswapv2.PoolView{}, err
	}

	newPoolState := pool

	if tokenInID == pool.Token0 {
		c.newReserve0.Add(pool.Reserve0, amountIn)
		c.newReserve1.Sub(pool.Reserve1, amountOut)
	} else { // tokenInID == pool.Token1
		c.newReserve1.Add(pool.Reserve1, amountIn)
		c.newReserve0.Sub(pool.Reserve0, amountOut)
	}

	newPoolState.Reserve0 = new(big.Int).Set(c.newReserve0)
	newPoolState.Reserve1 = new(big.Int).Set(c.newReserve1)

	return amountOut, newPoolState, nil
}

// GetReserves returns the reserves for the given token pair. For V2, this is a direct lookup.
func GetReserves(tokenInID, tokenOutID uint64, pool uniswapv2.PoolView) (reserveIn, reserveOut *big.Int, err error) {
	if tokenInID == pool.Token0 && tokenOutID == pool.Token1 {
		return pool.Reserve0, pool.Reserve1, nil
	} else if tokenInID == pool.Token1 && tokenOutID == pool.Token0 {
		return pool.Reserve1, pool.Reserve0, nil
	}
	return nil, nil, fmt.Errorf("%w: pool %d does not contain the pair %d -> %d", ErrTokenMismatch, pool.ID, tokenInID, tokenOutID)
}

func GetExchangeRate(
	tokenInID, tokenOutID uint64,
	decimalsIn uint8,
	decimalsOut uint8, // unused, kept for compatibility
	pool uniswapv2.PoolView,
) (*big.Int, error) {

	// grab temps up front so we can just defer cleanup once
	amountIn := getBig()
	temp := getBig()

	defer func() {
		putBig(amountIn)
		putBig(temp)
	}()

	// figure out amountIn = reserveSide / 100
	switch tokenInID {
	case pool.Token0:
		if pool.Reserve0.Sign() == 0 {
			return nil, errors.New("zero reserve for token0")
		}
		amountIn.Div(pool.Reserve0, hundred)
	case pool.Token1:
		if pool.Reserve1.Sign() == 0 {
			return nil, errors.New("zero reserve for token1")
		}
		amountIn.Div(pool.Reserve1, hundred)
	default:
		return nil, errors.New("tokenInID not in pool")
	}

	if amountIn.Sign() == 0 {
		return nil, errors.New("computed amountIn is zero")
	}

	amountOut, err := GetAmountOut(amountIn, tokenInID, tokenOutID, pool)
	if err != nil {
		return nil, err
	}

	scaledDecimalsIn := GetScaledDecimal(decimalsIn) // read-only

	// temp = scaledDecimalsIn * amountOut
	temp.Mul(scaledDecimalsIn, amountOut)

	// final result must NOT come from pool
	exchangeRate := new(big.Int).Div(temp, amountIn)

	return exchangeRate, nil
}
