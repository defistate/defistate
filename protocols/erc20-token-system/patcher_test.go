package token

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper to find a token by ID in a slice, for testing assertions.
func findTokenByID(tokens []TokenView, id uint64) *TokenView {
	for i := range tokens {
		if tokens[i].ID == id {
			return &tokens[i]
		}
	}
	return nil
}

func TestPatcher(t *testing.T) {
	// --- Base Data for Tests ---
	token1Old := newTestToken(1, "WETH", 0.0, 21000)
	token2Old := newTestToken(2, "USDC", 0.0, 35000)
	token3Old := newTestToken(3, "DAI", 0.1, 40000)

	initialState := []TokenView{token1Old, token2Old, token3Old}

	t.Run("should handle only additions", func(t *testing.T) {
		token4New := newTestToken(4, "WBTC", 0.0, 50000)
		diff := TokenSystemDiff{
			Additions: []TokenView{token4New},
		}

		newState, err := Patcher(initialState, diff)
		require.NoError(t, err)

		assert.Len(t, newState, 4, "Should have 4 tokens after addition")
		newToken := findTokenByID(newState, 4)
		require.NotNil(t, newToken)
		assert.Equal(t, "WBTC", newToken.Symbol)
	})

	t.Run("should handle only deletions", func(t *testing.T) {
		diff := TokenSystemDiff{
			Deletions: []uint64{2}, // Delete token with ID 2
		}

		newState, err := Patcher(initialState, diff)
		require.NoError(t, err)

		assert.Len(t, newState, 2, "Should have 2 tokens after deletion")
		assert.Nil(t, findTokenByID(newState, 2), "Token 2 should be deleted")
	})

	t.Run("should handle only updates", func(t *testing.T) {
		token1Updated := newTestToken(1, "WETH", 0.3, 22000) // Fee and Gas changed
		diff := TokenSystemDiff{
			Updates: []TokenView{token1Updated},
		}

		newState, err := Patcher(initialState, diff)
		require.NoError(t, err)

		assert.Len(t, newState, 3, "Should still have 3 tokens after update")
		updatedToken := findTokenByID(newState, 1)
		require.NotNil(t, updatedToken)
		assert.Equal(t, 0.3, updatedToken.FeeOnTransferPercent)
		assert.Equal(t, uint64(22000), updatedToken.GasForTransfer)
	})

	t.Run("should handle a mix of operations", func(t *testing.T) {
		// Add token 4, update token 2, delete token 3
		token4New := newTestToken(4, "WBTC", 0.0, 50000)
		token2Updated := newTestToken(2, "USDC", 0.0, 36000)
		diff := TokenSystemDiff{
			Additions: []TokenView{token4New},
			Updates:   []TokenView{token2Updated},
			Deletions: []uint64{3},
		}

		newState, err := Patcher(initialState, diff)
		require.NoError(t, err)

		assert.Len(t, newState, 3, "Final state should have 3 tokens")
		// Verify addition
		assert.NotNil(t, findTokenByID(newState, 4))
		// Verify update
		updatedToken := findTokenByID(newState, 2)
		require.NotNil(t, updatedToken)
		assert.Equal(t, uint64(36000), updatedToken.GasForTransfer)
		// Verify deletion
		assert.Nil(t, findTokenByID(newState, 3))
		// Verify unchanged token is still present
		assert.NotNil(t, findTokenByID(newState, 1))
	})

	t.Run("should handle an empty diff", func(t *testing.T) {
		diff := TokenSystemDiff{}

		newState, err := Patcher(initialState, diff)
		require.NoError(t, err)

		assert.ElementsMatch(t, initialState, newState, "State should be unchanged for an empty diff")
	})
}
