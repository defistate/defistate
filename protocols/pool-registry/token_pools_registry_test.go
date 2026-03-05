package poolregistry

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testGetPoolsForToken is a helper to get the unique pool IDs for a specific token from a view.
func testGetPoolsForToken(t *testing.T, view *TokenPoolsRegistryView, tokenID uint64) []uint64 {
	tokenToIndex := make(map[uint64]int)
	for i, id := range view.Tokens {
		tokenToIndex[id] = i
	}

	tokenIndex, ok := tokenToIndex[tokenID]
	if !ok {
		return nil
	}

	// Use a map to collect unique pool IDs.
	uniquePoolIDs := make(map[uint64]struct{})
	for _, edgeIndex := range view.Adjacency[tokenIndex] {
		// A live edge is one with one or more pools.
		if len(view.EdgePools[edgeIndex]) > 0 {
			// Iterate through the list of pools for this specific edge.
			for _, poolIndex := range view.EdgePools[edgeIndex] {
				poolID := view.Pools[poolIndex]
				uniquePoolIDs[poolID] = struct{}{}
			}
		}
	}

	if len(uniquePoolIDs) == 0 {
		return nil
	}

	// Convert map keys to a slice for stable sorting and comparison.
	poolIDs := make([]uint64, 0, len(uniquePoolIDs))
	for id := range uniquePoolIDs {
		poolIDs = append(poolIDs, id)
	}

	sort.Slice(poolIDs, func(i, j int) bool { return poolIDs[i] < poolIDs[j] })
	return poolIDs
}

func TestTokenPoolsRegistry(t *testing.T) {

	t.Run("NewTokenPoolsRegistry", func(t *testing.T) {
		r := NewTokenPoolsRegistry(500)
		require.NotNil(t, r)
		assert.Equal(t, 500, r.compactionThreshold)
		assert.NotNil(t, r.tokenToIndex)
		assert.NotNil(t, r.tokens)
		assert.Len(t, r.view().Tokens, 0)

		// Test default threshold
		r2 := NewTokenPoolsRegistry(0)
		assert.Equal(t, 1000, r2.compactionThreshold)
	})

	t.Run("AddPool", func(t *testing.T) {
		t.Run("WithTwoTokens", func(t *testing.T) {
			r := NewTokenPoolsRegistry(100)
			tokenIDs := []uint64{10, 20}
			poolID := uint64(101)

			r.add(tokenIDs, poolID)

			view := r.view()
			require.Len(t, view.Tokens, 2)
			require.Len(t, view.Pools, 1)
			require.Len(t, view.EdgeTargets, 2)

			// Check connections
			poolsForToken10 := testGetPoolsForToken(t, view, 10)
			poolsForToken20 := testGetPoolsForToken(t, view, 20)
			assert.Equal(t, []uint64{poolID}, poolsForToken10)
			assert.Equal(t, []uint64{poolID}, poolsForToken20)
		})

		t.Run("WithMultiplePoolsOnSameEdge", func(t *testing.T) {
			r := NewTokenPoolsRegistry(100)
			r.add([]uint64{10, 20}, 101)
			r.add([]uint64{10, 20}, 102) // Add a second pool to the same token pair

			view := r.view()
			poolsForToken10 := testGetPoolsForToken(t, view, 10)
			poolsForToken20 := testGetPoolsForToken(t, view, 20)

			// Both tokens should be associated with both pools
			assert.Equal(t, []uint64{101, 102}, poolsForToken10)
			assert.Equal(t, []uint64{101, 102}, poolsForToken20)
			// Ensure no duplicate edges were created
			assert.Len(t, view.EdgeTargets, 2, "Should still only have two edges for the pair")
			require.Len(t, view.EdgePools, 2, "Should still only have two edges")
			assert.Len(t, view.EdgePools[0], 2, "Edge should list two pools")
		})

		t.Run("IsIdempotent", func(t *testing.T) {
			r := NewTokenPoolsRegistry(100)
			r.add([]uint64{10, 20}, 101)
			view1 := r.view()

			// Add the same pool again
			r.add([]uint64{10, 20}, 101)
			view2 := r.view()

			assert.Equal(t, view1, view2, "adding the same pool relationship should be a no-op")
		})
	})

	t.Run("RemovePool", func(t *testing.T) {
		r := NewTokenPoolsRegistry(100)
		r.add([]uint64{10, 20}, 101)
		r.add([]uint64{10, 30}, 102)
		r.add([]uint64{10, 20}, 103) // Add a second pool on the 10-20 edge

		r.removePool(101)

		view := r.view()
		poolsForToken10 := testGetPoolsForToken(t, view, 10)
		poolsForToken20 := testGetPoolsForToken(t, view, 20)
		poolsForToken30 := testGetPoolsForToken(t, view, 30)

		assert.Equal(t, []uint64{102, 103}, poolsForToken10, "token 10 should be in pools 102 and 103")
		assert.Equal(t, []uint64{103}, poolsForToken20, "token 20 should only be in pool 103")
		assert.Equal(t, []uint64{102}, poolsForToken30, "token 30 should still be in pool 102")
		assert.Equal(t, 0, r.danglingEdgeCount, "no edges should be dangling yet")

		// Now remove the last pool on the 10-20 edge
		r.removePool(103)
		assert.Equal(t, 2, r.danglingEdgeCount, "should have 2 dangling edges after last pool removed")
		poolsForToken20_v2 := testGetPoolsForToken(t, r.view(), 20)
		assert.Nil(t, poolsForToken20_v2, "token 20 should have no pools")
	})

	t.Run("RemoveToken", func(t *testing.T) {
		r := NewTokenPoolsRegistry(100)
		r.add([]uint64{10, 20}, 101)
		r.add([]uint64{10, 30}, 102)
		r.add([]uint64{20, 30}, 103)

		r.removeToken(10) // Should remove edges for (10,20) and (10,30)

		view := r.view()
		poolsForToken10 := testGetPoolsForToken(t, view, 10)
		poolsForToken20 := testGetPoolsForToken(t, view, 20)
		poolsForToken30 := testGetPoolsForToken(t, view, 30)

		assert.Nil(t, poolsForToken10, "token 10 should have no pools")
		assert.Equal(t, []uint64{103}, poolsForToken20, "token 20 should only be in pool 103")
		assert.Equal(t, []uint64{103}, poolsForToken30, "token 30 should only be in pool 103")
		assert.Equal(t, 4, r.danglingEdgeCount, "should have 4 dangling edges from token 10 removal")
	})

	t.Run("Compaction", func(t *testing.T) {
		// Use a low threshold to reliably trigger compaction
		r := NewTokenPoolsRegistry(3)
		r.add([]uint64{10, 20}, 101) // 2 edges
		r.add([]uint64{30, 40}, 102) // 2 edges
		r.add([]uint64{50, 60}, 103) // 2 edges

		// This will mark 2 edges as dangling, count=2. No compaction yet.
		r.removePool(101)
		assert.Equal(t, 2, r.danglingEdgeCount)

		// This will mark 2 more edges as dangling, count=4. Compaction should trigger.
		r.removePool(102)

		// Assertions after compaction
		assert.Equal(t, 0, r.danglingEdgeCount, "dangling edge count should be reset to 0 after compaction")

		view := r.view()
		// Only tokens 50 and 60 should remain, connected by pool 103
		require.Len(t, view.Tokens, 2)
		require.Len(t, view.Pools, 1)
		require.Len(t, view.EdgeTargets, 2)

		assert.ElementsMatch(t, []uint64{50, 60}, view.Tokens)
		assert.Contains(t, view.Pools, uint64(103))

		poolsForToken50 := testGetPoolsForToken(t, view, 50)
		assert.Equal(t, []uint64{103}, poolsForToken50)
	})

	// --- NEW TEST BLOCK ---
	t.Run("poolsForToken", func(t *testing.T) {
		// Setup a registry with a reasonably complex state
		r := NewTokenPoolsRegistry(100)
		r.add([]uint64{10, 20}, 101) // Pool 101 has tokens 10, 20
		r.add([]uint64{10, 30}, 102) // Pool 102 has tokens 10, 30
		r.add([]uint64{10, 20}, 103) // Pool 103 also has tokens 10, 20

		t.Run("ReturnsCorrectPoolsForExistingTokens", func(t *testing.T) {
			// Expected results:
			// Token 10 is in pools 101, 102, 103
			// Token 20 is in pools 101, 103
			// Token 30 is in pool 102
			expectedPools := map[uint64][]uint64{
				10: {101, 102, 103},
				20: {101, 103},
				30: {102},
			}

			for tokenID, expected := range expectedPools {
				actual := r.poolsForToken(tokenID)
				// The method doesn't guarantee order, so we sort for stable comparison
				sort.Slice(actual, func(i, j int) bool { return actual[i] < actual[j] })
				assert.Equal(t, expected, actual, "pools for token %d should be correct", tokenID)
			}
		})

		t.Run("ReturnsNilForTokenWithNoLivePools", func(t *testing.T) {
			// Setup: Add a token and pool, then remove the pool
			r := NewTokenPoolsRegistry(100)
			r.add([]uint64{40, 50}, 201)
			r.removePool(201) // Token 40 and 50 still exist, but their edge is dangling

			assert.Nil(t, r.poolsForToken(40), "should return nil for a token whose pools have all been removed")
			assert.Nil(t, r.poolsForToken(50), "should return nil for a token whose pools have all been removed")
		})

		t.Run("ReturnsNilForNonExistentToken", func(t *testing.T) {
			assert.Nil(t, r.poolsForToken(9999), "should return nil for a token that was never added")
		})
	})
	// --- END NEW TEST BLOCK ---

	t.Run("View_ReturnsDeepCopy", func(t *testing.T) {
		r := NewTokenPoolsRegistry(100)
		r.add([]uint64{10, 20}, 101)

		view1 := r.view()
		require.Len(t, view1.Tokens, 2)
		require.NotEmpty(t, view1.EdgePools)
		require.NotEmpty(t, view1.EdgePools[0])

		// Store original values before tampering
		originalTokenID := view1.Tokens[0]
		originalPoolIndex := view1.EdgePools[0][0]

		// Maliciously modify the returned view's slices
		view1.Tokens[0] = 9999
		view1.EdgePools[0][0] = 8888

		// Get a new view and check if the original registry was affected
		view2 := r.view()
		require.Len(t, view2.Tokens, 2)
		assert.Equal(t, originalTokenID, view2.Tokens[0], "modifying a view's Tokens slice should not affect the internal registry data")

		// Check the nested slice to confirm it was also deep-copied
		require.NotEmpty(t, view2.EdgePools)
		require.NotEmpty(t, view2.EdgePools[0])
		assert.Equal(t, originalPoolIndex, view2.EdgePools[0][0], "modifying a view's nested EdgePools slice should not affect internal data")
	})
}

func TestNewTokenPoolsRegistryFromView(t *testing.T) {
	t.Parallel()

	t.Run("SuccessWithValidView", func(t *testing.T) {
		t.Parallel()
		// 1. Setup: Create a realistic view representing a small graph.
		// Token 10 is connected to Token 20 via Pool 100.
		originalView := &TokenPoolsRegistryView{
			Tokens:      []uint64{10, 20},
			Pools:       []uint64{100},
			Adjacency:   [][]int{{0}, nil}, // Token 10 (index 0) has one edge (edge 0)
			EdgeTargets: []int{1},          // Edge 0 targets Token 20 (index 1)
			EdgePools:   [][]int{{0}},      // Edge 0 is associated with Pool 100 (index 0)
		}

		// 2. Act: Create the registry from the view.
		registry := NewTokenPoolsRegistryFromView(originalView, 500)
		require.NotNil(t, registry)

		// 3. Assert: Verify that all internal data structures were correctly reconstructed.
		// Check lookup maps
		assert.Equal(t, 0, registry.tokenToIndex[10])
		assert.Equal(t, 1, registry.tokenToIndex[20])
		assert.Equal(t, 0, registry.poolToIndex[100])
		assert.Len(t, registry.tokenToIndex, 2)
		assert.Len(t, registry.poolToIndex, 1)

		// Check data slices (ensure they are equal in value)
		assert.Equal(t, originalView.Tokens, registry.tokens)
		assert.Equal(t, originalView.Pools, registry.pools)
		assert.Equal(t, originalView.Adjacency, registry.adjacency)
		assert.Equal(t, originalView.EdgeTargets, registry.edgeTargets)
		assert.Equal(t, originalView.EdgePools, registry.edgePools)

		// Check other fields
		assert.Equal(t, 500, registry.compactionThreshold)
		assert.Equal(t, 0, registry.danglingEdgeCount, "A restored view should have no dangling edges")
	})

	t.Run("DeepCopyVerification", func(t *testing.T) {
		t.Parallel()
		// 1. Setup: Create a simple view.
		originalView := &TokenPoolsRegistryView{
			Tokens:    []uint64{10},
			Pools:     []uint64{100},
			Adjacency: [][]int{{}},
			EdgePools: [][]int{{0}},
		}

		// 2. Act: Create the registry.
		registry := NewTokenPoolsRegistryFromView(originalView, 1000)
		require.NotNil(t, registry)

		// 3. Act: Maliciously modify the original view's slices after creation.
		originalView.Tokens[0] = 9999
		originalView.Adjacency[0] = append(originalView.Adjacency[0], 123)
		originalView.EdgePools[0][0] = 777

		// 4. Assert: The internal data of the registry should remain unaffected,
		// proving that a deep copy was made.
		assert.Equal(t, uint64(10), registry.tokens[0], "Modifying the original Tokens slice should not affect the registry")
		assert.Empty(t, registry.adjacency[0], "Modifying the original Adjacency slice should not affect the registry")
		assert.Equal(t, 0, registry.edgePools[0][0], "Modifying a nested slice in the original EdgePools should not affect the registry")
	})

	t.Run("SuccessWithEmptyView", func(t *testing.T) {
		t.Parallel()
		// 1. Setup: Create an empty view.
		emptyView := &TokenPoolsRegistryView{
			Tokens:      []uint64{},
			Pools:       []uint64{},
			Adjacency:   [][]int{},
			EdgeTargets: []int{},
			EdgePools:   [][]int{},
		}

		// 2. Act: Create a registry from the empty view.
		registry := NewTokenPoolsRegistryFromView(emptyView, 100)
		require.NotNil(t, registry)

		// 3. Assert: All internal structures should be initialized but empty.
		assert.Empty(t, registry.tokens)
		assert.Empty(t, registry.pools)
		assert.Empty(t, registry.adjacency)
		assert.Empty(t, registry.edgeTargets)
		assert.Empty(t, registry.edgePools)
		assert.Empty(t, registry.tokenToIndex)
		assert.Empty(t, registry.poolToIndex)
		assert.Equal(t, 100, registry.compactionThreshold)
	})
}
