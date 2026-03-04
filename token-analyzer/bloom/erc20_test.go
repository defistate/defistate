package bloom

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/assert"
)

// TestERC20TransferLikelyInBloom tests the ERC20TransferLikelyInBloom function.
// It's important to remember that Bloom filters can produce false positives (reporting that a topic
// may be present when it is not) but not false negatives (if it reports a topic is not
// present, it is guaranteed to be absent). These tests verify this core functionality.
func TestERC20TransferLikelyInBloom(t *testing.T) {
	// Test case 1: The bloom filter contains the ERC20 Transfer topic.
	// The function should return true.
	t.Run("should return true when topic is in bloom", func(t *testing.T) {
		bloom := types.Bloom{}
		bloom.Add(ERC20TransferTopic.Bytes())
		assert.True(t, ERC20TransferLikelyInBloom(bloom), "Expected bloom test to be true")
	})

	// Test case 2: The bloom filter does not contain the ERC20 Transfer topic.
	// The function should return false.
	t.Run("should return false when topic is not in bloom", func(t *testing.T) {
		bloom := types.Bloom{} // Empty bloom
		assert.False(t, ERC20TransferLikelyInBloom(bloom), "Expected bloom test to be false for an empty bloom")
	})

	// Test case 3: The bloom filter contains a different topic.
	// The function should return false.
	t.Run("should return false when bloom contains other topics", func(t *testing.T) {
		bloom := types.Bloom{}
		otherTopic := common.HexToHash("0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef")
		bloom.Add(otherTopic.Bytes())
		assert.False(t, ERC20TransferLikelyInBloom(bloom), "Expected bloom test to be false when topic is not present")
	})

	// Test case 4: Add multiple other topics to the bloom filter.
	// This test demonstrates that adding other items does not necessarily trigger a positive result
	// for the target topic. While a "false positive" is theoretically possible if the other
	// topics' hashes happen to set all the same bits as our target topic, it is highly unlikely
	// with a small number of items. This test confirms the negative case holds true under normal conditions.
	t.Run("should not find topic when multiple other topics are added", func(t *testing.T) {
		bloom := types.Bloom{}
		// Add a few unrelated topics
		bloom.Add(common.HexToHash("0x1111111111111111111111111111111111111111111111111111111111111111").Bytes())
		bloom.Add(common.HexToHash("0x2222222222222222222222222222222222222222222222222222222222222222").Bytes())
		bloom.Add(common.HexToHash("0x3333333333333333333333333333333333333333333333333333333333333333").Bytes())

		// The ERC20 transfer topic was not added, so it should not be found.
		assert.False(t, ERC20TransferLikelyInBloom(bloom), "Expected bloom test to be false even with other topics present")
	})
}
