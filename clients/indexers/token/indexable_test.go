package token

import (
	"testing"

	basetoken "github.com/defistate/defistate/protocols/erc20-token-system"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIndexableTokenSystem(t *testing.T) {
	// --- Test Data Setup ---
	wethAddress := common.HexToAddress("0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2")
	usdcAddress := common.HexToAddress("0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48")
	nonExistentAddress := common.HexToAddress("0x1111111111111111111111111111111111111111")

	testTokens := []basetoken.TokenView{
		{ID: 1, Address: wethAddress, Name: "Wrapped Ether", Symbol: "WETH"},
		{ID: 2, Address: usdcAddress, Name: "USD Coin", Symbol: "USDC"},
	}

	// Create the indexer instance to be tested.
	indexer := NewIndexableTokenSystem(testTokens)
	require.NotNil(t, indexer)

	// --- Sub-tests for different scenarios ---

	t.Run("Successful Lookups", func(t *testing.T) {
		// Test GetByID
		weth, found := indexer.GetByID(1)
		assert.True(t, found, "WETH should be found by ID 1")
		assert.Equal(t, "WETH", weth.Symbol)

		// Test GetByAddress
		usdc, found := indexer.GetByAddress(usdcAddress)
		assert.True(t, found, "USDC should be found by its address")
		assert.Equal(t, "USDC", usdc.Symbol)
	})

	t.Run("Not Found Lookups", func(t *testing.T) {
		// Test GetByID with a non-existent ID
		_, found := indexer.GetByID(999)
		assert.False(t, found, "Should not find a token with ID 999")

		// Test GetByAddress with a non-existent address
		_, found = indexer.GetByAddress(nonExistentAddress)
		assert.False(t, found, "Should not find a token with a non-existent address")
	})

	t.Run("All Method", func(t *testing.T) {
		allTokens := indexer.All()
		assert.Len(t, allTokens, 2, "All() should return 2 tokens")

		// Verify it's a copy by modifying the returned slice and checking the original
		if len(allTokens) > 0 {
			allTokens[0].Symbol = "MODIFIED"
			originalToken, _ := indexer.GetByID(1)
			assert.Equal(t, "WETH", originalToken.Symbol, "Modifying the returned slice should not affect the internal state")
		}
	})

	t.Run("Edge Case - Empty Slice", func(t *testing.T) {
		emptyIndexer := NewIndexableTokenSystem([]basetoken.TokenView{})
		require.NotNil(t, emptyIndexer)

		_, found := emptyIndexer.GetByID(1)
		assert.False(t, found)

		allTokens := emptyIndexer.All()
		assert.Len(t, allTokens, 0)
	})

	t.Run("Edge Case - Nil Slice", func(t *testing.T) {
		nilIndexer := NewIndexableTokenSystem(nil)
		require.NotNil(t, nilIndexer)

		_, found := nilIndexer.GetByID(1)
		assert.False(t, found)

		allTokens := nilIndexer.All()
		assert.Len(t, allTokens, 0)
		assert.NotNil(t, allTokens, "All() should return an empty slice, not nil")
	})
}
