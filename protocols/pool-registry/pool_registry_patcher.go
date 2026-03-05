package poolregistry

import (
	"github.com/defistate/defistate/engine"
)

// Patcher (PoolRegistryPatcher) constructs a new registry state by applying a diff to a previous state.
// It handles updates to both the list of pools and the protocol dictionary.
func Patcher(prevState PoolRegistryView, diff PoolRegistryDiff) (PoolRegistryView, error) {
	// --- 1. Patch Pools ---

	// Create a working map from the previous state for O(1) manipulation.
	poolMap := make(map[uint64]PoolView, len(prevState.Pools))
	for _, pool := range prevState.Pools {
		poolMap[pool.ID] = pool
	}

	// Process Deletions
	for _, idToDelete := range diff.PoolDeletions {
		delete(poolMap, idToDelete)
	}

	// Process Additions
	for _, addedPool := range diff.PoolAdditions {
		poolMap[addedPool.ID] = addedPool
	}

	// Reconstruct the slice
	finalPools := make([]PoolView, 0, len(poolMap))
	for _, pool := range poolMap {
		finalPools = append(finalPools, pool)
	}

	// --- 2. Patch Protocols ---

	// Deep copy the old protocol map to ensure immutability of prevState
	finalProtocols := make(map[uint16]engine.ProtocolID, len(prevState.Protocols))
	for k, v := range prevState.Protocols {
		finalProtocols[k] = v
	}

	// Process Protocol Deletions
	for _, idToDelete := range diff.ProtocolDeletions {
		delete(finalProtocols, idToDelete)
	}

	// Process Protocol Additions
	for id, name := range diff.ProtocolAdditions {
		finalProtocols[id] = name
	}

	return PoolRegistryView{
		Pools:     finalPools,
		Protocols: finalProtocols,
	}, nil
}
