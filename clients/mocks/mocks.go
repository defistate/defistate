package mocks

import (
	"sync"

	"github.com/defistate/defistate/engine"
	token "github.com/defistate/defistate/protocols/erc20-token-system"
	poolregistry "github.com/defistate/defistate/protocols/pool-registry"
	"github.com/ethereum/go-ethereum/common"
)

// MockIndexedTokenSystem provides a simple, map-backed implementation for testing.
type MockIndexedTokenSystem struct {
	byID      map[uint64]token.TokenView
	byAddress map[common.Address]token.TokenView
}

// NewMockIndexedTokenSystem creates a system pre-populated with the provided tokens.
func NewMockIndexedTokenSystem(tokens []token.TokenView) *MockIndexedTokenSystem {
	m := &MockIndexedTokenSystem{
		byID:      make(map[uint64]token.TokenView),
		byAddress: make(map[common.Address]token.TokenView),
	}
	m.Add(tokens...)
	return m
}

// Add allows adding one or many tokens to the mock system.
func (m *MockIndexedTokenSystem) Add(tokens ...token.TokenView) {
	for _, t := range tokens {
		m.byID[t.ID] = t
		m.byAddress[t.Address] = t
	}
}

func (m *MockIndexedTokenSystem) GetByID(id uint64) (token.TokenView, bool) {
	t, ok := m.byID[id]
	return t, ok
}

func (m *MockIndexedTokenSystem) GetByAddress(address common.Address) (token.TokenView, bool) {
	t, ok := m.byAddress[address]
	return t, ok
}

func (m *MockIndexedTokenSystem) All() []token.TokenView {
	tokens := make([]token.TokenView, 0, len(m.byID))
	for _, t := range m.byID {
		tokens = append(tokens, t)
	}
	return tokens
}

// MockIndexedPoolRegistry provides a thread-safe, map-backed implementation for testing.
type MockIndexedPoolRegistry struct {
	mu          sync.RWMutex
	byID        map[uint64]poolregistry.PoolView
	byKey       map[poolregistry.PoolKey]poolregistry.PoolView
	byAddress   map[common.Address]poolregistry.PoolView
	protocolMap map[uint16]engine.ProtocolID
}

// NewMockIndexedPoolRegistry creates a registry pre-populated with the provided view.
func NewMockIndexedPoolRegistry(view poolregistry.PoolRegistryView) *MockIndexedPoolRegistry {
	m := &MockIndexedPoolRegistry{
		byID:        make(map[uint64]poolregistry.PoolView),
		byKey:       make(map[poolregistry.PoolKey]poolregistry.PoolView),
		byAddress:   make(map[common.Address]poolregistry.PoolView),
		protocolMap: view.Protocols,
	}
	if m.protocolMap == nil {
		m.protocolMap = make(map[uint16]engine.ProtocolID)
	}
	m.Add(view.Pools...)
	return m
}

// Add injects pools into the mock and handles Address indexing if the key is ABI-aligned.
func (m *MockIndexedPoolRegistry) Add(pools ...poolregistry.PoolView) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, p := range pools {
		m.byID[p.ID] = p
		m.byKey[p.Key] = p
		if addr, err := p.Key.ToAddress(); err == nil {
			m.byAddress[addr] = p
		}
	}
}

func (m *MockIndexedPoolRegistry) GetByID(id uint64) (poolregistry.PoolView, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.byID[id]
	return p, ok
}

func (m *MockIndexedPoolRegistry) GetByAddress(address common.Address) (poolregistry.PoolView, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.byAddress[address]
	return p, ok
}

func (m *MockIndexedPoolRegistry) GetByPoolKey(key poolregistry.PoolKey) (poolregistry.PoolView, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.byKey[key]
	return p, ok
}

func (m *MockIndexedPoolRegistry) All() []poolregistry.PoolView {
	m.mu.RLock()
	defer m.mu.RUnlock()
	pools := make([]poolregistry.PoolView, 0, len(m.byID))
	for _, p := range m.byID {
		pools = append(pools, p)
	}
	return pools
}

func (m *MockIndexedPoolRegistry) GetProtocols() map[uint16]engine.ProtocolID {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.protocolMap
}
