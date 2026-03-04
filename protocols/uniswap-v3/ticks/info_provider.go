package ticks

import (
	"context"
	"fmt"
	"math/big"
	"sync"

	ethclients "github.com/defistate/defistate/clients/eth-clients"
	"github.com/ethereum/go-ethereum/common"
)

// TickDataProvider is responsible for fetching tick data from an underlying source.
type TickDataProvider struct {
	getClient          func() (ethclients.ETHClient, error)
	bitmapProvider     func(ctx context.Context, pool common.Address, spacing uint64, client ethclients.ETHClient, blockNumber *big.Int) (Bitmap, error)
	infoProvider       func(ctx context.Context, pool common.Address, ticksToRequest []int64, client ethclients.ETHClient, blockNumber *big.Int) ([]TickInfo, error)
	maxConcurrentCalls int
}

// NewTickDataProvider creates a new provider.
// maxConcurrentCalls controls the number of simultaneous requests that each batch operation will perform.
func NewTickDataProvider(
	getClient func() (ethclients.ETHClient, error),
	bitmapProvider func(ctx context.Context, pool common.Address, spacing uint64, client ethclients.ETHClient, blockNumber *big.Int) (Bitmap, error),
	infoProvider func(ctx context.Context, pool common.Address, ticksToRequest []int64, client ethclients.ETHClient, blockNumber *big.Int) ([]TickInfo, error),
	maxConcurrentCalls int,
) *TickDataProvider {
	if maxConcurrentCalls <= 0 {
		maxConcurrentCalls = 10 // Default to a reasonable concurrency limit
	}
	return &TickDataProvider{
		getClient:          getClient,
		bitmapProvider:     bitmapProvider,
		infoProvider:       infoProvider,
		maxConcurrentCalls: maxConcurrentCalls,
	}
}

// GetTickBitMap fetches the tick bitmap for multiple pools concurrently.
// It creates and manages a concurrency pool for this specific call.
func (provider *TickDataProvider) GetTickBitMap(
	ctx context.Context,
	pools []common.Address,
	spacings []uint64,
	blockNumber *big.Int,
) ([]Bitmap, []error) {
	semaphore := make(chan struct{}, provider.maxConcurrentCalls)
	var wg sync.WaitGroup
	results := make([]Bitmap, len(pools))
	errs := make([]error, len(pools))

	for i, pool := range pools {
		wg.Add(1)
		semaphore <- struct{}{}
		go func(index int, poolAddress common.Address, spacing uint64) {
			defer wg.Done()
			defer func() { <-semaphore }()

			if ctx.Err() != nil {
				errs[index] = ctx.Err()
				return
			}

			client, err := provider.getClient()
			if err != nil {
				errs[index] = fmt.Errorf("failed to get eth client for pool %s: %w", poolAddress.Hex(), err)
				return
			}

			bitmap, err := provider.bitmapProvider(ctx, poolAddress, spacing, client, blockNumber)
			if err != nil {
				errs[index] = fmt.Errorf("failed to get bitmap for pool %s: %w", poolAddress.Hex(), err)
				return
			}
			results[index] = bitmap

		}(i, pool, spacings[i])
	}

	wg.Wait()
	return results, errs
}

// GetInitializedTicks fetches all initialized tick data for multiple pools concurrently.
// It creates and manages a concurrency pool for this specific call.
func (provider *TickDataProvider) GetInitializedTicks(
	ctx context.Context,
	pools []common.Address,
	bitmaps []Bitmap,
	spacings []uint64,
	blockNumber *big.Int,
) ([][]TickInfo, []error) {
	if len(pools) != len(bitmaps) {
		err := fmt.Errorf("mismatched input lengths: %d pools, %d bitmaps", len(pools), len(bitmaps))
		errs := make([]error, len(pools))
		for i := range errs {
			errs[i] = err
		}
		return nil, errs
	}

	semaphore := make(chan struct{}, provider.maxConcurrentCalls)
	var wg sync.WaitGroup
	results := make([][]TickInfo, len(pools))
	errs := make([]error, len(pools))

	for i, pool := range pools {
		wg.Add(1)
		semaphore <- struct{}{}
		go func(index int, poolAddress common.Address, bitmap Bitmap, spacing uint64) {
			defer wg.Done()
			defer func() { <-semaphore }()

			if ctx.Err() != nil {
				errs[index] = ctx.Err()
				return
			}

			ticksToRequest := TicksFromBitmap(bitmap, int64(spacing))

			if len(ticksToRequest) == 0 {
				results[index] = []TickInfo{}
				return
			}

			client, err := provider.getClient()
			if err != nil {
				errs[index] = fmt.Errorf("failed to get eth client for pool %s: %w", poolAddress.Hex(), err)
				return
			}

			infos, err := provider.infoProvider(ctx, poolAddress, ticksToRequest, client, blockNumber)
			if err != nil {
				errs[index] = fmt.Errorf("failed to get initialized ticks for pool %s: %w", poolAddress.Hex(), err)
				return
			}
			results[index] = infos

		}(i, pool, bitmaps[i], spacings[i])
	}

	wg.Wait()
	return results, errs
}

// GetTicks fetches specific tick data for multiple pools concurrently.
// It creates and manages a concurrency pool for this specific call.
func (provider *TickDataProvider) GetTicks(
	ctx context.Context,
	pools []common.Address,
	ticksToRequest [][]int64,
	blockNumber *big.Int,
) ([][]TickInfo, []error) {
	if len(pools) != len(ticksToRequest) {
		err := fmt.Errorf("mismatched input lengths: %d pools, %d ticks", len(pools), len(ticksToRequest))
		errs := make([]error, len(pools))
		for i := range errs {
			errs[i] = err
		}
		return nil, errs
	}

	semaphore := make(chan struct{}, provider.maxConcurrentCalls)
	var wg sync.WaitGroup
	results := make([][]TickInfo, len(pools))
	errs := make([]error, len(pools))

	for i, pool := range pools {
		wg.Add(1)
		semaphore <- struct{}{}
		go func(index int, poolAddress common.Address, ticks []int64) {
			defer wg.Done()
			defer func() { <-semaphore }()

			if ctx.Err() != nil {
				errs[index] = ctx.Err()
				return
			}

			if len(ticks) == 0 {
				results[index] = []TickInfo{}
				return
			}

			client, err := provider.getClient()
			if err != nil {
				errs[index] = fmt.Errorf("failed to get eth client for pool %s: %w", poolAddress.Hex(), err)
				return
			}

			infos, err := provider.infoProvider(ctx, poolAddress, ticks, client, blockNumber)
			if err != nil {
				errs[index] = fmt.Errorf("failed to get specific ticks for pool %s: %w", poolAddress.Hex(), err)
				return
			}
			results[index] = infos

		}(i, pool, ticksToRequest[i])
	}

	wg.Wait()
	return results, errs
}
