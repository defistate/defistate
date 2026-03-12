package uniswapv3

import (
	"fmt"

	clients "github.com/defistate/defistate/clients"
	"github.com/defistate/defistate/engine"
	uniswapv3 "github.com/defistate/defistate/protocols/uniswap-v3"
	"github.com/ethereum/go-ethereum/common"
)

// Indexer is a concrete implementation of the clients.UniswapV3Indexer interface.
type Indexer struct{}

// New creates a new Indexer.
func New() *Indexer {
	return &Indexer{}
}

// Index creates an indexed Uniswap V3 system from a raw slice of pool views.
func (i *Indexer) Index(
	protocolID engine.ProtocolID,
	pools []uniswapv3.PoolView,
	tokenRegistry clients.IndexedTokenSystem,
	poolRegistry clients.IndexedPoolRegistry,
) (clients.IndexedUniswapV3, error) {
	return NewIndexableUniswapV3System(protocolID, pools, tokenRegistry, poolRegistry)
}

// IndexableUniswapV3System provides fast, indexed access to Uniswap V3 pool data.
// It uses an index-based storage pattern (SOA - Structure of Arrays style) to
// minimize pointer chasing and GC pressure.
type IndexableUniswapV3System struct {
	// pools is a contiguous block of memory holding all pool data.
	// Iterating this slice is extremely cache-efficient.
	pools []uniswapv3.Pool

	// byAddress maps a pool's address to its index in the 'pools' slice.
	// Storing 'int' instead of pointers means the GC does not need to scan this map.
	byAddress map[common.Address]int

	// byID maps a pool's unique ID to its index in the 'pools' slice.
	byID map[uint64]int
}

// NewIndexableUniswapV3System creates a new indexed Uniswap V3 system.
func NewIndexableUniswapV3System(
	protocolID engine.ProtocolID,
	sourcePools []uniswapv3.PoolView,
	tokenRegistry clients.IndexedTokenSystem,
	poolRegistry clients.IndexedPoolRegistry,
) (*IndexableUniswapV3System, error) {
	count := len(sourcePools)

	// 1. Single Allocation: Allocate all storage at once.
	// This ensures all Pool structs are adjacent in memory.
	storage := make([]uniswapv3.Pool, count)

	// 2. Pre-allocate maps to avoid resizing and rehashing during the loop.
	byAddress := make(map[common.Address]int, count)
	byID := make(map[uint64]int, count)

	for i, p := range sourcePools {
		// --- Registry Lookups ---
		// We perform these checks upfront to ensure data integrity.

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
		storage[i] = uniswapv3.Pool{
			Protocol: string(protocolID),
			IDs: uniswapv3.IDs{
				Pool:   p.ID,
				Token0: p.Token0,
				Token1: p.Token1,
			},
			Address:      poolAddress,
			Token0:       token0.Address,
			Token1:       token1.Address,
			Fee:          p.Fee,
			TickSpacing:  p.TickSpacing,
			Liquidity:    p.Liquidity,
			SqrtPriceX96: p.SqrtPriceX96,
			Tick:         p.Tick,
			Ticks:        p.Ticks,
		}

		// --- Indexing ---
		// Map the keys to the integer index 'i'.
		byAddress[poolAddress] = i
		byID[p.ID] = i
	}

	return &IndexableUniswapV3System{
		pools:     storage,
		byAddress: byAddress,
		byID:      byID,
	}, nil
}

// GetByID retrieves a pool by its unique ID.
func (ius *IndexableUniswapV3System) GetByID(id uint64) (uniswapv3.Pool, bool) {
	idx, ok := ius.byID[id]
	if !ok {
		return uniswapv3.Pool{}, false
	}
	// Return by value (copy). Since uniswapv3.Pool contains pointers (Big.Int, Slices),
	// this is a shallow copy of the pointers, which is efficient.
	return ius.pools[idx], true
}

// GetByAddress retrieves a pool by its address.
func (ius *IndexableUniswapV3System) GetByAddress(addr common.Address) (uniswapv3.Pool, bool) {
	idx, ok := ius.byAddress[addr]
	if !ok {
		return uniswapv3.Pool{}, false
	}
	return ius.pools[idx], true
}

// All returns the raw slice of all pools.
func (ius *IndexableUniswapV3System) All() []uniswapv3.Pool {
	all := make([]uniswapv3.Pool, len(ius.pools))
	copy(all, ius.pools)
	return all
}
