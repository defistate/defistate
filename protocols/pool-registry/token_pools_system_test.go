package poolregistry

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenPoolSystem(t *testing.T) {

	t.Run("API_Correctness_AddAndRemove", func(t *testing.T) {
		s := NewTokenPoolSystem(1000)

		// Add a pool
		s.AddPool([]uint64{10, 20}, 101)
		// Add another pool with a shared token
		s.AddPool([]uint64{10, 30}, 102)
		// Add a third pool that uses the same tokens as the first, creating a multi-pool edge.
		s.AddPool([]uint64{10, 20}, 103)

		// --- Verify initial state ---
		// Token 10 is in all three pools
		assert.ElementsMatch(t, []uint64{101, 102, 103}, s.PoolsForToken(10))
		// Token 20 is connected via the multi-pool edge
		assert.ElementsMatch(t, []uint64{101, 103}, s.PoolsForToken(20))
		// Token 30 is in its own pool with token 10
		assert.ElementsMatch(t, []uint64{102}, s.PoolsForToken(30))

		// --- Remove the first pool ---
		s.RemovePool(101)
		// Token 10 should now be in the remaining two pools
		assert.ElementsMatch(t, []uint64{102, 103}, s.PoolsForToken(10))
		// Token 20 should only be in pool 103 now
		assert.ElementsMatch(t, []uint64{103}, s.PoolsForToken(20))

		// --- Remove a token entirely ---
		s.RemoveToken(10)
		assert.Nil(t, s.PoolsForToken(10), "Token 10 should be completely gone")
		// Removing token 10 breaks the edges that supported pools 102 and 103.
		assert.Nil(t, s.PoolsForToken(20), "Token 20 should have no pools after token 10 is removed")
		assert.Nil(t, s.PoolsForToken(30), "Token 30 should have no pools after token 10 is removed")
	})

	t.Run("API_Correctness_BatchOperations", func(t *testing.T) {
		s := NewTokenPoolSystem(1000)

		// Batch add pools
		poolIDsToAdd := []uint64{101, 102, 103}
		tokenIDSetsToAdd := [][]uint64{{10, 20}, {10, 30}, {10, 20}}
		s.AddPools(poolIDsToAdd, tokenIDSetsToAdd)

		// Verify initial state
		assert.ElementsMatch(t, []uint64{101, 102, 103}, s.PoolsForToken(10))
		assert.ElementsMatch(t, []uint64{101, 103}, s.PoolsForToken(20))

		// Batch remove pools
		s.RemovePools([]uint64{101, 102})
		assert.ElementsMatch(t, []uint64{103}, s.PoolsForToken(10))
		assert.ElementsMatch(t, []uint64{103}, s.PoolsForToken(20))
		assert.Nil(t, s.PoolsForToken(30))

		// Batch remove tokens
		s.AddPools([]uint64{201, 202}, [][]uint64{{40, 50}, {40, 60}})
		s.RemoveTokens([]uint64{10, 40}) // remove token 10 (and its pool 103) and token 40 (and its pools)
		assert.Nil(t, s.PoolsForToken(10))
		assert.Nil(t, s.PoolsForToken(20))
		assert.Nil(t, s.PoolsForToken(40))
		assert.Nil(t, s.PoolsForToken(50))

		// Test that AddPools panics on mismatched lengths
		assert.Panics(t, func() {
			s.AddPools([]uint64{1}, [][]uint64{{1, 2}, {3, 4}})
		}, "AddPools should panic if slice lengths are mismatched")
	})

	t.Run("PoolsForToken_Correctness", func(t *testing.T) {
		s := NewTokenPoolSystem(1000)
		s.AddPool([]uint64{10, 20}, 101)
		s.AddPool([]uint64{10, 30}, 102)
		s.AddPool([]uint64{10, 20}, 103)

		// Verify initial state using the direct method call
		assert.ElementsMatch(t, []uint64{101, 102, 103}, s.PoolsForToken(10))
		assert.ElementsMatch(t, []uint64{101, 103}, s.PoolsForToken(20))
		assert.ElementsMatch(t, []uint64{102}, s.PoolsForToken(30))
		assert.Nil(t, s.PoolsForToken(999), "should be nil for non-existent token")

		// Verify state after a removal
		s.RemovePool(101)
		assert.ElementsMatch(t, []uint64{102, 103}, s.PoolsForToken(10))
		assert.ElementsMatch(t, []uint64{103}, s.PoolsForToken(20))
	})

	t.Run("View_IsLockFreeAndReturnsCopy", func(t *testing.T) {
		s := NewTokenPoolSystem(1000)
		s.AddPool([]uint64{10, 20}, 101)

		view1 := s.View()
		require.NotNil(t, view1)
		require.Len(t, view1.Tokens, 2)
		require.NotEmpty(t, view1.EdgePools, "EdgePools should not be empty")
		require.NotEmpty(t, view1.EdgePools[0], "First edge's pool list should not be empty")

		// Store original values before tampering
		originalToken := view1.Tokens[0]
		originalPoolIndex := view1.EdgePools[0][0]

		// Maliciously modify the received view. This should not affect the system's internal state.
		view1.Tokens[0] = 9999
		view1.EdgePools[0][0] = 8888

		// Get a new view. It should be unchanged.
		view2 := s.View()
		require.NotNil(t, view2)
		require.Len(t, view2.Tokens, 2)

		assert.Equal(t, originalToken, view2.Tokens[0], "internal state should not be affected by modification of view copy")
		require.NotEmpty(t, view2.EdgePools, "view2 EdgePools should not be empty")
		require.NotEmpty(t, view2.EdgePools[0], "view2 First edge's pool list should not be empty")
		assert.Equal(t, originalPoolIndex, view2.EdgePools[0][0], "internal state of EdgePools should not be affected by modification")
	})

	t.Run("Concurrency_ReadsAndWrites_WithBatching", func(t *testing.T) {
		s := NewTokenPoolSystem(50) // Low threshold to trigger compaction
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		writerWg := &sync.WaitGroup{}

		writerWg.Add(1)
		go func() {
			defer writerWg.Done()
			const batchSize = 20
			var poolsToAdd []uint64
			var tokenIDSetsToAdd [][]uint64
			var poolsToRemove []uint64
			var tokensToRemove []uint64

			for i := 0; i < 200; i++ {
				tokenA := uint64(i)
				tokenB := uint64(i + 1)
				poolID := uint64(1000 + i)
				poolsToAdd = append(poolsToAdd, poolID)
				tokenIDSetsToAdd = append(tokenIDSetsToAdd, []uint64{tokenA, tokenB})

				if i%10 == 0 && i > 5 {
					poolsToRemove = append(poolsToRemove, uint64(1000+i-5))
				}
				if i%20 == 0 && i > 15 {
					tokensToRemove = append(tokensToRemove, uint64(i-15))
				}

				if len(poolsToAdd) >= batchSize {
					s.AddPools(poolsToAdd, tokenIDSetsToAdd)
					s.RemovePools(poolsToRemove)
					s.RemoveTokens(tokensToRemove)
					poolsToAdd, tokenIDSetsToAdd, poolsToRemove, tokensToRemove = nil, nil, nil, nil
				}
			}
			if len(poolsToAdd) > 0 {
				s.AddPools(poolsToAdd, tokenIDSetsToAdd)
				s.RemovePools(poolsToRemove)
				s.RemoveTokens(tokensToRemove)
			}
		}()

		readerWg := &sync.WaitGroup{}
		numReaders := 10
		readerWg.Add(numReaders)
		for i := 0; i < numReaders; i++ {
			isViewReader := i%2 == 0
			go func(isViewReader bool) {
				defer readerWg.Done()
				for {
					select {
					case <-ctx.Done():
						return
					default:
						if isViewReader {
							_ = s.View()
						} else {
							randomTokenID := uint64(rand.Intn(150))
							_ = s.PoolsForToken(randomTokenID)
						}
					}
				}
			}(isViewReader)
		}

		writerWg.Wait()
		cancel()
		readerWg.Wait()

		finalView := s.View()
		assert.NotEmpty(t, finalView.Tokens, "final state should not be empty")
		assert.NotEmpty(t, finalView.Pools, "final state should have pools")
	})
}

func TestNewTokenPoolSystemFromView(t *testing.T) {
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

	// 2. Act: Create the system from the view.
	s := NewTokenPoolSystemFromView(originalView, 500)
	require.NotNil(t, s)

	// 3. Assert: Verify the state using the system's public API.
	// This implicitly tests that the underlying registry and the cached view were initialized correctly.
	pools := s.PoolsForToken(10)
	assert.Equal(t, []uint64{100}, pools)

	// Also check the lock-free View() method.
	systemView := s.View()
	require.NotNil(t, systemView)
	assert.Equal(t, originalView.Tokens, systemView.Tokens)
	assert.Equal(t, originalView.Pools, systemView.Pools)
}

func BenchmarkTokenPoolSystem(b *testing.B) {
	sizes := []int{100, 1000, 10000}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("Size%d", size), func(b *testing.B) {
			numPools := size
			numTokens := size / 2

			b.Run("AddPool_Single", func(b *testing.B) {
				s := NewTokenPoolSystem(1000)
				for i := 0; i < numTokens; i++ {
					s.AddPool([]uint64{uint64(i), uint64(i + 1)}, uint64(i))
				}
				b.ResetTimer()
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					s.AddPool(
						[]uint64{uint64(rand.Intn(numTokens)), uint64(rand.Intn(numTokens))},
						uint64(numPools+i),
					)
				}
			})

			b.Run("AddPools_Batch", func(b *testing.B) {
				const batchSize = 100
				poolsToAdd := make([]uint64, batchSize)
				tokenIDSetsToAdd := make([][]uint64, batchSize)
				for i := 0; i < batchSize; i++ {
					poolsToAdd[i] = uint64(numPools + i)
					tokenIDSetsToAdd[i] = []uint64{uint64(rand.Intn(numTokens)), uint64(rand.Intn(numTokens))}
				}

				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					b.StopTimer()
					s := NewTokenPoolSystem(1000)
					for j := 0; j < numTokens; j++ {
						s.AddPool([]uint64{uint64(j), uint64(j + 1)}, uint64(j))
					}
					b.StartTimer()
					s.AddPools(poolsToAdd, tokenIDSetsToAdd)
				}
			})

			b.Run("RemovePools_BatchAndCompact", func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					b.StopTimer()
					const compactionThreshold = 1000
					s := NewTokenPoolSystem(compactionThreshold)
					for j := 0; j < numPools; j++ {
						s.AddPool([]uint64{uint64(j), uint64(j + 1)}, uint64(j))
					}
					numToRemove := (compactionThreshold / 2) + 1
					if numToRemove > numPools {
						numToRemove = numPools
					}
					poolsToRemove := make([]uint64, numToRemove)
					for j := 0; j < numToRemove; j++ {
						poolsToRemove[j] = uint64(j)
					}
					b.StartTimer()
					s.RemovePools(poolsToRemove)
				}
			})

			s := NewTokenPoolSystem(1000)
			for i := 0; i < numPools; i++ {
				s.AddPool([]uint64{uint64(i), uint64(i + 1)}, uint64(i))
			}

			b.Run("View", func(b *testing.B) {
				b.ReportAllocs()
				b.RunParallel(func(pb *testing.PB) {
					for pb.Next() {
						_ = s.View()
					}
				})
			})

			b.Run("PoolsForToken", func(b *testing.B) {
				b.ReportAllocs()
				b.RunParallel(func(pb *testing.PB) {
					localRand := rand.New(rand.NewSource(int64(b.N)))
					for pb.Next() {
						randomTokenID := uint64(localRand.Intn(numTokens))
						_ = s.PoolsForToken(randomTokenID)
					}
				})
			})
		})
	}
}
