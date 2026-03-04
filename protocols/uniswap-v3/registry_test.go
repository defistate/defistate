package uniswapv3

import (
	"math/big"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUniswapV3Registry provides a comprehensive test suite for the registry functions.
func TestUniswapV3Registry(t *testing.T) {
	// --- Test Fixtures ---
	poolID1, token0ID1, token1ID1, fee1, tickSpacing1 := uint64(101), uint64(1), uint64(2), uint64(3000), uint64(60)
	poolID2, token0ID2, token1ID2, fee2, tickSpacing2 := uint64(102), uint64(3), uint64(4), uint64(500), uint64(10)
	poolID3, token0ID3, token1ID3, fee3, tickSpacing3 := uint64(103), uint64(5), uint64(6), uint64(10000), uint64(200)

	// Test NewUniswapV3Registry
	t.Run("NewUniswapV3Registry", func(t *testing.T) {
		registry := NewUniswapV3Registry()
		assert.NotNil(t, registry, "Registry should not be nil")
		assert.Empty(t, registry.id, "ID slice should be empty")
		assert.NotNil(t, registry.idToIndex, "idToIndex map should be initialized")
		assert.Empty(t, registry.idToIndex, "idToIndex map should be empty")
	})

	// Test Add, Update, and GetPool
	t.Run("Add_Update_And_GetPool", func(t *testing.T) {
		registry := NewUniswapV3Registry()

		// Add first pool
		err := addPool(poolID1, token0ID1, token1ID1, fee1, tickSpacing1, registry)
		assert.NoError(t, err, "Should not error when adding a new pool")
		assert.Len(t, registry.id, 1, "Should have one pool after adding")
		assert.Equal(t, 0, registry.idToIndex[poolID1], "Index for pool 1 should be 0")

		// Verify internal lastUpdatedAt is initialized to 0
		lastUp, err := getLastUpdatedAt(poolID1, registry)
		assert.NoError(t, err)
		assert.Equal(t, uint64(0), lastUp, "Initial lastUpdatedAt should be 0")

		// Get the added pool
		view, err := getPoolById(poolID1, registry)
		assert.NoError(t, err, "Should not error when getting an existing pool")
		assert.Equal(t, poolID1, view.ID)
		assert.Equal(t, token0ID1, view.Token0)
		assert.Equal(t, fee1, view.Fee)
		assert.Equal(t, int64(0), view.Tick, "Initial tick should be 0")
		assert.Equal(t, big.NewInt(0), view.Liquidity, "Initial liquidity should be 0")

		// Try to add the same pool again
		err = addPool(poolID1, token0ID1, token1ID1, fee1, tickSpacing1, registry)
		assert.ErrorIs(t, err, ErrPoolExists, "Should error when adding a duplicate pool")

		// Update the pool
		newFee := uint64(50000)
		newTick := int64(12345)
		newLiquidity := big.NewInt(1e18)
		newSqrtPriceX96 := new(big.Int).Lsh(big.NewInt(1), 96) // 1 << 96
		err = updatePool(poolID1, newFee, newTick, newLiquidity, newSqrtPriceX96, registry)
		assert.NoError(t, err, "Should not error when updating an existing pool")

		// Verify update
		view, _ = getPoolById(poolID1, registry)
		assert.Equal(t, newFee, view.Fee, "Fee should be updated")
		assert.Equal(t, newTick, view.Tick, "Tick should be updated")
		assert.Equal(t, newLiquidity, view.Liquidity, "Liquidity should be updated")
		assert.Equal(t, newSqrtPriceX96, view.SqrtPriceX96, "SqrtPriceX96 should be updated")

		// Test error cases for non-existent pools
		_, err = getPoolById(999, registry)
		assert.ErrorIs(t, err, ErrPoolNotFound, "Should error when getting a non-existent pool")
		err = updatePool(999, newFee, newTick, newLiquidity, newSqrtPriceX96, registry)
		assert.ErrorIs(t, err, ErrPoolNotFound, "Should error when updating a non-existent pool")
	})

	// Test Deletion logic
	t.Run("DeletePool", func(t *testing.T) {
		registry := NewUniswapV3Registry()
		addPool(poolID1, token0ID1, token1ID1, fee1, tickSpacing1, registry)
		addPool(poolID2, token0ID2, token1ID2, fee2, tickSpacing2, registry)
		addPool(poolID3, token0ID3, token1ID3, fee3, tickSpacing3, registry)
		assert.Len(t, registry.id, 3, "Registry should have 3 pools initially")

		// Set distinct lastUpdatedAt values to verify swap-and-pop integrity
		require.NoError(t, setLastUpdatedAt(poolID1, 1000, registry))
		require.NoError(t, setLastUpdatedAt(poolID2, 2000, registry))
		require.NoError(t, setLastUpdatedAt(poolID3, 3000, registry))

		// 1. Delete a pool from the middle (poolID2 at index 1)
		// Expect poolID3 (at index 2) to be swapped into its place.
		err := deletePool(poolID2, registry)
		assert.NoError(t, err, "Should not error when deleting a pool from the middle")
		assert.Len(t, registry.id, 2, "Registry size should be 2 after deletion")

		// Check that the deleted pool ID is gone from the map
		_, exists := registry.idToIndex[poolID2]
		assert.False(t, exists, "Deleted pool ID should be removed from map")

		// Check that the swapped pool (poolID3) has its index updated
		assert.Equal(t, 1, registry.idToIndex[poolID3], "Swapped pool's index should be updated")

		// Check that the data at the old index now belongs to the swapped pool
		view, _ := getPoolById(poolID3, registry)
		assert.Equal(t, token1ID3, view.Token1)

		// Check that lastUpdatedAt was correctly swapped
		swappedLastUp, _ := getLastUpdatedAt(poolID3, registry)
		assert.Equal(t, uint64(3000), swappedLastUp, "Swapped pool should retain its correct lastUpdatedAt value")

		// 2. Delete a pool from the end (now poolID1 is at the end)
		// To make it interesting, let's delete poolID1 (at index 0)
		// This will swap poolID3 (at index 1) into index 0.
		err = deletePool(poolID1, registry)
		assert.NoError(t, err, "Should not error when deleting a pool")
		assert.Len(t, registry.id, 1, "Registry size should be 1")
		_, exists = registry.idToIndex[poolID1]
		assert.False(t, exists)
		assert.Equal(t, 0, registry.idToIndex[poolID3], "Pool 3 should now be at index 0")

		// 3. Delete the last pool
		err = deletePool(poolID3, registry)
		assert.NoError(t, err)
		assert.Empty(t, registry.id, "Registry should be empty after deleting all pools")
		assert.Empty(t, registry.idToIndex, "Map should be empty after deleting all pools")

		// 4. Test error on deleting non-existent pool
		err = deletePool(999, registry)
		assert.ErrorIs(t, err, ErrPoolNotFound, "Should error when deleting non-existent pool")
	})

	// Test the new lastUpdatedAt helpers
	t.Run("LastUpdatedAt_Helpers", func(t *testing.T) {
		registry := NewUniswapV3Registry()
		addPool(poolID1, token0ID1, token1ID1, fee1, tickSpacing1, registry)
		addPool(poolID2, token0ID2, token1ID2, fee2, tickSpacing2, registry)

		// 1. Initial state check
		ts1, err := getLastUpdatedAt(poolID1, registry)
		require.NoError(t, err)
		assert.Equal(t, uint64(0), ts1, "Should initialize to 0")

		// 2. Set values
		require.NoError(t, setLastUpdatedAt(poolID1, 12345, registry))
		require.NoError(t, setLastUpdatedAt(poolID2, 67890, registry))

		// 3. Get values individually
		ts1, _ = getLastUpdatedAt(poolID1, registry)
		assert.Equal(t, uint64(12345), ts1)

		// 4. Test Map generation
		timeMap := getLastUpdatedAtMap(registry)
		require.NotNil(t, timeMap)
		require.Len(t, timeMap, 2)
		assert.Equal(t, uint64(12345), timeMap[poolID1])
		assert.Equal(t, uint64(67890), timeMap[poolID2])

		// 5. Test Error cases
		_, err = getLastUpdatedAt(999, registry)
		assert.ErrorIs(t, err, ErrPoolNotFound)

		err = setLastUpdatedAt(999, 111, registry)
		assert.ErrorIs(t, err, ErrPoolNotFound)

		// 6. Test Map generation on empty registry
		emptyReg := NewUniswapV3Registry()
		assert.Nil(t, getLastUpdatedAtMap(emptyReg))
	})

	// Test ViewRegistry
	t.Run("ViewRegistry", func(t *testing.T) {
		registry := NewUniswapV3Registry()

		// Test empty view
		views := viewRegistry(registry)
		assert.Nil(t, views, "View of an empty registry should be nil")

		// Add pools
		addPool(poolID1, token0ID1, token1ID1, fee1, tickSpacing1, registry)
		addPool(poolID2, token0ID2, token1ID2, fee2, tickSpacing2, registry)

		views = viewRegistry(registry)
		assert.Len(t, views, 2, "View should contain 2 pools")

		// Check that the views contain the correct data.
		// Note: The order isn't guaranteed, so we check for existence.
		viewMap := make(map[uint64]PoolViewMinimal)
		for _, v := range views {
			viewMap[v.ID] = v
		}
		assert.Equal(t, token0ID1, viewMap[poolID1].Token0)
		assert.Equal(t, fee2, viewMap[poolID2].Fee)
	})

	// Test that mutating a returned view does not affect the registry's internal state.
	t.Run("ViewImmutability", func(t *testing.T) {
		registry := NewUniswapV3Registry()
		originalLiquidity := big.NewInt(5000)
		originalSqrtPrice := big.NewInt(9999)

		addPool(poolID1, token0ID1, token1ID1, fee1, tickSpacing1, registry)
		updatePool(poolID1, fee1, 1, originalLiquidity, originalSqrtPrice, registry)

		// Get the view for the first time
		mutableView, err := getPoolById(poolID1, registry)
		assert.NoError(t, err)

		// Mutate the fields of the retrieved view
		mutableView.Token0 = 999
		mutableView.Liquidity.SetInt64(0) // This is the crucial test for the pointer type

		// Get the view again to check if the underlying data has changed
		freshView, err := getPoolById(poolID1, registry)
		assert.NoError(t, err)

		// Assert that the data in the registry remains unchanged
		assert.Equal(t, token0ID1, freshView.Token0, "Modifying view's Token0 should not affect the registry")
		assert.Equal(t, originalLiquidity, freshView.Liquidity, "Modifying view's Liquidity pointer should not affect the registry")
		assert.NotEqual(t, mutableView.Token0, freshView.Token0, "The mutated view should differ from the fresh view")
		assert.NotEqual(t, mutableView.Liquidity, freshView.Liquidity, "The mutated view's liquidity should differ")
	})

	t.Run("HasPool", func(t *testing.T) {
		registry := NewUniswapV3Registry()
		require.NoError(t, addPool(poolID1, token0ID1, token1ID1, fee1, tickSpacing1, registry))

		// Check for an existing pool
		assert.True(t, hasPool(poolID1, registry), "hasPool should return true for an existing pool")

		// Check for a non-existent pool
		assert.False(t, hasPool(999, registry), "hasPool should return false for a non-existent pool")

		// Delete the pool
		require.NoError(t, deletePool(poolID1, registry))

		// Check for the deleted pool
		assert.False(t, hasPool(poolID1, registry), "hasPool should return false for a deleted pool")
	})
}

// TestNewUniswapV3RegistryFromViews provides a dedicated test suite for the "rehydration" constructor.
// It verifies that the registry can be correctly and safely initialized from a snapshot view.
func TestNewUniswapV3RegistryFromViews(t *testing.T) {
	t.Parallel()

	t.Run("SuccessWithValidView", func(t *testing.T) {
		// 1. Arrange: Create a source view representing a valid snapshot.
		sourceView := []PoolViewMinimal{
			{ID: 101, Token0: 1, Token1: 2, Fee: 3000, TickSpacing: 60, Tick: 190000, Liquidity: big.NewInt(1e18), SqrtPriceX96: big.NewInt(12345)},
			{ID: 102, Token0: 3, Token1: 4, Fee: 500, TickSpacing: 10, Tick: -50000, Liquidity: big.NewInt(2e18), SqrtPriceX96: big.NewInt(67890)},
		}

		// 2. Act: Create a new registry from the source view.
		registry := NewUniswapV3RegistryFromViews(sourceView)
		require.NotNil(t, registry)

		// 3. Assert: Verify the internal state is correctly reconstructed.
		rehydratedView := viewRegistry(registry)
		require.Len(t, rehydratedView, 2)

		// Since big.Int are pointers, we sort both slices by ID for a stable, element-by-element comparison.
		sort.Slice(sourceView, func(i, j int) bool { return sourceView[i].ID < sourceView[j].ID })
		sort.Slice(rehydratedView, func(i, j int) bool { return rehydratedView[i].ID < rehydratedView[j].ID })

		for i := range sourceView {
			expected := sourceView[i]
			actual := rehydratedView[i]
			assert.Equal(t, expected.ID, actual.ID)
			assert.Equal(t, expected.Token0, actual.Token0)
			assert.Equal(t, expected.Fee, actual.Fee)
			assert.Equal(t, expected.Tick, actual.Tick)
			assert.True(t, expected.Liquidity.Cmp(actual.Liquidity) == 0, "Liquidity mismatch for pool %d", expected.ID)
			assert.True(t, expected.SqrtPriceX96.Cmp(actual.SqrtPriceX96) == 0, "SqrtPriceX96 mismatch for pool %d", expected.ID)
		}

		// Also verify the lookup map is correctly built.
		assert.Equal(t, 0, registry.idToIndex[101])
		assert.Equal(t, 1, registry.idToIndex[102])

		// And verify that lastUpdatedAt correctly initialized to 0
		lastUp1, _ := getLastUpdatedAt(101, registry)
		lastUp2, _ := getLastUpdatedAt(102, registry)
		assert.Equal(t, uint64(0), lastUp1)
		assert.Equal(t, uint64(0), lastUp2)
	})

	t.Run("PerformsDeepCopyOfBigInts", func(t *testing.T) {
		// 1. Arrange: Create a view with big.Int pointers that we will mutate.
		originalLiquidity := big.NewInt(5000)
		originalSqrtPrice := big.NewInt(9999)
		sourceView := []PoolViewMinimal{
			{ID: 101, Liquidity: originalLiquidity, SqrtPriceX96: originalSqrtPrice},
		}

		// 2. Act: Create the registry from the view.
		registry := NewUniswapV3RegistryFromViews(sourceView)

		// 3. Act: Maliciously mutate the original big.Int objects *after* the constructor has returned.
		sourceView[0].Liquidity.SetInt64(0)
		sourceView[0].SqrtPriceX96.SetInt64(0)

		// 4. Assert: The data inside the registry should be unchanged, proving it holds a deep copy.
		internalView, err := getPoolById(101, registry)
		require.NoError(t, err)

		assert.Equal(t, 0, internalView.Liquidity.Cmp(big.NewInt(5000)), "Registry's Liquidity should be a copy and remain unchanged")
		assert.Equal(t, 0, internalView.SqrtPriceX96.Cmp(big.NewInt(9999)), "Registry's SqrtPriceX96 should be a copy and remain unchanged")
	})

	t.Run("HandlesEmptyAndNilViews", func(t *testing.T) {
		// Test with an empty slice
		registryEmpty := NewUniswapV3RegistryFromViews([]PoolViewMinimal{})
		require.NotNil(t, registryEmpty)
		assert.Empty(t, viewRegistry(registryEmpty))
		assert.Empty(t, registryEmpty.idToIndex)

		// Test with a nil slice
		registryNil := NewUniswapV3RegistryFromViews(nil)
		require.NotNil(t, registryNil)
		assert.Empty(t, viewRegistry(registryNil))
		assert.Empty(t, registryNil.idToIndex)
	})
}
