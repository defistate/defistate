package erc20analyzer

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/assert"
)

// TestFilterERC20TransferLogs tests the FilterERC20TransferLogs function.
func TestFilterERC20TransferLogs(t *testing.T) {
	otherTopic := common.HexToHash("0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef")

	// Create some sample logs
	erc20TransferLog1 := types.Log{
		Topics: []common.Hash{ERC20TransferTopic, common.HexToHash("0x1"), common.HexToHash("0x2")},
		Data:   []byte("log1"),
	}
	erc20TransferLog2 := types.Log{
		Topics: []common.Hash{ERC20TransferTopic, common.HexToHash("0x3"), common.HexToHash("0x4")},
		Data:   []byte("log2"),
	}
	otherLog := types.Log{
		Topics: []common.Hash{otherTopic},
		Data:   []byte("otherlog"),
	}
	logWithNoTopics := types.Log{
		Data: []byte("notopics"),
	}

	// Test case 1: A mix of ERC20 transfer logs and other logs.
	// The function should only return the ERC20 transfer logs.
	t.Run("should filter mixed logs correctly", func(t *testing.T) {
		logs := []types.Log{erc20TransferLog1, otherLog, erc20TransferLog2}
		filtered := FilterERC20TransferLogs(logs)
		assert.Equal(t, 2, len(filtered), "Expected 2 logs to be filtered")
		assert.Contains(t, filtered, erc20TransferLog1, "Filtered logs should contain the first transfer logger")
		assert.Contains(t, filtered, erc20TransferLog2, "Filtered logs should contain the second transfer logger")
		assert.NotContains(t, filtered, otherLog, "Filtered logs should not contain the other logger")
	})

	// Test case 2: Only non-ERC20 transfer logs.
	// The function should return an empty slice.
	t.Run("should return empty slice when no transfer logs are present", func(t *testing.T) {
		logs := []types.Log{otherLog, logWithNoTopics}
		filtered := FilterERC20TransferLogs(logs)
		assert.Empty(t, filtered, "Expected filtered logs to be empty")
	})

	// Test case 3: Only ERC20 transfer logs.
	// The function should return all the logs.
	t.Run("should return all logs when all are transfer logs", func(t *testing.T) {
		logs := []types.Log{erc20TransferLog1, erc20TransferLog2}
		filtered := FilterERC20TransferLogs(logs)
		assert.Equal(t, 2, len(filtered), "Expected all logs to be returned")
		assert.Equal(t, logs, filtered, "Expected filtered logs to be identical to original logs")
	})

	// Test case 4: An empty slice of logs.
	// The function should return an empty slice.
	t.Run("should return empty slice for empty input", func(t *testing.T) {
		logs := []types.Log{}
		filtered := FilterERC20TransferLogs(logs)
		assert.Empty(t, filtered, "Expected filtered logs to be empty for empty input")
	})

	// Test case 5: A logger with no topics.
	// This tests for index out of bounds errors.
	t.Run("should handle logs with no topics", func(t *testing.T) {
		logs := []types.Log{logWithNoTopics, erc20TransferLog1}
		// This should not panic
		filtered := FilterERC20TransferLogs(logs)
		assert.Equal(t, 1, len(filtered), "Expected only one logger to be filtered")
		assert.Equal(t, erc20TransferLog1, filtered[0], "The correct logger should be in the filtered slice")
	})
}

// FuzzFilterERC20TransferLogs fuzz tests the FilterERC20TransferLogs function.
// It helps ensure the function does not panic on a wide variety of malformed and unexpected inputs.
func FuzzFilterERC20TransferLogs(f *testing.F) {
	// Add seed values to guide the fuzzer.
	// These seeds represent different types of logger data.
	f.Add([]byte("some random data"), true, 1)
	f.Add([]byte{}, false, 0)
	f.Add([]byte{0xDE, 0xAD, 0xBE, 0xEF}, true, 3)

	// The fuzzing target function.
	f.Fuzz(func(t *testing.T, data []byte, isTransfer bool, topicCount int) {
		// Construct a logger based on the fuzzed input.
		fuzzedLog := types.Log{Data: data}
		var topics []common.Hash

		// Avoid negative topic counts
		if topicCount < 0 {
			topicCount = 0
		}
		// Cap topic count to avoid excessive memory allocation
		if topicCount > 10 {
			topicCount = 10
		}

		if topicCount > 0 {
			// If it's supposed to be a transfer logger, make the first topic correct.
			if isTransfer {
				topics = append(topics, ERC20TransferTopic)
			} else {
				// Otherwise, use a different topic.
				topics = append(topics, common.HexToHash("0xdeadbeef"))
			}

			if len(data) > 0 {
				// Add more random topics.
				for i := 1; i < topicCount; i++ {
					topics = append(topics, common.BytesToHash(data[i%len(data):]))
				}
			}
		}
		fuzzedLog.Topics = topics

		// Create a slice of logs to test, including the fuzzed one.
		logs := []types.Log{
			fuzzedLog,
			{Topics: []common.Hash{ERC20TransferTopic}},         // A valid transfer logger
			{Topics: []common.Hash{common.HexToHash("0x1234")}}, // A non-transfer logger
			{}, // A logger with no topics and no data
		}

		// Run the function under test. The primary goal is to ensure it doesn't panic.
		filtered := FilterERC20TransferLogs(logs)

		// Assert invariants: properties that should always be true after filtering.
		// 1. The filtered slice should not be nil.
		assert.NotNil(t, filtered, "Filtered slice should never be nil")

		// 2. Every logger in the result must have the correct transfer topic.
		for _, l := range filtered {
			assert.NotEmpty(t, l.Topics, "Filtered logger should have topics")
			assert.Equal(t, ERC20TransferTopic, l.Topics[0], "Filtered logger must have the ERC20 Transfer topic")
		}
	})
}
