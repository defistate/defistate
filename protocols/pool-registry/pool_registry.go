package poolregistry

import (
	"errors"

	"github.com/defistate/defistate/engine"
)

var (
	ErrPoolNotFound            = errors.New("pool not found")
	ErrPoolBlocked             = errors.New("pool is on the blocklist")
	ErrDuplicateID             = errors.New("duplicate ID in view")
	ErrDuplicateKey            = errors.New("duplicate key in view") // Renamed
	ErrProtocolOverflow        = errors.New("max protocol limit reached (65535)")
	ErrProtocolMappingNotFound = errors.New("protocol mapping not found for internal ID")
)

// --- Registry Types ---

// PoolView represents the data for a single pool.
type PoolView struct {
	ID       uint64  `json:"id"`
	Key      PoolKey `json:"key"`      // Renamed from Identifier
	Protocol uint16  `json:"protocol"` // Internal uint16 representation
}

// PoolRegistryView represents the complete state of the registry.
type PoolRegistryView struct {
	Pools     []PoolView                   `json:"pools"`
	Protocols map[uint16]engine.ProtocolID `json:"protocols"`
}

// PoolRegistry is a simple, high-performance, data-oriented registry.
type PoolRegistry struct {
	// Parallel slices (Structure of Arrays)
	id       []uint64
	key      []PoolKey // Renamed from Identifier
	protocol []uint16

	// Lookups
	idToIndex  map[uint64]int
	keyToIndex map[PoolKey]int // Renamed from identifierToIndex

	// Dictionary Encoding Maps
	protocolToInternal map[engine.ProtocolID]uint16
	internalToProtocol map[uint16]engine.ProtocolID

	blockList map[PoolKey]struct{}
	nextID    uint64
	nextProto uint16
}

func NewPoolRegistry() *PoolRegistry {
	return &PoolRegistry{
		id:                 make([]uint64, 0),
		key:                make([]PoolKey, 0),
		protocol:           make([]uint16, 0),
		idToIndex:          make(map[uint64]int),
		keyToIndex:         make(map[PoolKey]int),
		protocolToInternal: make(map[engine.ProtocolID]uint16),
		internalToProtocol: make(map[uint16]engine.ProtocolID),
		blockList:          make(map[PoolKey]struct{}),
		nextID:             1,
		nextProto:          0,
	}
}

func NewPoolRegistryFromView(view PoolRegistryView) (*PoolRegistry, error) {
	numPools := len(view.Pools)
	r := NewPoolRegistry()

	// 1. Restore Protocol Dictionary
	var maxProto uint16 = 0
	for id, name := range view.Protocols {
		r.internalToProtocol[id] = name
		r.protocolToInternal[name] = id
		if id > maxProto {
			maxProto = id
		}
	}
	if len(view.Protocols) > 0 {
		r.nextProto = maxProto + 1
	}

	// 2. Pre-allocate
	r.id = make([]uint64, numPools)
	r.key = make([]PoolKey, numPools)
	r.protocol = make([]uint16, numPools)

	var maxID uint64 = 0

	// 3. Restore Pools
	for i, poolData := range view.Pools {
		if _, exists := r.idToIndex[poolData.ID]; exists {
			return nil, ErrDuplicateID
		}
		if _, exists := r.keyToIndex[poolData.Key]; exists {
			return nil, ErrDuplicateKey
		}
		if _, known := r.internalToProtocol[poolData.Protocol]; !known {
			return nil, ErrProtocolMappingNotFound
		}

		r.id[i] = poolData.ID
		r.key[i] = poolData.Key
		r.protocol[i] = poolData.Protocol

		r.idToIndex[poolData.ID] = i
		r.keyToIndex[poolData.Key] = i

		if poolData.ID > maxID {
			maxID = poolData.ID
		}
	}

	r.nextID = maxID + 1
	return r, nil
}

// --- Internal Helper ---

func (r *PoolRegistry) registerProtocol(pID engine.ProtocolID) (uint16, error) {
	if internalID, exists := r.protocolToInternal[pID]; exists {
		return internalID, nil
	}
	if r.nextProto == 65535 {
		return 0, ErrProtocolOverflow
	}
	newInternalID := r.nextProto
	r.nextProto++
	r.protocolToInternal[pID] = newInternalID
	r.internalToProtocol[newInternalID] = pID
	return newInternalID, nil
}

// --- Write Methods (Private) ---

func (r *PoolRegistry) addPool(key PoolKey, pID engine.ProtocolID) (uint64, error) {
	if r.isOnBlockList(key) {
		return 0, ErrPoolBlocked
	}
	if index, exists := r.keyToIndex[key]; exists {
		return r.id[index], nil
	}

	internalProtoID, err := r.registerProtocol(pID)
	if err != nil {
		return 0, err
	}

	poolID := r.nextID
	r.nextID++

	r.id = append(r.id, poolID)
	r.key = append(r.key, key)
	r.protocol = append(r.protocol, internalProtoID)

	newIndex := len(r.id) - 1
	r.idToIndex[poolID] = newIndex
	r.keyToIndex[key] = newIndex

	return poolID, nil
}

func (r *PoolRegistry) deletePool(poolID uint64) error {
	indexToDelete, ok := r.idToIndex[poolID]
	if !ok {
		return ErrPoolNotFound
	}
	keyToDelete := r.key[indexToDelete]

	lastIndex := len(r.id) - 1
	lastElementKey := r.key[lastIndex]
	lastElementID := r.id[lastIndex]
	lastElementProto := r.protocol[lastIndex]

	r.id[indexToDelete] = lastElementID
	r.key[indexToDelete] = lastElementKey
	r.protocol[indexToDelete] = lastElementProto

	r.idToIndex[lastElementID] = indexToDelete
	r.keyToIndex[lastElementKey] = indexToDelete

	delete(r.idToIndex, poolID)
	delete(r.keyToIndex, keyToDelete)

	r.id = r.id[:lastIndex]
	r.key = r.key[:lastIndex]
	r.protocol = r.protocol[:lastIndex]

	return nil
}

func (r *PoolRegistry) addToBlockList(key PoolKey) {
	r.blockList[key] = struct{}{}
}

func (r *PoolRegistry) removeFromBlockList(key PoolKey) {
	delete(r.blockList, key)
}

// --- Read Methods (Private) ---

func (r *PoolRegistry) isOnBlockList(key PoolKey) bool {
	_, forbidden := r.blockList[key]
	return forbidden
}

func (r *PoolRegistry) view() PoolRegistryView {
	pools := make([]PoolView, len(r.id))
	for i := 0; i < len(r.id); i++ {
		pools[i] = PoolView{
			ID:       r.id[i],
			Key:      r.key[i],
			Protocol: r.protocol[i],
		}
	}

	protocols := make(map[uint16]engine.ProtocolID, len(r.internalToProtocol))
	for k, v := range r.internalToProtocol {
		protocols[k] = v
	}

	return PoolRegistryView{
		Pools:     pools,
		Protocols: protocols,
	}
}

func (r *PoolRegistry) getID(key PoolKey) (uint64, error) {
	if index, ok := r.keyToIndex[key]; ok {
		return r.id[index], nil
	}
	return 0, ErrPoolNotFound
}

func (r *PoolRegistry) getKey(id uint64) (PoolKey, error) {
	if index, ok := r.idToIndex[id]; ok {
		return r.key[index], nil
	}
	return PoolKey{}, ErrPoolNotFound
}

func (r *PoolRegistry) getProtocolID(poolID uint64) (engine.ProtocolID, error) {
	index, ok := r.idToIndex[poolID]
	if !ok {
		return "", ErrPoolNotFound
	}
	internalID := r.protocol[index]
	pID, ok := r.internalToProtocol[internalID]
	if !ok {
		return "", ErrProtocolMappingNotFound
	}
	return pID, nil
}

func (r *PoolRegistry) getByID(id uint64) (PoolView, error) {
	index, ok := r.idToIndex[id]
	if !ok {
		return PoolView{}, ErrPoolNotFound
	}
	return PoolView{
		ID:       r.id[index],
		Key:      r.key[index],
		Protocol: r.protocol[index],
	}, nil
}
