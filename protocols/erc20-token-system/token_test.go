package token

import (
	"math/rand"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Test Helpers ---

// addr creates a predictable common.Address for testing.
func addr(b byte) common.Address {
	var a common.Address
	a[0] = b
	return a
}

// newTestRegistry sets up a registry with some initial tokens for test isolation.
func newTestRegistry(t *testing.T) (*TokenRegistry, []uint64) {
	registry := NewTokenRegistry()
	ids := make([]uint64, 4)
	var err error

	ids[0], err = addToken(addr(1), "Token A", "TKA", 18, registry)
	require.NoError(t, err)
	ids[1], err = addToken(addr(2), "Token B", "TKB", 18, registry)
	require.NoError(t, err)
	ids[2], err = addToken(addr(3), "Token C", "TKC", 18, registry)
	require.NoError(t, err)
	ids[3], err = addToken(addr(4), "Token D", "TKD", 18, registry)
	require.NoError(t, err)

	return registry, ids
}

// --- Unit Tests ---

func TestAddToken(t *testing.T) {
	t.Parallel()
	registry := NewTokenRegistry()
	addressA := addr(1)

	id, err := addToken(addressA, "Token A", "TKA", 18, registry)
	assert.NoError(t, err)
	assert.Equal(t, uint64(1), id)
	assert.Equal(t, 1, len(registry.address))

	_, err = addToken(addressA, "Token A", "TKA", 18, registry)
	assert.ErrorIs(t, err, ErrAlreadyExists, "should not allow adding duplicate address")
}

// TestDeleteToken uses a table-driven approach for comprehensive case coverage.
func TestDeleteToken(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name        string
		idToDelete  uint64
		expectedLen int
		expectedErr error
		postCheck   func(t *testing.T, r *TokenRegistry, originalIDs []uint64) // Optional check on the final state
	}{
		{
			name:        "Delete from middle",
			idToDelete:  2, // Token B
			expectedLen: 3,
			expectedErr: nil,
			postCheck: func(t *testing.T, r *TokenRegistry, originalIDs []uint64) {
				// The last element (Token D, ID 4) should now be at the deleted index (1)
				view, err := getTokenByID(originalIDs[3], r) // ID 4
				require.NoError(t, err)
				assert.Equal(t, "Token D", view.Name)
				assert.Equal(t, 1, r.idToIndex[originalIDs[3]])
			},
		},
		{
			name:        "Delete last element",
			idToDelete:  4, // Token D
			expectedLen: 3,
			expectedErr: nil,
			postCheck: func(t *testing.T, r *TokenRegistry, originalIDs []uint64) {
				// The third element (Token C, ID 3) should still be at index 2
				view, err := getTokenByID(originalIDs[2], r) // ID 3
				require.NoError(t, err)
				assert.Equal(t, "Token C", view.Name)
				assert.Equal(t, 2, r.idToIndex[originalIDs[2]])
			},
		},
		{
			name:        "Delete first element",
			idToDelete:  1, // Token A
			expectedLen: 3,
			expectedErr: nil,
		},
		{
			name:        "Delete non-existent element",
			idToDelete:  999,
			expectedLen: 4,
			expectedErr: ErrTokenNotFound,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			registry, ids := newTestRegistry(t)
			err := deleteToken(tc.idToDelete, registry)

			if tc.expectedErr != nil {
				assert.ErrorIs(t, err, tc.expectedErr)
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tc.expectedLen, len(registry.address))
			if tc.postCheck != nil {
				tc.postCheck(t, registry, ids)
			}
		})
	}
}

func TestUpdateToken(t *testing.T) {
	t.Parallel()
	registry, ids := newTestRegistry(t)
	idToUpdate := ids[0] // Token A

	err := updateToken(idToUpdate, 5.5, 21000, registry)
	require.NoError(t, err)

	view, err := getTokenByID(idToUpdate, registry)
	require.NoError(t, err)
	assert.Equal(t, 5.5, view.FeeOnTransferPercent)
	assert.Equal(t, uint64(21000), view.GasForTransfer)

	err = updateToken(999, 1.0, 100, registry)
	assert.ErrorIs(t, err, ErrTokenNotFound, "should return an error for a non-existent token")
}

func TestNewTokenRegistryFromViews(t *testing.T) {
	t.Parallel()
	t.Run("SuccessWithValidView", func(t *testing.T) {
		// Use non-sequential IDs to test the maxID logic
		views := []TokenView{
			{ID: 1, Address: addr(1), Name: "Token A"},
			{ID: 5, Address: addr(5), Name: "Token E"},
			{ID: 3, Address: addr(3), Name: "Token C"},
		}

		registry, err := NewTokenRegistryFromViews(views)
		require.NoError(t, err)
		require.NotNil(t, registry)

		assert.Len(t, registry.address, 3)
		assert.Len(t, registry.idToIndex, 3)
		assert.Len(t, registry.addressToID, 3)

		// Check that nextID is correctly set to maxID + 1
		assert.Equal(t, uint64(6), registry.nextID)

		// Verify internal mapping is correct
		viewE, err := getTokenByID(5, registry)
		require.NoError(t, err)
		assert.Equal(t, "Token E", viewE.Name)
	})

	t.Run("SuccessWithEmptyView", func(t *testing.T) {
		registry, err := NewTokenRegistryFromViews([]TokenView{})
		require.NoError(t, err)
		require.NotNil(t, registry)

		assert.Empty(t, registry.address)
		assert.Empty(t, registry.idToIndex)
		assert.Equal(t, uint64(1), registry.nextID)
	})

	t.Run("FailureOnDuplicateID", func(t *testing.T) {
		views := []TokenView{
			{ID: 1, Address: addr(1)},
			{ID: 2, Address: addr(2)},
			{ID: 1, Address: addr(3)}, // Duplicate ID
		}

		_, err := NewTokenRegistryFromViews(views)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrDuplicateID)
	})

	t.Run("FailureOnDuplicateAddress", func(t *testing.T) {
		views := []TokenView{
			{ID: 1, Address: addr(1)},
			{ID: 2, Address: addr(2)},
			{ID: 3, Address: addr(1)}, // Duplicate Address
		}

		_, err := NewTokenRegistryFromViews(views)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrDuplicateAddress)
	})
}

func TestGetters(t *testing.T) {
	t.Parallel()
	registry, ids := newTestRegistry(t)
	idA, idB := ids[0], ids[1]
	addrA, addrB := addr(1), addr(2)

	t.Run("Getters before deletion", func(t *testing.T) {
		viewA, err := getTokenByID(idA, registry)
		require.NoError(t, err)
		assert.Equal(t, idA, viewA.ID)
		assert.Equal(t, "Token A", viewA.Name)

		viewB, err := getTokenByAddress(addrB, registry)
		require.NoError(t, err)
		assert.Equal(t, idB, viewB.ID)
		assert.Equal(t, "Token B", viewB.Name)

		_, err = getTokenByID(999, registry)
		assert.ErrorIs(t, err, ErrTokenNotFound)

		_, err = getTokenByAddress(addr(99), registry)
		assert.ErrorIs(t, err, ErrTokenNotFound)
	})

	// Delete a token to test getter behavior after state changes
	err := deleteToken(idA, registry)
	require.NoError(t, err)

	t.Run("Getters after deletion", func(t *testing.T) {
		// The deleted token should no longer be retrievable by ID or Address
		_, err := getTokenByID(idA, registry)
		assert.ErrorIs(t, err, ErrTokenNotFound)

		_, err = getTokenByAddress(addrA, registry)
		assert.ErrorIs(t, err, ErrTokenNotFound)

		// The other token should still be present and correct
		viewB, err := getTokenByID(idB, registry)
		require.NoError(t, err)
		assert.Equal(t, "Token B", viewB.Name)
	})
}
func TestViewRegistry(t *testing.T) {
	t.Parallel()
	registry, ids := newTestRegistry(t)

	err := deleteToken(ids[1], registry) // Delete Token B
	require.NoError(t, err)

	views := viewRegistry(registry)
	assert.Len(t, views, 3, "view should contain exactly 3 tokens")

	viewNames := make(map[string]bool)
	for _, v := range views {
		viewNames[v.Name] = true
	}
	assert.True(t, viewNames["Token A"])
	assert.True(t, viewNames["Token C"])
	assert.True(t, viewNames["Token D"])
	assert.False(t, viewNames["Token B"], "deleted token should not be in the view")
}

// --- Fuzz Testing ---

// FuzzDeleteToken performs property-based testing on the add/delete logic.
// It ensures that no matter the sequence of valid operations, the registry never panics.
func FuzzDeleteToken(f *testing.F) {
	f.Add(uint64(10), uint64(5))
	f.Add(uint64(1), uint64(1))
	f.Add(uint64(100), uint64(100))

	f.Fuzz(func(t *testing.T, numAdds uint64, numDeletes uint64) {
		if numAdds > 200 {
			numAdds = 200 // Constrain inputs to avoid extremely long tests
		}
		registry := NewTokenRegistry()
		addedIDs := make([]uint64, 0, numAdds)

		for i := uint64(0); i < numAdds; i++ {
			id, err := addToken(addr(byte(i)), "fuzz", "FUZZ", 18, registry)
			if err == nil {
				addedIDs = append(addedIDs, id)
			}
		}

		if numDeletes > uint64(len(addedIDs)) {
			numDeletes = uint64(len(addedIDs))
		}
		rand.Shuffle(len(addedIDs), func(i, j int) {
			addedIDs[i], addedIDs[j] = addedIDs[j], addedIDs[i]
		})

		for i := uint64(0); i < numDeletes; i++ {
			deleteToken(addedIDs[i], registry)
		}

		finalLen := len(registry.address)
		expectedLen := len(addedIDs) - int(numDeletes)
		assert.Equal(t, expectedLen, finalLen, "final length should match adds minus deletes")
	})
}

// --- Benchmarking ---

// To run benchmarks: go test -bench=.

func BenchmarkAddToken(b *testing.B) {
	for i := 0; i < b.N; i++ {
		registry := NewTokenRegistry()
		// Benchmark the time it takes to add 1000 tokens
		for j := 0; j < 1000; j++ {
			addToken(addr(byte(j)), "bench", "B", 18, registry)
		}
	}
}

func BenchmarkDeleteToken(b *testing.B) {
	for i := 0; i < b.N; i++ {
		b.StopTimer() // Pause timer while we set up
		registry := NewTokenRegistry()
		ids := make([]uint64, 1000)
		for j := 0; j < 1000; j++ {
			id, _ := addToken(addr(byte(j)), "bench", "B", 18, registry)
			ids[j] = id
		}
		b.StartTimer() // Resume timer for the actual operation

		// Benchmark the time it takes to delete all 1000 tokens
		for j := 0; j < 1000; j++ {
			deleteToken(ids[j], registry)
		}
	}
}

func BenchmarkGetTokenByID(b *testing.B) {
	registry := NewTokenRegistry()
	for j := 0; j < 10000; j++ { // Setup a large registry
		// Use a unique address for each token to avoid ErrAlreadyExists
		id, _ := addToken(addr(byte(j%256)), "bench", "B", 18, registry)
		// To ensure we have a token with ID 5000 to test against:
		if id == 5000 {
			registry.id[j] = 5000
			registry.idToIndex[5000] = j
		}
	}
	// Reset timer to exclude setup time
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Benchmark lookup speed by repeatedly getting the middle element
		getTokenByID(5000, registry)
	}
}
