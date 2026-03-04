package initializer

import (
	"context"
	"fmt"
	"math/big"
	"sync"
	"time"

	// This import path is assumed based on your provided example.
	// Please adjust if your project structure is different.
	ethclients "github.com/defistate/defistate/clients/eth-clients"
	system "github.com/defistate/defistate/protocols/uniswap-v3"
	"github.com/defistate/defistate/protocols/uniswap-v3/abi"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
)

var (
	factorySig = abi.UniswapV3ABI.Methods["factory"].ID // factory method has the same signature across all forks so this is safe
)

const (
	// defaultRPCTimeout defines the default timeout for individual RPC calls made by the initializer.
	// This prevents a single slow request from blocking a goroutine indefinitely.
	defaultRPCTimeout = 10 * time.Second
)

// NewPoolInitializerFunc creates and returns a closure of type PoolInitializerFunc.
// This returned function is pre-configured with a specific data provider and a concurrency limit.
// 'maxConcurrentCalls' controls the number of simultaneous requests made to the underlying provider.
// It returns an error if maxConcurrentCalls is not a positive number.
func NewPoolInitializerFunc(
	infoProvider func(
		ctx context.Context,
		poolAddress common.Address,
		client ethclients.ETHClient,
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
	),
	maxConcurrentCalls int,
	knownFactories []common.Address,
) (system.PoolInitializerFunc, error) {
	if maxConcurrentCalls <= 0 {
		return nil, fmt.Errorf("maxConcurrentCalls must be a positive number, but got %d", maxConcurrentCalls)
	}

	// The semaphore is created here and will be "captured" by the closure below.
	// It persists for the lifetime of the returned function, maintaining the concurrency limit.
	semaphore := make(chan struct{}, maxConcurrentCalls)

	knownFactoriesMap := make(map[common.Address]struct{}, len(knownFactories))
	for _, f := range knownFactories {
		knownFactoriesMap[f] = struct{}{}
	}

	// Return a closure that implements the pool initialization logic.
	// This function "closes over" the infoProvider and semaphore variables from its parent scope.
	return func(
		ctx context.Context,
		poolAddrs []common.Address,
		getClient system.GetClientFunc,
		blockNumber *big.Int,
	) (
		token0s []common.Address,
		token1s []common.Address,
		fees []uint64,
		tickSpacings []uint64,
		ticks []int64,
		liquidities []*big.Int,
		sqrtPriceX96s []*big.Int,
		errs []error,
	) {
		numPools := len(poolAddrs)

		// Pre-allocate result slices to their final size. This allows for safe concurrent writes
		// to distinct indices without needing a mutex.
		token0s = make([]common.Address, numPools)
		token1s = make([]common.Address, numPools)
		fees = make([]uint64, numPools)
		tickSpacings = make([]uint64, numPools)
		ticks = make([]int64, numPools)
		liquidities = make([]*big.Int, numPools)
		sqrtPriceX96s = make([]*big.Int, numPools)
		errs = make([]error, numPools)

		var wg sync.WaitGroup

		for i, poolAddr := range poolAddrs {
			wg.Add(1)
			// Acquire a semaphore slot to limit concurrency. This will block if the
			// number of active goroutines reaches the 'maxConcurrentCalls' limit.
			// It accesses the 'semaphore' variable captured from the parent function's scope.
			semaphore <- struct{}{}
			go func(index int, address common.Address) {
				defer wg.Done()

				defer func() { <-semaphore }()

				// Early exit if the parent context has been canceled.
				if ctx.Err() != nil {
					errs[index] = ctx.Err()
					return
				}

				// ARCHITECTURAL DECISION: The client is fetched for each concurrent request.
				// This design allows the getClient implementation to handle sophisticated
				// load balancing (e.g., round-robin through multiple RPC nodes).
				client, err := getClient()
				if err != nil {
					errs[index] = fmt.Errorf("failed to get eth client for pool %s: %w", address.Hex(), err)
					return
				}

				// check factory
				// 1. Identify the pool's factory by calling the contract. This is the security check.
				factoryAddr, err := getFactory(ctx, poolAddr, client)
				if err != nil {
					errs[index] = fmt.Errorf("could not get factory for pool %s: %w", poolAddr.Hex(), err)
					return
				}

				_, isKnownFactory := knownFactoriesMap[factoryAddr]
				if !isKnownFactory {
					errs[index] = fmt.Errorf("pool %s has an unknown factory %s", poolAddr.Hex(), factoryAddr.Hex())
					return
				}

				// Delegate the actual data fetching to the captured infoProvider.
				token0, token1, fee, tickSpacing, tick, liquidity, sqrtPriceX96, err := infoProvider(ctx, address, client, blockNumber)
				if err != nil {
					errs[index] = fmt.Errorf("provider failed for pool %s: %w", address.Hex(), err)
					return
				}

				// Assign the results to their respective indices in the pre-allocated slices.
				token0s[index] = token0
				token1s[index] = token1
				fees[index] = fee
				tickSpacings[index] = tickSpacing
				ticks[index] = tick
				liquidities[index] = liquidity
				sqrtPriceX96s[index] = sqrtPriceX96
			}(i, poolAddr)
		}

		// Wait for all launched goroutines to complete their execution.
		wg.Wait()

		return token0s, token1s, fees, tickSpacings, ticks, liquidities, sqrtPriceX96s, errs
	}, nil
}

// getFactory fetches the factory address for a single pool by making an RPC call.
func getFactory(parentCtx context.Context, poolAddr common.Address, client ethclients.ETHClient) (common.Address, error) {
	// Create a new context with a specific timeout for this RPC call.
	ctx, cancel := context.WithTimeout(parentCtx, defaultRPCTimeout)
	defer cancel()

	factoryCallData, err := client.CallContract(ctx, ethereum.CallMsg{
		To:   &poolAddr,
		Data: factorySig,
	}, nil)
	if err != nil {
		return common.Address{}, fmt.Errorf("eth_call for factory failed: %w", err)
	}
	// A valid address response from a view function is always 32 bytes long.
	if len(factoryCallData) != 32 {
		return common.Address{}, fmt.Errorf("invalid response length for factory: got %d bytes", len(factoryCallData))
	}

	return common.BytesToAddress(factoryCallData), nil
}
