package pancakeswap

import (
	"math/big"
	"testing"

	// Assumes this is where your ABI is located
	"github.com/defistate/defistate/protocols/uniswap-v3/abi/forks/omniexchange"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// int64ToTopicHash correctly converts an int64 to a 32-byte topic hash,
// handling two's complement for negative values.
func int64ToTopicHash(val int64) common.Hash {
	bint := new(big.Int).SetInt64(val)
	// If the number is negative, calculate its 256-bit two's complement,
	// which is how the EVM represents it.
	if bint.Sign() < 0 {
		// This is equivalent to adding 2^256 to the negative number.
		bint.Add(bint, new(big.Int).Lsh(big.NewInt(1), 256))
	}
	return common.BigToHash(bint)
}

// Helper to create a test logger. Note that for this test, we only care about Topics.
// It uses the actual event topics from your ABI package.
func createTestLog(address common.Address, topic0 common.Hash, tickLower, tickUpper int64) *types.Log {
	// Use the corrected helper to create topics.
	topic2 := int64ToTopicHash(tickLower)
	topic3 := int64ToTopicHash(tickUpper)

	return &types.Log{
		Address: address,
		Topics: []common.Hash{
			topic0,
			common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000001"), // Dummy owner topic
			topic2,
			topic3,
		},
	}
}

func TestOmniExchangePoolsUpdatedInBlock(t *testing.T) {
	// Define constants using the actual ABI definitions
	mintEventTopic := omniexchange.OmniExchangeABI.Events["Mint"].ID
	burnEventTopic := omniexchange.OmniExchangeABI.Events["Burn"].ID
	pool1 := common.HexToAddress("0x1")
	pool2 := common.HexToAddress("0x2")
	otherEventTopic := common.HexToHash("0x1234")

	// Boundary values for int24
	const minInt24 int64 = -8388608
	const maxInt24 int64 = 8388607

	testCases := []struct {
		name          string
		inputLogs     []types.Log
		expectedPools []common.Address
		expectedTicks map[common.Address][]int64 // Use a map for easy, order-agnostic comparison
	}{
		{
			name:          "empty logs slice",
			inputLogs:     []types.Log{},
			expectedPools: nil,
			expectedTicks: nil,
		},
		{
			name:          "no relevant logs",
			inputLogs:     []types.Log{*createTestLog(pool1, otherEventTopic, 1, 2)},
			expectedPools: nil,
			expectedTicks: nil,
		},
		{
			name:          "single mint event with negative ticks",
			inputLogs:     []types.Log{*createTestLog(pool1, mintEventTopic, -100, 100)},
			expectedPools: []common.Address{pool1},
			expectedTicks: map[common.Address][]int64{
				pool1: {-100, 100},
			},
		},
		{
			name:          "single burn event",
			inputLogs:     []types.Log{*createTestLog(pool2, burnEventTopic, 500, 1000)},
			expectedPools: []common.Address{pool2},
			expectedTicks: map[common.Address][]int64{
				pool2: {500, 1000},
			},
		},
		{
			name: "multiple events for the same pool",
			inputLogs: []types.Log{
				*createTestLog(pool1, mintEventTopic, 10, 20),
				*createTestLog(pool1, burnEventTopic, 30, 40),
			},
			expectedPools: []common.Address{pool1},
			expectedTicks: map[common.Address][]int64{
				pool1: {10, 20, 30, 40},
			},
		},
		{
			name: "duplicate ticks for the same pool are handled correctly",
			inputLogs: []types.Log{
				*createTestLog(pool1, mintEventTopic, 10, 20),
				*createTestLog(pool1, burnEventTopic, 10, 30), // tick 10 is duplicated
			},
			expectedPools: []common.Address{pool1},
			expectedTicks: map[common.Address][]int64{
				pool1: {10, 20, 30},
			},
		},
		{
			name: "multiple events for different pools",
			inputLogs: []types.Log{
				*createTestLog(pool1, mintEventTopic, -50, 50),
				*createTestLog(pool2, burnEventTopic, 1000, 2000),
			},
			expectedPools: []common.Address{pool1, pool2},
			expectedTicks: map[common.Address][]int64{
				pool1: {-50, 50},
				pool2: {1000, 2000},
			},
		},
		{
			name:          "malformed logger with insufficient topics is ignored",
			inputLogs:     []types.Log{{Address: pool1, Topics: []common.Hash{mintEventTopic}}},
			expectedPools: nil,
			expectedTicks: nil,
		},
		{
			name:          "boundary values for int24 ticks",
			inputLogs:     []types.Log{*createTestLog(pool1, mintEventTopic, minInt24, maxInt24)},
			expectedPools: []common.Address{pool1},
			expectedTicks: map[common.Address][]int64{
				pool1: {minInt24, maxInt24},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Call the function under test
			gotPools, gotTicksByPool, gotErrs := OmniExchangePoolsUpdatedInBlock(tc.inputLogs)

			// Assertions
			require.Empty(t, gotErrs, "should not return any errors for valid or ignorable logs")

			// Check that the returned pools match the expected pools, regardless of order.
			assert.ElementsMatch(t, tc.expectedPools, gotPools, "should return the correct set of pools")

			// If we expect ticks, convert the returned slices into a map for easier comparison.
			if tc.expectedTicks != nil {
				resultsMap := make(map[common.Address][]int64)
				for i, pool := range gotPools {
					resultsMap[pool] = gotTicksByPool[i]
				}

				require.Len(t, resultsMap, len(tc.expectedTicks), "should have the same number of pools with updates")

				// For each expected pool, check that the ticks match, regardless of order.
				for poolAddr, expectedTicks := range tc.expectedTicks {
					gotTicks, ok := resultsMap[poolAddr]
					require.True(t, ok, "expected pool %s was not found in results", poolAddr.Hex())
					assert.ElementsMatch(t, expectedTicks, gotTicks, "ticks for pool %s should match", poolAddr.Hex())
				}
			} else {
				// If we expect no ticks, the returned slices should be empty or nil.
				assert.Empty(t, gotPools, "pools slice should be empty")
				assert.Empty(t, gotTicksByPool, "ticks slice should be empty")
			}
		})
	}
}
