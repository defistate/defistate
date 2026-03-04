package uniswapv3

import (
	"math/big"
	"testing"

	"github.com/defistate/defistate/clients/mocks"
	"github.com/defistate/defistate/engine"
	token "github.com/defistate/defistate/protocols/erc20-token-system"
	poolregistry "github.com/defistate/defistate/protocols/pool-registry"
	uniswapv3 "github.com/defistate/defistate/protocols/uniswap-v3"
	"github.com/defistate/defistate/protocols/uniswap-v3/ticks"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIndexableUniswapV3System(t *testing.T) {
	// --- 1. Setup Mock Environment ---
	addrWETH := common.HexToAddress("0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2")
	addrUSDC := common.HexToAddress("0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48")
	addrPool := common.HexToAddress("0x88e6A0c2dDD26feEB64f039a2c41296fcB3f5640")

	// Setup Tokens
	tokenMock := mocks.NewMockIndexedTokenSystem([]token.TokenView{
		{ID: 0, Address: addrWETH, Symbol: "WETH"},
		{ID: 1, Address: addrUSDC, Symbol: "USDC"},
	})

	// Setup Pool Registry with Protocol Mapping
	registryMock := mocks.NewMockIndexedPoolRegistry(poolregistry.PoolRegistryView{
		Protocols: map[uint16]engine.ProtocolID{
			3: "uniswap_v3_ethereum",
		},
	})

	// Hydrate Registry with Pool Metadata
	registryMock.Add(poolregistry.PoolView{
		ID:       201,
		Key:      poolregistry.AddressToPoolKey(addrPool),
		Protocol: 3,
	})

	// --- 2. Test Input (Raw Views from Chain) ---
	testPools := []uniswapv3.PoolView{
		{
			PoolViewMinimal: uniswapv3.PoolViewMinimal{
				ID:           201,
				Token0:       0, // Maps to WETH
				Token1:       1, // Maps to USDC
				Tick:         200000,
				Liquidity:    big.NewInt(1234567890),
				SqrtPriceX96: big.NewInt(5602277097478614198),
				Fee:          500,
				TickSpacing:  10,
			},
			Ticks: []ticks.TickInfo{
				{Index: 199980, LiquidityGross: big.NewInt(10000), LiquidityNet: big.NewInt(10000)},
				{Index: 200040, LiquidityGross: big.NewInt(10000), LiquidityNet: big.NewInt(-10000)},
			},
		},
	}

	// --- 3. Run the Indexer ---
	// Note: We now pass the mocks into the constructor
	indexer, err := NewIndexableUniswapV3System(testPools, tokenMock, registryMock)
	require.NoError(t, err)
	require.NotNil(t, indexer)

	// --- 4. Sub-tests ---

	t.Run("Successful Hydrated Lookups", func(t *testing.T) {
		pool, found := indexer.GetByID(201)
		assert.True(t, found)

		// Verify System IDs (New feature)
		assert.Equal(t, uint64(201), pool.IDs.Pool)
		assert.Equal(t, uint64(0), pool.IDs.Token0)

		// Verify Resolved Addresses/Protocols
		assert.Equal(t, addrPool, pool.Address)
		assert.Equal(t, addrWETH, pool.Token0)
		assert.Equal(t, addrUSDC, pool.Token1)
		assert.Equal(t, "uniswap_v3_ethereum", pool.Protocol)

		// Verify Complex State (Ticks)
		require.Len(t, pool.Ticks, 2)
		assert.Equal(t, int64(199980), pool.Ticks[0].Index)
	})

	t.Run("GetByAddress", func(t *testing.T) {
		pool, found := indexer.GetByAddress(addrPool)
		assert.True(t, found)
		assert.Equal(t, uint64(201), pool.IDs.Pool)
	})

	t.Run("Not Found Lookups", func(t *testing.T) {
		_, found := indexer.GetByID(999)
		assert.False(t, found)
	})

	t.Run("All Method", func(t *testing.T) {
		allPools := indexer.All()
		assert.Len(t, allPools, 1)
		assert.Equal(t, int64(200000), allPools[0].Tick)
	})

	t.Run("Empty Initialization", func(t *testing.T) {
		emptyIdx, err := NewIndexableUniswapV3System([]uniswapv3.PoolView{}, tokenMock, registryMock)
		require.NoError(t, err)
		assert.Len(t, emptyIdx.All(), 0)
	})
}
