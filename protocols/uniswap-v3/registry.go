package uniswapv3

import (
	"errors"
	"fmt"
	"math/big"
)

var (
	// ErrPoolExists is returned when attempting to add a pool that is already in the registry.
	ErrPoolExists = errors.New("pool already exists in registry")
	// ErrPoolNotFound is returned when attempting to access a pool that is not in the registry.
	ErrPoolNotFound = errors.New("pool not found in registry")
)

// DependencyError is returned when a function dependency required by the registry fails.
type DependencyError struct {
	// Dependency is the name of the function that failed (e.g., "poolAddressToID").
	Dependency string
	// Input is the value that was passed to the failing dependency.
	Input any
	// Err is the underlying error returned by the dependency.
	Err error
}

func (e *DependencyError) Error() string {
	return fmt.Sprintf("registry dependency '%s' failed for input '%v': %v", e.Dependency, e.Input, e.Err)
}

// Unwrap allows the error to be inspected with errors.Is and errors.As.
func (e *DependencyError) Unwrap() error {
	return e.Err
}

// PoolViewMinimal provides a view of a single Uniswap V3 pool's data.
type PoolViewMinimal struct {
	ID           uint64   `json:"id"`
	Token0       uint64   `json:"token0"`
	Token1       uint64   `json:"token1"`
	Fee          uint64   `json:"fee"`
	TickSpacing  uint64   `json:"tickSpacing"`
	Tick         int64    `json:"tick"`
	Liquidity    *big.Int `json:"liquidity"`
	SqrtPriceX96 *big.Int `json:"sqrtPriceX96"`
}

// UniswapV3Registry manages a large number of v3 pools using a data-oriented design.
type UniswapV3Registry struct {
	id            []uint64
	token0        []uint64
	token1        []uint64
	fee           []uint64
	tickSpacing   []uint64
	tick          []int64
	liquidity     []*big.Int
	sqrtPriceX96  []*big.Int
	lastUpdatedAt []uint64 // approximation of last time pool was updated

	// --- Mapping layers to separate logical ID from physical index ---
	idToIndex map[uint64]int // Maps a permanent ID to its current slice index
}

// NewUniswapV3Registry creates and initializes a new UniswapV3Registry.
func NewUniswapV3Registry() *UniswapV3Registry {
	return &UniswapV3Registry{
		idToIndex: make(map[uint64]int),
	}
}

// NewUniswapV3RegistryFromViews reconstructs a UniswapV3Registry from a slice of PoolViewMinimal structs.
// This function is the primary mechanism for rehydrating the registry's state from a snapshot.
// It pre-allocates all memory and performs deep copies of mutable data (*big.Int) for performance and safety.
func NewUniswapV3RegistryFromViews(views []PoolViewMinimal) *UniswapV3Registry {
	if len(views) == 0 {
		return NewUniswapV3Registry()
	}

	numPools := len(views)
	registry := &UniswapV3Registry{
		id:            make([]uint64, numPools),
		token0:        make([]uint64, numPools),
		token1:        make([]uint64, numPools),
		fee:           make([]uint64, numPools),
		tickSpacing:   make([]uint64, numPools),
		tick:          make([]int64, numPools),
		liquidity:     make([]*big.Int, numPools),
		sqrtPriceX96:  make([]*big.Int, numPools),
		lastUpdatedAt: make([]uint64, numPools),
		idToIndex:     make(map[uint64]int, numPools),
	}

	for i, view := range views {
		registry.id[i] = view.ID
		registry.token0[i] = view.Token0
		registry.token1[i] = view.Token1
		registry.fee[i] = view.Fee
		registry.tickSpacing[i] = view.TickSpacing
		registry.tick[i] = view.Tick

		// CRITICAL: Perform a deep copy of the big.Int pointers to ensure data integrity
		// and prevent the new registry from sharing memory with the input view.
		registry.liquidity[i] = new(big.Int).Set(view.Liquidity)
		registry.sqrtPriceX96[i] = new(big.Int).Set(view.SqrtPriceX96)

		// Rebuild the lookup map.
		registry.idToIndex[view.ID] = i
	}

	return registry
}

// addPool adds a new pool to the registry with its static data.
// Dynamic data like liquidity, tick, and sqrtPriceX96 are initialized to zero values.
func addPool(
	poolID,
	token0ID,
	token1ID,
	fee,
	tickSpacing uint64,
	registry *UniswapV3Registry,
) error {
	if _, ok := registry.idToIndex[poolID]; ok {
		return ErrPoolExists
	}

	registry.id = append(registry.id, poolID)
	registry.token0 = append(registry.token0, token0ID)
	registry.token1 = append(registry.token1, token1ID)
	registry.fee = append(registry.fee, fee)
	registry.tickSpacing = append(registry.tickSpacing, tickSpacing)
	registry.tick = append(registry.tick, 0)                             // Initial tick
	registry.liquidity = append(registry.liquidity, big.NewInt(0))       // Initial liquidity
	registry.sqrtPriceX96 = append(registry.sqrtPriceX96, big.NewInt(0)) // Initial sqrtPriceX96
	registry.lastUpdatedAt = append(registry.lastUpdatedAt, 0)           // Initial lastUpdatedAt

	newIndex := len(registry.id) - 1
	registry.idToIndex[poolID] = newIndex

	return nil
}

// updatePool updates the dynamic data of an existing pool in the registry.
func updatePool(
	poolID uint64,
	fee uint64,
	tick int64,
	liquidity,
	sqrtPriceX96 *big.Int,
	registry *UniswapV3Registry,
) error {
	index, ok := registry.idToIndex[poolID]
	if !ok {
		return ErrPoolNotFound
	}

	registry.fee[index] = fee
	registry.tick[index] = tick
	registry.liquidity[index].Set(liquidity)
	registry.sqrtPriceX96[index].Set(sqrtPriceX96)

	return nil
}

// deletePool removes a pool from the registry using the swap-and-pop method.
func deletePool(poolID uint64, registry *UniswapV3Registry) error {
	indexToDelete, ok := registry.idToIndex[poolID]
	if !ok {
		return ErrPoolNotFound
	}

	lastIndex := len(registry.id) - 1
	lastPoolID := registry.id[lastIndex]

	// If the pool to delete is not the last one, swap it with the last one
	if indexToDelete != lastIndex {
		registry.id[indexToDelete] = lastPoolID
		registry.token0[indexToDelete] = registry.token0[lastIndex]
		registry.token1[indexToDelete] = registry.token1[lastIndex]
		registry.fee[indexToDelete] = registry.fee[lastIndex]
		registry.tickSpacing[indexToDelete] = registry.tickSpacing[lastIndex]
		registry.tick[indexToDelete] = registry.tick[lastIndex]
		registry.liquidity[indexToDelete] = registry.liquidity[lastIndex]
		registry.sqrtPriceX96[indexToDelete] = registry.sqrtPriceX96[lastIndex]
		registry.lastUpdatedAt[indexToDelete] = registry.lastUpdatedAt[lastIndex]

		// Update the index mapping for the swapped pool
		registry.idToIndex[lastPoolID] = indexToDelete
	}

	// Delete the old pool's ID from the map
	delete(registry.idToIndex, poolID)

	// Truncate the slices to remove the last element
	registry.id = registry.id[:lastIndex]
	registry.token0 = registry.token0[:lastIndex]
	registry.token1 = registry.token1[:lastIndex]
	registry.fee = registry.fee[:lastIndex]
	registry.tickSpacing = registry.tickSpacing[:lastIndex]
	registry.tick = registry.tick[:lastIndex]
	registry.liquidity = registry.liquidity[:lastIndex]
	registry.sqrtPriceX96 = registry.sqrtPriceX96[:lastIndex]
	registry.lastUpdatedAt = registry.lastUpdatedAt[:lastIndex]

	return nil
}

// getPoolById retrieves a single pool's view by its permanent ID.
func getPoolById(
	poolID uint64,
	registry *UniswapV3Registry,
) (PoolViewMinimal, error) {
	index, ok := registry.idToIndex[poolID]
	if !ok {
		return PoolViewMinimal{}, ErrPoolNotFound
	}

	view := PoolViewMinimal{
		ID:           registry.id[index],
		Token0:       registry.token0[index],
		Token1:       registry.token1[index],
		Fee:          registry.fee[index],
		TickSpacing:  registry.tickSpacing[index],
		Tick:         registry.tick[index],
		Liquidity:    new(big.Int).Set(registry.liquidity[index]),
		SqrtPriceX96: new(big.Int).Set(registry.sqrtPriceX96[index]),
	}

	return view, nil
}

// viewRegistry returns a slice of PoolViewMinimal for all pools currently in the registry.
func viewRegistry(
	registry *UniswapV3Registry,
) []PoolViewMinimal {
	numPools := len(registry.id)
	if numPools == 0 {
		return nil
	}

	views := make([]PoolViewMinimal, numPools)
	for i := range numPools {
		views[i] = PoolViewMinimal{
			ID:           registry.id[i],
			Token0:       registry.token0[i],
			Token1:       registry.token1[i],
			Fee:          registry.fee[i],
			TickSpacing:  registry.tickSpacing[i],
			Tick:         registry.tick[i],
			Liquidity:    new(big.Int).Set(registry.liquidity[i]),
			SqrtPriceX96: new(big.Int).Set(registry.sqrtPriceX96[i]),
		}
	}
	return views
}

func hasPool(
	poolID uint64,
	registry *UniswapV3Registry,
) bool {
	_, ok := registry.idToIndex[poolID]
	return ok
}

// setLastUpdatedAt updates the approximate last updated time for a pool
func setLastUpdatedAt(
	poolID uint64,
	time uint64,
	registry *UniswapV3Registry,
) error {
	index, ok := registry.idToIndex[poolID]
	if !ok {
		return ErrPoolNotFound
	}

	registry.lastUpdatedAt[index] = time
	return nil
}

// getLastUpdatedAt retrieves the approximate last updated time for a pool.
func getLastUpdatedAt(
	poolID uint64,
	registry *UniswapV3Registry,
) (uint64, error) {
	index, ok := registry.idToIndex[poolID]
	if !ok {
		return 0, ErrPoolNotFound
	}

	return registry.lastUpdatedAt[index], nil
}

// getLastUpdatedAtMap returns a map of all pool IDs to their last updated timestamps.
func getLastUpdatedAtMap(
	registry *UniswapV3Registry,
) map[uint64]uint64 {
	numPools := len(registry.id)
	if numPools == 0 {
		return nil
	}

	updatedMap := make(map[uint64]uint64, numPools)
	for i := 0; i < numPools; i++ {
		updatedMap[registry.id[i]] = registry.lastUpdatedAt[i]
	}

	return updatedMap
}
