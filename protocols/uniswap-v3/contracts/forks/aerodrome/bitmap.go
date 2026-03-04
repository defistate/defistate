package aerodrome

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sync"

	ethclients "github.com/defistate/defistate/clients/eth-clients"
	uniswapv3abi "github.com/defistate/defistate/protocols/uniswap-v3/abi"
	aerodromeabi "github.com/defistate/defistate/protocols/uniswap-v3/abi/forks/aerodrome"

	"github.com/defistate/defistate/protocols/uniswap-v3/ticks"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"golang.org/x/sync/errgroup"
)

// Constants for Uniswap V3 tick calculations.
const (
	MinTick int64 = -887272
	MaxTick int64 = 887272
)

// Pre-defined errors for the BitMapProvider for easier error checking.
var (
	ErrPackTickBitmapFailed  = errors.New("failed to pack call data for tickBitmap")
	ErrPackMulticallFailed   = errors.New("failed to pack multicall input")
	ErrMulticallRPCFailed    = errors.New("multicall rpc call failed")
	ErrUnpackMulticallFailed = errors.New("failed to unpack multicall results")
)

// multicallResult defines the structure of a single result from a tryAggregate call.
type multicallResult struct {
	Success    bool
	ReturnData []byte
}

// NewBitMapProvider creates a provider function that fetches a pool's complete tick bitmap
// using batched, concurrent Multicall3 requests.
//
// Parameters:
//   - multicallAddress: The address of the deployed Multicall3 contract.
//   - batchSize: The number of tickBitmap calls to include in a single multicall.
//   - maxConcurrency: The maximum number of multicall batches to execute in parallel.
func NewBitMapProvider(
	multicallAddress common.Address,
	batchSize int,
	maxConcurrency int,
) func(ctx context.Context, pool common.Address, tickSpacing uint64, client ethclients.ETHClient, blockNumber *big.Int) (ticks.Bitmap, error) {
	return func(ctx context.Context, pool common.Address, tickSpacing uint64, client ethclients.ETHClient, blockNumber *big.Int) (ticks.Bitmap, error) {
		// 1. Calculate the full range of word positions to fetch.
		minWordPos := int16((MinTick / int64(tickSpacing)) >> 8)
		maxWordPos := int16((MaxTick / int64(tickSpacing)) >> 8)
		totalWords := int(maxWordPos - minWordPos + 1)
		wordPositions := make([]int16, totalWords)
		for i := 0; i < totalWords; i++ {
			wordPositions[i] = minWordPos + int16(i)
		}

		// 2. Prepare for concurrent execution.
		bitmap := make(ticks.Bitmap)
		var mu sync.Mutex
		g, gCtx := errgroup.WithContext(ctx)
		g.SetLimit(maxConcurrency)

		// Define the ABI for the `tickBitmap` return value (a single uint256)
		// for safer unpacking of the results.
		uint256Type, _ := abi.NewType("uint256", "", nil)
		tickBitmapReturnArgs := abi.Arguments{{Type: uint256Type}}

		// 3. Process all word positions in concurrent batches.
		for i := 0; i < len(wordPositions); i += batchSize {
			end := i + batchSize
			if end > len(wordPositions) {
				end = len(wordPositions)
			}
			batch := wordPositions[i:end]

			g.Go(func() error {
				results, err := fetchBitMapBatch(gCtx, multicallAddress, pool, batch, client, blockNumber)
				if err != nil {
					return err
				}

				mu.Lock()
				defer mu.Unlock()
				for j, result := range results {
					if result.Success && len(result.ReturnData) > 0 {
						// Unpack the ABI-encoded return data for type safety.
						unpacked, err := tickBitmapReturnArgs.Unpack(result.ReturnData)
						if err != nil {
							return err
						}

						if len(unpacked) == 0 {
							continue
						}

						wordValue := unpacked[0].(*big.Int)
						if wordValue.Cmp(big.NewInt(0)) > 0 {
							wordPos := batch[j]
							bitmap[wordPos] = wordValue
						}
					}
				}
				return nil
			})
		}

		// 4. Wait for all goroutines to complete.
		if err := g.Wait(); err != nil {
			return nil, err
		}
		return bitmap, nil
	}
}

// fetchBitMapBatch is a helper that executes a multicall for a batch of word positions.
func fetchBitMapBatch(
	ctx context.Context,
	multicallAddress common.Address,
	pool common.Address,
	batch []int16,
	client ethclients.ETHClient,
	blockNumber *big.Int,
) ([]multicallResult, error) {
	// 1. Prepare call data for each word position in the batch.
	calls := make([]struct {
		Target   common.Address
		CallData []byte
	}, len(batch))
	for i, wordPos := range batch {
		callData, err := aerodromeabi.AerodromeABI.Pack("tickBitmap", wordPos)
		if err != nil {
			return nil, fmt.Errorf("%w for wordPos %d: %w", ErrPackTickBitmapFailed, wordPos, err)
		}
		calls[i] = struct {
			Target   common.Address
			CallData []byte
		}{Target: pool, CallData: callData}
	}

	// 2. Pack the calls into a multicall request.
	multicallInput, err := uniswapv3abi.Multicall3ABI.Pack("tryAggregate", false, calls)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrPackMulticallFailed, err)
	}

	msg := ethereum.CallMsg{
		To:   &multicallAddress,
		Data: multicallInput,
	}
	// 3. Execute the multicall RPC request.
	returnData, err := client.CallContract(ctx, msg, blockNumber)
	if err != nil {
		return nil, fmt.Errorf("%w for pool %s: %w", ErrMulticallRPCFailed, pool.Hex(), err)
	}
	if len(returnData) == 0 {
		return nil, nil
	}

	// 4. Unpack the results.
	var results []multicallResult
	if err := uniswapv3abi.Multicall3ABI.UnpackIntoInterface(&results, "tryAggregate", returnData); err != nil {
		return nil, fmt.Errorf("%w for pool %s: %w", ErrUnpackMulticallFailed, pool.Hex(), err)
	}
	return results, nil
}
