package ticks

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	ethclients "github.com/defistate/defistate/clients/eth-clients"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockProvider struct {
	t                       *testing.T
	getTickBitmapFunc       func(context.Context, []common.Address, []uint64) ([]Bitmap, []error)
	getInitializedTicksFunc func(context.Context, []common.Address, []Bitmap, []uint64) ([][]TickInfo, []error)
	getTicksFunc            func(context.Context, []common.Address, [][]int64) ([][]TickInfo, []error)
	updatedInBlockFunc      func([]types.Log) ([]common.Address, [][]int64, error)
	getCurrentTickFunc      func(context.Context, []common.Address) ([]int64, []error)
	getClientFunc           func() (ethclients.ETHClient, error)
	testBloomFunc           func(types.Bloom) bool

	tickFreshnessGuaranteePercent uint64

	mu sync.RWMutex
}

func newMockProvider(t *testing.T) *mockProvider {
	return &mockProvider{t: t}
}

func (p *mockProvider) OnGetTickBitmap(f func(context.Context, []common.Address, []uint64) ([]Bitmap, []error)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.getTickBitmapFunc = f
}

func (p *mockProvider) OnGetInitializedTicks(f func(context.Context, []common.Address, []Bitmap, []uint64) ([][]TickInfo, []error)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.getInitializedTicksFunc = f
}

func (p *mockProvider) OnGetTicks(f func(context.Context, []common.Address, [][]int64) ([][]TickInfo, []error)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.getTicksFunc = f
}

func (p *mockProvider) OnUpdatedInBlock(f func([]types.Log) ([]common.Address, [][]int64, error)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.updatedInBlockFunc = f
}

func (p *mockProvider) OnGetCurrentTick(f func(context.Context, []common.Address) ([]int64, []error)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.getCurrentTickFunc = f
}

func (p *mockProvider) OnGetClient(f func() (ethclients.ETHClient, error)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.getClientFunc = f
}

func (p *mockProvider) OnTestBloomFunc(f func(types.Bloom) bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.testBloomFunc = f
}

func (p *mockProvider) OnTickFreshnessGuaranteePercent(v uint64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.tickFreshnessGuaranteePercent = v
}

func (p *mockProvider) TickFreshnessGuaranteePercent() uint64 {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.tickFreshnessGuaranteePercent
}

// --- Public Methods Passed to the Indexer (Read-Locked) ---

func (p *mockProvider) GetTickBitmap(ctx context.Context, pools []common.Address, spacing []uint64, blockNumber *big.Int) ([]Bitmap, []error) {
	p.mu.RLock()
	f := p.getTickBitmapFunc
	p.mu.RUnlock()

	if f == nil {
		p.t.Fatal("mockProvider.GetTickBitmap was called but not configured")
	}
	return f(ctx, pools, spacing)
}

func (p *mockProvider) GetInitializedTicks(ctx context.Context, pools []common.Address, bitmaps []Bitmap, spacing []uint64, blockNumber *big.Int) ([][]TickInfo, []error) {
	p.mu.RLock()
	f := p.getInitializedTicksFunc
	p.mu.RUnlock()

	if f == nil {
		p.t.Fatal("mockProvider.GetInitializedTicks was called but not configured")
	}
	return f(ctx, pools, bitmaps, spacing)
}

func (p *mockProvider) GetTicks(ctx context.Context, pools []common.Address, ticks [][]int64, blockNumber *big.Int) ([][]TickInfo, []error) {
	p.mu.RLock()
	f := p.getTicksFunc
	p.mu.RUnlock()

	if f == nil {
		p.t.Fatal("mockProvider.GetTicks was called but not configured")
	}
	return f(ctx, pools, ticks)
}

func (p *mockProvider) UpdatedInBlock(logs []types.Log) ([]common.Address, [][]int64, error) {
	p.mu.RLock()
	f := p.updatedInBlockFunc
	p.mu.RUnlock()

	if f == nil {
		p.t.Fatal("mockProvider.UpdatedInBlock was called but not configured")
	}
	return f(logs)
}

func (p *mockProvider) GetCurrentTick(ctx context.Context, pools []common.Address) ([]int64, []error) {
	p.mu.RLock()
	f := p.getCurrentTickFunc
	p.mu.RUnlock()

	if f == nil {
		p.t.Fatal("mockProvider.GetCurrentTick was called but not configured")
	}
	return f(ctx, pools)
}

func (p *mockProvider) GetClient() (ethclients.ETHClient, error) {
	p.mu.RLock()
	f := p.getClientFunc
	p.mu.RUnlock()

	if f == nil {
		p.t.Fatal("mockProvider.GetClient was called but not configured")
	}
	return f()
}

func (p *mockProvider) TestBloomFunc(b types.Bloom) bool {
	p.mu.RLock()
	f := p.testBloomFunc
	p.mu.RUnlock()

	if f == nil {
		p.t.Fatal("mockProvider.TestBloomFunc was called but not configured")
	}
	return f(b)
}

// TestIndexerLifecycle provides a robust, deterministic harness for testing the TickIndexer's full lifecycle.
func TestIndexerLifecycle(t *testing.T) {

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	provider := newMockProvider(t)

	var receivedErrors []error
	var errMu sync.Mutex
	errorHandler := func(err error) {
		errMu.Lock()
		receivedErrors = append(receivedErrors, err)
		errMu.Unlock()
	}

	getErrors := func() []error {
		errMu.Lock()
		defer errMu.Unlock()
		errCp := make([]error, len(receivedErrors))
		copy(errCp, receivedErrors)

		return errCp
	}

	_ = func() { // resetErrors
		errMu.Lock()
		defer errMu.Unlock()
		receivedErrors = nil
	}

	newBlockEventer := make(chan *types.Block, 10)
	testFrequency := 50 * time.Millisecond

	provider.OnGetCurrentTick(func(ctx context.Context, pools []common.Address) ([]int64, []error) {
		currentTicks := make([]int64, len(pools))
		errs := make([]error, len(pools))
		return currentTicks, errs
	})
	provider.OnTickFreshnessGuaranteePercent(5)
	// Instantiate the TickIndexer using the new Config struct
	cfg := &Config{
		Registry:                      prometheus.NewRegistry(),
		NewBlockEventer:               newBlockEventer,
		GetClient:                     provider.GetClient,
		GetTickBitmap:                 provider.GetTickBitmap,
		GetInitializedTicks:           provider.GetInitializedTicks,
		GetTicks:                      provider.GetTicks,
		UpdatedInBlock:                provider.UpdatedInBlock,
		ErrorHandler:                  errorHandler,
		TestBloomFunc:                 provider.TestBloomFunc,
		FilterTopics:                  [][]common.Hash{{common.HexToHash("0x1234")}}, // Provide a mock topic
		InitFrequency:                 testFrequency,
		ResyncFrequency:               testFrequency,
		UpdateFrequency:               testFrequency,
		LogMaxRetries:                 0,
		LogRetryDelay:                 0,
		Logger:                        slog.New(slog.NewTextHandler(io.Discard, nil)),
		GetCurrentTick:                provider.GetCurrentTick,
		TickFreshnessGuaranteePercent: provider.TickFreshnessGuaranteePercent(),
	}
	tickIndexer, err := NewTickIndexer(ctx, cfg)
	require.NoError(t, err)

	// --- Test Initialization ---
	initialTickInfo := TickInfo{Index: 120, LiquidityGross: big.NewInt(500), LiquidityNet: big.NewInt(-500)}
	t.Run("should initialize a new pool correctly", func(t *testing.T) {

		poolID := uint64(1)
		poolAddr := common.HexToAddress("0x01")
		poolTickSpacing := uint64(60)

		provider.OnGetTickBitmap(func(ctx context.Context, a []common.Address, s []uint64) ([]Bitmap, []error) {
			return []Bitmap{createTestBitmapFromTicks(initialTickInfo.Index / int64(poolTickSpacing))}, nil
		})
		provider.OnGetInitializedTicks(func(ctx context.Context, a []common.Address, b []Bitmap, s []uint64) ([][]TickInfo, []error) {
			// Decode the tick from the bitmap to ensure the logic is consistent.
			tick := TicksFromBitmap(b[0], int64(s[0]))[0]
			// Return a descriptive error if the decoded tick doesn't match.
			if tick != initialTickInfo.Index {
				return nil, []error{
					fmt.Errorf("mismatched tick decoded from bitmap: expected %d, got %d", initialTickInfo.Index, tick),
				}
			}
			return [][]TickInfo{{initialTickInfo}}, make([]error, len(a))
		})

		err := tickIndexer.Add(poolID, poolAddr, poolTickSpacing)
		require.NoError(t, err)

		require.Eventually(t, func() bool {
			ticks, err := tickIndexer.Get(poolID)
			return err == nil && len(ticks) == 1
		}, 1*time.Second, 10*time.Millisecond, "indexer should eventually contain one tick")

		ticks, err := tickIndexer.Get(poolID)
		require.NoError(t, err)
		assert.Equal(t, initialTickInfo.Index, ticks[0].Index)
		require.NotNil(t, ticks[0].LiquidityGross)
		require.NotNil(t, ticks[0].LiquidityNet)
		assert.True(t, initialTickInfo.LiquidityGross.Cmp(ticks[0].LiquidityGross) == 0)
		assert.True(t, initialTickInfo.LiquidityNet.Cmp(ticks[0].LiquidityNet) == 0)

		require.Empty(t, getErrors(), "no errors should have been reported during the lifecycle")

	})

	// --- Test Update ---
	t.Run("should process a block and update an existing pool", func(t *testing.T) {

		poolID := uint64(1)
		poolAddr := common.HexToAddress("0x01")
		updatedTick := int64(180)
		updatedTickInfo := TickInfo{Index: updatedTick, LiquidityGross: big.NewInt(999), LiquidityNet: big.NewInt(-999)}

		blockNumber := atomic.Uint64{}

		testEthClient := ethclients.NewTestETHClient()
		testEthClient.SetFilterLogsHandler(func(ctx context.Context, q ethereum.FilterQuery) ([]types.Log, error) {
			return []types.Log{{BlockNumber: blockNumber.Load()}}, nil
		})
		provider.OnGetClient(func() (ethclients.ETHClient, error) { return testEthClient, nil })
		provider.OnUpdatedInBlock(func(logs []types.Log) ([]common.Address, [][]int64, error) {
			return []common.Address{poolAddr}, [][]int64{{updatedTick}}, nil
		})

		provider.OnGetTicks(func(ctx context.Context, a []common.Address, t [][]int64) ([][]TickInfo, []error) {
			return [][]TickInfo{{initialTickInfo, updatedTickInfo}}, make([]error, len(a))
		})

		provider.OnTestBloomFunc(func(b types.Bloom) bool { return true })
		blockNumber.Store(123)
		block := types.NewBlock(&types.Header{Number: big.NewInt(int64(blockNumber.Load()))}, nil, nil, nil)
		newBlockEventer <- block

		require.Eventually(t, func() bool {
			ticks, err := tickIndexer.Get(poolID)
			return err == nil && len(ticks) == 2
		}, 1*time.Second, 10*time.Millisecond, "indexer should eventually contain two ticks after update")

		_, err := tickIndexer.Get(poolID)
		require.NoError(t, err)
		require.Equal(t, block.Number().Uint64(), tickIndexer.LastUpdatedAtBlock())
		require.Empty(t, getErrors(), "no errors should have been reported during the lifecycle")

	})

	// --- Test Removal of Existing Pool ---
	t.Run("should remove an existing pool", func(t *testing.T) {
		// This test depends on the state from the previous sub-tests (pool 1 exists)
		poolID := uint64(1)

		// Action: Remove the pool
		err := tickIndexer.Remove(poolID)
		require.NoError(t, err, "should not return an error when removing an existing pool")

		// Assertion: The pool should no longer be found
		_, err = tickIndexer.Get(poolID)
		require.ErrorIs(t, err, ErrPoolNotFound, "Get() should return ErrPoolNotFound after removal")
	})

	// --- Test Removal of Non-Existent Pool ---
	t.Run("should fail to remove a non-existent pool", func(t *testing.T) {

		// Action: Attempt to remove a pool that was never added
		poolID := uint64(99)
		err := tickIndexer.Remove(poolID)

		// Assertion: The correct error should be returned
		require.ErrorIs(t, err, ErrPoolNotFound, "should return ErrPoolNotFound for a non-existent pool")
	})

}

// TestIndexerInitializationFailureAndRetry tests that the indexer correctly
// handles a failure during initialization and successfully retries on a subsequent cycle.
func TestIndexerInitializationFailureAndRetry(t *testing.T) {
	// --- Boilerplate Test Setup ---
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	provider := newMockProvider(t)
	var receivedErrors []error
	var errMu sync.Mutex
	errorHandler := func(err error) {
		errMu.Lock()
		defer errMu.Unlock()
		receivedErrors = append(receivedErrors, err)
	}

	getErrors := func() []error {
		errMu.Lock()
		defer errMu.Unlock()
		errCp := make([]error, len(receivedErrors))
		copy(errCp, receivedErrors)

		return errCp
	}

	_ = func() {
		errMu.Lock()
		defer errMu.Unlock()
		receivedErrors = nil
	}

	newBlockEventer := make(chan *types.Block, 10)
	testFrequency := 50 * time.Millisecond
	testEthClient := ethclients.NewTestETHClient()
	provider.OnGetClient(func() (ethclients.ETHClient, error) { return testEthClient, nil })
	provider.OnTestBloomFunc(func(b types.Bloom) bool { return true })
	provider.OnGetCurrentTick(func(ctx context.Context, pools []common.Address) ([]int64, []error) {
		currentTicks := make([]int64, len(pools))
		errs := make([]error, len(pools))
		return currentTicks, errs
	})
	provider.OnTickFreshnessGuaranteePercent(5)

	// --- Instantiate the System Under Test ---
	cfg := &Config{
		Registry:                      prometheus.NewRegistry(),
		NewBlockEventer:               newBlockEventer,
		GetClient:                     provider.GetClient,
		GetTickBitmap:                 provider.GetTickBitmap,
		GetInitializedTicks:           provider.GetInitializedTicks,
		GetTicks:                      provider.GetTicks,
		UpdatedInBlock:                provider.UpdatedInBlock,
		ErrorHandler:                  errorHandler,
		TestBloomFunc:                 provider.TestBloomFunc,
		FilterTopics:                  [][]common.Hash{{common.HexToHash("0x1234")}}, // Provide a mock topic
		InitFrequency:                 testFrequency,
		ResyncFrequency:               testFrequency,
		UpdateFrequency:               testFrequency,
		Logger:                        slog.New(slog.NewTextHandler(io.Discard, nil)),
		GetCurrentTick:                provider.GetCurrentTick,
		TickFreshnessGuaranteePercent: provider.TickFreshnessGuaranteePercent(),
	}
	tickIndexer, err := NewTickIndexer(ctx, cfg)
	require.NoError(t, err)
	require.NotNil(t, tickIndexer)

	// --- Test Constants ---
	poolID := uint64(2)
	poolAddr := common.HexToAddress("0x02")
	poolTickSpacing := uint64(10)
	simulatedErr := errors.New("simulated rpc failure")

	// --- Phase 1: Test the initial failure ---
	t.Run("should fail initialization and queue for retry", func(t *testing.T) {

		// Use atomic flags to ensure the correct mocks are called.
		var calledGetTickBitmap atomic.Bool
		var calledGetInitializedTick atomic.Bool

		// Configure mocks to simulate a failure on the first RPC call.
		provider.OnGetTickBitmap(func(ctx context.Context, a []common.Address, s []uint64) ([]Bitmap, []error) {
			calledGetTickBitmap.Store(true)
			errs := []error{}
			for range a {
				errs = append(errs, simulatedErr)
			}
			return nil, errs
		})
		// This mock should NOT be called if our error handling logic is correct.
		provider.OnGetInitializedTicks(func(ctx context.Context, a []common.Address, b []Bitmap, s []uint64) ([][]TickInfo, []error) {
			calledGetInitializedTick.Store(true)
			return nil, make([]error, len(a))
		})

		// Action: Add the pool, which will trigger the failing initialization.
		err := tickIndexer.Add(poolID, poolAddr, poolTickSpacing)
		require.NoError(t, err, "Add() itself should not fail, it only queues the pool")

		// Assertion: Wait deterministically for the errorHandler to receive our specific error.
		require.Eventually(t, func() bool {
			return len(getErrors()) > 0
		}, 1*time.Second, 10*time.Millisecond, "errorHandler should have received a notification")

		// Verify the contents of the error.
		assert.ErrorContains(t, getErrors()[0], simulatedErr.Error())

		// Verify that the correct functions were called.
		assert.True(t, calledGetTickBitmap.Load(), "GetTickBitmap should have been called")
		assert.False(t, calledGetInitializedTick.Load(), "GetInitializedTicks should NOT be called after a prior error for bitmap")

		// Verify that the pool was not added to the main state.
		_, err = tickIndexer.Get(poolID)
		assert.ErrorIs(t, err, ErrPoolNotFound, "pool should not be found after a failed initialization")
	})

	// --- Phase 2: Test the retry success ---
	t.Run("should succeed on retry", func(t *testing.T) {

		// "Fix" the RPC node by configuring mocks to succeed.
		initialTick := TickInfo{Index: 100, LiquidityGross: big.NewInt(100), LiquidityNet: big.NewInt(-100)}
		provider.OnGetTickBitmap(func(ctx context.Context, a []common.Address, s []uint64) ([]Bitmap, []error) {
			return []Bitmap{createTestBitmapFromTicks(initialTick.Index / int64(poolTickSpacing))}, make([]error, len(a))
		})
		provider.OnGetInitializedTicks(func(ctx context.Context, a []common.Address, b []Bitmap, s []uint64) ([][]TickInfo, []error) {
			// Decode the tick from the bitmap to ensure the logic is consistent.
			tick := TicksFromBitmap(b[0], int64(s[0]))[0]

			// Return a descriptive error if the decoded tick doesn't match.
			if tick != initialTick.Index {
				return nil, []error{
					fmt.Errorf("mismatched tick decoded from bitmap: expected %d, got %d", initialTick.Index, tick),
				}
			}
			return [][]TickInfo{{initialTick}}, make([]error, len(a))
		})

		// Assertion: The initializer's ticker should run again and process the pending pool.
		require.Eventually(t, func() bool {
			ticks, err := tickIndexer.Get(poolID)
			return err == nil && len(ticks) == 1
		}, 1*time.Second, 10*time.Millisecond, "indexer should eventually initialize the pool on retry")

		// Final check of the state.
		ticks, err := tickIndexer.Get(poolID)
		require.NoError(t, err)
		assert.Equal(t, initialTick.Index, ticks[0].Index)
		assert.True(t, initialTick.LiquidityGross.Cmp(ticks[0].LiquidityGross) == 0)

	})
}

// TestIndexerPartialInitializationFailure tests the scenario where a batch
// initialization partially fails and is correctly retried.
func TestIndexerPartialInitializationFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	provider := newMockProvider(t)
	var receivedErrors []error
	var errMu sync.Mutex
	errorHandler := func(err error) {
		errMu.Lock()
		defer errMu.Unlock()
		receivedErrors = append(receivedErrors, err)
	}

	getErrors := func() []error {
		errMu.Lock()
		defer errMu.Unlock()
		errCp := make([]error, len(receivedErrors))
		copy(errCp, receivedErrors)

		return errCp
	}

	resetErrors := func() {
		errMu.Lock()
		defer errMu.Unlock()
		receivedErrors = nil
	}

	newBlockEventer := make(chan *types.Block, 10)
	testFrequency := 50 * time.Millisecond
	resyncFrequency := 1 * time.Hour // very high so resync isn't called
	testEthClient := ethclients.NewTestETHClient()
	provider.OnGetClient(func() (ethclients.ETHClient, error) { return testEthClient, nil })
	provider.OnTestBloomFunc(func(b types.Bloom) bool { return true })
	provider.OnGetCurrentTick(func(ctx context.Context, pools []common.Address) ([]int64, []error) {
		currentTicks := make([]int64, len(pools))
		errs := make([]error, len(pools))
		return currentTicks, errs
	})
	provider.OnTickFreshnessGuaranteePercent(5)

	// --- Instantiate ---
	cfg := &Config{
		Registry:                      prometheus.NewRegistry(),
		NewBlockEventer:               newBlockEventer,
		GetClient:                     provider.GetClient,
		GetTickBitmap:                 provider.GetTickBitmap,
		GetInitializedTicks:           provider.GetInitializedTicks,
		GetTicks:                      provider.GetTicks,
		UpdatedInBlock:                provider.UpdatedInBlock,
		ErrorHandler:                  errorHandler,
		TestBloomFunc:                 provider.TestBloomFunc,
		FilterTopics:                  [][]common.Hash{{common.HexToHash("0x1234")}}, // Provide a mock topic
		InitFrequency:                 testFrequency,
		ResyncFrequency:               resyncFrequency,
		UpdateFrequency:               testFrequency,
		Logger:                        slog.New(slog.NewTextHandler(io.Discard, nil)),
		GetCurrentTick:                provider.GetCurrentTick,
		TickFreshnessGuaranteePercent: provider.TickFreshnessGuaranteePercent(),
	}
	tickIndexer, err := NewTickIndexer(ctx, cfg)
	require.NoError(t, err)

	// --- Test Constants ---
	poolIDSuccess := uint64(10)
	poolAddrSuccess := common.HexToAddress("0x10")
	poolTickSpacingSuccess := uint64(60)

	poolIDFail := uint64(20)
	poolAddrFail := common.HexToAddress("0x20")
	poolTickSpacingFail := uint64(10)

	simulatedErr := fmt.Errorf("simulated rpc failure for pool %s", poolAddrFail)
	tickSuccess := TickInfo{Index: 120, LiquidityGross: big.NewInt(100), LiquidityNet: big.NewInt(-100)}
	tickFail := TickInfo{Index: 200, LiquidityGross: big.NewInt(200), LiquidityNet: big.NewInt(-200)}

	// --- Phase 1: Test the partial failure ---
	t.Log("Testing partial batch failure...")

	// Configure mock to fail for poolAddrFail but succeed for poolAddrSuccess.
	provider.OnGetTickBitmap(func(ctx context.Context, addrs []common.Address, s []uint64) ([]Bitmap, []error) {
		bitmaps := make([]Bitmap, len(addrs))
		errs := make([]error, len(addrs))
		for i, addr := range addrs {
			switch addr {
			case poolAddrSuccess:
				bitmaps[i] = createTestBitmapFromTicks(tickSuccess.Index / int64(poolTickSpacingSuccess))
				errs[i] = nil
			case poolAddrFail:
				bitmaps[i] = nil
				errs[i] = simulatedErr
			}
		}
		return bitmaps, errs
	})

	// Configure GetInitializedTicks to only be called for the successful pool.
	provider.OnGetInitializedTicks(func(ctx context.Context, addrs []common.Address, b []Bitmap, s []uint64) ([][]TickInfo, []error) {
		// Decode the tick from the bitmap to ensure the logic is consistent.
		tick := TicksFromBitmap(b[0], int64(s[0]))[0]

		// Return a descriptive error if the decoded tick doesn't match.
		if tick != tickSuccess.Index {
			return nil, []error{
				fmt.Errorf("mismatched tick decoded from bitmap: expected %d, got %d", tickSuccess.Index, tick),
			}
		}
		return [][]TickInfo{{tickSuccess}}, make([]error, len(addrs))
	})

	// Action: Add both pools.
	require.NoError(t, tickIndexer.Add(poolIDSuccess, poolAddrSuccess, poolTickSpacingSuccess))
	require.NoError(t, tickIndexer.Add(poolIDFail, poolAddrFail, poolTickSpacingFail))

	// Assertion: Check that the successful pool is added and the failed one is not.
	require.Eventually(t, func() bool {
		// The successful pool should exist.
		_, errSuccess := tickIndexer.Get(poolIDSuccess)
		// The failed pool should NOT exist yet.
		_, errFail := tickIndexer.Get(poolIDFail)
		return errSuccess == nil && errors.Is(errFail, ErrPoolNotFound)
	}, 1*time.Second, 10*time.Millisecond, "successful pool should be added, failed pool should not")

	// Verify that the correct error was reported for the failed pool.
	errs := getErrors()
	require.NotEmpty(t, errs)

	// crucial check
	for _, err := range errs {
		assert.True(t, !strings.Contains(err.Error(), poolAddrSuccess.String()))
		assert.True(t, strings.Contains(err.Error(), poolAddrFail.String()))
	}

	assert.ErrorContains(t, getErrors()[0], simulatedErr.Error())
	resetErrors() // Clear errors for the next phase.

	// --- Phase 2: Test the retry of the failed pool ---
	t.Log("Testing successful retry of the failed pool...")

	// Reconfigure mocks to succeed for the previously failed pool.
	provider.OnGetTickBitmap(func(ctx context.Context, addrs []common.Address, s []uint64) ([]Bitmap, []error) {
		return []Bitmap{createTestBitmapFromTicks(tickFail.Index / int64(poolTickSpacingFail))}, make([]error, len(addrs))
	})
	provider.OnGetInitializedTicks(func(ctx context.Context, addrs []common.Address, b []Bitmap, s []uint64) ([][]TickInfo, []error) {
		// Decode the tick from the bitmap to ensure the logic is consistent.
		tick := TicksFromBitmap(b[0], int64(s[0]))[0]

		// Return a descriptive error if the decoded tick doesn't match.
		if tick != tickFail.Index {
			return nil, []error{
				fmt.Errorf("mismatched tick decoded from bitmap: expected %d, got %d", tickFail.Index, tick),
			}
		}
		return [][]TickInfo{{tickFail}}, make([]error, len(addrs))
	})

	// Assertion: Eventually, the failed pool should also be successfully initialized.
	require.Eventually(t, func() bool {
		_, err := tickIndexer.Get(poolIDFail)
		return err == nil
	}, 1*time.Second, 10*time.Millisecond, "failed pool should eventually be initialized on retry")

	// Final check: Both pools should now exist, and no new errors should have been reported.
	_, errSuccess := tickIndexer.Get(poolIDSuccess)
	_, errFail := tickIndexer.Get(poolIDFail)
	assert.NoError(t, errSuccess, "successful pool should still exist")
	assert.NoError(t, errFail, "failed pool should now exist")
	assert.Empty(t, getErrors(), "no new errors should have been reported")
}

// TestIndexerResyncer tests that the resyncer correctly identifies state drift
// and brings the indexer's state back into alignment with the chain.
func TestIndexerResyncer(t *testing.T) {

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	provider := newMockProvider(t)
	var receivedErrors []error
	var errMu sync.Mutex
	errorHandler := func(err error) {
		errMu.Lock()
		defer errMu.Unlock()
		receivedErrors = append(receivedErrors, err)
	}

	getErrors := func() []error {
		errMu.Lock()
		defer errMu.Unlock()
		errCp := make([]error, len(receivedErrors))
		copy(errCp, receivedErrors)

		return errCp
	}

	_ = func() { // resetErrors
		errMu.Lock()
		defer errMu.Unlock()
		receivedErrors = nil
	}

	newBlockEventer := make(chan *types.Block, 10)
	testFrequency := 50 * time.Millisecond
	resyncFrequency := 100 * time.Millisecond // Faster resync for testing

	testEthClient := ethclients.NewTestETHClient()
	provider.OnGetClient(func() (ethclients.ETHClient, error) { return testEthClient, nil })
	provider.OnTestBloomFunc(func(b types.Bloom) bool { return true })
	provider.OnGetCurrentTick(func(ctx context.Context, pools []common.Address) ([]int64, []error) {
		currentTicks := make([]int64, len(pools))
		errs := make([]error, len(pools))
		return currentTicks, errs
	})
	provider.OnTickFreshnessGuaranteePercent(5)

	// --- Instantiate ---
	cfg := &Config{
		Registry:                      prometheus.NewRegistry(),
		NewBlockEventer:               newBlockEventer,
		GetClient:                     provider.GetClient,
		GetTickBitmap:                 provider.GetTickBitmap,
		GetInitializedTicks:           provider.GetInitializedTicks,
		GetTicks:                      provider.GetTicks,
		UpdatedInBlock:                provider.UpdatedInBlock,
		ErrorHandler:                  errorHandler,
		TestBloomFunc:                 provider.TestBloomFunc,
		FilterTopics:                  [][]common.Hash{{common.HexToHash("0x1234")}}, // Provide a mock topic
		InitFrequency:                 testFrequency,
		ResyncFrequency:               resyncFrequency,
		UpdateFrequency:               testFrequency,
		Logger:                        slog.New(slog.NewTextHandler(io.Discard, nil)),
		GetCurrentTick:                provider.GetCurrentTick,
		TickFreshnessGuaranteePercent: provider.TickFreshnessGuaranteePercent(),
	}
	tickIndexer, err := NewTickIndexer(ctx, cfg)
	require.NoError(t, err)

	// --- Phase 1: Initialize the indexer to a "stale" state ---
	t.Log("Initializing indexer to a known, stale state...")

	poolID := uint64(30)
	poolAddr := common.HexToAddress("0x30")
	poolTickSpacing := uint64(60)
	// The indexer will believe ticks 120 and 240 are initialized.
	initialTick120 := TickInfo{Index: 120, LiquidityGross: big.NewInt(100), LiquidityNet: big.NewInt(-100)}
	initialTick240 := TickInfo{Index: 240, LiquidityGross: big.NewInt(200), LiquidityNet: big.NewInt(-200)}
	initialBitMap := []Bitmap{createTestBitmapFromTicks(initialTick120.Index/int64(poolTickSpacing), initialTick240.Index/int64(poolTickSpacing))}

	provider.OnGetTickBitmap(func(ctx context.Context, a []common.Address, s []uint64) ([]Bitmap, []error) {
		return initialBitMap, nil
	})

	provider.OnGetInitializedTicks(func(ctx context.Context, a []common.Address, b []Bitmap, s []uint64) ([][]TickInfo, []error) {
		return [][]TickInfo{{initialTick120, initialTick240}}, make([]error, len(a))
	})

	require.NoError(t, tickIndexer.Add(poolID, poolAddr, poolTickSpacing))
	require.Eventually(t, func() bool {
		ticks, err := tickIndexer.Get(poolID)
		return err == nil && len(ticks) == 2
	}, 2*time.Second, 10*time.Millisecond, "indexer should initialize with two ticks")

	// --- Phase 2: Configure mocks for the "true" on-chain state and trigger resync ---
	t.Log("Simulating state drift and triggering resyncer...")

	// The "real" on-chain state is now different: tick 240 is gone, and tick 300 is new.
	newTick300 := TickInfo{Index: 300, LiquidityGross: big.NewInt(300), LiquidityNet: big.NewInt(-300)}
	onChainBitmap := createTestBitmapFromTicks(initialTick120.Index/int64(poolTickSpacing), newTick300.Index/int64(poolTickSpacing))

	// Re-configure GetTickBitmap to return this new reality for the resyncer.
	provider.OnGetTickBitmap(func(ctx context.Context, addrs []common.Address, s []uint64) ([]Bitmap, []error) {
		require.Equal(t, poolTickSpacing, s[0])
		return []Bitmap{onChainBitmap}, make([]error, len(addrs))
	})

	// Re-configure GetInitializedTicks to return the new, correct tick data.
	provider.OnGetInitializedTicks(func(ctx context.Context, addrs []common.Address, b []Bitmap, s []uint64) ([][]TickInfo, []error) {
		return [][]TickInfo{{initialTick120, newTick300}}, make([]error, len(addrs))
	})

	// --- Phase 3: Assert that the state was corrected ---
	t.Log("Verifying state has been corrected...")

	require.Eventually(t, func() bool {
		ticks, err := tickIndexer.Get(poolID)
		if err != nil || len(ticks) != 2 {
			return false
		}
		// Check that the correct ticks are now present.
		has120 := ticks[0].Index == initialTick120.Index
		has300 := ticks[1].Index == newTick300.Index
		return has120 && has300
	}, 2*time.Second, 10*time.Millisecond, "indexer state should be corrected to contain ticks 120 and 300")

	// Final check of the data and errors.
	finalTicks, err := tickIndexer.Get(poolID)
	require.NoError(t, err)
	assert.Len(t, finalTicks, 2)
	assert.Equal(t, int64(120), finalTicks[0].Index)
	assert.Equal(t, int64(300), finalTicks[1].Index)
	assert.True(t, newTick300.LiquidityGross.Cmp(finalTicks[1].LiquidityGross) == 0)

	// Verify no errors were reported during the successful resync.
	assert.Empty(t, getErrors())
}

// --- 3. Benchmark Helper Functions ---

// generateTickSlices creates a realistic dataset of N pools, each with T ticks.
// It returns two slices (a and b) that are identical in value but distinct
// in memory, which is the "worst case" (full comparison) for our function.
func generateTickSlices(nPools, nTicks int) ([][]TickInfo, [][]TickInfo) {
	a := make([][]TickInfo, nPools)
	b := make([][]TickInfo, nPools)

	// --- NEW: Define large, realistic base numbers ONCE ---
	// These are ~30-digit numbers, well into the uint256 range.
	baseGross, _ := new(big.Int).SetString("123456789012345678901234567890", 10)
	baseNet, _ := new(big.Int).SetString("987654321098765432109876543210", 10)

	// Pre-allocate a *big.Int for the loop counter to re-use
	loopJ := new(big.Int)

	for i := 0; i < nPools; i++ {
		a[i] = make([]TickInfo, nTicks)
		b[i] = make([]TickInfo, nTicks)
		for j := 0; j < nTicks; j++ {
			idx := int64(j)
			loopJ.SetInt64(int64(j)) // Set the loop variable

			// --- NEW: Create new *big.Ints by adding to the large base ---
			// This simulates large, unique values for each tick.
			grossA := new(big.Int).Add(baseGross, loopJ)
			netA := new(big.Int).Add(baseNet, loopJ)

			// Create B by deep-copying A's values
			grossB := new(big.Int).Set(grossA)
			netB := new(big.Int).Set(netA)

			a[i][j] = TickInfo{Index: idx, LiquidityGross: grossA, LiquidityNet: netA}
			b[i][j] = TickInfo{Index: idx, LiquidityGross: grossB, LiquidityNet: netB}
		}
	}
	return a, b
}

// benchmarkResyncLoop is the "worker" for our benchmarks.
// It simulates the *full* workload inside the resyncPools write lock:
// looping N times and calling areTickSlicesEqual on T-sized slices.
func benchmarkResyncLoop(nPools, nTicks int, b *testing.B) {
	// 1. Setup: Create the data *outside* the timer
	b.StopTimer()
	slicesA, slicesB := generateTickSlices(nPools, nTicks)
	b.StartTimer()

	// 2. The Benchmark Loop (controlled by `go test`)
	for i := 0; i < b.N; i++ {

		// 3. The Work: This is the O(N*T) loop we are measuring
		for j := 0; j < nPools; j++ {
			// We assign to a 'volatile' var to prevent the compiler
			// from optimizing away the function call.
			_ = areTickSlicesEqual(slicesA[j], slicesB[j])
		}
	}
}

// --- 4. The Benchmarks (Showing O(N) and O(T) Scaling) ---

// --- Test O(N) scaling (pools) while T is constant ---

func BenchmarkResync_1000Pools_100Ticks(b *testing.B) {
	benchmarkResyncLoop(1000, 100, b)
}

func BenchmarkResync_10000Pools_100Ticks(b *testing.B) {
	benchmarkResyncLoop(10000, 100, b)
}

func BenchmarkResync_100000Pools_100Ticks(b *testing.B) {
	benchmarkResyncLoop(100000, 100, b)
}

// --- Test O(T) scaling (ticks) while N is constant ---

func BenchmarkResync_1000Pools_500Ticks(b *testing.B) {
	benchmarkResyncLoop(1000, 500, b)
}

func BenchmarkResync_1000Pools_2000Ticks(b *testing.B) {
	benchmarkResyncLoop(1000, 2000, b)
}
