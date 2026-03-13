package router

import (
	"errors"
	"fmt"
	"math/big"
	"sync"

	"github.com/defistate/defistate/clients/chains/katana"
	"github.com/defistate/defistate/examples/swap-platform/chains/katana/server/app/router/bitset"
	uniswapv3 "github.com/defistate/defistate/protocols/uniswap-v3"
	uniswapv3math "github.com/defistate/defistate/protocols/uniswap-v3/math"
)

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

type Router struct {
	state                *katana.State
	tokenToIndex         map[uint64]int
	poolToIndex          map[uint64]int
	allGetAmountOutFuncs []GetAmountOutFunc
}

// a very simple router using our katana state
func NewRouter(
	state *katana.State,
) (*Router, error) {
	tokenToIndex := make(map[uint64]int, len(state.Tokens.All()))
	for i, id := range state.Graph.Tokens {
		tokenToIndex[id] = i
	}

	poolToIndex := make(map[uint64]int, len(state.Graph.Pools))
	for i, id := range state.Graph.Pools {
		poolToIndex[id] = i
	}

	allGetAmountOutFuncs := make([]GetAmountOutFunc, len(state.Graph.Pools))

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

		allGetAmountOutFuncs[poolIndex] = func(amountIn *big.Int, tokenInID, tokenOutID uint64) (*big.Int, error) {
			//@todo update math library to support the uniswapv3.Pool type.
			return uniswapv3math.GetAmountOut(amountIn, nil, tokenInID, uniswapv3.PoolView{
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
			})
		}

	}

	router := &Router{
		state:                state,
		tokenToIndex:         tokenToIndex,
		poolToIndex:          poolToIndex,
		allGetAmountOutFuncs: allGetAmountOutFuncs,
	}

	return router, nil

}

// GetAmountOutFunc is for high-fidelity quoting using big.Int.
type GetAmountOutFunc func(amountIn *big.Int, tokenInID, tokenOutID uint64) (*big.Int, error)

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
func (r *Router) FindBestSwapPath(
	tokenInID uint64,
	tokenOutID uint64,
	amountIn *big.Int,
	runs int,
) ([]TokenPoolPath, *big.Int, error) {

	// --- Step 1: Create a temporary, patched slice of swap functions ---
	getAmountOutFuncs := make([]GetAmountOutFunc, len(r.allGetAmountOutFuncs))
	copy(getAmountOutFuncs, r.allGetAmountOutFuncs)

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
		return nil, nil, nil // No path found between the two tokens.
	}

	return bestPath, new(big.Int).Set(state.costs[endIndex]), nil
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
