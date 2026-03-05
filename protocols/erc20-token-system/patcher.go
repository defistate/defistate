package token

// Patcher is a concrete implementation of the TokenSubsystemPatcher function type.
// It efficiently constructs a new state for the token system by applying a diff to a previous state.
// The logic is optimized for performance using a map for O(1) average time complexity lookups.
func Patcher(prevState []TokenView, diff TokenSystemDiff) ([]TokenView, error) {
	// 1. Create a map from the previous state for efficient manipulation.
	// Since TokenView contains no pointer fields, a direct copy is safe.
	newStateMap := make(map[uint64]TokenView, len(prevState))
	for _, token := range prevState {
		newStateMap[token.ID] = token
	}

	// 2. Process deletions.
	for _, tokenIDToDelete := range diff.Deletions {
		delete(newStateMap, tokenIDToDelete)
	}

	// 3. Process updates.
	for _, updatedToken := range diff.Updates {
		newStateMap[updatedToken.ID] = updatedToken
	}

	// 4. Process additions.
	for _, addedToken := range diff.Additions {
		newStateMap[addedToken.ID] = addedToken
	}

	// 5. Convert the map back to a slice for the final state.
	finalState := make([]TokenView, 0, len(newStateMap))
	for _, token := range newStateMap {
		finalState = append(finalState, token)
	}

	return finalState, nil
}
