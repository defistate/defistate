package omniexchange

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Test Suites ---

func TestDiscoverPools(t *testing.T) {
	// --- Setup ---
	pool1 := common.HexToAddress("0x1")
	pool2 := common.HexToAddress("0x2")
	pool3 := common.HexToAddress("0x3")

	// Create a mix of logs:
	// - Swaps from pool1 and pool2
	// - A duplicate swap from pool1 to test uniqueness
	// - A non-swap event logger (with a different topic)
	// - A logger with no topics
	logs := []types.Log{
		createSwapLog(t, pool1, 100, big.NewInt(1000), big.NewInt(1)),
		createSwapLog(t, pool2, 200, big.NewInt(2000), big.NewInt(2)),
		{Address: pool3, Topics: []common.Hash{common.HexToHash("0x1234")}}, // Not a swap logger
		createSwapLog(t, pool1, 300, big.NewInt(3000), big.NewInt(3)),       // Duplicate pool
		{Address: common.HexToAddress("0x4")},                               // No topics
	}

	// --- Action ---
	discoveredPools, err := DiscoverPools(logs)

	// --- Assertions ---
	require.NoError(t, err)
	require.NotNil(t, discoveredPools)

	// We expect to find pool1 and pool2, but not pool3 or pool4.
	// Use ElementsMatch because the order of returned addresses is not guaranteed.
	assert.ElementsMatch(t, []common.Address{pool1, pool2}, discoveredPools)
	assert.Len(t, discoveredPools, 2)
}
