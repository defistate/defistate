package tickmath

import (
	"errors"
	"math/big"
	"sync"

	"github.com/holiman/uint256"
)

var (
	// MIN_TICK is the minimum tick that may be passed to getSqrtRatioAtTick.
	MIN_TICK = int64(-887272)
	// MAX_TICK is the maximum tick that may be passed to getSqrtRatioAtTick.
	MAX_TICK = int64(887272)

	// MIN_SQRT_RATIO is the minimum value that can be returned from getSqrtRatioAtTick.
	MIN_SQRT_RATIO, _ = new(big.Int).SetString("4295128739", 10)
	// MAX_SQRT_RATIO is the maximum value that can be returned from getSqrtRatioAtTick.
	MAX_SQRT_RATIO, _ = new(big.Int).SetString("1461446703485210103287273052203988822378723970342", 10)

	ErrTickOutOfBounds      = errors.New("tick out of bounds")
	ErrSqrtPriceOutOfBounds = errors.New("sqrt price out of bounds")

	// Pre-computed constants for performance
	one        = uint256.NewInt(1)
	maxUint256 = uint256.MustFromBig(new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1)))

	// Constants for getSqrtRatioAtTick, pre-parsed from hex.
	// These represent sqrt(1.0001^2^i) for i in 0..20, and a mask.
	ratioConstants = [22]*uint256.Int{
		uint256.MustFromBig(fromHex("0xfffcb933bd6fad37aa2d162d1a594001")),  // sqrt(1.0001^1)
		uint256.MustFromBig(fromHex("0x100000000000000000000000000000000")), // 1 in UQ128.128
		uint256.MustFromBig(fromHex("0xfff97272373d413259a46990580e213a")),  // sqrt(1.0001^2)
		uint256.MustFromBig(fromHex("0xfff2e50f5f656932ef12357cf3c7fdcc")),  // sqrt(1.0001^4)
		uint256.MustFromBig(fromHex("0xffe5caca7e10e4e61c3624eaa0941cd0")),  // sqrt(1.0001^8)
		uint256.MustFromBig(fromHex("0xffcb9843d60f6159c9db58835c926644")),  // sqrt(1.0001^16)
		uint256.MustFromBig(fromHex("0xff973b41fa98c081472e6896dfb254c0")),  // sqrt(1.0001^32)
		uint256.MustFromBig(fromHex("0xff2ea16466c96a3843ec78b326b52861")),  // sqrt(1.0001^64)
		uint256.MustFromBig(fromHex("0xfe5dee046a99a2a811c461f1969c3053")),  // sqrt(1.0001^128)
		uint256.MustFromBig(fromHex("0xfcbe86c7900a88aedcffc83b479aa3a4")),  // sqrt(1.0001^256)
		uint256.MustFromBig(fromHex("0xf987a7253ac413176f2b074cf7815e54")),  // sqrt(1.0001^512)
		uint256.MustFromBig(fromHex("0xf3392b0822b70005940c7a398e4b70f3")),  // sqrt(1.0001^1024)
		uint256.MustFromBig(fromHex("0xe7159475a2c29b7443b29c7fa6e889d9")),  // sqrt(1.0001^2048)
		uint256.MustFromBig(fromHex("0xd097f3bdfd2022b8845ad8f792aa5825")),  // sqrt(1.0001^4096)
		uint256.MustFromBig(fromHex("0xa9f746462d870fdf8a65dc1f90e061e5")),  // sqrt(1.0001^8192)
		uint256.MustFromBig(fromHex("0x70d869a156d2a1b890bb3df62baf32f7")),  // sqrt(1.0001^16384)
		uint256.MustFromBig(fromHex("0x31be135f97d08fd981231505542fcfa6")),  // sqrt(1.0001^32768)
		uint256.MustFromBig(fromHex("0x9aa508b5b7a84e1c677de54f3e99bc9")),   // sqrt(1.0001^65536)
		uint256.MustFromBig(fromHex("0x5d6af8dedb81196699c329225ee604")),    // sqrt(1.0001^131072)
		uint256.MustFromBig(fromHex("0x2216e584f5fa1ea926041bedfe98")),      // sqrt(1.0001^262144)
		uint256.MustFromBig(fromHex("0x48a170391f7dc42444e8fa2")),           // sqrt(1.0001^524288)
		uint256.MustFromBig(fromHex("0xffffffff")),                          // mask for rounding
	}
)

// tickMath holds reusable big.Int objects to avoid memory allocations.
type tickMath struct {
	ratio *uint256.Int
	rem   *uint256.Int
	temp  *big.Int
}

// pool manages a pool of tickMath objects for safe concurrent use.
var pool = sync.Pool{
	New: func() any {
		return &tickMath{
			ratio: new(uint256.Int),
			rem:   new(uint256.Int),
			temp:  new(big.Int),
		}
	},
}

// GetSqrtRatioAtTick calculates sqrt(1.0001^tick) * 2^96.
// This is a high-performance, allocation-free Go implementation.
func GetSqrtRatioAtTick(dest *big.Int, tick int64) error {
	if tick < MIN_TICK || tick > MAX_TICK {
		return ErrTickOutOfBounds
	}

	tm := pool.Get().(*tickMath)
	defer pool.Put(tm)

	absTick := tick
	if tick < 0 {
		absTick = -tick
	}

	// Initialize ratio based on the least significant bit of absTick.
	if (absTick & 0x1) != 0 {
		tm.ratio.Set(ratioConstants[0])
	} else {
		tm.ratio.Set(ratioConstants[1])
	}

	// Use a loop for a more compact and idiomatic implementation.
	// This replaces the long chain of if-statements.
	for i := 2; i < 21; i++ {
		if (absTick & (1 << (i - 1))) != 0 {
			tm.ratio.Mul(tm.ratio, ratioConstants[i]).Rsh(tm.ratio, 128)
		}
	}

	// If the tick is positive, compute the reciprocal.
	if tick > 0 {
		tm.ratio.Div(maxUint256, tm.ratio)
	}

	// Final rounding step: divide by 2^32 and round up.
	tm.rem.And(tm.ratio, ratioConstants[21]) // Use the mask constant
	tm.ratio.Rsh(tm.ratio, 32)
	if tm.rem.Sign() > 0 {
		tm.ratio.Add(tm.ratio, one)
	}

	// set destination
	tm.ratio.IntoBig(&dest)
	return nil
}

// GetTickAtSqrtRatio calculates the greatest tick value such that getRatioAtTick(tick) <= ratio.
// It uses a binary search for an efficient and accurate result.
func GetTickAtSqrtRatio(sqrtPriceX96 *big.Int) (int64, error) {
	if sqrtPriceX96.Cmp(MIN_SQRT_RATIO) < 0 || sqrtPriceX96.Cmp(MAX_SQRT_RATIO) >= 0 {
		return 0, ErrSqrtPriceOutOfBounds
	}

	// The binary search range is the full set of valid ticks.
	low := MIN_TICK
	high := MAX_TICK
	var tick int64
	var err error
	// Reusable variable for the loop to avoid allocations.
	p := pool.Get().(*tickMath)
	defer pool.Put(p)

	sqrtRatio := p.temp

	for low <= high {
		mid := (low + high) / 2
		err = GetSqrtRatioAtTick(sqrtRatio, mid)
		if err != nil {
			return 0, err // Should not happen within the valid range
		}

		if sqrtRatio.Cmp(sqrtPriceX96) <= 0 {
			// If the price at mid is <= target, mid is a potential answer.
			// Try to find a larger tick that also satisfies the condition.
			tick = mid
			low = mid + 1
		} else {
			// If the price at mid is > target, the answer must be in the lower half.
			high = mid - 1
		}
	}

	return tick, nil
}

// Helper to create a big.Int from a hex string.
func fromHex(s string) *big.Int {
	n, _ := new(big.Int).SetString(s[2:], 16)
	return n
}
