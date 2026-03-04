package initializer

import (
	"context"
	"fmt"
	"math/big"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	// Adjust import path if necessary
	ethclients "github.com/defistate/defistate/clients/eth-clients"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mock Implementations for Testing ---

// mockPoolData holds all the state for a single mock pool.
type mockPoolData struct {
	token0       common.Address
	token1       common.Address
	fee          uint64
	tickSpacing  uint64
	tick         int64
	liquidity    *big.Int
	sqrtPriceX96 *big.Int
}

// mockPoolDataSource simulates a remote data source (e.g., an RPC node).
// It's used by the mockPoolInfoProvider function.
type mockPoolDataSource struct {
	mu         sync.RWMutex
	poolsByAdd map[common.Address]mockPoolData
}

func newMockPoolDataSource() *mockPoolDataSource {
	return &mockPoolDataSource{
		poolsByAdd: make(map[common.Address]mockPoolData),
	}
}

// Helper function to create mock return data for an address.
func addressToBytes(addr common.Address) []byte {
	return common.LeftPadBytes(addr.Bytes(), 32)
}

// mockPoolInfoProvider is a test implementation of the infoProvider function.
// It simulates fetching data for a single pool from the mock data source.
func (ds *mockPoolDataSource) mockPoolInfoProvider(
	ctx context.Context,
	poolAddress common.Address,
	client ethclients.ETHClient, // Client is unused in the mock but matches the required signature.
	blockNumber *big.Int,
) (
	token0 common.Address,
	token1 common.Address,
	fee uint64,
	tickSpacing uint64,
	tick int64,
	liquidity *big.Int,
	sqrtPriceX96 *big.Int,
	err error,
) {
	ds.mu.RLock()
	defer ds.mu.RUnlock()

	if data, ok := ds.poolsByAdd[poolAddress]; ok {
		// Simulate network delay to make concurrency tests meaningful.
		time.Sleep(50 * time.Millisecond)
		return data.token0, data.token1, data.fee, data.tickSpacing, data.tick, data.liquidity, data.sqrtPriceX96, nil
	}

	return common.Address{}, common.Address{}, 0, 0, 0, nil, nil, fmt.Errorf("pool not found: %s", poolAddress.Hex())
}

// --- Test Suite ---

func TestPoolInitializerFunc(t *testing.T) {
	// --- Setup shared test data ---
	dataSource := newMockPoolDataSource()
	mockClient := ethclients.NewTestETHClient()
	mockGetClient := func() (ethclients.ETHClient, error) { return mockClient, nil }
	blockNumber := big.NewInt(1)
	pool1 := common.HexToAddress("0x1")
	pool2 := common.HexToAddress("0x2")
	pool3 := common.HexToAddress("0x3") // Used for concurrency/cancellation tests
	_ = common.HexToAddress("0x4")
	knownFactory := common.HexToAddress("0xf")
	knownFactories := []common.Address{knownFactory}

	mockClient.SetCallContractHandler(func(ctx context.Context, msg ethereum.CallMsg, blockNumber *big.Int) ([]byte, error) {
		if common.Bytes2Hex(msg.Data) == common.Bytes2Hex(factorySig) {
			to := *msg.To
			if to == pool1 || to == pool2 || to == pool3 {
				return addressToBytes(knownFactory), nil
			}
			return addressToBytes(common.Address{}), nil
		}

		return nil, fmt.Errorf("unexpected contract call")
	})

	dataSource.poolsByAdd[pool1] = mockPoolData{
		token0:       common.HexToAddress("0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48"), // USDC
		token1:       common.HexToAddress("0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2"), // WETH
		fee:          500,
		tickSpacing:  10,
		tick:         198750,
		liquidity:    big.NewInt(1000),
		sqrtPriceX96: new(big.Int).Lsh(big.NewInt(1), 96), // 1.0
	}
	dataSource.poolsByAdd[pool2] = mockPoolData{
		token0:       common.HexToAddress("0x6B175474E89094C44Da98b954EedeAC495271d0F"), // DAI
		token1:       common.HexToAddress("0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2"), // WETH
		fee:          3000,
		tickSpacing:  60,
		tick:         -87500,
		liquidity:    big.NewInt(2000),
		sqrtPriceX96: new(big.Int).Lsh(big.NewInt(2), 96), // 2.0
	}
	// Pool3 data is the same as pool1 for simplicity in tests that need a third valid pool.
	dataSource.poolsByAdd[pool3] = dataSource.poolsByAdd[pool1]

	// --- Sub-test for NewPoolInitializerFunc ---
	t.Run("NewPoolInitializerFunc validates input", func(t *testing.T) {
		_, err := NewPoolInitializerFunc(dataSource.mockPoolInfoProvider, 0, knownFactories)
		assert.Error(t, err, "Should return error for maxConcurrentCalls = 0")

		_, err = NewPoolInitializerFunc(dataSource.mockPoolInfoProvider, -1, knownFactories)
		assert.Error(t, err, "Should return error for negative maxConcurrentCalls")

		initFunc, err := NewPoolInitializerFunc(dataSource.mockPoolInfoProvider, 1, knownFactories)
		assert.NoError(t, err, "Should not return error for valid input")
		assert.NotNil(t, initFunc, "Should return a non-nil initializer function for valid input")
	})

	// --- Sub-test for the returned Initializer Function ---
	t.Run("Initializer function correctly fetches data", func(t *testing.T) {
		initFunc, err := NewPoolInitializerFunc(dataSource.mockPoolInfoProvider, 10, knownFactories)
		require.NoError(t, err)

		token0s, token1s, fees, tickSpacings, ticks, liquidities, sqrtPriceX96s, errs := initFunc(
			context.Background(),
			[]common.Address{pool1, pool2},
			mockGetClient,
			blockNumber,
		)

		require.Len(t, errs, 2)
		assert.NoError(t, errs[0])
		assert.NoError(t, errs[1])

		// Validate data for Pool 1
		assert.Equal(t, dataSource.poolsByAdd[pool1].token0, token0s[0])
		assert.Equal(t, dataSource.poolsByAdd[pool1].fee, fees[0])
		assert.Equal(t, dataSource.poolsByAdd[pool1].tick, ticks[0])
		assert.Equal(t, dataSource.poolsByAdd[pool1].liquidity, liquidities[0])

		// Validate data for Pool 2
		assert.Equal(t, dataSource.poolsByAdd[pool2].token1, token1s[1])
		assert.Equal(t, dataSource.poolsByAdd[pool2].tickSpacing, tickSpacings[1])
		assert.Equal(t, dataSource.poolsByAdd[pool2].sqrtPriceX96, sqrtPriceX96s[1])
	})

	// --- Sub-test for Concurrency Limiting ---
	t.Run("Semaphore correctly limits concurrency", func(t *testing.T) {
		concurrencyLimit := 2
		var activeGoroutines atomic.Int32
		var maxObservedConcurrency atomic.Int32

		// Create a provider that wraps the mock and tracks concurrency.
		concurrencyTrackingProvider := func(ctx context.Context, poolAddress common.Address, client ethclients.ETHClient, blockNumber *big.Int) (common.Address, common.Address, uint64, uint64, int64, *big.Int, *big.Int, error) {
			currentActive := activeGoroutines.Add(1)
			// Atomically update the max observed concurrency
			for {
				max := maxObservedConcurrency.Load()
				if currentActive <= max {
					break
				}
				if maxObservedConcurrency.CompareAndSwap(max, currentActive) {
					break
				}
			}
			defer activeGoroutines.Add(-1)
			return dataSource.mockPoolInfoProvider(ctx, poolAddress, client, blockNumber)
		}

		initFunc, err := NewPoolInitializerFunc(concurrencyTrackingProvider, concurrencyLimit, knownFactories)
		require.NoError(t, err)

		poolsToFetch := []common.Address{pool1, pool2, pool3, pool1, pool2}

		initFunc(context.Background(), poolsToFetch, mockGetClient, blockNumber)

		assert.LessOrEqual(t, maxObservedConcurrency.Load(), int32(concurrencyLimit), "The number of concurrent goroutines should not exceed the semaphore limit")
		assert.Greater(t, maxObservedConcurrency.Load(), int32(0), "At least one goroutine should have run")
	})

	// --- Sub-test for Context Cancellation ---
	t.Run("Context cancellation stops processing", func(t *testing.T) {
		var callCount atomic.Int32
		var startWg sync.WaitGroup
		concurrencyLimit := 2
		startWg.Add(concurrencyLimit)

		// slowProvider signals it has started, then blocks until context is canceled.
		slowProvider := func(ctx context.Context, pool common.Address, client ethclients.ETHClient, blockNumber *big.Int) (common.Address, common.Address, uint64, uint64, int64, *big.Int, *big.Int, error) {
			callCount.Add(1)
			startWg.Done() // Signal that this invocation has started.
			<-ctx.Done()   // Block until the context is canceled.
			return common.Address{}, common.Address{}, 0, 0, 0, nil, nil, ctx.Err()
		}

		initFunc, err := NewPoolInitializerFunc(slowProvider, concurrencyLimit, knownFactories)
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		var finalErrs []error
		var mainWg sync.WaitGroup
		mainWg.Add(1)

		// Run the function to be tested in a separate goroutine.
		go func() {
			defer mainWg.Done()
			_, _, _, _, _, _, _, finalErrs = initFunc(ctx, []common.Address{pool1, pool2, pool3}, mockGetClient, blockNumber)
		}()

		// Wait until the 'concurrencyLimit' number of provider calls have started.
		startWg.Wait()

		// Now that we know the providers are running, cancel the context.
		cancel()

		// Wait for the main worker goroutine (which calls the initializer) to finish.
		mainWg.Wait()

		// Assertions
		assert.Equal(t, int32(concurrencyLimit), callCount.Load(), "provider should have been called only twice")
		require.Len(t, finalErrs, 3, "should receive one error per pool")
		for i, err := range finalErrs {
			assert.ErrorIs(t, err, context.Canceled, "error for pool %d should be context.Canceled", i+1)
		}
	})

	// --- Sub-test for Unknown Factory ---
	t.Run("Initializer function handles unknown factory", func(t *testing.T) {
		poolUnknownFactory := common.HexToAddress("0x4") // This pool will return the zero address factory from the mock handler

		initFunc, err := NewPoolInitializerFunc(dataSource.mockPoolInfoProvider, 10, knownFactories)
		require.NoError(t, err)

		// We fetch one pool that should succeed and one that should fail
		_, _, _, _, _, _, _, errs := initFunc(
			context.Background(),
			[]common.Address{pool1, poolUnknownFactory},
			mockGetClient,
			blockNumber,
		)

		// Assertions
		require.Len(t, errs, 2)
		assert.NoError(t, errs[0], "pool1 with a known factory should succeed")
		assert.Error(t, errs[1], "pool with an unknown factory should return an error")
		// The mock returns the zero address for unknown pools, which is not in our known factories list.
		assert.Contains(t, errs[1].Error(), "has an unknown factory 0x0000000000000000000000000000000000000000")
	})

}
