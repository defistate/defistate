package omniexchange

import (
	"testing"

	// This test assumes that the 'abi' package is available in the test scope.

	abi "github.com/defistate/defistate/protocols/uniswap-v3/abi/forks/omniexchange"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/assert"
)

// --- Test for TestOmniExchangeSwap ---

func TestTestOmniExchangeSwap(t *testing.T) {
	// Get the event signature topic for the "Swap" event from the Pancakeswap V3 ABI.
	swapEventTopic := abi.OmniExchangeABI.Events["Swap"].ID
	// Define a random topic that is different from the Swap event topic for negative testing.
	otherTopic := common.HexToHash("0x3333333333333333333333333333333333333333333333333333333333333333")

	// Test case: The bloom filter should correctly identify the presence of the Swap event topic.s
	t.Run("should return true when swap topic is present", func(t *testing.T) {
		// Create a logger entry with the Swap event topic.
		logger := &types.Log{
			Topics: []common.Hash{swapEventTopic},
		}
		// Generate a bloom filter from a receipt containing the logger.
		bloom := types.CreateBloom(&types.Receipt{Logs: []*types.Log{logger}})

		// Assert that the function returns true, as the topic should be found.
		assert.True(t, TestOmniExchangeSwap(bloom))
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
		assert.False(t, TestOmniExchangeSwap(bloom))
	})

	// Test case: An empty bloom filter should not contain the topic.
	t.Run("should return false for an empty bloom filter", func(t *testing.T) {
		// Create a new, empty bloom filter.
		var bloom types.Bloom

		// Assert that testing against an empty filter returns false.
		assert.False(t, TestOmniExchangeSwap(bloom))
	})
}
