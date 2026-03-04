package poolregistry

import (
	"testing"

	"github.com/defistate/defistate/engine"
	poolregistry "github.com/defistate/defistate/protocols/pool-registry"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIndexablePoolRegistry(t *testing.T) {
	// --- Test Data Setup ---
	addr1 := common.HexToAddress("0x88e6A0c2dDD26FEEb64F039a2c41296FcB3f5640")
	addr2 := common.HexToAddress("0x3416cF6C708Da44DB2624D63ea0AAef7113527C6")
	nonExistentAddr := common.HexToAddress("0x1111111111111111111111111111111111111111")

	key1 := poolregistry.AddressToPoolKey(addr1)
	key2 := poolregistry.AddressToPoolKey(addr2)
	nonExistentKey := poolregistry.AddressToPoolKey(nonExistentAddr)

	testPools := []poolregistry.PoolView{
		{ID: 100, Key: key1, Protocol: 2},
		{ID: 200, Key: key2, Protocol: 1},
	}

	// CORRECTED: ProtocolID is a string alias, not a struct
	testProtocols := map[uint16]engine.ProtocolID{
		1: "uniswap-v2",
		2: "uniswap-v3",
	}

	testView := poolregistry.PoolRegistryView{
		Pools:     testPools,
		Protocols: testProtocols,
	}

	// --- Test 1: Indexer Component (Factory) ---
	t.Run("Indexer Factory and Method", func(t *testing.T) {
		idx := New()
		require.NotNil(t, idx)

		registry := idx.Index(testView)
		require.NotNil(t, registry)
		assert.Equal(t, 2, len(registry.All()))

		// Verify protocols were indexed
		protos := registry.GetProtocols()
		assert.Equal(t, 2, len(protos))
		assert.Equal(t, engine.ProtocolID("uniswap-v2"), protos[1])
	})

	// --- Test 2: Registry Lookups ---
	registry := NewIndexablePoolRegistry(testView)
	require.NotNil(t, registry)

	t.Run("Successful Lookups", func(t *testing.T) {
		// Test GetByID
		pool, found := registry.GetByID(100)
		assert.True(t, found, "Pool should be found by ID 100")
		assert.Equal(t, uint16(2), pool.Protocol)

		// Test GetByAddress
		pool, found = registry.GetByAddress(addr2)
		assert.True(t, found, "Pool should be found by its address (via key)")
		assert.Equal(t, uint64(200), pool.ID)

		// Test GetByPoolKey
		pool, found = registry.GetByPoolKey(key1)
		assert.True(t, found, "Pool should be found by its Key")
		assert.Equal(t, uint64(100), pool.ID)
	})

	t.Run("Protocol Lookups", func(t *testing.T) {
		protos := registry.GetProtocols()
		require.Len(t, protos, 2)

		p1, ok := protos[1]
		assert.True(t, ok)
		assert.Equal(t, engine.ProtocolID("uniswap-v2"), p1)

		p2, ok := protos[2]
		assert.True(t, ok)
		assert.Equal(t, engine.ProtocolID("uniswap-v3"), p2)
	})

	t.Run("Protocol Map Immutability", func(t *testing.T) {
		protos := registry.GetProtocols()
		// Modify the returned map
		protos[1] = "hacked-protocol"

		// Verify internal state is unchanged
		freshProtos := registry.GetProtocols()
		assert.Equal(t, engine.ProtocolID("uniswap-v2"), freshProtos[1])
	})

	t.Run("Not Found Lookups", func(t *testing.T) {
		_, found := registry.GetByID(999)
		assert.False(t, found)

		_, found = registry.GetByAddress(nonExistentAddr)
		assert.False(t, found)

		_, found = registry.GetByPoolKey(nonExistentKey)
		assert.False(t, found)
	})

	t.Run("All Method", func(t *testing.T) {
		allPools := registry.All()
		assert.Len(t, allPools, 2)

		if len(allPools) > 0 {
			allPools[0].Protocol = 99
			originalPool, _ := registry.GetByID(100)
			assert.Equal(t, uint16(2), originalPool.Protocol)
		}
	})

	t.Run("Edge Case - Empty View", func(t *testing.T) {
		emptyView := poolregistry.PoolRegistryView{
			Pools:     []poolregistry.PoolView{},
			Protocols: map[uint16]engine.ProtocolID{},
		}
		emptyIndexer := NewIndexablePoolRegistry(emptyView)
		require.NotNil(t, emptyIndexer)

		_, found := emptyIndexer.GetByID(1)
		assert.False(t, found)
		assert.Len(t, emptyIndexer.All(), 0)
		assert.Len(t, emptyIndexer.GetProtocols(), 0)
	})
}
