package uniswapv2

import (
	"fmt"

	clients "github.com/defistate/defistate/clients"
	"github.com/defistate/defistate/engine"
	uniswapv2 "github.com/defistate/defistate/protocols/uniswap-v2"
	"github.com/ethereum/go-ethereum/common"
)

// Indexer is a concrete implementation of the clients.UniswapV2Indexer interface.
type Indexer struct{}

// New creates a new Indexer.
func New() *Indexer {
	return &Indexer{}
}

// Index creates an indexed Uniswap V2 system from a raw slice of pool views,
// resolving IDs to full addresses using the provided registries.
func (i *Indexer) Index(
	protocolID engine.ProtocolID,
	pools []uniswapv2.PoolView,
	tokenRegistry clients.IndexedTokenSystem,
	poolRegistry clients.IndexedPoolRegistry,
) (clients.IndexedUniswapV2, error) {
	return NewIndexableUniswapV2System(protocolID, pools, tokenRegistry, poolRegistry)
}

// IndexableUniswapV2System provides fast, indexed access to Uniswap V2 pool data.
// It uses an index-based storage pattern (SOA - Structure of Arrays style) to
// minimize pointer chasing and GC pressure.
type IndexableUniswapV2System struct {
	// pools is a contiguous block of memory holding all pool data.
	// Iterating this slice is extremely cache-efficient.
	pools []uniswapv2.Pool

	// byAddress maps a pool's address to its index in the 'pools' slice.
	// Storing 'int' instead of pointers means the GC does not need to scan this map.
	byAddress map[common.Address]int

	// byID maps a pool's unique ID to its index in the 'pools' slice.
	byID map[uint64]int
}

// NewIndexableUniswapV2System creates a new indexed Uniswap V2 system.
func NewIndexableUniswapV2System(
	protocolID engine.ProtocolID,
	sourcePools []uniswapv2.PoolView,
	tokenRegistry clients.IndexedTokenSystem,
	poolRegistry clients.IndexedPoolRegistry,
) (*IndexableUniswapV2System, error) {
	count := len(sourcePools)

	// 1. Single Allocation: Allocate all storage at once.
	// This ensures all Pool structs are adjacent in memory.
	storage := make([]uniswapv2.Pool, count)

	// 2. Pre-allocate maps to avoid resizing and rehashing during the loop.
	byAddress := make(map[common.Address]int, count)
	byID := make(map[uint64]int, count)

	for i, p := range sourcePools {
		// --- Registry Lookups ---

		pr, ok := poolRegistry.GetByID(p.ID)
		if !ok {
			return nil, fmt.Errorf("pool with ID %d not found in indexed pool registry", p.ID)
		}

		poolAddress, err := pr.Key.ToAddress()
		if err != nil {
			return nil, fmt.Errorf("invalid address for pool ID %d: %w", p.ID, err)
		}

		token0, ok := tokenRegistry.GetByID(p.Token0)
		if !ok {
			return nil, fmt.Errorf("token0 with ID %d not found in indexed token registry", p.Token0)
		}

		token1, ok := tokenRegistry.GetByID(p.Token1)
		if !ok {
			return nil, fmt.Errorf("token1 with ID %d not found in indexed token registry", p.Token1)
		}

		// --- Construction ---
		// Write directly into the pre-allocated storage slice at index 'i'.
		storage[i] = uniswapv2.Pool{
			Protocol: string(protocolID),
			IDs: uniswapv2.IDs{
				Pool:   p.ID,
				Token0: p.Token0,
				Token1: p.Token1,
			},
			Address:  poolAddress,
			Token0:   token0.Address,
			Token1:   token1.Address,
			Reserve0: p.Reserve0,
			Reserve1: p.Reserve1,
			FeeBps:   p.FeeBps,
		}

		// --- Indexing ---
		// Map the keys to the integer index 'i'.
		byAddress[poolAddress] = i
		byID[p.ID] = i
	}

	return &IndexableUniswapV2System{
		pools:     storage,
		byAddress: byAddress,
		byID:      byID,
	}, nil
}

// GetByID retrieves a pool by its unique ID.
func (ius *IndexableUniswapV2System) GetByID(id uint64) (uniswapv2.Pool, bool) {
	idx, ok := ius.byID[id]
	if !ok {
		return uniswapv2.Pool{}, false
	}
	return ius.pools[idx], true
}

// GetByAddress retrieves a pool by its address.
func (ius *IndexableUniswapV2System) GetByAddress(addr common.Address) (uniswapv2.Pool, bool) {
	idx, ok := ius.byAddress[addr]
	if !ok {
		return uniswapv2.Pool{}, false
	}
	return ius.pools[idx], true
}

// All returns the a copy of the pools, note that big.Int pointers will be shared.
func (ius *IndexableUniswapV2System) All() []uniswapv2.Pool {
	all := make([]uniswapv2.Pool, 1)
	copy(all, ius.pools)
	return all
}
