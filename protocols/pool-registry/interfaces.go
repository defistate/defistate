package poolregistry

import (
	"github.com/defistate/defistate/engine"
	token "github.com/defistate/defistate/protocols/erc20-token-system"
	"github.com/ethereum/go-ethereum/common"
)

// TokenSystemInterface defines the minimal capabilities required from a token system
// for the RegistryManager to function. The manager only needs to look up tokens;
// it is not responsible for creating them.
type TokenSystemInterface interface {
	GetTokenByAddress(addr common.Address) (token.TokenView, error)
}

// PoolSystemInterface defines the minimal capabilities required from a pool system.
type PoolSystemInterface interface {
	// AddPool adds a new pool using a PoolKey and a semantic ProtocolID.
	AddPool(key PoolKey, pID engine.ProtocolID) (uint64, error)

	// AddPools adds multiple pools in a single, atomic operation.
	// It returns a slice of errors, with each index corresponding to an input pool.
	AddPools(keys []PoolKey, pIDs []engine.ProtocolID) ([]uint64, []error)

	// DeletePool removes a single pool from the system.
	DeletePool(poolID uint64) error

	// DeletePools removes multiple pools in a single, atomic operation.
	DeletePools(poolIDs []uint64) []error

	// Blocklist management
	AddToBlockList(key PoolKey)
	RemoveFromBlockList(key PoolKey)
	IsOnBlockList(key PoolKey) bool

	// Read methods
	GetID(key PoolKey) (uint64, error)
	GetKey(id uint64) (PoolKey, error)
	GetProtocolID(id uint64) (engine.ProtocolID, error)

	// View returns the complete registry state (Pools + Protocols).
	View() PoolRegistryView
}

// TokenPoolSystemInterface defines the minimal capabilities for managing the graph-based
// token-to-pool relationships.
type TokenPoolSystemInterface interface {
	// AddPool adds a new pool and creates the connections between all of its tokens.
	AddPool(tokenIDs []uint64, poolID uint64)

	// AddPools adds multiple pools and their token connections. It panics if slice lengths mismatch.
	AddPools(poolIDs []uint64, tokenIDSets [][]uint64)

	// RemovePool removes a pool and all of its associated connections.
	RemovePool(poolID uint64)

	// RemovePools removes multiple pools and all of their associated connections.
	RemovePools(poolIDs []uint64)

	// PoolsForToken returns all pool IDs associated with a given token.
	PoolsForToken(tokenID uint64) []uint64

	// View returns a snapshot of all token-to-pool connections.
	// Note: Ensure TokenPoolsRegistryView is defined or imported correctly in your project.
	View() *TokenPoolsRegistryView
}
