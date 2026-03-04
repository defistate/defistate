package pancakeswap

import (
	"testing"

	"github.com/defistate/defistate/protocols/uniswap-v3/abi/forks/pancakeswap"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/assert"
)

// --- Tests for TestPancakeswapV3PoolMint ---

func TestTestPancakeswapV3PoolMint(t *testing.T) {
	mintEventTopic := pancakeswap.PancakeswapV3ABI.Events["Mint"].ID
	otherTopic := common.HexToHash("0x1111111111111111111111111111111111111111111111111111111111111111")

	t.Run("should return true when mint topic is present", func(t *testing.T) {
		// Create a logger with the Mint event topic
		logger := &types.Log{
			Topics: []common.Hash{mintEventTopic},
		}
		// Create a bloom filter from a receipt containing that logger
		bloom := types.CreateBloom(&types.Receipt{Logs: []*types.Log{logger}})

		// Assert that the function correctly identifies the topic
		assert.True(t, TestPancakeswapV3PoolMint(&bloom))
	})

	t.Run("should return false when mint topic is absent", func(t *testing.T) {
		// Create a logger with a different topic
		logger := &types.Log{
			Topics: []common.Hash{otherTopic},
		}
		bloom := types.CreateBloom(&types.Receipt{Logs: []*types.Log{logger}})

		// Assert that the function correctly reports the topic is not present
		assert.False(t, TestPancakeswapV3PoolMint(&bloom))
	})

	t.Run("should return false for an empty bloom filter", func(t *testing.T) {
		var bloom types.Bloom // Zero-value bloom filter

		assert.False(t, TestPancakeswapV3PoolMint(&bloom))
	})
}

// --- Tests for TestPancakeswapV3PoolBurn ---

func TestTestPancakeswapV3PoolBurn(t *testing.T) {
	burnEventTopic := pancakeswap.PancakeswapV3ABI.Events["Burn"].ID
	otherTopic := common.HexToHash("0x2222222222222222222222222222222222222222222222222222222222222222")

	t.Run("should return true when burn topic is present", func(t *testing.T) {
		// Create a logger with the Burn event topic
		logger := &types.Log{
			Topics: []common.Hash{burnEventTopic},
		}
		bloom := types.CreateBloom(&types.Receipt{Logs: []*types.Log{logger}})

		// Assert that the function correctly identifies the topic
		assert.True(t, TestPancakeswapV3PoolBurn(&bloom))
	})

	t.Run("should return false when burn topic is absent", func(t *testing.T) {
		// Create a logger with a different topic
		logger := &types.Log{
			Topics: []common.Hash{otherTopic},
		}
		bloom := types.CreateBloom(&types.Receipt{Logs: []*types.Log{logger}})

		// Assert that the function correctly reports the topic is not present
		assert.False(t, TestPancakeswapV3PoolBurn(&bloom))
	})

	t.Run("should return false for an empty bloom filter", func(t *testing.T) {
		var bloom types.Bloom // Zero-value bloom filter

		assert.False(t, TestPancakeswapV3PoolBurn(&bloom))
	})
}
