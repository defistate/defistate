package token

import (
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTokenSystem_Methods provides a basic sanity check of the TokenSystem's wrapper methods.
// It ensures that the mutex-protected operations correctly delegate to the underlying registry
// in a simple, sequential workflow.
func TestTokenSystem_Methods(t *testing.T) {
	t.Parallel()
	ts := NewTokenSystem()

	// State variables to be used across sub-tests
	var tokenID uint64
	tokenAddr := addr(1)
	var err error

	t.Run("Add Token", func(t *testing.T) {
		tokenID, err = ts.AddToken(tokenAddr, "Token A", "TKA", 18)
		require.NoError(t, err)
		assert.Equal(t, uint64(1), tokenID)

		// Test adding a duplicate
		_, err = ts.AddToken(tokenAddr, "Token A", "TKA", 18)
		assert.ErrorIs(t, err, ErrAlreadyExists)
	})

	// Ensure the ID was set before proceeding to tests that depend on it.
	require.NotZero(t, tokenID, "tokenID should have been set in the Add Token sub-test")

	t.Run("Get Token", func(t *testing.T) {
		view, err := ts.GetTokenByID(tokenID)
		require.NoError(t, err)
		assert.Equal(t, "Token A", view.Name)

		view, err = ts.GetTokenByAddress(tokenAddr)
		require.NoError(t, err)
		assert.Equal(t, "Token A", view.Name)
	})

	t.Run("Update Token", func(t *testing.T) {
		err = ts.UpdateToken(tokenID, 5.0, 25000)
		require.NoError(t, err)
		updatedView, err := ts.GetTokenByID(tokenID)
		require.NoError(t, err)
		assert.Equal(t, 5.0, updatedView.FeeOnTransferPercent)
		assert.Equal(t, uint64(25000), updatedView.GasForTransfer)
	})

	t.Run("View System", func(t *testing.T) {
		allViews := ts.View()
		require.Len(t, allViews, 1)
		assert.Equal(t, "Token A", allViews[0].Name)
	})

	t.Run("Delete Token", func(t *testing.T) {
		err = ts.DeleteToken(tokenID)
		require.NoError(t, err)

		// Verify deletion by checking getters and the main view
		_, err = ts.GetTokenByID(tokenID)
		assert.ErrorIs(t, err, ErrTokenNotFound)
		assert.Empty(t, ts.View())
	})
}

func TestNewTokenSystemFromViews(t *testing.T) {
	t.Parallel()
	t.Run("Success", func(t *testing.T) {
		views := []TokenView{
			{ID: 10, Address: addr(10), Name: "Token X"},
			{ID: 20, Address: addr(20), Name: "Token Y"},
		}

		ts, err := NewTokenSystemFromViews(views)
		require.NoError(t, err)
		require.NotNil(t, ts)

		// Verify the contents by using the concurrency-safe getter
		view, err := ts.GetTokenByID(20)
		require.NoError(t, err)
		assert.Equal(t, "Token Y", view.Name)
	})

	t.Run("FailureOnInvalidView", func(t *testing.T) {
		views := []TokenView{
			{ID: 1, Address: addr(1)},
			{ID: 1, Address: addr(2)}, // Duplicate ID
		}

		ts, err := NewTokenSystemFromViews(views)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrDuplicateID)
		assert.Nil(t, ts)
	})
}

// TestTokenSystem_StressConcurrent is a more chaotic and realistic concurrency test.
// It simulates a mixed workload of reads, adds, updates, and deletes.
// Run with `go test -race` to detect race conditions.
func TestTokenSystem_StressConcurrent(t *testing.T) {
	t.Parallel()
	ts := NewTokenSystem()
	wg := &sync.WaitGroup{}
	const initialTokens = 20
	const routines = 50
	const opsPerRoutine = 100

	// Setup an initial pool of tokens to create contention on updates/deletes
	initialIDs := make([]uint64, initialTokens)
	for i := 0; i < initialTokens; i++ {
		id, err := ts.AddToken(addr(byte(i)), "init", "INIT", 18)
		require.NoError(t, err)
		initialIDs[i] = id
	}

	// Launch many goroutines that perform a random mix of operations
	for i := 0; i < routines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := rand.New(rand.NewSource(time.Now().UnixNano()))
			for j := 0; j < opsPerRoutine; j++ {
				op := r.Intn(100) // Skew towards reads
				randomInitialID := initialIDs[r.Intn(len(initialIDs))]

				switch {
				case op < 70: // 70% chance of a Read operation
					_, _ = ts.GetTokenByID(randomInitialID)
					_ = ts.View()
				case op < 85: // 15% chance of an Update operation
					_ = ts.UpdateToken(randomInitialID, r.Float64()*10, 21000)
				case op < 95: // 10% chance of an Add operation
					newAddr := addr(byte(r.Intn(255)))
					// Ignore error, as duplicate adds are expected and fine
					_, _ = ts.AddToken(newAddr, "chaos", "CHS", 18)
				default: // 5% chance of a Delete operation
					// To avoid deleting all initial tokens, we only delete if we have > 10
					if len(ts.View()) > 10 {
						_ = ts.DeleteToken(randomInitialID)
					}
				}
			}
		}()
	}

	wg.Wait()
	// The primary assertion is that this test completes without panicking
	// and without the race detector finding any data races.
	assert.True(t, true, "test completed without race conditions")
}

// --- Benchmarking the Concurrent System ---

// To run benchmarks: go test -bench=.
func BenchmarkTokenSystem_Reads(b *testing.B) {
	ts := NewTokenSystem()
	id, _ := ts.AddToken(addr(1), "bench", "B", 18)

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := ts.GetTokenByID(id)
			if err != nil && err != ErrTokenNotFound { // Allow for not found if a delete happens
				b.Error(err)
			}
		}
	})
}

func BenchmarkTokenSystem_Writes(b *testing.B) {
	ts := NewTokenSystem()
	var counter uint64 // Use an atomic counter to generate unique addresses race-free

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			i := atomic.AddUint64(&counter, 1)
			addr := common.Address{}
			addr[0] = byte(i / 256)
			addr[1] = byte(i % 256)
			// Ignore error on purpose as some concurrent adds may collide, which is fine
			_, _ = ts.AddToken(addr, "bench", "B", 18)
		}
	})
}

func BenchmarkTokenSystem_Mixed(b *testing.B) {
	ts := NewTokenSystem()
	id, _ := ts.AddToken(addr(1), "bench", "B", 18)
	var counter uint64 // Use an atomic counter

	b.RunParallel(func(pb *testing.PB) {
		// Each goroutine gets its own random source to avoid lock contention on the global rand.
		r := rand.New(rand.NewSource(time.Now().UnixNano()))

		for pb.Next() {
			if r.Intn(10) == 0 { // 10% writes
				i := atomic.AddUint64(&counter, 1)
				addr := common.Address{}
				addr[0] = byte(i / 256)
				addr[1] = byte(i % 256)
				_, _ = ts.AddToken(addr, "bench", "B", 18)
			} else { // 90% reads
				_, _ = ts.GetTokenByID(id)
			}
		}
	})
}
