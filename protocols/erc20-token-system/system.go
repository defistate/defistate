package token

import (
	"sync"

	"github.com/ethereum/go-ethereum/common"
)

// TokenSystem provides a concurrency-safe layer for managing the TokenRegistry.
// It uses a sync.RWMutex to protect a single instance of the registry, allowing
// for multiple concurrent reads when no writes are active.
type TokenSystem struct {
	mu       sync.RWMutex
	registry *TokenRegistry
}

// NewTokenSystem creates and initializes a new, concurrency-safe TokenSystem.
func NewTokenSystem() *TokenSystem {
	return &TokenSystem{
		registry: NewTokenRegistry(),
	}
}

func NewTokenSystemFromViews(view []TokenView) (*TokenSystem, error) {
	registry, err := NewTokenRegistryFromViews(view)
	if err != nil {
		return nil, err
	}
	return &TokenSystem{
		registry: registry,
	}, nil
}

// AddToken adds a token to the registry in a thread-safe manner.
// It acquires a full write lock.
func (ts *TokenSystem) AddToken(addr common.Address, name, symbol string, decimals uint8) (uint64, error) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return addToken(addr, name, symbol, decimals, ts.registry)
}

// DeleteToken removes a token from the registry in a thread-safe manner.
// It acquires a full write lock.
func (ts *TokenSystem) DeleteToken(idToDelete uint64) error {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return deleteToken(idToDelete, ts.registry)
}

// UpdateToken updates token data in a thread-safe manner.
// It acquires a full write lock.
func (ts *TokenSystem) UpdateToken(id uint64, fee float64, gas uint64) error {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return updateToken(id, fee, gas, ts.registry)
}

// View returns a view of all tokens.
// It acquires a read lock, allowing multiple concurrent readers.
func (ts *TokenSystem) View() []TokenView {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	return viewRegistry(ts.registry)
}

// GetTokenByID performs a lookup for a single token.
// It acquires a read lock, allowing multiple concurrent readers.
func (ts *TokenSystem) GetTokenByID(id uint64) (TokenView, error) {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	return getTokenByID(id, ts.registry)
}

// GetTokenByAddress performs a lookup for a single token.
// It acquires a read lock, allowing multiple concurrent readers.
func (ts *TokenSystem) GetTokenByAddress(addr common.Address) (TokenView, error) {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	return getTokenByAddress(addr, ts.registry)
}
