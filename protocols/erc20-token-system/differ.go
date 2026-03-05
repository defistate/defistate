package token

type TokenSystemDiff struct {
	Additions []TokenView `json:"additions,omitempty"`
	Updates   []TokenView `json:"updates,omitempty"`
	Deletions []uint64    `json:"deletions,omitempty"`
}

// IsEmpty returns true if the diff contains no changes.
func (d TokenSystemDiff) IsEmpty() bool {
	return len(d.Additions) == 0 && len(d.Updates) == 0 && len(d.Deletions) == 0
}

// Differ is a concrete implementation of the TokenSystemDiffer function type.
// It efficiently calculates the difference between two states of the token system.
// The logic uses maps for O(1) average time complexity lookups to ensure performance.
func Differ(old, new []TokenView) TokenSystemDiff {
	// --- 1. Create maps for efficient lookups ---
	// The key is the token's unique ID.
	oldTokensMap := make(map[uint64]TokenView, len(old))
	for _, token := range old {
		oldTokensMap[token.ID] = token
	}

	newTokensMap := make(map[uint64]TokenView, len(new))
	for _, token := range new {
		newTokensMap[token.ID] = token
	}

	var additions []TokenView
	var updates []TokenView
	var deletions []uint64

	// --- 2. Identify Additions and Updates ---
	// Iterate through the new set of tokens.
	for newID, newToken := range newTokensMap {
		oldToken, exists := oldTokensMap[newID]

		if !exists {
			// If the token from the new list does not exist in the old list, it's an addition.
			additions = append(additions, newToken)
		} else {
			// If the token exists in both, perform a high-performance, manual check
			// on the specific fields that are expected to change.
			if oldToken.FeeOnTransferPercent != newToken.FeeOnTransferPercent ||
				oldToken.GasForTransfer != newToken.GasForTransfer {
				updates = append(updates, newToken)
			}
		}
	}

	// --- 3. Identify Deletions ---
	// Iterate through the old set of tokens.
	for oldID := range oldTokensMap {
		_, exists := newTokensMap[oldID]
		if !exists {
			// If a token from the old list does not exist in the new list, it has been deleted.
			deletions = append(deletions, oldID)
		}
	}

	return TokenSystemDiff{
		Additions: additions,
		Updates:   updates,
		Deletions: deletions,
	}
}
