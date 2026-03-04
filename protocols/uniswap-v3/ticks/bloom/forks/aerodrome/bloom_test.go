package pancakeswap

import (
	"testing"

	"github.com/defistate/defistate/protocols/uniswap-v3/abi/forks/aerodrome"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/assert"
)

// --- Tests for TestAerodromePoolMint ---

func TestTestAerodromePoolMint(t *testing.T) {
	mintEventTopic := aerodrome.AerodromeABI.Events["Mint"].ID
	otherTopic := common.HexToHash("0x1111111111111111111111111111111111111111111111111111111111111111")

	t.Run("should return true when mint topic is present", func(t *testing.T) {
		// Create a logger with the Mint event topic
		logger := &types.Log{
			Topics: []common.Hash{mintEventTopic},
		}
		// Create a bloom filter from a receipt containing that logger
		bloom := types.CreateBloom(&types.Receipt{Logs: []*types.Log{logger}})

		// Assert that the function correctly identifies the topic
		assert.True(t, TestAerodromePoolMint(&bloom))
	})

	t.Run("should return false when mint topic is absent", func(t *testing.T) {
		// Create a logger with a different topic
		logger := &types.Log{
			Topics: []common.Hash{otherTopic},
		}
		bloom := types.CreateBloom(&types.Receipt{Logs: []*types.Log{logger}})

		// Assert that the function correctly reports the topic is not present
		assert.False(t, TestAerodromePoolMint(&bloom))
	})

	t.Run("should return false for an empty bloom filter", func(t *testing.T) {
		var bloom types.Bloom // Zero-value bloom filter

		assert.False(t, TestAerodromePoolMint(&bloom))
	})
}

// --- Tests for TestAerodromePoolBurn ---

func TestTestAerodromePoolBurn(t *testing.T) {
	burnEventTopic := aerodrome.AerodromeABI.Events["Burn"].ID
	otherTopic := common.HexToHash("0x2222222222222222222222222222222222222222222222222222222222222222")

	t.Run("should return true when burn topic is present", func(t *testing.T) {
		// Create a logger with the Burn event topic
		logger := &types.Log{
			Topics: []common.Hash{burnEventTopic},
		}
		bloom := types.CreateBloom(&types.Receipt{Logs: []*types.Log{logger}})

		// Assert that the function correctly identifies the topic
		assert.True(t, TestAerodromePoolBurn(&bloom))
	})

	t.Run("should return false when burn topic is absent", func(t *testing.T) {
		// Create a logger with a different topic
		logger := &types.Log{
			Topics: []common.Hash{otherTopic},
		}
		bloom := types.CreateBloom(&types.Receipt{Logs: []*types.Log{logger}})

		// Assert that the function correctly reports the topic is not present
		assert.False(t, TestAerodromePoolBurn(&bloom))
	})

	t.Run("should return false for an empty bloom filter", func(t *testing.T) {
		var bloom types.Bloom // Zero-value bloom filter

		assert.False(t, TestAerodromePoolBurn(&bloom))
	})
}
