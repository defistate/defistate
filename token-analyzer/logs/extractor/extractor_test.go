package logextractor

import (
	"context"
	"errors"
	"math/big"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLiveExtractor_Run_HappyPath(t *testing.T) {
	const numberOfBlocksToEmit = 10
	var processingWg sync.WaitGroup
	processingWg.Add(numberOfBlocksToEmit)

	var capturedErrs []error
	var errsMu sync.Mutex
	errorHandler := func(err error) {
		errsMu.Lock()
		capturedErrs = append(capturedErrs, err)
		errsMu.Unlock()
	}
	logsHandler := func(ctx context.Context, logs []types.Log) error {
		processingWg.Done()
		return nil
	}

	testBloom := func(types.Bloom) bool { return true }
	getFilterer := func() (func(context.Context, ethereum.FilterQuery) ([]types.Log, error), error) {
		return func(context.Context, ethereum.FilterQuery) ([]types.Log, error) {
			return []types.Log{{}}, nil
		}, nil
	}

	extractor, err := NewLiveExtractor(testBloom, getFilterer)
	require.NoError(t, err)

	newBlockEventCh := make(chan *types.Block, numberOfBlocksToEmit)
	ctx, cancel := context.WithCancel(context.Background())
	var lifecycleWg sync.WaitGroup
	lifecycleWg.Add(1)
	go func() {
		defer lifecycleWg.Done()
		extractor.Run(ctx, newBlockEventCh, logsHandler, errorHandler)
	}()

	for i := 1; i <= numberOfBlocksToEmit; i++ {
		newBlockEventCh <- types.NewBlockWithHeader(&types.Header{Number: big.NewInt(int64(i))})
	}

	processingWg.Wait()
	cancel()
	lifecycleWg.Wait()

	assert.Empty(t, capturedErrs)
}

func TestLiveExtractor_Run_Scenarios(t *testing.T) {
	t.Run("skips block when bloom filter returns false", func(t *testing.T) {
		var logsHandlerCalled atomic.Bool
		testBloom := func(types.Bloom) bool { return false }
		getFilterer := func() (func(context.Context, ethereum.FilterQuery) ([]types.Log, error), error) {
			return nil, nil
		}

		extractor, _ := NewLiveExtractor(testBloom, getFilterer)
		newBlockEventCh := make(chan *types.Block, 1)
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		go extractor.Run(ctx, newBlockEventCh, func(context.Context, []types.Log) error {
			logsHandlerCalled.Store(true)
			return nil
		}, func(error) {})

		newBlockEventCh <- types.NewBlockWithHeader(&types.Header{Number: big.NewInt(1)})
		<-ctx.Done()

		assert.False(t, logsHandlerCalled.Load())
	})

	t.Run("continues processing after a recoverable error", func(t *testing.T) {
		var (
			wg           sync.WaitGroup
			capturedErrs []error
			errsMu       sync.Mutex
			successCount atomic.Int32
		)

		// 1. Setup specific blocks with known hashes
		block1 := types.NewBlockWithHeader(&types.Header{Number: big.NewInt(1)})
		block2 := types.NewBlockWithHeader(&types.Header{Number: big.NewInt(2)})

		// Map hashes to numbers for the mock filterer
		hashToNum := map[common.Hash]uint64{
			block1.Hash(): 1,
			block2.Hash(): 2,
		}

		// We expect 2 completions: Block 1 (errorHandler) and Block 2 (logsHandler)
		wg.Add(2)

		errorHandler := func(err error) {
			errsMu.Lock()
			capturedErrs = append(capturedErrs, err)
			errsMu.Unlock()
			wg.Done()
		}

		logsHandler := func(ctx context.Context, logs []types.Log) error {
			if len(logs) > 0 && logs[0].BlockNumber == 1 {
				// This should be called for Block 1
				return errors.New("transient error on block 1")
			}
			// This should be called for Block 2
			successCount.Add(1)
			wg.Done()
			return nil
		}

		getFilterer := func() (func(context.Context, ethereum.FilterQuery) ([]types.Log, error), error) {
			return func(ctx context.Context, q ethereum.FilterQuery) ([]types.Log, error) {
				if q.BlockHash == nil {
					return nil, errors.New("missing block hash in query")
				}
				num, ok := hashToNum[*q.BlockHash]
				if !ok {
					return nil, errors.New("unknown block hash")
				}
				return []types.Log{{BlockNumber: num}}, nil
			}, nil
		}

		extractor, _ := NewLiveExtractor(func(types.Bloom) bool { return true }, getFilterer)
		newBlockEventCh := make(chan *types.Block, 2)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		go extractor.Run(ctx, newBlockEventCh, logsHandler, errorHandler)

		// 2. Execute
		newBlockEventCh <- block1
		newBlockEventCh <- block2

		// 3. Wait with Timeout (to avoid hanging on failure)
		waitCh := make(chan struct{})
		go func() {
			wg.Wait()
			close(waitCh)
		}()

		select {
		case <-waitCh:
			// Success
		case <-time.After(1 * time.Second):
			t.Fatal("Timeout: Extractor failed to process both blocks in time")
		}

		// 4. Verification
		errsMu.Lock()
		defer errsMu.Unlock()
		require.NotEmpty(t, capturedErrs, "An error should have been captured for Block 1")
		assert.Contains(t, capturedErrs[0].Error(), "transient error on block 1")
		assert.Equal(t, int32(1), successCount.Load(), "Block 2 should have processed successfully")
	})

	t.Run("shuts down gracefully on context cancellation", func(t *testing.T) {
		var lifecycleWg sync.WaitGroup
		lifecycleWg.Add(1)
		extractor, _ := NewLiveExtractor(
			func(types.Bloom) bool { return false },
			func() (func(context.Context, ethereum.FilterQuery) ([]types.Log, error), error) { return nil, nil },
		)

		newBlockEventCh := make(chan *types.Block)
		ctx, cancel := context.WithCancel(context.Background())

		go func() {
			defer lifecycleWg.Done()
			extractor.Run(ctx, newBlockEventCh, nil, nil)
		}()

		cancel()

		done := make(chan struct{})
		go func() {
			lifecycleWg.Wait()
			close(done)
		}()

		select {
		case <-done:
		case <-time.After(500 * time.Millisecond):
			t.Fatal("Run method did not return after context cancellation")
		}
	})
}
