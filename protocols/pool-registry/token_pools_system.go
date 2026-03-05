package poolregistry

import (
	"fmt"
	"sync"
	"sync/atomic"
)

// TokenPoolSystem provides a concurrency-safe layer for managing the graph-based TokenPoolsRegistry.
// It uses a sync.RWMutex for writes and an atomic.Pointer for lock-free reads to ensure high performance.
type TokenPoolSystem struct {
	mu         sync.RWMutex
	registry   *TokenPoolsRegistry
	cachedView atomic.Pointer[TokenPoolsRegistryView] // Read-optimized cache for the registry view
}

// NewTokenPoolSystem creates and initializes a new, concurrency-safe TokenPoolSystem.
// It takes a threshold that determines when the internal graph structure should be compacted.
func NewTokenPoolSystem(compactionThreshold int) *TokenPoolSystem {
	s := &TokenPoolSystem{
		registry: NewTokenPoolsRegistry(compactionThreshold),
	}
	// Initialize the cached view with an empty, non-nil snapshot.
	s.cachedView.Store(s.registry.view())
	return s
}

// NewTokenPoolSystemFromView creates a concurrency-safe system from a snapshot view.
// It reconstructs the underlying registry and immediately initializes the read-optimized cache.
func NewTokenPoolSystemFromView(view *TokenPoolsRegistryView, compactionThreshold int) *TokenPoolSystem {
	registry := NewTokenPoolsRegistryFromView(view, compactionThreshold)
	s := &TokenPoolSystem{
		registry: registry,
	}
	// Initialize the cached view with the state from the restored registry.
	s.cachedView.Store(s.registry.view())
	return s
}

// updateCachedView generates a fresh view from the registry and atomically updates the pointer.
// This method MUST be called from within a write lock (s.mu.Lock).
func (s *TokenPoolSystem) updateCachedView() {
	newView := s.registry.view()
	s.cachedView.Store(newView)
}

// --- Write Methods ---

// AddPool adds a single liquidity pool. For multiple additions, use AddPools for better performance.
func (s *TokenPoolSystem) AddPool(tokenIDs []uint64, poolID uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.registry.add(tokenIDs, poolID)
	s.updateCachedView() // Update the atomic pointer after modification.
}

// AddPools adds multiple pools in a single, atomic, and efficient operation.
// It updates the cached view only once after all additions are complete.
// It will panic if the input slices have mismatched lengths, as this is a programmer error.
func (s *TokenPoolSystem) AddPools(poolIDs []uint64, tokenIDSets [][]uint64) {
	if len(poolIDs) != len(tokenIDSets) {
		// This is a programmer error, not a runtime error. The caller has violated
		// the function's contract. Panicking is the idiomatic Go way to handle this.
		panic(fmt.Sprintf("mismatched input lengths: %d pool IDs and %d token ID sets", len(poolIDs), len(tokenIDSets)))
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if len(poolIDs) == 0 {
		return
	}

	for i, poolID := range poolIDs {
		s.registry.add(tokenIDSets[i], poolID)
	}

	s.updateCachedView()
}

// RemovePool removes a single pool. For multiple removals, use RemovePools.
func (s *TokenPoolSystem) RemovePool(poolID uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.registry.removePool(poolID)
	s.updateCachedView()
}

// RemovePools removes multiple pools in a single, atomic, and efficient operation.
func (s *TokenPoolSystem) RemovePools(poolIDs []uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(poolIDs) == 0 {
		return
	}

	for _, poolID := range poolIDs {
		s.registry.removePool(poolID)
	}

	s.updateCachedView()
}

// RemoveToken removes a single token. For multiple removals, use RemoveTokens.
func (s *TokenPoolSystem) RemoveToken(tokenID uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.registry.removeToken(tokenID)
	s.updateCachedView()
}

// RemoveTokens removes multiple tokens in a single, atomic, and efficient operation.
func (s *TokenPoolSystem) RemoveTokens(tokenIDs []uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(tokenIDs) == 0 {
		return
	}

	for _, tokenID := range tokenIDs {
		s.registry.removeToken(tokenID)
	}

	s.updateCachedView()
}

func (s *TokenPoolSystem) PoolsForToken(tokenID uint64) []uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.registry.poolsForToken(tokenID)
}

// View returns a thread-safe, deep copy of the graph's core data structures.
// This operation is highly performant for concurrent reads and provides a safe,
// mutable snapshot for the caller.
func (s *TokenPoolSystem) View() *TokenPoolsRegistryView {
	// Atomically load the pointer to the cached, safe-to-read snapshot.
	cachedViewPtr := s.cachedView.Load()
	if cachedViewPtr == nil {
		return &TokenPoolsRegistryView{}
	}

	// Create a DEEP COPY of the cached view to return to the caller.
	// This prevents the caller from modifying the shared cache, ensuring absolute thread safety.
	tokensCopy := make([]uint64, len(cachedViewPtr.Tokens))
	copy(tokensCopy, cachedViewPtr.Tokens)

	poolsCopy := make([]uint64, len(cachedViewPtr.Pools))
	copy(poolsCopy, cachedViewPtr.Pools)

	adjacencyCopy := make([][]int, len(cachedViewPtr.Adjacency))
	for i, adj := range cachedViewPtr.Adjacency {
		adjCopy := make([]int, len(adj))
		copy(adjCopy, adj)
		adjacencyCopy[i] = adjCopy
	}

	edgeTargetsCopy := make([]int, len(cachedViewPtr.EdgeTargets))
	copy(edgeTargetsCopy, cachedViewPtr.EdgeTargets)

	edgePoolsCopy := make([][]int, len(cachedViewPtr.EdgePools))
	for i, poolList := range cachedViewPtr.EdgePools {
		listCopy := make([]int, len(poolList))
		copy(listCopy, poolList)
		edgePoolsCopy[i] = listCopy
	}

	return &TokenPoolsRegistryView{
		Tokens:      tokensCopy,
		Pools:       poolsCopy,
		Adjacency:   adjacencyCopy,
		EdgeTargets: edgeTargetsCopy,
		EdgePools:   edgePoolsCopy,
	}
}
