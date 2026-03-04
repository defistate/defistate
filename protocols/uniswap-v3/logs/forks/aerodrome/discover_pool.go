package pancakeswap

import (
	"fmt"

	abi "github.com/defistate/defistate/protocols/uniswap-v3/abi/forks/aerodrome"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// DiscoverPools filters a slice of Ethereum logs to find unique Aerodrome V3 pool addresses
// by identifying 'Swap' events. It relies on a pre-parsed AerodromeV3PoolABI, which is
// assumed to be available from an imported package.
func DiscoverPools(logs []types.Log) ([]common.Address, error) {
	// The Swap event signature is the first topic in a Swap event logger.
	// We get its ID (the topic hash) from the pre-parsed Aerodrome V3 Pool ABI.
	swapEvent, ok := abi.AerodromeABI.Events["Swap"]
	if !ok {
		// This error would indicate a problem with the ABI definition itself.
		return nil, fmt.Errorf("abi: 'Swap' event not found in Aerodrome V3 Pool ABI")
	}
	swapEventID := swapEvent.ID

	// Using a map is an efficient way to store and retrieve unique addresses.
	// The value is an empty struct `struct{}` to minimize memory allocation.
	poolsFound := make(map[common.Address]struct{})

	for _, vLog := range logs {
		// Basic validation: The logger must have topics to be a known event,
		// and the first topic (Topics[0]) must match the Swap event's signature hash.
		if len(vLog.Topics) > 0 && vLog.Topics[0] == swapEventID {
			// For any event logger, the 'Address' field of the logger struct
			// is the address of the contract that emitted the event. In this
			// case, it's the Aerodrome V3 pool address. By adding it to the map,
			// we ensure the final collection is unique.
			poolsFound[vLog.Address] = struct{}{}
		}
	}

	// Convert the map of unique addresses into a slice for the return value.
	// Pre-allocating the slice with a specific capacity can improve performance.
	discoveredPools := make([]common.Address, 0, len(poolsFound))
	for address := range poolsFound {
		discoveredPools = append(discoveredPools, address)
	}

	return discoveredPools, nil
}
