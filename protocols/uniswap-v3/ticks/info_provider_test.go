package ticks

import (
	"context"
	"fmt"
	"math/big"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	ethclients "github.com/defistate/defistate/clients/eth-clients"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mock Implementations for Testing ---

// mockDataSource simulates a remote data source (e.g., an RPC node).
// It's used by the mock provider functions.
type mockDataSource struct {
	mu             sync.RWMutex
	bitmapsByPool  map[common.Address]Bitmap
	tickInfoByPool map[common.Address]map[int64]TickInfo
}

func newMockDataSource() *mockDataSource {
	return &mockDataSource{
		bitmapsByPool:  make(map[common.Address]Bitmap),
		tickInfoByPool: make(map[common.Address]map[int64]TickInfo),
	}
}

// mockBitmapProvider is a test implementation of the bitmapProvider function.
// It now correctly accepts a `spacing` and `client` argument to match the required signature.
func (ds *mockDataSource) mockBitmapProvider(ctx context.Context, pool common.Address, spacing uint64, client ethclients.ETHClient, blockNumber *big.Int) (Bitmap, error) {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	if bitmap, ok := ds.bitmapsByPool[pool]; ok {
		// Simulate network delay
		time.Sleep(10 * time.Millisecond)
		return bitmap, nil
	}
	return nil, fmt.Errorf("pool not found: %s", pool.Hex())
}

// mockInfoProvider is a test implementation of the infoProvider function.
// It accepts an ETHClient to match the required signature.
func (ds *mockDataSource) mockInfoProvider(ctx context.Context, pool common.Address, ticksToRequest []int64, client ethclients.ETHClient, blockNumber *big.Int) ([]TickInfo, error) {
	ds.mu.RLock()
	defer ds.mu.RUnlock()

	poolTicks, ok := ds.tickInfoByPool[pool]
	if !ok {
		return nil, fmt.Errorf("pool not found: %s", pool.Hex())
	}

	results := make([]TickInfo, 0, len(ticksToRequest))
	for _, tickIdx := range ticksToRequest {
		if info, ok := poolTicks[tickIdx]; ok {
			results = append(results, info)
		} else {
			// In a real scenario, you might want to handle this differently,
			// but for this test, returning an error is fine.
			return nil, fmt.Errorf("tick %d not found for pool %s", tickIdx, pool.Hex())
		}
	}
	// Simulate network delay
	time.Sleep(10 * time.Millisecond)
	return results, nil
}

func TestTickDataProvider(t *testing.T) {
	// --- Setup shared test data ---
	dataSource := newMockDataSource()
	blockNumber := big.NewInt(1)
	// mockGetClient simulates a function that returns a working ETH client.
	mockGetClient := func() (ethclients.ETHClient, error) { return ethclients.NewTestETHClient(), nil }

	pool1 := common.HexToAddress("0x1")
	pool2 := common.HexToAddress("0x2")
	pool3 := common.HexToAddress("0x3") // Used for concurrency/cancellation tests

	tick100 := TickInfo{Index: 100, LiquidityGross: big.NewInt(100)}
	tick200 := TickInfo{Index: 200, LiquidityGross: big.NewInt(200)}

	// Populate the mock data source
	dataSource.bitmapsByPool[pool1] = createTestBitmapFromTicks(100, 200)
	dataSource.bitmapsByPool[pool2] = createTestBitmapFromTicks(100)
	dataSource.bitmapsByPool[pool3] = createTestBitmapFromTicks(100) // For concurrency test
	dataSource.tickInfoByPool[pool1] = map[int64]TickInfo{100: tick100, 200: tick200}
	dataSource.tickInfoByPool[pool2] = map[int64]TickInfo{100: tick100}

	// --- Sub-test for GetTickBitMap ---
	t.Run("GetTickBitMap correctly fetches data", func(t *testing.T) {
		provider := NewTickDataProvider(mockGetClient, dataSource.mockBitmapProvider, dataSource.mockInfoProvider, 10)
		pools := []common.Address{pool1, pool2}
		spacings := []uint64{1, 10} // Provide spacings for each pool

		bitmaps, errs := provider.GetTickBitMap(context.Background(), pools, spacings, blockNumber)

		require.Len(t, bitmaps, 2)
		require.Len(t, errs, 2)

		assert.NoError(t, errs[0])
		assert.NoError(t, errs[1])
		assert.NotNil(t, bitmaps[0])
		assert.NotNil(t, bitmaps[1])

		// Check content validity by decoding the bitmap
		ticksFromBitmap1 := TicksFromBitmap(bitmaps[0], 1)
		assert.ElementsMatch(t, []int64{100, 200}, ticksFromBitmap1)
	})

	// --- Sub-test for GetInitializedTicks ---
	t.Run("GetInitializedTicks correctly fetches data", func(t *testing.T) {
		provider := NewTickDataProvider(mockGetClient, dataSource.mockBitmapProvider, dataSource.mockInfoProvider, 10)
		pools := []common.Address{pool1, pool2}
		bitmapsToRequest := []Bitmap{dataSource.bitmapsByPool[pool1], dataSource.bitmapsByPool[pool2]}
		spacings := []uint64{1, 1} // Provide spacings for each pool

		infos, errs := provider.GetInitializedTicks(context.Background(), pools, bitmapsToRequest, spacings, blockNumber)

		require.Len(t, infos, 2)
		require.Len(t, errs, 2)

		assert.NoError(t, errs[0])
		assert.NoError(t, errs[1])
		assert.Len(t, infos[0], 2, "Pool 1 should have 2 ticks")
		assert.Len(t, infos[1], 1, "Pool 2 should have 1 tick")
	})

	// --- Sub-test for Concurrency Limiting ---
	t.Run("Semaphore correctly limits concurrency", func(t *testing.T) {
		concurrencyLimit := 2
		var activeGoroutines atomic.Int32
		var maxObservedConcurrency atomic.Int32

		// Create a provider with a bitmapProvider that tracks concurrency.
		// Note the corrected signature with the 'spacing' parameter.
		concurrencyTrackingProvider := func(ctx context.Context, pool common.Address, spacing uint64, client ethclients.ETHClient, blockNumber *big.Int) (Bitmap, error) {
			currentActive := activeGoroutines.Add(1)
			// Use CompareAndSwap for thread-safe update of the max value.
			for {
				max := maxObservedConcurrency.Load()
				if currentActive > max {
					if maxObservedConcurrency.CompareAndSwap(max, currentActive) {
						break
					}
				} else {
					break
				}
			}
			defer activeGoroutines.Add(-1)
			time.Sleep(50 * time.Millisecond)
			return dataSource.bitmapsByPool[pool], nil
		}

		provider := NewTickDataProvider(mockGetClient, concurrencyTrackingProvider, nil, concurrencyLimit)

		poolsToFetch := []common.Address{pool1, pool2, pool3, pool1, pool2}
		spacings := []uint64{1, 1, 1, 1, 1} // Provide spacings for all pools

		// Action: Run the function.
		provider.GetTickBitMap(context.Background(), poolsToFetch, spacings, blockNumber)

		// Assert: Check that the maximum observed concurrency never exceeded the limit.
		assert.LessOrEqual(t, maxObservedConcurrency.Load(), int32(concurrencyLimit), "The number of concurrent goroutines should not exceed the semaphore limit")
		assert.Greater(t, maxObservedConcurrency.Load(), int32(0), "At least one goroutine should have run")
	})

	// --- Sub-test for Context Cancellation ---
	t.Run("Context cancellation stops processing", func(t *testing.T) {
		var callCount atomic.Int32
		var startWg sync.WaitGroup
		concurrencyLimit := 2
		startWg.Add(concurrencyLimit)

		// slowProvider now has the correct signature with the 'spacing' parameter.
		slowProvider := func(ctx context.Context, pool common.Address, spacing uint64, client ethclients.ETHClient, blockNumber *big.Int) (Bitmap, error) {
			callCount.Add(1)
			startWg.Done() // Signal that this invocation has started.
			<-ctx.Done()   // Block until the context is canceled.
			return nil, ctx.Err()
		}

		provider := NewTickDataProvider(mockGetClient, slowProvider, nil, concurrencyLimit)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		var finalErrs []error
		var mainWg sync.WaitGroup
		mainWg.Add(1)

		go func() {
			defer mainWg.Done()
			pools := []common.Address{pool1, pool2, pool3}
			spacings := []uint64{1, 1, 1}
			_, finalErrs = provider.GetTickBitMap(ctx, pools, spacings, blockNumber)
		}()

		startWg.Wait() // Wait for 2 goroutines to start
		cancel()       // Cancel the context
		mainWg.Wait()  // Wait for the main function to return

		// Assertions
		assert.Equal(t, int32(concurrencyLimit), callCount.Load(), "provider should have been called only twice")
		require.Len(t, finalErrs, 3, "should receive one error per pool")
		for i, err := range finalErrs {
			// The error could be context.Canceled directly, or a wrapped error.
			// Using ErrorIs is the correct way to check.
			assert.ErrorIs(t, err, context.Canceled, "error for pool %d should be context.Canceled", i+1)
		}
	})
}
