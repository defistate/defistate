package uniswapv3

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"sync"

	uniswapv3 "github.com/defistate/defistate/protocols/uniswap-v3"
	"github.com/defistate/defistate/protocols/uniswap-v3/math/liquiditymath"
	"github.com/defistate/defistate/protocols/uniswap-v3/math/swapmath"
	"github.com/defistate/defistate/protocols/uniswap-v3/math/tickbitmap"
	"github.com/defistate/defistate/protocols/uniswap-v3/math/tickmath"
)

var (
	ErrInvalidAmountIn    = errors.New("amountIn must be greater than zero")
	ErrTokenMismatch      = errors.New("token mismatch")
	ErrLiquidityUnderflow = errors.New("liquidity underflow")

	Q96, _        = new(big.Int).SetString("79228162514264337593543950336", 10)
	Q64F          = new(big.Float).SetInt(Q96)
	MaxUint256, _ = new(big.Int).SetString("ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", 16)
)

// swapState represents the state of a swap as it progresses.
// It includes all temporary variables needed for the simulation to avoid allocations.
type swapState struct {
	// --- Input parameters ---
	amountSpecifiedRemaining *big.Int
	amountCalculated         *big.Int
	sqrtPriceX96             *big.Int
	tick                     int64
	liquidity                *big.Int

	// --- Reusable temporary variables for the loop ---
	sqrtPriceStartX96 *big.Int
	sqrtPriceNextX96  *big.Int
	targetPrice       *big.Int
	stepAmountIn      *big.Int
	stepAmountOut     *big.Int
	stepFeeAmount     *big.Int
	tempAmount        *big.Int
	liquidityNet      *big.Int
}

// swapStatePool manages a pool of swapState objects for safe concurrent use.
var swapStatePool = sync.Pool{
	New: func() any {
		return &swapState{
			amountSpecifiedRemaining: new(big.Int),
			amountCalculated:         new(big.Int),
			sqrtPriceX96:             new(big.Int),
			liquidity:                new(big.Int),
			sqrtPriceStartX96:        new(big.Int),
			sqrtPriceNextX96:         new(big.Int),
			targetPrice:              new(big.Int),
			stepAmountIn:             new(big.Int),
			stepAmountOut:            new(big.Int),
			stepFeeAmount:            new(big.Int),
			tempAmount:               new(big.Int),
			liquidityNet:             new(big.Int),
		}
	},
}

// _swap is the internal, core simulation engine, fully optimized to be allocation-free.
func _swap(
	state *swapState,
	pool uniswapv3.PoolView,
	sqrtPriceLimitX96 *big.Int,
	zeroForOne bool,
) error {

	if sqrtPriceLimitX96 == nil {
		if zeroForOne {
			sqrtPriceLimitX96 = tickmath.MIN_SQRT_RATIO
		} else {
			sqrtPriceLimitX96 = tickmath.MAX_SQRT_RATIO
		}
	}

	exactInput := state.amountSpecifiedRemaining.Sign() > 0

	// Main simulation loop.
	for state.amountSpecifiedRemaining.Sign() != 0 && state.sqrtPriceX96.Cmp(sqrtPriceLimitX96) != 0 {
		state.sqrtPriceStartX96.Set(state.sqrtPriceX96)

		tickNext, initialized := tickbitmap.NextInitializedTickWithinOneWord(pool.Ticks, state.tick, zeroForOne)
		if !initialized {
			break
		}
		if tickNext < tickmath.MIN_TICK {
			tickNext = tickmath.MIN_TICK
		} else if tickNext > tickmath.MAX_TICK {
			tickNext = tickmath.MAX_TICK
		}

		err := tickmath.GetSqrtRatioAtTick(state.sqrtPriceNextX96, tickNext)
		if err != nil {
			return err
		}

		if (zeroForOne && state.sqrtPriceNextX96.Cmp(sqrtPriceLimitX96) < 0) ||
			(!zeroForOne && state.sqrtPriceNextX96.Cmp(sqrtPriceLimitX96) > 0) {
			state.targetPrice.Set(sqrtPriceLimitX96)
		} else {
			state.targetPrice.Set(state.sqrtPriceNextX96)
		}

		err = swapmath.ComputeSwapStep(
			state.sqrtPriceX96, state.stepAmountIn, state.stepAmountOut, state.stepFeeAmount, // Destination pointers
			state.sqrtPriceStartX96,
			state.targetPrice,
			state.liquidity,
			state.amountSpecifiedRemaining,
			state.tempAmount.SetUint64(pool.Fee),
		)
		if err != nil {
			break // Can happen if liquidity is zero
		}

		if exactInput {
			state.amountSpecifiedRemaining.Sub(state.amountSpecifiedRemaining, state.tempAmount.Add(state.stepAmountIn, state.stepFeeAmount))
			state.amountCalculated.Add(state.amountCalculated, state.stepAmountOut)
		} else {
			state.amountSpecifiedRemaining.Add(state.amountSpecifiedRemaining, state.stepAmountOut)
			state.amountCalculated.Add(state.amountCalculated, state.tempAmount.Add(state.stepAmountIn, state.stepFeeAmount))
		}

		if state.sqrtPriceX96.Cmp(state.sqrtPriceNextX96) == 0 {
			var foundTick bool
			for _, t := range pool.Ticks {
				if t.Index == tickNext {
					state.liquidityNet.Set(t.LiquidityNet)
					foundTick = true
					break
				}
			}

			if foundTick {
				if zeroForOne {
					state.liquidityNet.Neg(state.liquidityNet)
				}
				err = liquiditymath.AddDelta(state.liquidity, state.liquidity, state.liquidityNet)
				if err != nil {
					if errors.Is(err, liquiditymath.ErrLiquidityUnderflow) {
						break
					}
					return err
				}
			}

			if zeroForOne {
				state.tick = tickNext - 1
			} else {
				state.tick = tickNext
			}
		} else if state.sqrtPriceX96.Cmp(state.sqrtPriceStartX96) != 0 {
			state.tick, err = tickmath.GetTickAtSqrtRatio(state.sqrtPriceX96)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// SimulateExactInSwap calculates the resulting amount out and the new pool state for a given amount in.
func SimulateExactInSwap(
	amountIn *big.Int,
	sqrtPriceLimitX96 *big.Int,
	tokenInID uint64,
	pool uniswapv3.PoolView,
) (amountOut *big.Int, newPoolState uniswapv3.PoolView, err error) {
	if amountIn == nil || amountIn.Sign() <= 0 {
		return nil, uniswapv3.PoolView{}, ErrInvalidAmountIn
	}

	zeroForOne := tokenInID == pool.Token0
	if !zeroForOne && tokenInID != pool.Token1 {
		return nil, uniswapv3.PoolView{}, fmt.Errorf("%w: token %d is not in pool %d", ErrTokenMismatch, tokenInID, pool.ID)
	}

	state := swapStatePool.Get().(*swapState)
	defer swapStatePool.Put(state)

	// Set the amount to be swapped (a positive number)
	state.amountSpecifiedRemaining.Set(amountIn)
	state.amountCalculated.SetInt64(0)
	state.sqrtPriceX96.Set(pool.SqrtPriceX96)
	state.tick = pool.Tick
	state.liquidity.Set(pool.Liquidity)

	if err := _swap(state, pool, sqrtPriceLimitX96, zeroForOne); err != nil {
		return nil, uniswapv3.PoolView{}, err
	}

	newPoolState = pool
	newPoolState.SqrtPriceX96 = new(big.Int).Set(state.sqrtPriceX96)
	newPoolState.Tick = int64(state.tick)
	newPoolState.Liquidity = new(big.Int).Set(state.liquidity)

	// amountCalculated now holds the amountOut
	amountOut = new(big.Int).Set(state.amountCalculated)
	return amountOut, newPoolState, nil
}

// SimulateExactOutSwap calculates the required amount in and the new pool state for a given amount out.
func SimulateExactOutSwap(
	amountOut *big.Int,
	sqrtPriceLimitX96 *big.Int,
	tokenInID uint64,
	pool uniswapv3.PoolView,
) (amountIn *big.Int, newPoolState uniswapv3.PoolView, err error) {
	if amountOut == nil || amountOut.Sign() >= 0 {
		return nil, uniswapv3.PoolView{}, ErrInvalidAmountIn // Or a new error like ErrInvalidAmountOut
	}

	zeroForOne := tokenInID == pool.Token0
	if !zeroForOne && tokenInID != pool.Token1 {
		return nil, uniswapv3.PoolView{}, fmt.Errorf("%w: token %d is not in pool %d", ErrTokenMismatch, tokenInID, pool.ID)
	}

	state := swapStatePool.Get().(*swapState)
	defer swapStatePool.Put(state)

	// Set the amount to be received (a negative number to trigger exact-out logic in _swap)
	state.amountSpecifiedRemaining.Set(amountOut)
	state.amountCalculated.SetInt64(0)
	state.sqrtPriceX96.Set(pool.SqrtPriceX96)
	state.tick = pool.Tick
	state.liquidity.Set(pool.Liquidity)

	if err := _swap(state, pool, sqrtPriceLimitX96, zeroForOne); err != nil {
		return nil, uniswapv3.PoolView{}, err
	}

	newPoolState = pool
	newPoolState.SqrtPriceX96 = new(big.Int).Set(state.sqrtPriceX96)
	newPoolState.Tick = int64(state.tick)
	newPoolState.Liquidity = new(big.Int).Set(state.liquidity)

	// amountCalculated now holds the required amountIn
	amountIn = new(big.Int).Set(state.amountCalculated)
	return amountIn, newPoolState, nil
}

// GetAmountOut calculates the amount out for a given exact amount in.
func GetAmountOut(
	amountIn *big.Int,
	sqrtPriceLimitX96 *big.Int,
	tokenInID uint64,
	pool uniswapv3.PoolView,
) (*big.Int, error) {
	if amountIn == nil || amountIn.Sign() <= 0 {
		return nil, ErrInvalidAmountIn
	}

	zeroForOne := tokenInID == pool.Token0
	if !zeroForOne && tokenInID != pool.Token1 {
		return nil, fmt.Errorf("%w: token %d is not in pool %d", ErrTokenMismatch, tokenInID, pool.ID)
	}

	state := swapStatePool.Get().(*swapState)
	defer swapStatePool.Put(state)

	state.amountSpecifiedRemaining.Set(amountIn)
	state.amountCalculated.SetInt64(0)
	state.sqrtPriceX96.Set(pool.SqrtPriceX96)
	state.tick = pool.Tick
	state.liquidity.Set(pool.Liquidity)

	if err := _swap(state, pool, sqrtPriceLimitX96, zeroForOne); err != nil {
		return nil, err
	}
	return new(big.Int).Set(state.amountCalculated), nil
}

// GetAmountIn calculates the required amount in for a given exact amount out.
// NOTE: It expects a negative amountOut to signal the exact-output swap type.
func GetAmountIn(
	amountOut *big.Int,
	sqrtPriceLimitX96 *big.Int,
	tokenInID uint64,
	pool uniswapv3.PoolView,
) (*big.Int, error) {
	if amountOut == nil || amountOut.Sign() >= 0 {
		return nil, errors.New("amountOut must be negative for an exact-output swap")
	}

	zeroForOne := tokenInID == pool.Token0
	if !zeroForOne && tokenInID != pool.Token1 {
		return nil, fmt.Errorf("%w: token %d is not in pool %d", ErrTokenMismatch, tokenInID, pool.ID)
	}

	state := swapStatePool.Get().(*swapState)
	defer swapStatePool.Put(state)

	state.amountSpecifiedRemaining.Set(amountOut)
	state.amountCalculated.SetInt64(0)
	state.sqrtPriceX96.Set(pool.SqrtPriceX96)
	state.tick = pool.Tick
	state.liquidity.Set(pool.Liquidity)

	if err := _swap(state, pool, sqrtPriceLimitX96, zeroForOne); err != nil {
		return nil, err
	}
	return new(big.Int).Set(state.amountCalculated), nil
}

// GetVirtualReserves calculates the virtual reserves of a Uniswap V3 pool based on its
// current liquidity and price.
func GetVirtualReserves(tokenInID, tokenOutID uint64, pool uniswapv3.PoolView) (reserveIn, reserveOut *big.Int, err error) {
	if !((tokenInID == pool.Token0 && tokenOutID == pool.Token1) || (tokenInID == pool.Token1 && tokenOutID == pool.Token0)) {
		return nil, nil, fmt.Errorf("%w: provided tokens do not match pool tokens", ErrTokenMismatch)
	}

	// This function is not on a hot path, so a few allocations are acceptable for clarity.
	reserve0 := new(big.Int).Div(new(big.Int).Lsh(pool.Liquidity, 96), pool.SqrtPriceX96)
	reserve1 := new(big.Int).Div(new(big.Int).Mul(pool.Liquidity, pool.SqrtPriceX96), Q96)

	if tokenInID == pool.Token0 {
		return reserve0, reserve1, nil
	} else {
		return reserve1, reserve0, nil
	}
}

// GetSpotPrice calculates the spot price of tokenIn in terms of tokenOut,
// adjusted for token decimals. The returned big.Int represents the price
// with precision matching the decimals of tokenOut.
// For example, if tokenOut is USDT (6 decimals), a return value of 3045123456
// represents a price of 3045.123456.
func GetSpotPrice(
	tokenInID, tokenOutID uint64,
	decimalsIn, decimalsOut uint8,
	pool uniswapv3.PoolView,
) (*big.Int, error) {
	// SqrtPriceX96 is a Q64.96 fixed-point number: sqrt(token1/token0) * 2^96
	sqrtPriceX96 := pool.SqrtPriceX96
	decimalsInF := big.NewFloat(math.Pow(10, float64(decimalsIn)))
	decimalsOutF := big.NewFloat(math.Pow(10, float64(decimalsOut)))

	sqrtPriceX96F := new(big.Float).SetInt(sqrtPriceX96)
	intermediate := sqrtPriceX96F.Quo(sqrtPriceX96F, Q64F)
	price := new(big.Float).Mul(intermediate, intermediate)
	if tokenInID == pool.Token0 {
		spotPrice := new(big.Float).Quo(price, new(big.Float).Quo(decimalsOutF, decimalsInF))
		spotPrice.Mul(spotPrice, decimalsOutF)
		sp, _ := spotPrice.Int(nil)
		return sp, nil

	} else {
		spotPrice := new(big.Float).Quo(big.NewFloat(1), price)
		spotPrice.Quo(spotPrice, new(big.Float).Quo(decimalsOutF, decimalsInF))
		spotPrice.Mul(spotPrice, decimalsOutF)
		sp, _ := spotPrice.Int(nil)
		return sp, nil
	}
}
