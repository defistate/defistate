package clients

import (
	"github.com/defistate/defistate/engine"
	token "github.com/defistate/defistate/protocols/erc20-token-system"
	poolregistry "github.com/defistate/defistate/protocols/pool-registry"
	uniswapv2 "github.com/defistate/defistate/protocols/uniswap-v2"
	uniswapv3 "github.com/defistate/defistate/protocols/uniswap-v3"
	"github.com/ethereum/go-ethereum/common"
)

// Logger defines a standard interface for structured, leveled logging.
type Logger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
}

// Client defines the interface that DefiState depends on.
type Client interface {
	State() <-chan *engine.State
	Err() <-chan error
}

// IndexedTokenSystem defines the methods for accessing indexed token data.
type IndexedTokenSystem interface {
	GetByID(id uint64) (token.TokenView, bool)
	GetByAddress(address common.Address) (token.TokenView, bool)
	All() []token.TokenView
}

// TokenIndexer defines the interface for any component that can index tokens.
type TokenIndexer interface {
	Index(tokens []token.TokenView) IndexedTokenSystem
}

// IndexedPoolRegistry defines the methods for accessing indexed pool registry data.
type IndexedPoolRegistry interface {
	GetByID(id uint64) (poolregistry.PoolView, bool)
	GetByAddress(address common.Address) (poolregistry.PoolView, bool)
	GetByPoolKey(key poolregistry.PoolKey) (poolregistry.PoolView, bool)
	All() []poolregistry.PoolView
	GetProtocols() map[uint16]engine.ProtocolID
}

// PoolRegistryIndexer defines the interface for any component that can index pool registries.
type PoolRegistryIndexer interface {
	Index(poolregistry.PoolRegistryView) IndexedPoolRegistry
}

// IndexedUniswapV2 defines the methods for accessing indexed Uniswap V2 pool data.
type IndexedUniswapV2 interface {
	GetByID(id uint64) (uniswapv2.Pool, bool)
	GetByAddress(address common.Address) (uniswapv2.Pool, bool)
	All() []uniswapv2.Pool
}

// UniswapV2Indexer defines the interface for any component that can index Uniswap V2 pools.
type UniswapV2Indexer interface {
	Index(
		pools []uniswapv2.PoolView,
		tokenRegistry IndexedTokenSystem,
		poolRegistry IndexedPoolRegistry,
	) (IndexedUniswapV2, error)
}

// IndexedUniswapV3 provides a unified, read-only view of all indexed Uniswap V3
// and V3-like pools. As the output of the UniswapV3Indexer, it contains merged
// data from the primary protocol and its forks, offering a consolidated state
// for querying.
// Always check Pool.Type to confirm the actual pool type
type IndexedUniswapV3 interface {
	GetByID(id uint64) (uniswapv3.Pool, bool)
	GetByAddress(address common.Address) (uniswapv3.Pool, bool)
	All() []uniswapv3.Pool
}

// UniswapV3Indexer defines the interface for any component that can index Uniswap V3 pools.
//
// This indexer is responsible for processing a merged list of pools from the primary
// Uniswap V3 protocol and its forks (e.g., PancakeswapV3). It returns a single,
// unified UniswapV3View, which simplifies downstream processing by providing a
// consolidated state. Pools from different protocols can be distinguished by their unique IDs.
// Always check Pool.Type to confirm the actual pool type
type UniswapV3Indexer interface {
	Index(
		pools []uniswapv3.PoolView,
		tokenRegistry IndexedTokenSystem,
		poolRegistry IndexedPoolRegistry,
	) (IndexedUniswapV3, error)
}

// ProtocolResolver handles the resolution of high-level protocol schemas
// from low-level pool identifiers. It centralizes the multi-step lookup logic.
type ProtocolResolver struct {
	protocolIDToSchema  map[engine.ProtocolID]engine.ProtocolSchema
	indexedPoolRegistry IndexedPoolRegistry
}

// NewProtocolResolver creates a new resolver instance.
func NewProtocolResolver(
	protocolIDToSchema map[engine.ProtocolID]engine.ProtocolSchema,
	registry IndexedPoolRegistry,
) *ProtocolResolver {
	return &ProtocolResolver{
		protocolIDToSchema:  protocolIDToSchema,
		indexedPoolRegistry: registry,
	}
}

// ResolveSchemaFromPoolID performs the full lookup chain to find the
// data schema for a specific pool ID.
//
// Lookup Chain:
// 1. PoolID -> PoolView (via Registry)
// 2. PoolView.Protocol (uint16) -> ProtocolID (string) (via Registry Mapping)
// 3. ProtocolID -> ProtocolSchema (via Engine Config)
func (pr *ProtocolResolver) ResolveSchemaFromPoolID(poolID uint64) (engine.ProtocolSchema, bool) {
	// 1. Get the pool from the registry
	pool, ok := pr.indexedPoolRegistry.GetByID(poolID)
	if !ok {
		return "", false
	}

	// 2. Get the protocol map from the registry
	protocols := pr.indexedPoolRegistry.GetProtocols()

	// 3. Resolve the internal uint16 ID to the engine's string ID
	protocolID, ok := protocols[pool.Protocol]
	if !ok {
		return "", false
	}

	// 4. Resolve the string ID to the schema
	schema, ok := pr.protocolIDToSchema[protocolID]
	return schema, ok
}

// ResolveSchema directly maps a known ProtocolID string to its schema.
func (pr *ProtocolResolver) ResolveSchema(protocolID engine.ProtocolID) (engine.ProtocolSchema, bool) {
	schema, exists := pr.protocolIDToSchema[protocolID]
	return schema, exists
}
