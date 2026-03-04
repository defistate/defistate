package uniswapv2

// --- Diff Structures with Helper Methods ---

type UniswapV2SystemDiff struct {
	Additions []PoolView `json:"additions,omitempty"`
	Updates   []PoolView `json:"updates,omitempty"`
	Deletions []uint64   `json:"deletions,omitempty"`
}

// IsEmpty returns true if the diff contains no changes.
func (d UniswapV2SystemDiff) IsEmpty() bool {
	return len(d.Additions) == 0 && len(d.Updates) == 0 && len(d.Deletions) == 0
}

// Differ is a concrete implementation of the UniswapV2SystemDiffer function type.
// It efficiently calculates the difference between two states of Uniswap V2 pools.
func Differ(old, new []PoolView) UniswapV2SystemDiff {
	// --- 1. Create maps for efficient lookups ---
	// The key is the pool's unique ID, and the value is the PoolView itself.
	oldPoolsMap := make(map[uint64]PoolView, len(old))
	for _, pool := range old {
		oldPoolsMap[pool.ID] = pool
	}

	newPoolsMap := make(map[uint64]PoolView, len(new))
	for _, pool := range new {
		newPoolsMap[pool.ID] = pool
	}

	var additions []PoolView
	var updates []PoolView
	var deletions []uint64

	// --- 2. Identify Additions and Updates ---
	// Iterate through the new set of pools.
	for newID, newPool := range newPoolsMap {
		oldPool, exists := oldPoolsMap[newID]

		if !exists {
			// If the pool from the new list does not exist in the old list, it's an addition.
			additions = append(additions, newPool)
		} else {
			// If the pool exists in both, we perform a high-performance, manual check
			// on the fields that are expected to change: the reserves.
			// This is significantly faster than using reflect.DeepEqual.
			// The Cmp method on big.Int returns 0 if the numbers are equal.
			if oldPool.Reserve0.Cmp(newPool.Reserve0) != 0 || oldPool.Reserve1.Cmp(newPool.Reserve1) != 0 {
				updates = append(updates, newPool)
			}
		}
	}

	// --- 3. Identify Deletions ---
	// Iterate through the old set of pools.
	for oldID := range oldPoolsMap {
		_, exists := newPoolsMap[oldID]
		if !exists {
			// If a pool from the old list does not exist in the new list, it has been deleted.
			deletions = append(deletions, oldID)
		}
	}

	return UniswapV2SystemDiff{
		Additions: additions,
		Updates:   updates,
		Deletions: deletions,
	}
}
