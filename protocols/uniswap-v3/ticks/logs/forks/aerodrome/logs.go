package pancakeswap

import (
	"github.com/defistate/defistate/protocols/uniswap-v3/abi/forks/aerodrome"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

var (
	// Pre-calculate the topic hashes and event definitions for efficient comparison.
	mintEvent = aerodrome.AerodromeABI.Events["Mint"]
	burnEvent = aerodrome.AerodromeABI.Events["Burn"]
)

// AerodromePoolsUpdatedInBlock parses a slice of logs and returns the pool addresses
// and a corresponding list of unique tick indexes that were updated via Mint or Burn events.
func AerodromePoolsUpdatedInBlock(logs []types.Log) (pools []common.Address, updatedTicks [][]int64, err error) {
	// Use a map to aggregate unique tick updates for each pool address.
	// The inner map[int64]struct{} acts as a set to prevent duplicate tick indexes.
	updatesByPool := make(map[common.Address]map[int64]struct{})

	for _, logger := range logs {
		// A valid Uniswap V3 Mint/Burn event must have at least 4 topics:
		// Topics[0]: Event Signature
		// Topics[1]: owner (address)
		// Topics[2]: tickLower (int24)
		// Topics[3]: tickUpper (int24)
		if len(logger.Topics) < 4 {
			continue
		}

		// Check if the logger is a Mint or Burn event.
		switch logger.Topics[0] {
		case mintEvent.ID, burnEvent.ID:
			tickLower := logger.Topics[2].Big().Int64()

			tickUpper := logger.Topics[3].Big().Int64()

			poolAddr := logger.Address
			if _, ok := updatesByPool[poolAddr]; !ok {
				updatesByPool[poolAddr] = make(map[int64]struct{})
			}

			// Add the ticks to the set for this pool.
			updatesByPool[poolAddr][tickLower] = struct{}{}
			updatesByPool[poolAddr][tickUpper] = struct{}{}
		default:
			// Not a Mint or Burn event, so we ignore it.
			continue
		}
	}

	// If no relevant logs were found, return nil to avoid empty slices.
	if len(updatesByPool) == 0 {
		return nil, nil, nil
	}

	// Convert the aggregated map into the required slice format for the return value.
	pools = make([]common.Address, 0, len(updatesByPool))
	updatedTicks = make([][]int64, 0, len(updatesByPool))

	for pool, ticksSet := range updatesByPool {
		pools = append(pools, pool)

		ticks := make([]int64, 0, len(ticksSet))
		for tick := range ticksSet {
			ticks = append(ticks, tick)
		}
		updatedTicks = append(updatedTicks, ticks)
	}

	// For now, we are skipping malformed logs and not returning errors.
	// The `errs` slice can be populated in the future if stricter parsing is needed.
	return pools, updatedTicks, nil
}
