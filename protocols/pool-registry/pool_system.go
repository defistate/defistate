package poolregistry

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/defistate/defistate/engine"
	"github.com/ethereum/go-ethereum/common"
)

// PoolSystem provides a concurrency-safe layer for managing the PoolRegistry.
// It uses a sync.RWMutex for writes and an atomic.Pointer for lock-free reads.
type PoolSystem struct {
	mu         sync.RWMutex
	registry   *PoolRegistry
	cachedView atomic.Pointer[PoolRegistryView] // Stores the complete view (Pools + Protocols)
}

// NewPoolSystem creates and initializes a new, concurrency-safe PoolSystem.
func NewPoolSystem() *PoolSystem {
	s := &PoolSystem{
		registry: NewPoolRegistry(),
	}
	// Initialize with an empty view.
	emptyView := s.registry.view()
	s.cachedView.Store(&emptyView)
	return s
}

// NewPoolSystemFromView restores a system from a complete snapshot.
func NewPoolSystemFromView(view PoolRegistryView) (*PoolSystem, error) {
	registry, err := NewPoolRegistryFromView(view)
	if err != nil {
		return nil, err
	}
	s := &PoolSystem{
		registry: registry,
	}
	// Initialize the cached view with the data from the new registry.
	initialView := registry.view()
	s.cachedView.Store(&initialView)
	return s, nil
}

// updateCachedView generates a fresh view and atomically updates the pointer.
// This must be called from within a write lock.
func (s *PoolSystem) updateCachedView() {
	newView := s.registry.view()
	s.cachedView.Store(&newView)
}

// --- Write Methods ---

// AddPool adds a pool using the semantic ProtocolID string.
func (s *PoolSystem) AddPool(key PoolKey, pID engine.ProtocolID) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id, err := s.registry.addPool(key, pID)
	if err != nil {
		return 0, err
	}
	s.updateCachedView()
	return id, nil
}

// AddPools adds multiple pools in a single atomic operation.
func (s *PoolSystem) AddPools(keys []PoolKey, pIDs []engine.ProtocolID) ([]uint64, []error) {
	if len(keys) != len(pIDs) {
		panic(fmt.Sprintf("mismatched input lengths: %d keys and %d protocols", len(keys), len(pIDs)))
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if len(keys) == 0 {
		return nil, nil
	}

	ids := make([]uint64, len(keys))
	errs := make([]error, len(keys))
	var changed bool
	var hasErrors bool

	for i, key := range keys {
		id, err := s.registry.addPool(key, pIDs[i])
		if err != nil {
			errs[i] = fmt.Errorf("pool %s: %w", key.String(), err)
			hasErrors = true
		} else {
			changed = true
			ids[i] = id
		}
	}

	if changed {
		s.updateCachedView()
	}

	if !hasErrors {
		return ids, nil
	}
	return ids, errs
}

// DeletePool removes a pool from the registry.
func (s *PoolSystem) DeletePool(poolID uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	err := s.registry.deletePool(poolID)
	if err != nil {
		return err
	}
	s.updateCachedView()
	return nil
}

// DeletePools removes multiple pools efficiently.
func (s *PoolSystem) DeletePools(poolIDs []uint64) []error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(poolIDs) == 0 {
		return nil
	}

	errs := make([]error, len(poolIDs))
	var changed bool
	var hasError bool

	for i, poolID := range poolIDs {
		if err := s.registry.deletePool(poolID); err != nil {
			errs[i] = fmt.Errorf("poolID %d: %w", poolID, err)
			hasError = true
		} else {
			changed = true
		}
	}

	if changed {
		s.updateCachedView()
	}

	if hasError {
		return errs
	}
	return nil
}

// AddToBlockList adds a pool key to the blocklist.
func (s *PoolSystem) AddToBlockList(key PoolKey) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.registry.addToBlockList(key)
	s.updateCachedView()
}

// AddManyToBlockList adds multiple keys to the blocklist.
func (s *PoolSystem) AddManyToBlockList(keys []PoolKey) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(keys) == 0 {
		return
	}
	for _, key := range keys {
		s.registry.addToBlockList(key)
	}
	s.updateCachedView()
}

// RemoveFromBlockList removes a key from the blocklist.
func (s *PoolSystem) RemoveFromBlockList(key PoolKey) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.registry.removeFromBlockList(key)
	s.updateCachedView()
}

// RemoveManyFromBlockList removes multiple keys from the blocklist.
func (s *PoolSystem) RemoveManyFromBlockList(keys []PoolKey) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(keys) == 0 {
		return
	}
	for _, key := range keys {
		s.registry.removeFromBlockList(key)
	}
	s.updateCachedView()
}

// --- Read Methods ---

// IsOnBlockList checks if a pool is blocklisted (thread-safe).
func (s *PoolSystem) IsOnBlockList(key PoolKey) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.registry.isOnBlockList(key)
}

// View returns a thread-safe snapshot of the complete registry state.
// This operation is lock-free.
func (s *PoolSystem) View() PoolRegistryView {
	viewPtr := s.cachedView.Load()
	view := *viewPtr

	// Deep copy the Pools slice to prevent external mutation affecting the cache
	poolsCopy := make([]PoolView, len(view.Pools))
	copy(poolsCopy, view.Pools)

	// Deep copy the Protocols map
	protocolsCopy := make(map[uint16]engine.ProtocolID, len(view.Protocols))
	for k, v := range view.Protocols {
		protocolsCopy[k] = v
	}

	return PoolRegistryView{
		Pools:     poolsCopy,
		Protocols: protocolsCopy,
	}
}

// GetID returns a pool's ID by its key (thread-safe).
func (s *PoolSystem) GetID(key PoolKey) (uint64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.registry.getID(key)
}

// GetKey returns a pool's key by its ID (thread-safe).
func (s *PoolSystem) GetKey(id uint64) (PoolKey, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.registry.getKey(id)
}

// GetProtocolID returns the semantic protocol string (thread-safe).
func (s *PoolSystem) GetProtocolID(id uint64) (engine.ProtocolID, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.registry.getProtocolID(id)
}

// GetByID returns a single PoolView by its ID in a thread-safe manner.
// It attempts a lock-free cache lookup first.
func (s *PoolSystem) GetByID(id uint64) (PoolView, error) {
	// 1. Lock-free optimization: Scan the atomic cache first.
	viewPtr := s.cachedView.Load()
	for _, pool := range viewPtr.Pools {
		if pool.ID == id {
			return pool, nil
		}
	}

	// 2. Fallback: If cache is somehow stale (race condition) or item is truly missing,
	// check the authoritative source with a read lock.
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.registry.getByID(id)
}

// --- Helpers for common Address usage ---

// AddPoolFromAddress adds a pool using a common.Address converted to a Key.
func (s *PoolSystem) AddPoolFromAddress(addr common.Address, pID engine.ProtocolID) (uint64, error) {
	return s.AddPool(AddressToPoolKey(addr), pID)
}

// GetIDFromAddress looks up an ID using a common.Address.
func (s *PoolSystem) GetIDFromAddress(addr common.Address) (uint64, error) {
	return s.GetID(AddressToPoolKey(addr))
}
