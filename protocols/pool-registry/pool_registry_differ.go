package poolregistry

import (
	"github.com/defistate/defistate/engine"
)

// PoolRegistryDiff represents the changes required to transition from one registry state to another.
type PoolRegistryDiff struct {
	// PoolAdditions contains new pools that were created.
	PoolAdditions []PoolView `json:"poolAdditions,omitempty"`
	// PoolDeletions contains IDs of pools that were removed.
	PoolDeletions []uint64 `json:"poolDeletions,omitempty"`
	// ProtocolAdditions contains new dictionary mappings (uint16 -> string).
	ProtocolAdditions map[uint16]engine.ProtocolID `json:"protocolAdditions,omitempty"`
	// ProtocolDeletions contains internal IDs of protocols that were removed/retired.
	ProtocolDeletions []uint16 `json:"protocolDeletions,omitempty"`
}

// IsEmpty returns true if the diff contains no changes.
func (d PoolRegistryDiff) IsEmpty() bool {
	return len(d.PoolAdditions) == 0 &&
		len(d.PoolDeletions) == 0 &&
		len(d.ProtocolAdditions) == 0 &&
		len(d.ProtocolDeletions) == 0
}

// Differ calculates the difference between two full registry views (Old -> New).
func Differ(old, new PoolRegistryView) PoolRegistryDiff {
	// --- 1. Diff Pools ---

	// Map old pools for O(1) existence checks
	oldPoolsMap := make(map[uint64]struct{}, len(old.Pools))
	for _, pool := range old.Pools {
		oldPoolsMap[pool.ID] = struct{}{}
	}

	// Map new pools for O(1) existence checks and data retrieval
	newPoolsMap := make(map[uint64]PoolView, len(new.Pools))
	for _, pool := range new.Pools {
		newPoolsMap[pool.ID] = pool
	}

	var poolAdditions []PoolView
	var poolDeletions []uint64

	// Identify Pool Additions
	for newID, newPool := range newPoolsMap {
		if _, exists := oldPoolsMap[newID]; !exists {
			poolAdditions = append(poolAdditions, newPool)
		}
	}

	// Identify Pool Deletions
	for oldID := range oldPoolsMap {
		if _, exists := newPoolsMap[oldID]; !exists {
			poolDeletions = append(poolDeletions, oldID)
		}
	}

	// --- 2. Diff Protocols ---

	protocolAdditions := make(map[uint16]engine.ProtocolID)
	var protocolDeletions []uint16

	// Identify Protocol Additions (New has it, Old doesn't)
	for id, name := range new.Protocols {
		if oldName, exists := old.Protocols[id]; !exists || oldName != name {
			protocolAdditions[id] = name
		}
	}

	// Identify Protocol Deletions (Old has it, New doesn't)
	for id := range old.Protocols {
		if _, exists := new.Protocols[id]; !exists {
			protocolDeletions = append(protocolDeletions, id)
		}
	}

	return PoolRegistryDiff{
		PoolAdditions:     poolAdditions,
		PoolDeletions:     poolDeletions,
		ProtocolAdditions: protocolAdditions,
		ProtocolDeletions: protocolDeletions,
	}
}
