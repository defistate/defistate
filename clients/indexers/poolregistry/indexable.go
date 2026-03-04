package poolregistry

import (
	clients "github.com/defistate/defistate/clients"
	"github.com/defistate/defistate/engine"
	poolregistry "github.com/defistate/defistate/protocols/pool-registry"
	"github.com/ethereum/go-ethereum/common"
)

type Indexer struct{}

// New creates a new Indexer.
func New() *Indexer {
	return &Indexer{}
}

// Index creates an indexed pool registry from the full registry view.
// It indexes both the pools and the protocol map.
func (i *Indexer) Index(view poolregistry.PoolRegistryView) clients.IndexedPoolRegistry {
	return NewIndexablePoolRegistry(view)
}

// IndexablePoolRegistry provides fast, indexed access to pool registry data.
type IndexablePoolRegistry struct {
	byID      map[uint64]poolregistry.PoolView
	byKey     map[poolregistry.PoolKey]poolregistry.PoolView
	all       []poolregistry.PoolView
	protocols map[uint16]engine.ProtocolID
}

// NewIndexablePoolRegistry creates a new indexed pool registry from the view.
func NewIndexablePoolRegistry(view poolregistry.PoolRegistryView) *IndexablePoolRegistry {
	pools := view.Pools
	byID := make(map[uint64]poolregistry.PoolView, len(pools))
	byKey := make(map[poolregistry.PoolKey]poolregistry.PoolView, len(pools))

	for _, p := range pools {
		byID[p.ID] = p
		byKey[p.Key] = p
	}

	// Create a defensive copy of the protocols map to ensure immutability
	protocols := make(map[uint16]engine.ProtocolID, len(view.Protocols))
	for k, v := range view.Protocols {
		protocols[k] = v
	}

	return &IndexablePoolRegistry{
		byID:      byID,
		byKey:     byKey,
		all:       pools,
		protocols: protocols,
	}
}

// GetByID retrieves a pool by its unique ID.
func (ipr *IndexablePoolRegistry) GetByID(id uint64) (poolregistry.PoolView, bool) {
	p, ok := ipr.byID[id]
	return p, ok
}

// GetByAddress retrieves a pool by its contract address.
func (ipr *IndexablePoolRegistry) GetByAddress(address common.Address) (poolregistry.PoolView, bool) {
	p, ok := ipr.byKey[poolregistry.AddressToPoolKey(address)]
	return p, ok
}

// GetByPoolKey retrieves a pool by its poolregistry.PoolKey.
func (ipr *IndexablePoolRegistry) GetByPoolKey(key poolregistry.PoolKey) (poolregistry.PoolView, bool) {
	p, ok := ipr.byKey[key]
	return p, ok
}

// All returns a defensive copy of the slice of all pools in the system.
func (ipr *IndexablePoolRegistry) All() []poolregistry.PoolView {
	allCopy := make([]poolregistry.PoolView, len(ipr.all))
	copy(allCopy, ipr.all)
	return allCopy
}

// GetProtocols returns a safe, defensive copy of the protocol mapping.
// This ensures the internal state cannot be modified by the caller.
func (ipr *IndexablePoolRegistry) GetProtocols() map[uint16]engine.ProtocolID {
	// Defensive copy
	copyMap := make(map[uint16]engine.ProtocolID, len(ipr.protocols))
	for k, v := range ipr.protocols {
		copyMap[k] = v
	}
	return copyMap
}
