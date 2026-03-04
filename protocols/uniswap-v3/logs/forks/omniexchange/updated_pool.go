package omniexchange

import (
	"fmt"
	"math/big"

	abi "github.com/defistate/defistate/protocols/uniswap-v3/abi/forks/omniexchange"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// ExtractSwapsFromLogs filters a slice of Ethereum logs, identifies Uniswap V3 'Swap' events,
// and unpacks their data to return the latest state (tick, liquidity, price) for each affected pool.
func ExtractSwapsFromLogs(logs []types.Log) (
	pools []common.Address,
	ticks []int64,
	liquidities []*big.Int,
	sqrtPricesX96 []*big.Int,
	err error,
) {
	// Retrieve the 'Swap' event definition from the pre-parsed ABI.
	swapEvent, ok := abi.OmniExchangeABI.Events["Swap"]
	if !ok {
		// This is a critical error, indicating the ABI definition is missing the event.
		return nil, nil, nil, nil, fmt.Errorf("abi: 'Swap' event not found in Uniswap V3 Pool ABI")
	}
	swapEventID := swapEvent.ID

	// Iterate through all logs provided for the block.
	for _, vLog := range logs {
		// A Swap event logger has a specific structure: at least one topic, where the
		// first topic is the hash of the Swap event signature.
		if len(vLog.Topics) > 0 && vLog.Topics[0] == swapEventID {
			// Unpack the non-indexed data from the logger. The ABI library handles the
			// decoding of the `data` field into the Go types defined in the ABI.
			// For a Swap event, these are: amount0, amount1, sqrtPriceX96, liquidity, tick.
			unpacked, err := abi.OmniExchangeABI.Unpack("Swap", vLog.Data)
			if err != nil {
				// If unpacking fails, the logger data might be malformed. We return an error
				// as this is an unexpected state.
				return nil, nil, nil, nil, fmt.Errorf("failed to unpack Swap event logger for pool %s: %w", vLog.Address.Hex(), err)
			}

			// Type assertions to extract data. The order and types must match the
			// non-indexed fields in the Swap event definition precisely.
			// The order is: amount0, amount1, sqrtPriceX96, liquidity, tick.
			sqrtPrice, okPrice := unpacked[2].(*big.Int)
			liquidity, okLiq := unpacked[3].(*big.Int)
			// FIX: The 'int24' tick value is unpacked as a *big.Int, not an int64.
			tickBigInt, okTick := unpacked[4].(*big.Int)

			if !okPrice || !okLiq || !okTick {
				return nil, nil, nil, nil, fmt.Errorf("type assertion failed for Swap event data in pool %s", vLog.Address.Hex())
			}

			// Append the extracted data to the result slices.
			// The pool address is the address of the contract that emitted the logger.
			pools = append(pools, vLog.Address)
			ticks = append(ticks, tickBigInt.Int64()) // Convert the *big.Int to int64.
			liquidities = append(liquidities, liquidity)
			sqrtPricesX96 = append(sqrtPricesX96, sqrtPrice)
		}
	}

	return pools, ticks, liquidities, sqrtPricesX96, nil
}

// ExtractMintsAndBurnsFromLogs filters a slice of Ethereum logs, identifies Uniswap V3
// 'Mint' and 'Burn' events, and returns a de-duplicated list of the pool addresses
// that had liquidity changes.
func ExtractMintsAndBurnsFromLogs(logs []types.Log) (pools []common.Address) {
	// These events are guaranteed to be in the ABI. If they are not, it's a
	// critical developer error, and the program should panic.
	mintEventID := abi.OmniExchangeABI.Events["Mint"].ID
	burnEventID := abi.OmniExchangeABI.Events["Burn"].ID
	seenPools := make(map[common.Address]struct{})

	for _, vLog := range logs {
		if len(vLog.Topics) > 0 {
			// Check if the logger is a Mint or Burn event.
			if vLog.Topics[0] == mintEventID || vLog.Topics[0] == burnEventID {
				// Check if we have already seen this pool address in this block.
				if _, seen := seenPools[vLog.Address]; !seen {
					pools = append(pools, vLog.Address)
					seenPools[vLog.Address] = struct{}{}
				}
			}
		}
	}

	return pools
}
