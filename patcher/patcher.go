package patcher

import (
	"errors"
	"fmt"

	differ "github.com/defistate/defistate/differ"
	engine "github.com/defistate/defistate/engine"
)

// --- Type Definitions ---

// PatcherFunc applies a diff to a previous state to produce a new state.
//
// CONTRACT:
// 1. Immutability: Implementations MUST NOT mutate 'prevState'. They must create a copy.
// 2. nil Handling: 'prevState' may be nil if this is a newly added protocol.
type PatcherFunc func(prevState any, diffData any) (newState any, err error)

// --- Config and Main Struct ---

type StatePatcherConfig struct {
	// Map Schema -> Patcher Function
	// Example: "defistate/uniswapv2/poolView@v1" -> UniswapV2Patcher
	Patchers map[engine.ProtocolSchema]PatcherFunc
}

func (c *StatePatcherConfig) validate() error {
	for _, patcher := range c.Patchers {
		if patcher == nil {
			return errors.New("patcher cannot be nil")
		}
	}
	return nil
}

// StatePatcher is the generic engine for applying state updates.
type StatePatcher struct {
	patchers map[engine.ProtocolSchema]PatcherFunc
}

// NewStatePatcher constructs a new patcher from a configuration.
func NewStatePatcher(cfg *StatePatcherConfig) (*StatePatcher, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	// Copy map to ensure immutability
	patchers := make(map[engine.ProtocolSchema]PatcherFunc, len(cfg.Patchers))
	for k, v := range cfg.Patchers {
		patchers[k] = v
	}

	return &StatePatcher{
		patchers: patchers,
	}, nil
}

// --- Implementation ---

// Patch creates a new State by applying the Diff to the Old State.
// It uses "Structural Sharing": parts of the state that didn't change are shared
// by reference. Parts that changed are replaced by the PatcherFunc.
func (p *StatePatcher) Patch(oldState *engine.State, diff *differ.StateDiff) (*engine.State, error) {
	// 1. Integrity Check
	if oldState.Block.Number.Uint64() != diff.FromBlock {
		return nil, fmt.Errorf("patcher: mismatch fromBlock (state=%d, diff=%d)", oldState.Block.Number.Uint64(), diff.FromBlock)
	}

	// 2. Initialize New Protocols Map
	// We start with a shallow copy of the old map. This preserves all "Unchanged" data efficiently.
	newProtocols := make(map[engine.ProtocolID]engine.ProtocolState, len(oldState.Protocols))
	for k, v := range oldState.Protocols {
		newProtocols[k] = v
	}

	// 3. Apply Diffs
	// We iterate only over the protocols that have changes.
	for protocolID, protocolDiff := range diff.Protocols {

		// A. Find the Patcher logic for this specific data type
		patcherFunc, ok := p.patchers[protocolDiff.Schema]
		if !ok {
			return nil, fmt.Errorf("patcher: no patcher registered for schema %q (protocol=%s)", protocolDiff.Schema, protocolID)
		}

		// B. Retrieve Old Data (if it exists)
		var oldData any
		if oldResult, exists := oldState.Protocols[protocolID]; exists {
			// Safety check: Schema migration is complex; for now, assume schemas must match.
			if oldResult.Schema != protocolDiff.Schema {
				return nil, fmt.Errorf("patcher: schema mismatch for protocol %s (old=%s, diff=%s)", protocolID, oldResult.Schema, protocolDiff.Schema)
			}
			oldData = oldResult.Data
		}

		// C. Execute the Patch
		// The PatcherFunc is responsible for deep-copying oldData + applying diffData
		newData, err := patcherFunc(oldData, protocolDiff.Data)
		if err != nil {
			return nil, fmt.Errorf("patcher: failed to patch protocol %s: %w", protocolID, err)
		}

		// D. Construct the New Protocol Result
		// We use metadata from the Diff, as it represents the latest state truth.
		newResult := engine.ProtocolState{
			Meta:              protocolDiff.Meta,
			SyncedBlockNumber: protocolDiff.SyncedBlockNumber,
			Schema:            protocolDiff.Schema,
			Data:              newData,
			Error:             protocolDiff.Error,
		}

		// E. Update the map
		newProtocols[protocolID] = newResult
	}

	// 4. Return Final State
	return &engine.State{
		ChainID:   oldState.ChainID, // Chain ID implies fork consistency
		Timestamp: diff.Timestamp,   // The time the diff was calculated
		Block:     diff.ToBlock,     // The new target block
		Protocols: newProtocols,
	}, nil
}
