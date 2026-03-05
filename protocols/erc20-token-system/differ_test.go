package token

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper function to create a new TokenView for testing.
func newTestToken(id uint64, symbol string, fee float64, gas uint64) TokenView {
	return TokenView{
		ID:                   id,
		Symbol:               symbol,
		FeeOnTransferPercent: fee,
		GasForTransfer:       gas,
	}
}

func TestDiffer(t *testing.T) {
	// --- Base Data for Tests ---
	token1Old := newTestToken(1, "WETH", 0.0, 21000)
	token2Old := newTestToken(2, "USDC", 0.0, 35000)
	token3Old := newTestToken(3, "DAI", 0.1, 40000)

	t.Run("should identify additions correctly", func(t *testing.T) {
		oldState := []TokenView{token1Old}
		newState := []TokenView{token1Old, token2Old} // token2Old is the addition

		diff := Differ(oldState, newState)

		require.NotNil(t, diff)
		assert.Len(t, diff.Additions, 1, "Should have one addition")
		assert.Equal(t, token2Old.ID, diff.Additions[0].ID, "The correct token should be marked as an addition")
		assert.Empty(t, diff.Updates, "Should have no updates")
		assert.Empty(t, diff.Deletions, "Should have no deletions")
	})

	t.Run("should identify deletions correctly", func(t *testing.T) {
		oldState := []TokenView{token1Old, token2Old} // token2Old will be deleted
		newState := []TokenView{token1Old}

		diff := Differ(oldState, newState)

		require.NotNil(t, diff)
		assert.Empty(t, diff.Additions, "Should have no additions")
		assert.Empty(t, diff.Updates, "Should have no updates")
		assert.Len(t, diff.Deletions, 1, "Should have one deletion")
		assert.Equal(t, token2Old.ID, diff.Deletions[0], "The correct token ID should be marked for deletion")
	})

	t.Run("should identify updates when FeeOnTransferPercent changes", func(t *testing.T) {
		token1Updated := newTestToken(1, "WETH", 0.3, 21000) // Fee has changed

		oldState := []TokenView{token1Old}
		newState := []TokenView{token1Updated}

		diff := Differ(oldState, newState)

		require.NotNil(t, diff)
		assert.Empty(t, diff.Additions, "Should have no additions")
		assert.Len(t, diff.Updates, 1, "Should have one update")
		assert.Equal(t, token1Updated.ID, diff.Updates[0].ID, "The correct token should be marked as an update")
		assert.Empty(t, diff.Deletions, "Should have no deletions")
	})

	t.Run("should identify updates when GasForTransfer changes", func(t *testing.T) {
		token1Updated := newTestToken(1, "WETH", 0.0, 22000) // Gas has changed

		oldState := []TokenView{token1Old}
		newState := []TokenView{token1Updated}

		diff := Differ(oldState, newState)

		require.NotNil(t, diff)
		assert.Empty(t, diff.Additions)
		assert.Len(t, diff.Updates, 1)
		assert.Equal(t, token1Updated.ID, diff.Updates[0].ID)
		assert.Empty(t, diff.Deletions)
	})

	t.Run("should handle a mix of additions, updates, and deletions", func(t *testing.T) {
		// token1 is updated, token2 is unchanged, token3 is deleted
		// token4 is added
		token1Updated := newTestToken(1, "WETH", 0.0, 21001) // Gas updated
		token4New := newTestToken(4, "WBTC", 0.0, 50000)

		oldState := []TokenView{token1Old, token2Old, token3Old}
		newState := []TokenView{token1Updated, token2Old, token4New}

		diff := Differ(oldState, newState)

		require.NotNil(t, diff)
		assert.Len(t, diff.Additions, 1, "Should have one addition")
		assert.Equal(t, token4New.ID, diff.Additions[0].ID)

		assert.Len(t, diff.Updates, 1, "Should have one update")
		assert.Equal(t, token1Updated.ID, diff.Updates[0].ID)

		assert.Len(t, diff.Deletions, 1, "Should have one deletion")
		assert.Equal(t, token3Old.ID, diff.Deletions[0])
	})

	t.Run("should produce an empty diff when there are no changes", func(t *testing.T) {
		oldState := []TokenView{token1Old, token2Old}
		newState := []TokenView{token1Old, token2Old}

		diff := Differ(oldState, newState)

		require.NotNil(t, diff)
		assert.Empty(t, diff.Additions, "Should have no additions")
		assert.Empty(t, diff.Updates, "Should have no updates")
		assert.Empty(t, diff.Deletions, "Should have no deletions")
	})
}
