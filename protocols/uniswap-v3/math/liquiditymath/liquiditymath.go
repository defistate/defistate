package liquiditymath

import (
	"errors"
	"math/big"
)

var (
	// maxUint128 is the maximum value for a uint128 (2^128 - 1).
	maxUint128 = new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 128), big.NewInt(1))

	ErrLiquidityOverflow  = errors.New("liquidity overflow")
	ErrLiquidityUnderflow = errors.New("liquidity underflow")
)

// AddDelta adds a signed liquidity delta to an unsigned liquidity value,
// returning an error if the operation results in an overflow or underflow.
func AddDelta(dest *big.Int, x *big.Int, y *big.Int) error {
	// Perform the addition/subtraction.
	dest.Add(x, y)

	// Check for underflow (result is negative).
	if dest.Sign() < 0 {
		return ErrLiquidityUnderflow
	}

	// Check for overflow (result is greater than the max value for a uint128).
	if dest.Cmp(maxUint128) > 0 {
		return ErrLiquidityOverflow
	}

	return nil
}
