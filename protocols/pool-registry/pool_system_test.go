package poolregistry

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/defistate/defistate/engine"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper to find a pool in the view slice
func findPoolInSystemView(pools []PoolView, id uint64) *PoolView {
	for i := range pools {
		if pools[i].ID == id {
			return &pools[i]
		}
	}
	return nil
}

func TestPoolSystem(t *testing.T) {
	addr1 := common.HexToAddress("0x1")
	addr2 := common.HexToAddress("0x2")
	addr3 := common.HexToAddress("0x3")

	key1 := AddressToPoolKey(addr1)
	key2 := AddressToPoolKey(addr2)
	key3 := AddressToPoolKey(addr3)

	protoUni := engine.ProtocolID("uniswap")
	protoCurve := engine.ProtocolID("curve")

	t.Run("API_Correctness_Add_Get_View", func(t *testing.T) {
		s := NewPoolSystem()

		// Test AddPool
		id1, err := s.AddPool(key1, protoUni)
		require.NoError(t, err)
		assert.Equal(t, uint64(1), id1)

		// Test View() method after an add
		view := s.View()
		require.Len(t, view.Pools, 1)

		pool := view.Pools[0]
		assert.Equal(t, id1, pool.ID)
		assert.Equal(t, key1, pool.Key)

		// Check protocol mapping (internal uint16 -> string)
		assert.Equal(t, protoUni, view.Protocols[pool.Protocol])

		// Test GetID and GetKey
		retrievedID, err := s.GetID(key1)
		require.NoError(t, err)
		assert.Equal(t, id1, retrievedID)

		retrievedKey, err := s.GetKey(id1)
		require.NoError(t, err)
		assert.Equal(t, key1, retrievedKey)

		// Test GetProtocolID
		pID, err := s.GetProtocolID(id1)
		require.NoError(t, err)
		assert.Equal(t, protoUni, pID)

		// --- Test GetByID ---
		retrievedView, err := s.GetByID(id1)
		require.NoError(t, err, "GetByID should succeed for an existing pool")
		assert.Equal(t, id1, retrievedView.ID)
		assert.Equal(t, key1, retrievedView.Key)

		// Test GetByID for non-existent pool
		_, err = s.GetByID(999)
		require.Error(t, err, "GetByID should fail for a non-existent pool")
		assert.ErrorIs(t, err, ErrPoolNotFound)
	})

	t.Run("API_Correctness_BatchOperations", func(t *testing.T) {
		s := NewPoolSystem()
		keys := []PoolKey{key1, key2, key3}
		protos := []engine.ProtocolID{protoUni, protoCurve, protoUni}

		// Test AddPools success
		ids, errs := s.AddPools(keys, protos)
		require.Nil(t, errs, "AddPools should succeed with no errors")
		require.Equal(t, []uint64{1, 2, 3}, ids)

		view := s.View()
		require.Len(t, view.Pools, 3)

		// Test AddPools with partial failure (one pool is blocklisted)
		s.AddToBlockList(key2)

		key4 := AddressToPoolKey(common.HexToAddress("0x4"))

		// Attempt to add key4 (new) and key2 (blocklisted)
		newKeys := []PoolKey{key4, key2}
		newProtos := []engine.ProtocolID{protoUni, protoUni}

		ids, errs = s.AddPools(newKeys, newProtos)

		require.NotNil(t, errs, "AddPools should return errors for partial success")
		require.Len(t, errs, 2)

		assert.Nil(t, errs[0], "The first pool should be added successfully")
		assert.NotNil(t, errs[1], "The second pool should fail due to blocklist")
		assert.ErrorIs(t, errs[1], ErrPoolBlocked)

		assert.NotEqual(t, uint64(0), ids[0], "A valid ID should be returned for the successful pool")
		assert.Equal(t, uint64(0), ids[1], "A zero ID should be returned for the failed pool")

		require.Len(t, s.View().Pools, 4, "Only one new pool should have been added (3 original + 1 new)")

		// Test DeletePools with partial failure
		deleteErrs := s.DeletePools([]uint64{1, 999, 3}) // ID 999 does not exist
		require.NotNil(t, deleteErrs)
		require.Len(t, deleteErrs, 3)
		assert.Nil(t, deleteErrs[0], "Deletion of pool 1 should succeed")
		assert.NotNil(t, deleteErrs[1], "Deletion of pool 999 should fail")
		assert.Nil(t, deleteErrs[2], "Deletion of pool 3 should succeed")

		require.Len(t, s.View().Pools, 2, "Two pools should have been deleted (4 total - 2 deleted)")

		// Test AddPools panics on mismatched lengths
		assert.Panics(t, func() {
			s.AddPools([]PoolKey{key1}, []engine.ProtocolID{protoUni, protoCurve})
		}, "AddPools should panic if slice lengths are mismatched")
	})

	t.Run("API_Correctness_BlockList", func(t *testing.T) {
		s := NewPoolSystem()
		s.AddToBlockList(key1)
		assert.True(t, s.IsOnBlockList(key1))

		_, err := s.AddPool(key1, protoUni)
		require.Error(t, err)

		s.RemoveFromBlockList(key1)
		assert.False(t, s.IsOnBlockList(key1))

		_, err = s.AddPool(key1, protoUni)
		require.NoError(t, err)
	})

	t.Run("API_Correctness_Delete", func(t *testing.T) {
		s := NewPoolSystem()
		id1, _ := s.AddPool(key1, protoUni)
		s.AddPool(key2, protoCurve)

		require.Len(t, s.View().Pools, 2)

		err := s.DeletePool(id1)
		require.NoError(t, err)

		view := s.View()
		require.Len(t, view.Pools, 1)
		assert.Nil(t, findPoolInSystemView(view.Pools, id1))
	})

	t.Run("Concurrency_StressTest_WithBatching", func(t *testing.T) {
		s := NewPoolSystem()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		var writerErrs []error
		writerMutex := &sync.Mutex{}
		writerWg := &sync.WaitGroup{}

		writerWg.Add(1)
		go func() {
			defer writerWg.Done()
			const batchSize = 20
			var keysToAdd []PoolKey
			var protosToAdd []engine.ProtocolID
			var poolsToDelete []uint64
			var keysToBlock []PoolKey

			for i := 0; i < 100; i++ {
				key := AddressToPoolKey(common.HexToAddress(fmt.Sprintf("0x%x", i)))
				keysToAdd = append(keysToAdd, key)
				protosToAdd = append(protosToAdd, engine.ProtocolID(fmt.Sprintf("proto-%d", i%5)))

				if i%5 == 0 {
					keysToBlock = append(keysToBlock, key)
				}
				if i%11 == 0 && i > 0 {
					keyToDelete := AddressToPoolKey(common.HexToAddress(fmt.Sprintf("0x%x", i-1)))
					if id, err := s.GetID(keyToDelete); err == nil {
						poolsToDelete = append(poolsToDelete, id)
					}
				}

				if len(keysToAdd) >= batchSize {
					_, addErrs := s.AddPools(keysToAdd, protosToAdd)
					if addErrs != nil {
						writerMutex.Lock()
						for _, err := range addErrs {
							if err != nil {
								// We expect some errors due to blocklisting
							}
						}
						writerMutex.Unlock()
					}
					s.DeletePools(poolsToDelete)
					s.AddManyToBlockList(keysToBlock)
					keysToAdd, protosToAdd, poolsToDelete, keysToBlock = nil, nil, nil, nil
				}
			}
			if len(keysToAdd) > 0 {
				s.AddPools(keysToAdd, protosToAdd)
				s.DeletePools(poolsToDelete)
				s.AddManyToBlockList(keysToBlock)
			}
		}()

		readerWg := &sync.WaitGroup{}
		numReaders := 10
		readerWg.Add(numReaders)
		for i := 0; i < numReaders; i++ {
			go func() {
				defer readerWg.Done()
				for {
					select {
					case <-ctx.Done():
						return
					default:
						_ = s.View()
						_, _ = s.GetID(AddressToPoolKey(common.HexToAddress("0x1")))
						_, _ = s.GetByID(1)
						_ = s.IsOnBlockList(AddressToPoolKey(common.HexToAddress("0x5")))
					}
				}
			}()
		}

		writerWg.Wait()
		cancel()
		readerWg.Wait()

		writerMutex.Lock()
		require.Empty(t, writerErrs, "writer should not encounter unexpected errors")
		writerMutex.Unlock()
		assert.True(t, s.IsOnBlockList(AddressToPoolKey(common.HexToAddress("0x5a"))), "pool 90 (0x5a) should be on blocklist")
	})
}

func TestNewPoolSystemFromViews(t *testing.T) {
	key1 := AddressToPoolKey(common.HexToAddress("0x1"))
	key2 := AddressToPoolKey(common.HexToAddress("0x2"))
	protoUni := engine.ProtocolID("uniswap")
	protoSushi := engine.ProtocolID("sushi")

	t.Run("Success", func(t *testing.T) {
		viewData := PoolRegistryView{
			Pools: []PoolView{
				{ID: 10, Key: key1, Protocol: 0},
				{ID: 20, Key: key2, Protocol: 1},
			},
			Protocols: map[uint16]engine.ProtocolID{
				0: protoUni,
				1: protoSushi,
			},
		}

		s, err := NewPoolSystemFromView(viewData)
		require.NoError(t, err)
		require.NotNil(t, s)

		// Verify that the cached view was correctly initialized by reading from it.
		retrievedView, err := s.GetByID(20)
		require.NoError(t, err, "GetByID should succeed immediately after creation from view")
		assert.Equal(t, viewData.Pools[1], retrievedView)

		sysView := s.View()
		assert.Len(t, sysView.Pools, 2, "View() should return the hydrated pools")
		assert.Equal(t, protoSushi, sysView.Protocols[1])
	})

	t.Run("FailureOnInvalidView", func(t *testing.T) {
		viewData := PoolRegistryView{
			Pools: []PoolView{
				{ID: 1, Key: key1, Protocol: 0},
				{ID: 1, Key: key2, Protocol: 0}, // Duplicate ID
			},
			Protocols: map[uint16]engine.ProtocolID{0: protoUni},
		}

		s, err := NewPoolSystemFromView(viewData)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrDuplicateID)
		assert.Nil(t, s)
	})
}

func BenchmarkPoolSystem(b *testing.B) {
	sizes := []int{100, 1000, 10000}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("Size%d", size), func(b *testing.B) {
			b.Run("AddPool_Single", func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					s := NewPoolSystem()
					for j := 0; j < size; j++ {
						key := AddressToPoolKey(common.HexToAddress(fmt.Sprintf("0x%x", j)))
						pID := engine.ProtocolID(fmt.Sprintf("p-%d", j%2))
						s.AddPool(key, pID)
					}
				}
			})

			b.Run("AddPools_Batch", func(b *testing.B) {
				keys := make([]PoolKey, size)
				protos := make([]engine.ProtocolID, size)
				for i := 0; i < size; i++ {
					keys[i] = AddressToPoolKey(common.HexToAddress(fmt.Sprintf("0x%x", i)))
					protos[i] = engine.ProtocolID(fmt.Sprintf("p-%d", i%2))
				}
				b.ReportAllocs()
				b.ResetTimer()

				for i := 0; i < b.N; i++ {
					s := NewPoolSystem()
					s.AddPools(keys, protos)
				}
			})

			// Setup for concurrent benchmarks
			s := NewPoolSystem()
			keys := make([]PoolKey, size)
			protos := make([]engine.ProtocolID, size)
			for i := 0; i < size; i++ {
				keys[i] = AddressToPoolKey(common.HexToAddress(fmt.Sprintf("0x%x", i)))
				protos[i] = engine.ProtocolID(fmt.Sprintf("p-%d", i%2))
			}
			s.AddPools(keys, protos)

			b.Run("View_Concurrent", func(b *testing.B) {
				b.ReportAllocs()
				b.RunParallel(func(pb *testing.PB) {
					for pb.Next() {
						_ = s.View()
					}
				})
			})

			b.Run("DeletePools_Batch", func(b *testing.B) {
				poolsToDelete := make([]uint64, size/2)
				for i := 0; i < size/2; i++ {
					poolsToDelete[i] = uint64(i + 1)
				}
				b.ReportAllocs()
				b.ResetTimer()

				for i := 0; i < b.N; i++ {
					b.StopTimer()
					s := NewPoolSystem()
					s.AddPools(keys, protos)
					b.StartTimer()
					s.DeletePools(poolsToDelete)
				}
			})
		})
	}
}
