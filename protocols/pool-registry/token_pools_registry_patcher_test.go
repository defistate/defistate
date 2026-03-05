package poolregistry

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenPoolRegistryPatcher(t *testing.T) {
	// --- Base Data for Tests ---
	oldState := &TokenPoolsRegistryView{
		Tokens: []uint64{1, 2},
		Pools:  []uint64{10},
	}

	newStateData := &TokenPoolsRegistryView{
		Tokens: []uint64{1, 2, 3}, // Added token 3
		Pools:  []uint64{10, 11},  // Added pool 11
	}

	t.Run("should correctly replace the old state with the new state", func(t *testing.T) {
		diff := TokenPoolRegistryDiff{
			Data: newStateData,
		}

		newState, err := TokenPoolRegistryPatcher(oldState, diff)
		require.NoError(t, err)
		require.NotNil(t, newState)

		// Verify the content is from the new state, not the old one.
		assert.Equal(t, []uint64{1, 2, 3}, newState.Tokens)
		assert.Equal(t, []uint64{10, 11}, newState.Pools)
	})

	t.Run("should verify that a deep copy is returned", func(t *testing.T) {
		diff := TokenPoolRegistryDiff{
			Data: newStateData,
		}

		newState, err := TokenPoolRegistryPatcher(oldState, diff)
		require.NoError(t, err)
		require.NotNil(t, newState)

		// CRITICAL: Modify a slice in the original diff object *after* the patch has been applied.
		diff.Data.Tokens[0] = 9999

		// Verify that the new state was not affected, proving the deep copy worked.
		assert.Equal(t, uint64(1), newState.Tokens[0], "New state should be isolated from changes to the diff object")
	})

	t.Run("should handle nil data in diff", func(t *testing.T) {
		diff := TokenPoolRegistryDiff{
			Data: nil,
		}

		newState, err := TokenPoolRegistryPatcher(oldState, diff)
		require.NoError(t, err)
		assert.Nil(t, newState, "Should return nil if diff data is nil")
	})
}
