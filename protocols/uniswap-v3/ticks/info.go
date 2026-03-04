package ticks

import "math/big"

// TickInfo represents the information about a tick in a Uniswap V3 pool.
// i know big.Int is not the most cache-friendly type, but it is accurate and required for this implementation
// it will be replaced in the future.
type TickInfo struct {
	Index          int64    `json:"index"`
	LiquidityGross *big.Int `json:"liquidityGross"`
	LiquidityNet   *big.Int `json:"liquidityNet"`
	// all we care about for now are the liquidity fields
	//FeeGrowthOutside0x128           *big.Int
	//FeeGrowthOutside1x128           *big.Int
	//TickCumulativeOutside           *big.Int
	//SecondsPerLiquidityOutside0x128 *big.Int
	//SecondsOutside                  *big.Int
	//Initialized                     bool -presence of this object implicitly means tick is initialized
}
