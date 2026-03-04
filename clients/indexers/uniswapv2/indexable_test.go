package uniswapv2

import (
	"math/big"
	"testing"

	"github.com/defistate/defistate/clients/mocks"
	"github.com/defistate/defistate/engine"
	token "github.com/defistate/defistate/protocols/erc20-token-system"
	poolregistry "github.com/defistate/defistate/protocols/pool-registry"
	uniswapv2 "github.com/defistate/defistate/protocols/uniswap-v2"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIndexableUniswapV2System(t *testing.T) {
	// --- 1. Mock Data Setup ---
	addrWETH := common.HexToAddress("0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2")
	addrUSDC := common.HexToAddress("0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48")
	addrPool := common.HexToAddress("0xB4e16d0168e52d35CaCD2c6185b44281Ec28C9Dc")

	// Setup Mock Token System
	tokenMock := mocks.NewMockIndexedTokenSystem([]token.TokenView{
		{ID: 1, Address: addrWETH, Symbol: "WETH", Decimals: 18},
		{ID: 2, Address: addrUSDC, Symbol: "USDC", Decimals: 6},
	})

	// Setup Mock Pool Registry
	// Note: We define the protocol mapping (uint16 -> string) here
	registryMock := mocks.NewMockIndexedPoolRegistry(poolregistry.PoolRegistryView{
		Protocols: map[uint16]engine.ProtocolID{
			1: "uniswap_v2_ethereum",
		},
	})

	// Add the pool metadata to the registry so the indexer can find it
	registryMock.Add(poolregistry.PoolView{
		ID:       500,
		Key:      poolregistry.AddressToPoolKey(addrPool),
		Protocol: 1,
	})

	// --- 2. Test Input (Raw PoolViews from the chain/poller) ---
	testPools := []uniswapv2.PoolView{
		{
			ID:       500,
			Token0:   1, // WETH
			Token1:   2, // USDC
			Reserve0: big.NewInt(1000000),
			Reserve1: big.NewInt(2000000),
			FeeBps:   30,
		},
	}

	// --- 3. Run the Indexer ---
	// This uses the mocks to "Hydrate" the PoolView into a full Pool struct
	indexer, err := NewIndexableUniswapV2System(testPools, tokenMock, registryMock)
	require.NoError(t, err)
	require.NotNil(t, indexer)

	// --- 4. Sub-tests ---

	t.Run("Successful Enriched Lookup", func(t *testing.T) {
		pool, found := indexer.GetByID(500)
		assert.True(t, found)

		// Verify Hydration worked
		assert.Equal(t, addrPool, pool.Address)
		assert.Equal(t, "uniswap_v2_ethereum", pool.Protocol)

		// Explicitly check uint16 FeeBps
		assert.Equal(t, uint16(30), pool.FeeBps)
	})
	t.Run("Lookup by Address", func(t *testing.T) {
		pool, found := indexer.GetByAddress(addrPool)
		assert.True(t, found)
		assert.Equal(t, uint64(500), pool.IDs.Pool)
	})

	t.Run("Fails on Missing Token", func(t *testing.T) {
		// Create a pool view referencing a non-existent token ID 999
		brokenPools := []uniswapv2.PoolView{
			{ID: 600, Token0: 999, Token1: 1},
		}

		// Add pool 600 to registry so it passes the first check
		registryMock.Add(poolregistry.PoolView{ID: 600, Key: poolregistry.PoolKey{}})

		_, err := NewIndexableUniswapV2System(brokenPools, tokenMock, registryMock)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "token0 with ID 999 not found")
	})

	t.Run("All Method Integrity", func(t *testing.T) {
		all := indexer.All()
		assert.Len(t, all, 1)
		assert.Equal(t, addrPool, all[0].Address)
	})
}
