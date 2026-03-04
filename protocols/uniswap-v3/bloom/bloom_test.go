package bloom

import (
	"testing"

	// This test assumes that the 'abi' package is available in the test scope.
	"github.com/defistate/defistate/protocols/uniswap-v3/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/assert"
)

// --- Test for TestUniswapV3Swap ---

func TestTestUniswapV3Swap(t *testing.T) {
	// Get the event signature topic for the "Swap" event from the Uniswap V3 ABI.
	swapEventTopic := abi.UniswapV3ABI.Events["Swap"].ID
	// Define a random topic that is different from the Swap event topic for negative testing.
	otherTopic := common.HexToHash("0x3333333333333333333333333333333333333333333333333333333333333333")

	// Test case: The bloom filter should correctly identify the presence of the Swap event topic.
	t.Run("should return true when swap topic is present", func(t *testing.T) {
		// Create a logger entry with the Swap event topic.
		logger := &types.Log{
			Topics: []common.Hash{swapEventTopic},
		}
		// Generate a bloom filter from a receipt containing the logger.
		bloom := types.CreateBloom(&types.Receipt{Logs: []*types.Log{logger}})

		// Assert that the function returns true, as the topic should be found.
		assert.True(t, TestUniswapV3Swap(bloom))
	})

	// Test case: The bloom filter should correctly report the absence of the Swap event topic.
	t.Run("should return false when swap topic is absent", func(t *testing.T) {
		// Create a logger entry with a topic other than the Swap event topic.
		logger := &types.Log{
			Topics: []common.Hash{otherTopic},
		}
		// Generate a bloom filter from a receipt containing this logger.
		bloom := types.CreateBloom(&types.Receipt{Logs: []*types.Log{logger}})

		// Assert that the function returns false, as the Swap topic is not in the filter.
		assert.False(t, TestUniswapV3Swap(bloom))
	})

	// Test case: An empty bloom filter should not contain the topic.
	t.Run("should return false for an empty bloom filter", func(t *testing.T) {
		// Create a new, empty bloom filter.
		var bloom types.Bloom

		// Assert that testing against an empty filter returns false.
		assert.False(t, TestUniswapV3Swap(bloom))
	})
}
