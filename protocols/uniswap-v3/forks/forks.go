package forks

import (
	"context"
	"fmt"
	"math/big"

	ethclients "github.com/defistate/defistate/clients/eth-clients"
	uniswapv3abi "github.com/defistate/defistate/protocols/uniswap-v3/abi"
	aerodromeabi "github.com/defistate/defistate/protocols/uniswap-v3/abi/forks/aerodrome"
	omniexchangeabi "github.com/defistate/defistate/protocols/uniswap-v3/abi/forks/omniexchange"
	pancakeswapv3abi "github.com/defistate/defistate/protocols/uniswap-v3/abi/forks/pancakeswap"

	uniswapv3bloom "github.com/defistate/defistate/protocols/uniswap-v3/bloom"
	aerodromebloom "github.com/defistate/defistate/protocols/uniswap-v3/bloom/forks/aerodrome"
	omniexchangebloom "github.com/defistate/defistate/protocols/uniswap-v3/bloom/forks/omniexchange"
	pancakeswapv3bloom "github.com/defistate/defistate/protocols/uniswap-v3/bloom/forks/pancakeswap"

	uniswapv3contracthelpers "github.com/defistate/defistate/protocols/uniswap-v3/contracts"
	aerodromecontracthelpers "github.com/defistate/defistate/protocols/uniswap-v3/contracts/forks/aerodrome"
	omniexchangecontracthelpers "github.com/defistate/defistate/protocols/uniswap-v3/contracts/forks/omniexchange"
	pancakeswapv3contracthelpers "github.com/defistate/defistate/protocols/uniswap-v3/contracts/forks/pancakeswap"

	uniswapv3poollogs "github.com/defistate/defistate/protocols/uniswap-v3/logs"
	aerodromepoollogs "github.com/defistate/defistate/protocols/uniswap-v3/logs/forks/aerodrome"
	omniexchangepoollogs "github.com/defistate/defistate/protocols/uniswap-v3/logs/forks/omniexchange"
	pancakeswapv3poollogs "github.com/defistate/defistate/protocols/uniswap-v3/logs/forks/pancakeswap"

	uniswapv3ticksbloom "github.com/defistate/defistate/protocols/uniswap-v3/ticks/bloom"
	aerodrometicksbloom "github.com/defistate/defistate/protocols/uniswap-v3/ticks/bloom/forks/aerodrome"
	omniexchangeticksbloom "github.com/defistate/defistate/protocols/uniswap-v3/ticks/bloom/forks/omniexchange"
	pancakeswapv3ticksbloom "github.com/defistate/defistate/protocols/uniswap-v3/ticks/bloom/forks/pancakeswap"

	uniswapv3ticks "github.com/defistate/defistate/protocols/uniswap-v3/ticks"
	uniswapv3tickslogs "github.com/defistate/defistate/protocols/uniswap-v3/ticks/logs"
	aerodrometickslogs "github.com/defistate/defistate/protocols/uniswap-v3/ticks/logs/forks/aerodrome"
	omniexchangetickslogs "github.com/defistate/defistate/protocols/uniswap-v3/ticks/logs/forks/omniexchange"
	pancakeswapv3tickslogs "github.com/defistate/defistate/protocols/uniswap-v3/ticks/logs/forks/pancakeswap"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// This package provides a clean, configurable way to get the specific, fork-dependent
// helper functions needed to initialize a Uniswap V3-style subsystem.

// --- Fork IDs ---
const (
	UniswapV3     uint8 = iota // 0
	PancakeswapV3              // 1
	Aerodrome                  // 2
	OmniExchange               // 3
)

// --- Data Structures for Organization ---

// SystemFuncs holds all the fork-specific functions needed by the core UniswapV3System.
type SystemFuncs struct {
	PoolInitializer func(multicallAddress common.Address) func(
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
	)
	PoolInfoProvider     func(multicallAddress common.Address) func(ctx context.Context, poolAddresses []common.Address, getClient func() (ethclients.ETHClient, error), blockNumber *big.Int) (ticks []int64, liquidities []*big.Int, sqrtPricesX96 []*big.Int, fees []uint64, errs []error)
	DiscoverPools        func(logs []types.Log) ([]common.Address, error)
	ExtractSwaps         func(logs []types.Log) (pools []common.Address, ticks []int64, liquidities []*big.Int, sqrtPricesX96 []*big.Int, err error)
	ExtractMintsAndBurns func(logs []types.Log) (pools []common.Address)
	TestBloom            func(b types.Bloom) bool
	FilterTopics         [][]common.Hash
}

// TickIndexerFuncs holds all the fork-specific functions needed by the TickIndexer.
type TickIndexerFuncs struct {
	TickBitmapProvider func(multicallAddress common.Address, batchSize int, maxConcurrency int) func(ctx context.Context, pool common.Address, tickSpacing uint64, client ethclients.ETHClient, blockNumber *big.Int) (uniswapv3ticks.Bitmap, error)
	TickInfoProvider   func(multicallAddress common.Address, batchSize int, maxConcurrency int) func(ctx context.Context, pool common.Address, ticksToRequest []int64, client ethclients.ETHClient, blockNumber *big.Int) ([]uniswapv3ticks.TickInfo, error)

	UpdatedInBlock func(logs []types.Log) (pools []common.Address, updatedTicks [][]int64, err error)
	TestBloom      func(b types.Bloom) bool
	FilterTopics   [][]common.Hash
}

// ForkData is the main container struct. It provides a clean, organized, and
// extensible way to return all the necessary functions for a given fork.
type ForkData struct {
	System      SystemFuncs
	TickIndexer TickIndexerFuncs
}

// --- Main Dispatcher ---

// GetForkData is the main entry point. It acts as a dispatcher, returning the
// appropriate ForkData struct based on the provided forkID.
func GetForkData(forkID uint8) (ForkData, error) {
	switch forkID {
	case UniswapV3:
		return getUniswapV3Data(), nil
	case PancakeswapV3:
		return getPancakeswapData(), nil
	case Aerodrome:
		return getAerodromeData(), nil
	case OmniExchange:
		return getOmniExchangeData(), nil
	default:
		return ForkData{}, fmt.Errorf("unsupported fork %d", forkID)
	}
}

// --- Fork-Specific Implementations ---

// getUniswapV3Data returns the ForkData for the original Uniswap V3 protocol.
func getUniswapV3Data() ForkData {
	return ForkData{
		System: SystemFuncs{
			PoolInitializer:      uniswapv3contracthelpers.NewPoolInitializer,
			PoolInfoProvider:     uniswapv3contracthelpers.NewBatchPoolInfoProvider,
			DiscoverPools:        uniswapv3poollogs.DiscoverPools,
			ExtractSwaps:         uniswapv3poollogs.ExtractSwapsFromLogs,
			ExtractMintsAndBurns: uniswapv3poollogs.ExtractMintsAndBurnsFromLogs,
			TestBloom: func(b types.Bloom) bool {
				return uniswapv3bloom.TestUniswapV3Swap(b) || uniswapv3ticksbloom.TestUniswapV3PoolBurn(&b) || uniswapv3ticksbloom.TestUniswapV3PoolMint(&b)
			},
			FilterTopics: [][]common.Hash{{
				uniswapv3abi.UniswapV3ABI.Events["Swap"].ID,
				uniswapv3abi.UniswapV3ABI.Events["Burn"].ID,
				uniswapv3abi.UniswapV3ABI.Events["Mint"].ID,
			}},
		},
		TickIndexer: TickIndexerFuncs{
			TickBitmapProvider: uniswapv3contracthelpers.NewBitMapProvider,
			TickInfoProvider:   uniswapv3contracthelpers.NewTickInfoProvider,
			UpdatedInBlock:     uniswapv3tickslogs.UniswapV3PoolsUpdatedInBlock,
			TestBloom: func(b types.Bloom) bool {
				return uniswapv3ticksbloom.TestUniswapV3PoolBurn(&b) || uniswapv3ticksbloom.TestUniswapV3PoolMint(&b)
			},
			FilterTopics: [][]common.Hash{{
				uniswapv3abi.UniswapV3ABI.Events["Burn"].ID,
				uniswapv3abi.UniswapV3ABI.Events["Mint"].ID,
			}},
		},
	}
}

// getPancakeswapData returns the ForkData for the Pancakeswap V3 fork.
func getPancakeswapData() ForkData {
	return ForkData{
		System: SystemFuncs{
			PoolInitializer:      pancakeswapv3contracthelpers.NewPoolInitializer,
			PoolInfoProvider:     pancakeswapv3contracthelpers.NewBatchPoolInfoProvider,
			DiscoverPools:        pancakeswapv3poollogs.DiscoverPools,
			ExtractSwaps:         pancakeswapv3poollogs.ExtractSwapsFromLogs,
			ExtractMintsAndBurns: pancakeswapv3poollogs.ExtractMintsAndBurnsFromLogs,
			TestBloom: func(b types.Bloom) bool {
				return pancakeswapv3bloom.TestPancakeswapV3Swap(b) || pancakeswapv3ticksbloom.TestPancakeswapV3PoolMint(&b) || pancakeswapv3ticksbloom.TestPancakeswapV3PoolBurn(&b)
			},
			FilterTopics: [][]common.Hash{{
				pancakeswapv3abi.PancakeswapV3ABI.Events["Swap"].ID,
				pancakeswapv3abi.PancakeswapV3ABI.Events["Burn"].ID,
				pancakeswapv3abi.PancakeswapV3ABI.Events["Mint"].ID,
			}},
		},
		TickIndexer: TickIndexerFuncs{
			TickBitmapProvider: pancakeswapv3contracthelpers.NewBitMapProvider,
			TickInfoProvider:   pancakeswapv3contracthelpers.NewTickInfoProvider,
			UpdatedInBlock:     pancakeswapv3tickslogs.PancakeswapV3PoolsUpdatedInBlock,
			TestBloom: func(b types.Bloom) bool {
				return pancakeswapv3ticksbloom.TestPancakeswapV3PoolBurn(&b) || pancakeswapv3ticksbloom.TestPancakeswapV3PoolMint(&b)
			},
			FilterTopics: [][]common.Hash{{
				pancakeswapv3abi.PancakeswapV3ABI.Events["Burn"].ID,
				pancakeswapv3abi.PancakeswapV3ABI.Events["Mint"].ID,
			}},
		},
	}
}

// getAerodromeData returns the ForkData for the Aerodrome V3 fork.
func getAerodromeData() ForkData {
	return ForkData{
		System: SystemFuncs{
			PoolInitializer:      aerodromecontracthelpers.NewPoolInitializer,
			PoolInfoProvider:     aerodromecontracthelpers.NewBatchPoolInfoProvider,
			DiscoverPools:        aerodromepoollogs.DiscoverPools,
			ExtractSwaps:         aerodromepoollogs.ExtractSwapsFromLogs,
			ExtractMintsAndBurns: aerodromepoollogs.ExtractMintsAndBurnsFromLogs,
			TestBloom: func(b types.Bloom) bool {
				return aerodromebloom.TestAerodromeSwap(b) || aerodrometicksbloom.TestAerodromePoolMint(&b) || aerodrometicksbloom.TestAerodromePoolBurn(&b)
			},
			FilterTopics: [][]common.Hash{{
				aerodromeabi.AerodromeABI.Events["Swap"].ID,
				aerodromeabi.AerodromeABI.Events["Burn"].ID,
				aerodromeabi.AerodromeABI.Events["Mint"].ID,
			}},
		},
		TickIndexer: TickIndexerFuncs{
			TickBitmapProvider: aerodromecontracthelpers.NewBitMapProvider,
			TickInfoProvider:   aerodromecontracthelpers.NewTickInfoProvider,
			UpdatedInBlock:     aerodrometickslogs.AerodromePoolsUpdatedInBlock,
			TestBloom: func(b types.Bloom) bool {
				return aerodrometicksbloom.TestAerodromePoolBurn(&b) || aerodrometicksbloom.TestAerodromePoolMint(&b)
			},
			FilterTopics: [][]common.Hash{{
				aerodromeabi.AerodromeABI.Events["Burn"].ID,
				aerodromeabi.AerodromeABI.Events["Mint"].ID,
			}},
		},
	}
}

// getOmniExchangeData returns the ForkData for the OmniExchange V3 fork.
func getOmniExchangeData() ForkData {
	return ForkData{
		System: SystemFuncs{
			PoolInitializer:      omniexchangecontracthelpers.NewPoolInitializer,
			PoolInfoProvider:     omniexchangecontracthelpers.NewBatchPoolInfoProvider,
			DiscoverPools:        omniexchangepoollogs.DiscoverPools,
			ExtractSwaps:         omniexchangepoollogs.ExtractSwapsFromLogs,
			ExtractMintsAndBurns: omniexchangepoollogs.ExtractMintsAndBurnsFromLogs,
			TestBloom: func(b types.Bloom) bool {
				return omniexchangebloom.TestOmniExchangeSwap(b) || omniexchangeticksbloom.TestOmniExchangePoolMint(&b) || omniexchangeticksbloom.TestOmniExchangePoolBurn(&b)
			},
			FilterTopics: [][]common.Hash{{
				omniexchangeabi.OmniExchangeABI.Events["Swap"].ID,
				omniexchangeabi.OmniExchangeABI.Events["Burn"].ID,
				omniexchangeabi.OmniExchangeABI.Events["Mint"].ID,
			}},
		},
		TickIndexer: TickIndexerFuncs{
			TickBitmapProvider: omniexchangecontracthelpers.NewBitMapProvider,
			TickInfoProvider:   omniexchangecontracthelpers.NewTickInfoProvider,
			UpdatedInBlock:     omniexchangetickslogs.OmniExchangePoolsUpdatedInBlock,
			TestBloom: func(b types.Bloom) bool {
				return omniexchangeticksbloom.TestOmniExchangePoolBurn(&b) || omniexchangeticksbloom.TestOmniExchangePoolMint(&b)
			},
			FilterTopics: [][]common.Hash{{
				omniexchangeabi.OmniExchangeABI.Events["Burn"].ID,
				omniexchangeabi.OmniExchangeABI.Events["Mint"].ID,
			}},
		},
	}
}
