package uniswapv2

import (
	"math/big"
)

// deepCopyPool creates a new PoolView with its own memory for pointer types like *big.Int.
// This is essential to prevent the new state from sharing memory with the old state.
func deepCopyPool(p PoolView) PoolView {
	// Create a new copy of the struct.
	newPool := p
	// Create new *big.Int objects and set their values from the source.
	if p.Reserve0 != nil {
		newPool.Reserve0 = new(big.Int).Set(p.Reserve0)
	}
	if p.Reserve1 != nil {
		newPool.Reserve1 = new(big.Int).Set(p.Reserve1)
	}
	return newPool
}

// Patcher is a concrete implementation of the UniswapV2SystemDiff function type.
// It efficiently constructs a new state for Uniswap V2 pools by applying a diff to a previous state.
// The logic is optimized for performance and memory safety.
func Patcher(prevState []PoolView, diff UniswapV2SystemDiff) ([]PoolView, error) {
	// 1. Create a map from the previous state for efficient manipulation, ensuring a deep copy.
	newStateMap := make(map[uint64]PoolView, len(prevState))
	for _, pool := range prevState {
		newStateMap[pool.ID] = deepCopyPool(pool)
	}

	// 2. Process deletions.
	for _, poolIDToDelete := range diff.Deletions {
		delete(newStateMap, poolIDToDelete)
	}

	// 3. Process updates.
	// We perform a deep copy to ensure the new state is completely independent.
	for _, updatedPool := range diff.Updates {
		newStateMap[updatedPool.ID] = deepCopyPool(updatedPool)
	}

	// 4. Process additions.
	// We perform a deep copy to ensure the new state is completely independent.
	for _, addedPool := range diff.Additions {
		newStateMap[addedPool.ID] = deepCopyPool(addedPool)
	}

	// 5. Convert the map back to a slice for the final state.
	finalState := make([]PoolView, 0, len(newStateMap))
	for _, pool := range newStateMap {
		finalState = append(finalState, pool)
	}

	return finalState, nil
}
