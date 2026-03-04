package poolinfo

import (
	"context"
	"fmt"
	"math/big"
	"sync"

	ethclients "github.com/defistate/defistate/clients/eth-clients"
	system "github.com/defistate/defistate/protocols/uniswap-v3"
	"github.com/ethereum/go-ethereum/common"
)

// BatchInfoProvider is the new signature for our multicall-based providers.
type BatchInfoProvider func(
	ctx context.Context,
	poolAddrs []common.Address,
	getClient func() (ethclients.ETHClient, error),
	blockNumber *big.Int,
) (ticks []int64, liquidities []*big.Int, sqrtPricesX96 []*big.Int, fees []uint64, errs []error)

// NewPoolInfoFunc creates a system.GetPoolInfoFunc that executes batch calls in parallel chunks.
func NewPoolInfoFunc(
	infoProvider BatchInfoProvider,
	maxConcurrentCalls int, // Max simultaneous RPC requests
	chunkSize int, // Pools per RPC request
) (system.GetPoolInfoFunc, error) {
	if maxConcurrentCalls <= 0 {
		return nil, fmt.Errorf("maxConcurrentCalls must be positive, got %d", maxConcurrentCalls)
	}
	if chunkSize <= 0 {
		return nil, fmt.Errorf("chunkSize must be positive, got %d", chunkSize)
	}

	return func(
		ctx context.Context,
		poolAddrs []common.Address,
		getClient system.GetClientFunc,
		blockNumber *big.Int,
	) (
		ticks []int64,
		liquidities []*big.Int,
		sqrtPricesX96 []*big.Int,
		fees []uint64,
		errs []error,
	) {
		numPools := len(poolAddrs)
		if numPools == 0 {
			return nil, nil, nil, nil, nil
		}

		// Initialize results
		ticks = make([]int64, numPools)
		liquidities = make([]*big.Int, numPools)
		sqrtPricesX96 = make([]*big.Int, numPools)
		fees = make([]uint64, numPools)
		errs = make([]error, numPools)

		semaphore := make(chan struct{}, maxConcurrentCalls)
		var wg sync.WaitGroup

		for i := 0; i < numPools; i += chunkSize {
			end := i + chunkSize
			if end > numPools {
				end = numPools
			}

			wg.Add(1)
			semaphore <- struct{}{}

			go func(startIdx, endIdx int) {
				defer wg.Done()
				defer func() { <-semaphore }()

				// 1. Check for cancellation
				if ctx.Err() != nil {
					markRangeError(errs, startIdx, endIdx, ctx.Err())
					return
				}

				// 2. Execute Batch Call for the chunk
				chunkPools := poolAddrs[startIdx:endIdx]
				cTicks, cLiqs, cPrices, cFees, cErrs := infoProvider(ctx, chunkPools, getClient, blockNumber)

				// 3. Map chunk results back to global slices
				for j := 0; j < len(chunkPools); j++ {
					globalIdx := startIdx + j

					// If the provider returned an error for this specific pool
					if cErrs != nil && cErrs[j] != nil {
						errs[globalIdx] = cErrs[j]
						continue
					}

					ticks[globalIdx] = cTicks[j]
					liquidities[globalIdx] = cLiqs[j]
					sqrtPricesX96[globalIdx] = cPrices[j]
					fees[globalIdx] = cFees[j]
				}
			}(i, end)
		}

		wg.Wait()
		return ticks, liquidities, sqrtPricesX96, fees, errs
	}, nil
}

func markRangeError(errs []error, start, end int, err error) {
	for i := start; i < end; i++ {
		errs[i] = err
	}
}
