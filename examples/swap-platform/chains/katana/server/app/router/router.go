package router

import (
	"errors"
	"fmt"
	"math/big"
	"slices"
	"sync"

	"github.com/defistate/defistate/clients/chains/katana"
	"github.com/defistate/defistate/examples/swap-platform/chains/katana/server/app/router/bitset"
	uniswapv3 "github.com/defistate/defistate/protocols/uniswap-v3"
	uniswapv3math "github.com/defistate/defistate/protocols/uniswap-v3/math"
)

var ErrNoPathFound = errors.New("no swap path found")

// bigIntPool is a package-level pool for reusing *big.Int objects.
var bigIntPool = sync.Pool{
	New: func() any {
		return new(big.Int)
	},
}

type TokenPoolPath struct {
	TokenInID  uint64
	TokenOutID uint64
	PoolID     uint64
}

type GetAmountOutFunc func(amountIn *big.Int, tokenInID, tokenOutID uint64) (*big.Int, error)
type GetReservesFunc func(tokenInID, tokenOutID uint64) (reserveIn, reserveOut *big.Int, err error)

type Router struct {
	state                *katana.State
	tokenToIndex         map[uint64]int
	poolToIndex          map[uint64]int
	allGetAmountOutFuncs []GetAmountOutFunc
	allGetReservesFuncs  []GetReservesFunc
}

// a very simple router using our katana state
func NewRouter(
	state *katana.State,
) (*Router, error) {
	var tokenToIndex map[uint64]int
	var poolToIndex map[uint64]int

	{
		//@warning sort tokens and pools only for map creation.
		// do not use for anythin else!.
		tokens := append([]uint64(nil), state.Graph.Tokens...)
		slices.Sort(tokens)

		pools := append([]uint64(nil), state.Graph.Pools...)
		slices.Sort(pools)

		tokenToIndex = make(map[uint64]int, len(tokens))
		for i, id := range tokens {
			tokenToIndex[id] = i
		}

		poolToIndex = make(map[uint64]int, len(pools))
		for i, id := range pools {
			poolToIndex[id] = i
		}
	}

	allGetAmountOutFuncs := make([]GetAmountOutFunc, len(state.Graph.Pools))
	allGetReservesFuncs := make([]GetReservesFunc, len(state.Graph.Pools))
	// set allGetAmountOutFuncs for sushiV3 pools
	for _, pool := range state.SushiV3.All() {
		// get pool index from id
		poolIndex := poolToIndex[pool.IDs.Pool]
		// Check for Fee-On-Transfer tokens
		t0, ok0 := state.Tokens.GetByID(pool.IDs.Token0)
		t1, ok1 := state.Tokens.GetByID(pool.IDs.Token1)
		if (ok0 && t0.FeeOnTransferPercent > 0) || (ok1 && t1.FeeOnTransferPercent > 0) {
			// Fee-on-transfer tokens break standard amount out calculations.
			continue
		}

		p := uniswapv3.PoolView{
			PoolViewMinimal: uniswapv3.PoolViewMinimal{
				ID:           pool.IDs.Pool,
				Token0:       pool.IDs.Token0,
				Token1:       pool.IDs.Token1,
				Fee:          pool.Fee,
				TickSpacing:  pool.TickSpacing,
				Tick:         pool.Tick,
				Liquidity:    pool.Liquidity,
				SqrtPriceX96: pool.SqrtPriceX96,
			},
			Ticks: pool.Ticks,
		}

		allGetAmountOutFuncs[poolIndex] = func(amountIn *big.Int, tokenInID, tokenOutID uint64) (*big.Int, error) {
			//@todo update math library to support the uniswapv3.Pool type.
			return uniswapv3math.GetAmountOut(amountIn, nil, tokenInID, p)
		}

		allGetReservesFuncs[poolIndex] = func(tokenInID, tokenOutID uint64) (*big.Int, *big.Int, error) {
			reserveTokenOut, err := uniswapv3math.GetAmountOut(uniswapv3math.MaxUint256, nil, tokenInID, p)
			if err != nil {
				return nil, nil, err
			}

			reserveTokenIn, err := uniswapv3math.GetAmountOut(uniswapv3math.MaxUint256, nil, tokenOutID, p)
			if err != nil {
				return nil, nil, err
			}

			return reserveTokenIn, reserveTokenOut, nil
		}

	}

	router := &Router{
		state:                state,
		tokenToIndex:         tokenToIndex,
		poolToIndex:          poolToIndex,
		allGetAmountOutFuncs: allGetAmountOutFuncs,
		allGetReservesFuncs:  allGetReservesFuncs,
	}

	return router, nil

}

// GetPoolsForToken finds all pools connected to a given token by traversing the adjacency graph.
func (r *Router) GetPoolsForToken(tokenID uint64) []uint64 {
	tokenIndex, exists := r.tokenToIndex[tokenID]
	if !exists {
		return nil
	}
	edgeIndices := r.state.Graph.Adjacency[tokenIndex]
	if len(edgeIndices) == 0 {
		return nil
	}
	uniquePoolIDs := make(map[uint64]struct{})
	for _, edgeIndex := range edgeIndices {
		poolIndices := r.state.Graph.EdgePools[edgeIndex]
		for _, poolIndex := range poolIndices {
			poolID := r.state.Graph.Pools[poolIndex]
			uniquePoolIDs[poolID] = struct{}{}
		}
	}
	result := make([]uint64, 0, len(uniquePoolIDs))
	for id := range uniquePoolIDs {
		result = append(result, id)
	}
	return result
}

// findConversionPathState encapsulates the state required for the Bellman-Ford-like
// pathfinding algorithm used in GetExchangeRates.
type findConversionPathState struct {
	start                    int               // starting vertex index
	current                  int               // current vertex index being processed
	paths                    [][]TokenPoolPath // vertex index -> path to this token
	costs                    []*big.Int        // vertex index -> cost
	reserves                 []*big.Int        // vertex index -> reserve
	known                    []bitset.BitSet   // vertex index -> vertex index
	bestConnection           []int             // edge index -> pool index
	bestConnectionComputed   bitset.BitSet     // edge index -> whether the best connection has been computed
	reserveForBestConnection []*big.Int        // edge index -> reserve for the best connection
	temp                     *big.Int
}

// GetExchangeRates calculates the equivalent value of a given amount of a base token
// across all other tokens in the graph using a Bellman-Ford-like algorithm.
// It can be constrained to only propagate prices from a specific set of allowed source tokens.
func (r *Router) GetExchangeRates(
	baseAmountIn *big.Int,
	baseTokenID uint64,
	runs int,
	allowedSourceTokens map[uint64]struct{}, // New parameter
) (map[uint64]*big.Int, error) {
	//@todo account for fee of tokens with fee on transfer
	// Step 1: Find the internal index for the starting token.
	baseIndex, exists := r.tokenToIndex[baseTokenID]
	if !exists {
		return nil, fmt.Errorf("token %d not found in the graph", baseTokenID)
	}

	// Step 2: Initialize the state for the pathfinding search.
	numTokens := len(r.state.Graph.Tokens)
	numEdges := len(r.state.Graph.EdgePools)

	state := &findConversionPathState{
		start:                    baseIndex,
		paths:                    make([][]TokenPoolPath, numTokens),
		costs:                    make([]*big.Int, numTokens),
		known:                    make([]bitset.BitSet, numTokens),
		bestConnection:           make([]int, numEdges),
		bestConnectionComputed:   bitset.NewBitSet(uint64(numEdges)),
		reserveForBestConnection: make([]*big.Int, numEdges),
		reserves:                 make([]*big.Int, numTokens),
		temp:                     bigIntPool.Get().(*big.Int).SetUint64(0), // Get from pool
	}

	// This defer block ensures all temporary, pooled objects are returned.
	defer func() {
		bigIntPool.Put(state.temp.SetUint64(0))
		for _, r := range state.reserves {
			if r != nil {
				bigIntPool.Put(r.SetUint64(0))
			}
		}
		for _, r := range state.reserveForBestConnection {
			if r != nil {
				bigIntPool.Put(r.SetUint64(0))
			}
		}
	}()

	for i := range numTokens {
		state.known[i] = bitset.NewBitSet(uint64(numTokens))
		// Rent from pool for temporary state
		state.reserves[i] = bigIntPool.Get().(*big.Int).SetUint64(0) // ensure zero value
		// Allocate new for returned data
		state.costs[i] = new(big.Int)
	}
	state.costs[baseIndex].Set(baseAmountIn)
	for i := range numEdges {
		state.bestConnection[i] = -1 // -1 indicates no best connection yet
		// Rent from pool for temporary state
		state.reserveForBestConnection[i] = bigIntPool.Get().(*big.Int).SetUint64(0) // ensure zero value
	}

	// Step 3: Iteratively "relax" the edges for a set number of runs.
	for i := 0; i < runs; i++ {
		for j := 0; j < numTokens; j++ {
			if state.costs[j].Sign() == 0 {
				continue // Skip tokens that haven't been reached yet.
			}

			// Convert the internal index to the external token ID.
			currentTokenID := r.state.Graph.Tokens[j]
			// If a set of allowed source tokens is provided, check if the current
			// token is in that set before allowing it to propagate its price.
			if allowedSourceTokens != nil {
				if _, isAllowed := allowedSourceTokens[currentTokenID]; !isAllowed {
					continue // This token is not allowed to be a source.
				}
			}

			state.current = j
			if err := r.getExchangeRatesUsingMaxReservePath(state); err != nil {
				return nil, err
			}
		}
	}

	// Step 4: Convert the final costs slice back to a map for the user.
	finalExchangeRates := make(map[uint64]*big.Int, len(state.costs))
	for i, cost := range state.costs {
		if cost.Sign() != 0 {
			tokenID := r.state.Graph.Tokens[i]
			finalExchangeRates[tokenID] = cost
		}
	}

	// ensure baseToken equivalent equal to baseAmountIn
	finalExchangeRates[baseTokenID] = new(big.Int).Set(baseAmountIn)
	return finalExchangeRates, nil
}

// getExchangeRatesUsingMaxReservePath is the core of the algorithm. It uses the
// pre-computed swap functions for maximum performance.
// it sets connections based on maxReserve
func (r *Router) getExchangeRatesUsingMaxReservePath(
	state *findConversionPathState,
) error {
	currentIndex := state.current
	currentCost := state.costs[currentIndex]
	currentKnown := state.known[currentIndex]
	currentPath := state.paths[currentIndex]
	currentTokenID := r.state.Graph.Tokens[currentIndex]

	if currentKnown.IsSet(uint64(currentIndex)) {
		// we should never get here!
		return errors.New("cycle detected in path history")
	}

	// Iterate through all outgoing edges from the current token.
	for _, edgeIndex := range r.state.Graph.Adjacency[currentIndex] {
		targetIndex := r.state.Graph.EdgeTargets[edgeIndex]
		targetTokenID := r.state.Graph.Tokens[targetIndex]

		// Crucial cycle prevention: do not traverse to a token that is already in the current path.
		if currentKnown.IsSet(uint64(targetIndex)) {
			continue
		}

		bestReserve := state.temp

		if !state.bestConnectionComputed.IsSet(uint64(edgeIndex)) {
			// Iterate through all pools associated with this edge.
			bestConnection := -1
			bestReserve.SetUint64(0)
			for _, poolIndex := range r.state.Graph.EdgePools[edgeIndex] {
				getReserveFunc := r.allGetReservesFuncs[poolIndex]
				// can be nil
				if getReserveFunc == nil {
					continue
				}
				reserveIn, reserveOut, err := getReserveFunc(currentTokenID, targetTokenID)
				if err != nil {
					continue
				}

				// ensure that this pools has at least current cost
				if reserveIn.Cmp(currentCost) == -1 {
					continue
				}

				// we need the reserveOut
				if reserveOut.Cmp(bestReserve) == 1 {
					bestReserve.Set(reserveOut)
					bestConnection = poolIndex
				}
			}

			if bestConnection != -1 {
				// we have found a best connection for this edge (the pool with the highest reserve for targetID)
				state.bestConnection[edgeIndex] = bestConnection
				state.bestConnectionComputed.Set(uint64(edgeIndex))
				state.reserveForBestConnection[edgeIndex].Set(bestReserve)
			}
		}

		if state.bestConnection[edgeIndex] != -1 {
			poolIndex := state.bestConnection[edgeIndex]
			reserve := state.reserveForBestConnection[edgeIndex]

			if state.reserves[targetIndex].Cmp(reserve) == -1 {
				getAmountOut := r.allGetAmountOutFuncs[poolIndex]
				// can be nil
				if getAmountOut == nil {
					continue
				}
				amountOut, err := getAmountOut(currentCost, currentTokenID, targetTokenID)
				if err != nil || amountOut == nil || amountOut.Sign() <= 0 {
					continue
				}

				state.costs[targetIndex].Set(amountOut)
				poolID := r.state.Graph.Pools[poolIndex]
				newPath := make([]TokenPoolPath, len(currentPath)+1)
				copy(newPath, currentPath)
				newPath[len(currentPath)] = TokenPoolPath{
					TokenInID:  currentTokenID,
					TokenOutID: targetTokenID,
					PoolID:     poolID,
				}
				state.paths[targetIndex] = newPath
				state.known[targetIndex].SetFrom(currentKnown)
				state.known[targetIndex].Set(uint64(currentIndex))
				state.reserves[targetIndex].Set(reserve)
			}

		}
	}
	return nil
}

// findSwapPathsState encapsulates the state required for the Bellman-Ford-like
// swap path finding algorithm.
type findSwapPathsState struct {
	start   int
	current int
	end     int
	paths   [][]TokenPoolPath // vertex index -> path
	costs   []*big.Int        // vertex index -> cost
	known   []bitset.BitSet   // vertex index -> vertex index
	temp    *big.Int
}

// FindBestSwapPath searches the graph for the most profitable swap path between two tokens.
// It uses a "copy-and-patch" strategy to handle state overrides.

type PoolOverrides struct {
	SushiV3 map[uint64]uniswapv3.Pool
}

func (r *Router) FindBestSwapPath(
	runs int,
	amountIn *big.Int,
	tokenInID uint64,
	tokenOutID uint64,
	overrides *PoolOverrides,
) ([]TokenPoolPath, *big.Int, error) {

	// --- Step 1: Create a temporary, patched slice of swap functions ---
	getAmountOutFuncs := make([]GetAmountOutFunc, len(r.allGetAmountOutFuncs))
	copy(getAmountOutFuncs, r.allGetAmountOutFuncs)

	// handle overrides by updating getAmountOutFuncs
	for _, pool := range overrides.SushiV3 {
		// override getAmountOut function for that pool
		poolIndex, exists := r.poolToIndex[pool.IDs.Pool]
		if !exists {
			continue
		}
		if getAmountOutFuncs[poolIndex] == nil {
			// pool is inactive skip!
			continue
		}

		p := uniswapv3.PoolView{
			PoolViewMinimal: uniswapv3.PoolViewMinimal{
				ID:           pool.IDs.Pool,
				Token0:       pool.IDs.Token0,
				Token1:       pool.IDs.Token1,
				Fee:          pool.Fee,
				TickSpacing:  pool.TickSpacing,
				Tick:         pool.Tick,
				Liquidity:    pool.Liquidity,
				SqrtPriceX96: pool.SqrtPriceX96,
			},
			Ticks: pool.Ticks,
		}
		getAmountOutFuncs[poolIndex] = func(amountIn *big.Int, tokenInID, tokenOutID uint64) (*big.Int, error) {
			return uniswapv3math.GetAmountOut(amountIn, nil, tokenInID, p)
		}
	}

	// --- Step 2: Initialize and run the pathfinding algorithm ---
	startIndex, exists := r.tokenToIndex[tokenInID]
	if !exists {
		return nil, nil, fmt.Errorf("start tokenregistry %d not found in the graph", tokenInID)
	}

	endIndex, exists := r.tokenToIndex[tokenOutID]
	if !exists {
		return nil, nil, fmt.Errorf("end tokenregistry %d not found in the graph", tokenOutID)
	}

	numTokens := len(r.state.Graph.Tokens)
	state := &findSwapPathsState{
		start: startIndex,
		end:   endIndex,
		paths: make([][]TokenPoolPath, numTokens),
		costs: make([]*big.Int, numTokens),
		known: make([]bitset.BitSet, numTokens),
		temp:  bigIntPool.Get().(*big.Int).SetUint64(0),
	}

	// This defer block is CRITICAL. It ensures all rented objects are returned.
	defer func() {
		// Return the scratchpad int
		bigIntPool.Put(state.temp.SetUint64(0))
		// Return all integers used in the costs slice
		for _, cost := range state.costs {
			if cost != nil {
				bigIntPool.Put(cost.SetUint64(0))
			}
		}
	}()

	for i := range numTokens {
		state.known[i] = bitset.NewBitSet(uint64(numTokens))
		// Rent *big.Int objects from the pool instead of allocating new ones
		state.costs[i] = bigIntPool.Get().(*big.Int).SetUint64(0)

	}

	state.costs[startIndex].Set(amountIn)

	for i := 0; i < runs; i++ {
		for j := 0; j < numTokens; j++ {
			if state.costs[j].Sign() == 0 {
				continue
			}
			state.current = j
			if err := r.findSwapPath(state, getAmountOutFuncs); err != nil {
				return nil, nil, err
			}
		}
	}

	// --- Step 3: Reconstruct and return the best path found ---
	bestPath := state.paths[endIndex]
	if bestPath == nil {
		return nil, nil, ErrNoPathFound
	}

	return bestPath, new(big.Int).Set(state.costs[endIndex]), nil
}

func (r *Router) GetAmountOut(
	amountIn *big.Int,
	path []TokenPoolPath,
	overrides *PoolOverrides,
) (*big.Int, error) {

	// --- Step 1: Create a temporary, patched slice of swap functions ---
	getAmountOutFuncs := make([]GetAmountOutFunc, len(r.allGetAmountOutFuncs))
	copy(getAmountOutFuncs, r.allGetAmountOutFuncs)

	// handle overrides by updating getAmountOutFuncs
	for _, pool := range overrides.SushiV3 {
		// override getAmountOut function for that pool
		poolIndex, exists := r.poolToIndex[pool.IDs.Pool]
		if !exists {
			continue
		}
		if getAmountOutFuncs[poolIndex] == nil {
			// pool is inactive skip!
			continue
		}

		p := uniswapv3.PoolView{
			PoolViewMinimal: uniswapv3.PoolViewMinimal{
				ID:           pool.IDs.Pool,
				Token0:       pool.IDs.Token0,
				Token1:       pool.IDs.Token1,
				Fee:          pool.Fee,
				TickSpacing:  pool.TickSpacing,
				Tick:         pool.Tick,
				Liquidity:    pool.Liquidity,
				SqrtPriceX96: pool.SqrtPriceX96,
			},
			Ticks: pool.Ticks,
		}
		getAmountOutFuncs[poolIndex] = func(amountIn *big.Int, tokenInID, tokenOutID uint64) (*big.Int, error) {
			return uniswapv3math.GetAmountOut(amountIn, nil, tokenInID, p)
		}
	}

	amountIn = new(big.Int).Set(amountIn)

	for _, p := range path {
		poolIndex, ok := r.poolToIndex[p.PoolID]
		if !ok {
			return nil, fmt.Errorf("pool %d unknown", p.PoolID)
		}
		getAmountOut := getAmountOutFuncs[poolIndex]
		if getAmountOut == nil {
			return nil, fmt.Errorf(" getAmountOut not found for pool %d", p.PoolID)
		}
		amountOut, err := getAmountOut(amountIn, p.TokenInID, p.TokenInID)
		if err != nil {
			return nil, err
		}
		amountIn.Set(amountOut)
	}
	return amountIn, nil
}

// findSwapPath is the core Bellman-Ford-like relaxation step for finding the best swap paths.
func (r *Router) findSwapPath(state *findSwapPathsState, getAmountOutFuncs []GetAmountOutFunc) error {
	currentIndex := state.current
	currentCost := state.costs[currentIndex]
	currentKnown := state.known[currentIndex]
	currentPath := state.paths[currentIndex]
	currentTokenID := r.state.Graph.Tokens[currentIndex]

	if currentKnown.IsSet(uint64(currentIndex)) {
		return errors.New("cycle detected in path history")
	}

	maxAmountOut := state.temp
	for _, edgeIndex := range r.state.Graph.Adjacency[currentIndex] {
		targetIndex := r.state.Graph.EdgeTargets[edgeIndex]

		if currentKnown.IsSet(uint64(targetIndex)) {
			continue
		}

		targetTokenID := r.state.Graph.Tokens[targetIndex]
		bestPoolIndex := -1
		maxAmountOut.SetUint64(0)
		for _, poolIndex := range r.state.Graph.EdgePools[edgeIndex] {
			getAmountOut := getAmountOutFuncs[poolIndex]
			if getAmountOut == nil {
				continue
			}

			amountOut, err := getAmountOut(currentCost, currentTokenID, targetTokenID)
			if err == nil && amountOut.Cmp(maxAmountOut) == 1 {
				maxAmountOut.Set(amountOut)
				bestPoolIndex = poolIndex
			}
		}

		if bestPoolIndex == -1 {
			continue

		}
		if maxAmountOut.Cmp(state.costs[targetIndex]) == 1 {
			state.costs[targetIndex].Set(maxAmountOut)
			poolID := r.state.Graph.Pools[bestPoolIndex]
			newPath := make([]TokenPoolPath, len(currentPath)+1)
			copy(newPath, currentPath)
			newPath[len(currentPath)] = TokenPoolPath{
				TokenInID:  currentTokenID,
				TokenOutID: targetTokenID,
				PoolID:     poolID,
			}
			state.paths[targetIndex] = newPath
			state.known[targetIndex].SetFrom(currentKnown)
			state.known[targetIndex].Set(uint64(currentIndex))
		}
	}
	return nil
}

// equalTokenPoolPaths compares two paths to see if they are identical.
func EqualTokenPoolPaths(a, b []TokenPoolPath) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].TokenInID != b[i].TokenInID || a[i].TokenOutID != b[i].TokenOutID || a[i].PoolID != b[i].PoolID {
			return false
		}
	}
	return true
}
