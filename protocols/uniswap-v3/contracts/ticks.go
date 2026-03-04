package contracts

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sync"

	ethclients "github.com/defistate/defistate/clients/eth-clients"
	uniswapv3abi "github.com/defistate/defistate/protocols/uniswap-v3/abi"
	"github.com/defistate/defistate/protocols/uniswap-v3/ticks"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"golang.org/x/sync/errgroup"
)

// Pre-defined errors for the TickInfoProvider for easier error checking.
var (
	ErrPackTickInfoFailed      = errors.New("failed to pack call data for ticks function")
	ErrPackTickInfoMultiFailed = errors.New("failed to pack multicall input for tick info")
	ErrTickInfoMulticallFailed = errors.New("tick info multicall rpc call failed")
	ErrUnpackTickInfoFailed    = errors.New("failed to unpack tick info multicall results")
)

// NewTickInfoProvider creates a provider that fetches information for multiple ticks from a Uniswap V3 Pool
// using batched, concurrent multicall requests for maximum efficiency.
//
// Parameters:
//   - multicallAddress: The address of the deployed Multicall3 contract.
//   - batchSize: The number of `ticks` calls to include in a single multicall.
//   - maxConcurrency: The maximum number of multicall batches to execute in parallel.
func NewTickInfoProvider(
	multicallAddress common.Address,
	batchSize int,
	maxConcurrency int,
) func(ctx context.Context, pool common.Address, ticksToRequest []int64, client ethclients.ETHClient, blockNumber *big.Int) ([]ticks.TickInfo, error) {
	return func(ctx context.Context, pool common.Address, ticksToRequest []int64, client ethclients.ETHClient, blockNumber *big.Int) ([]ticks.TickInfo, error) {
		if len(ticksToRequest) == 0 {
			return []ticks.TickInfo{}, nil
		}

		// 1. Prepare for concurrent execution.
		tickInfos := make([]ticks.TickInfo, 0, len(ticksToRequest))
		var mu sync.Mutex // Mutex to protect concurrent appends to the tickInfos slice.

		g, gCtx := errgroup.WithContext(ctx)
		g.SetLimit(maxConcurrency) // Limit the number of concurrent goroutines.

		// 2. Process all requested ticks in concurrent batches.
		for i := 0; i < len(ticksToRequest); i += batchSize {
			end := i + batchSize
			if end > len(ticksToRequest) {
				end = len(ticksToRequest)
			}
			batch := ticksToRequest[i:end]

			// Launch a worker goroutine for this batch.
			g.Go(func() error {
				// The helper function fetches the raw data for one batch.
				results, err := fetchTickInfoBatch(gCtx, multicallAddress, pool, batch, client, blockNumber)
				if err != nil {
					return err
				}

				// This struct must match the full contract return type for unpacking.
				var tickOutput struct {
					LiquidityGross                 *big.Int
					LiquidityNet                   *big.Int
					FeeGrowthOutside0X128          *big.Int
					FeeGrowthOutside1X128          *big.Int
					TickCumulativeOutside          *big.Int
					SecondsPerLiquidityOutsideX128 *big.Int
					SecondsOutside                 uint32
					Initialized                    bool
				}

				// Lock the mutex to safely append results to the shared slice.
				mu.Lock()
				defer mu.Unlock()

				for j, result := range results {
					if result.Success && len(result.ReturnData) > 0 {
						err := uniswapv3abi.UniswapV3ABI.UnpackIntoInterface(&tickOutput, "ticks", result.ReturnData)
						if err != nil {
							return err
						}

						if tickOutput.Initialized {
							tickInfos = append(tickInfos, ticks.TickInfo{
								Index:          batch[j],
								LiquidityGross: tickOutput.LiquidityGross,
								LiquidityNet:   tickOutput.LiquidityNet,
							})
						}
					}
				}
				return nil
			})
		}

		// 3. Wait for all goroutines to complete and return any errors.
		if err := g.Wait(); err != nil {
			return nil, err
		}

		return tickInfos, nil
	}
}

// fetchTickInfoBatch is a helper that executes a multicall for a batch of tick indices.
func fetchTickInfoBatch(
	ctx context.Context,
	multicallAddress common.Address,
	pool common.Address,
	batch []int64,
	client ethclients.ETHClient,
	blockNumber *big.Int,
) ([]multicallResult, error) {
	// 1. Prepare call data for each tick in the batch.
	calls := make([]struct {
		Target   common.Address
		CallData []byte
	}, len(batch))

	for i, tick := range batch {
		callData, err := uniswapv3abi.UniswapV3ABI.Pack("ticks", big.NewInt(tick))
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrPackTickInfoFailed, err)
		}
		calls[i] = struct {
			Target   common.Address
			CallData []byte
		}{Target: pool, CallData: callData}
	}

	// 2. Pack the calls into a multicall request.
	multicallInput, err := uniswapv3abi.Multicall3ABI.Pack("tryAggregate", false, calls)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrPackTickInfoMultiFailed, err)
	}

	msg := ethereum.CallMsg{
		To:   &multicallAddress,
		Data: multicallInput,
	}
	// 3. Execute the multicall RPC request.
	returnData, err := client.CallContract(ctx, msg, blockNumber)
	if err != nil {
		return nil, fmt.Errorf("%w for pool %s: %w", ErrTickInfoMulticallFailed, pool.Hex(), err)
	}
	if len(returnData) == 0 {
		return nil, nil
	}

	// 4. Unpack the results.
	var results []multicallResult
	if err := uniswapv3abi.Multicall3ABI.UnpackIntoInterface(&results, "tryAggregate", returnData); err != nil {
		return nil, fmt.Errorf("%w for pool %s: %w", ErrUnpackTickInfoFailed, pool.Hex(), err)
	}
	return results, nil
}
