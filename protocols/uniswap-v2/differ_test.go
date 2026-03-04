package uniswapv2

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiffer(t *testing.T) {
	// --- Base Data for Tests ---
	pool1Old := PoolView{ID: 1, Reserve0: big.NewInt(1000), Reserve1: big.NewInt(2000)}
	pool2Old := PoolView{ID: 2, Reserve0: big.NewInt(3000), Reserve1: big.NewInt(4000)}
	pool3Old := PoolView{ID: 3, Reserve0: big.NewInt(5000), Reserve1: big.NewInt(6000)}

	t.Run("should identify additions correctly", func(t *testing.T) {
		oldState := []PoolView{pool1Old}
		newState := []PoolView{pool1Old, pool2Old} // pool2Old is the addition

		diff := Differ(oldState, newState)

		require.NotNil(t, diff)
		assert.Len(t, diff.Additions, 1, "Should have one addition")
		assert.Equal(t, pool2Old.ID, diff.Additions[0].ID, "The correct pool should be marked as an addition")
		assert.Empty(t, diff.Updates, "Should have no updates")
		assert.Empty(t, diff.Deletions, "Should have no deletions")
	})

	t.Run("should identify deletions correctly", func(t *testing.T) {
		oldState := []PoolView{pool1Old, pool2Old} // pool2Old will be deleted
		newState := []PoolView{pool1Old}

		diff := Differ(oldState, newState)

		require.NotNil(t, diff)
		assert.Empty(t, diff.Additions, "Should have no additions")
		assert.Empty(t, diff.Updates, "Should have no updates")
		assert.Len(t, diff.Deletions, 1, "Should have one deletion")
		assert.Equal(t, pool2Old.ID, diff.Deletions[0], "The correct pool ID should be marked for deletion")
	})

	t.Run("should identify updates correctly", func(t *testing.T) {
		pool1Updated := PoolView{ID: 1, Reserve0: big.NewInt(1001), Reserve1: big.NewInt(2000)} // Reserve0 changed

		oldState := []PoolView{pool1Old}
		newState := []PoolView{pool1Updated}

		diff := Differ(oldState, newState)

		require.NotNil(t, diff)
		assert.Empty(t, diff.Additions, "Should have no additions")
		assert.Len(t, diff.Updates, 1, "Should have one update")
		assert.Equal(t, pool1Updated.ID, diff.Updates[0].ID, "The correct pool should be marked as an update")
		assert.Empty(t, diff.Deletions, "Should have no deletions")
	})

	t.Run("should handle a mix of additions, updates, and deletions", func(t *testing.T) {
		// pool1 is updated, pool2 is unchanged, pool3 is deleted
		// pool4 is added
		pool1Updated := PoolView{ID: 1, Reserve0: big.NewInt(1001), Reserve1: big.NewInt(2000)}
		pool4New := PoolView{ID: 4, Reserve0: big.NewInt(7000), Reserve1: big.NewInt(8000)}

		oldState := []PoolView{pool1Old, pool2Old, pool3Old}
		newState := []PoolView{pool1Updated, pool2Old, pool4New}

		diff := Differ(oldState, newState)

		require.NotNil(t, diff)
		assert.Len(t, diff.Additions, 1, "Should have one addition")
		assert.Equal(t, pool4New.ID, diff.Additions[0].ID)

		assert.Len(t, diff.Updates, 1, "Should have one update")
		assert.Equal(t, pool1Updated.ID, diff.Updates[0].ID)

		assert.Len(t, diff.Deletions, 1, "Should have one deletion")
		assert.Equal(t, pool3Old.ID, diff.Deletions[0])
	})

	t.Run("should produce an empty diff when there are no changes", func(t *testing.T) {
		oldState := []PoolView{pool1Old, pool2Old}
		newState := []PoolView{pool1Old, pool2Old}

		diff := Differ(oldState, newState)

		require.NotNil(t, diff)
		assert.Empty(t, diff.Additions, "Should have no additions")
		assert.Empty(t, diff.Updates, "Should have no updates")
		assert.Empty(t, diff.Deletions, "Should have no deletions")
	})

	t.Run("should handle empty initial and new states", func(t *testing.T) {
		oldState := []PoolView{}
		newState := []PoolView{}

		diff := Differ(oldState, newState)

		require.NotNil(t, diff)
		assert.Empty(t, diff.Additions)
		assert.Empty(t, diff.Updates)
		assert.Empty(t, diff.Deletions)
	})
}
