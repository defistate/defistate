package uniswapv3

import (
	"math/big"

	"github.com/defistate/defistate/protocols/uniswap-v3/ticks"
)

// --- Deep Copy Helper Functions ---

// copyTickInfo creates a deep copy of a TickInfo struct, ensuring *big.Int pointers are new.
func copyTickInfo(t ticks.TickInfo) ticks.TickInfo {
	newTick := t
	newTick.LiquidityNet = new(big.Int).Set(t.LiquidityNet)

	// LiquidityGross is also a pointer and needs to be copied.
	newTick.LiquidityGross = new(big.Int).Set(t.LiquidityGross)

	return newTick
}

// deepCopyPool creates a new PoolView with its own memory for all pointer types,
// including the nested Ticks slice. This is essential for memory safety.
func deepCopyPool(p PoolView) PoolView {
	newPool := p
	// Deep copy the *big.Int fields. Based on the system's contract, these are never nil.
	newPool.Liquidity = new(big.Int).Set(p.Liquidity)
	newPool.SqrtPriceX96 = new(big.Int).Set(p.SqrtPriceX96)

	// Deep copy the Ticks slice by creating a new slice and copying each element.
	if p.Ticks != nil {
		newTicks := make([]ticks.TickInfo, len(p.Ticks))
		for i, tick := range p.Ticks {
			newTicks[i] = copyTickInfo(tick)
		}
		newPool.Ticks = newTicks
	}
	return newPool
}

// --- Patcher Implementation ---

// Patcher is a concrete implementation of the UniswapV3SubsystemPatcher function type.
// It efficiently constructs a new state for Uniswap V3 pools by applying a diff to a previous state.
func Patcher(prevState []PoolView, diff UniswapV3SystemDiff) ([]PoolView, error) {
	// 1. Create a map from the previous state for efficient manipulation, ensuring a deep copy.
	newStateMap := make(map[uint64]PoolView, len(prevState))
	for _, pool := range prevState {
		newStateMap[pool.ID] = deepCopyPool(pool)
	}

	// 2. Process deletions.
	for _, poolIDToDelete := range diff.Deletions {
		delete(newStateMap, poolIDToDelete)
	}

	// 3. Process updates by replacing the old pool with a deep copy of the new one.
	for _, updatedPool := range diff.Updates {
		newStateMap[updatedPool.ID] = deepCopyPool(updatedPool)
	}

	// 4. Process additions with a deep copy.
	for _, addedPool := range diff.Additions {
		newStateMap[addedPool.ID] = deepCopyPool(addedPool)
	}

	// 5. Convert the final map back into a slice.
	finalState := make([]PoolView, 0, len(newStateMap))
	for _, pool := range newStateMap {
		finalState = append(finalState, pool)
	}

	return finalState, nil
}
