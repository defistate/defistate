package erc20analyzer

import (
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMergeMaxTransferRecords provides comprehensive, production-quality testing for the
// MergeMaxTransferRecords function. It verifies correctness, edge cases, and ensures
// the function is pure (does not mutate its inputs).
func TestMergeMaxTransferRecords(t *testing.T) {
	t.Parallel() // This test suite can be run in parallel with others.

	// --- Test Fixtures ---
	token1 := common.HexToAddress("0x1")
	token2 := common.HexToAddress("0x2")
	token3 := common.HexToAddress("0x3")

	walletA := common.HexToAddress("0xA")
	walletB := common.HexToAddress("0xB")
	walletC := common.HexToAddress("0xC")
	walletD := common.HexToAddress("0xD")

	// --- Test Cases ---
	testCases := []struct {
		name        string
		oldMap      map[common.Address]MaxTransferRecord
		newMap      map[common.Address]MaxTransferRecord
		expectedMap map[common.Address]MaxTransferRecord
		description string
	}{
		{
			name: "Happy Path - Merge new and updated records",
			oldMap: map[common.Address]MaxTransferRecord{
				token1: {Address: walletA, Amount: big.NewInt(100)},
				token2: {Address: walletB, Amount: big.NewInt(500)},
			},
			newMap: map[common.Address]MaxTransferRecord{
				token2: {Address: walletC, Amount: big.NewInt(1000)}, // Update existing: new amount is larger
				token3: {Address: walletD, Amount: big.NewInt(2000)}, // Add new token
			},
			expectedMap: map[common.Address]MaxTransferRecord{
				token1: {Address: walletA, Amount: big.NewInt(100)},  // Unchanged
				token2: {Address: walletC, Amount: big.NewInt(1000)}, // Updated
				token3: {Address: walletD, Amount: big.NewInt(2000)}, // Added
			},
			description: "Should correctly add new tokens and update existing tokens with larger amounts.",
		},
		{
			name: "No Update - New amounts are smaller",
			oldMap: map[common.Address]MaxTransferRecord{
				token1: {Address: walletA, Amount: big.NewInt(1000)},
			},
			newMap: map[common.Address]MaxTransferRecord{
				token1: {Address: walletB, Amount: big.NewInt(500)}, // Should not update
			},
			expectedMap: map[common.Address]MaxTransferRecord{
				token1: {Address: walletA, Amount: big.NewInt(1000)}, // Remains unchanged
			},
			description: "Should not update a record if the new transfer amount is smaller.",
		},
		{
			name: "No Update - New amounts are equal",
			oldMap: map[common.Address]MaxTransferRecord{
				token1: {Address: walletA, Amount: big.NewInt(1000)},
			},
			newMap: map[common.Address]MaxTransferRecord{
				token1: {Address: walletB, Amount: big.NewInt(1000)}, // Should not update
			},
			expectedMap: map[common.Address]MaxTransferRecord{
				token1: {Address: walletA, Amount: big.NewInt(1000)}, // Remains unchanged, first seen wins
			},
			description: "Should not update a record if the new transfer amount is equal.",
		},
		{
			name:   "Edge Case - Old map is empty",
			oldMap: map[common.Address]MaxTransferRecord{},
			newMap: map[common.Address]MaxTransferRecord{
				token1: {Address: walletA, Amount: big.NewInt(100)},
			},
			expectedMap: map[common.Address]MaxTransferRecord{
				token1: {Address: walletA, Amount: big.NewInt(100)},
			},
			description: "Should correctly merge into an empty 'old' map.",
		},
		{
			name: "Edge Case - New map is empty",
			oldMap: map[common.Address]MaxTransferRecord{
				token1: {Address: walletA, Amount: big.NewInt(100)},
			},
			newMap: map[common.Address]MaxTransferRecord{},
			expectedMap: map[common.Address]MaxTransferRecord{
				token1: {Address: walletA, Amount: big.NewInt(100)},
			},
			description: "Should return a copy of the 'old' map if the 'new' map is empty.",
		},
		{
			name:        "Edge Case - Both maps are empty",
			oldMap:      map[common.Address]MaxTransferRecord{},
			newMap:      map[common.Address]MaxTransferRecord{},
			expectedMap: map[common.Address]MaxTransferRecord{},
			description: "Should return an empty map if both input maps are empty.",
		},
	}

	for _, tc := range testCases {
		tc := tc // Capture range variable for parallel execution.
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel() // Each sub-test can run in parallel.
			t.Log(tc.description)

			// --- Purity Check: Verify that input maps are not mutated ---
			// Create a deep copy of the original oldMap to check for mutation.
			oldMapCopy := make(map[common.Address]MaxTransferRecord, len(tc.oldMap))
			for k, v := range tc.oldMap {
				oldMapCopy[k] = v
			}

			mergedMap := MergeMaxTransferRecords(tc.oldMap, tc.newMap)

			// Assert that the original oldMap was not changed.
			assert.Equal(t, oldMapCopy, tc.oldMap, "The original 'old' map should not be mutated.")

			// --- Verification of Result ---
			require.Len(t, mergedMap, len(tc.expectedMap), "Result map has incorrect number of entries.")
			for token, expected := range tc.expectedMap {
				actual, ok := mergedMap[token]
				require.True(t, ok, "Expected token %s not found in result.", token.Hex())
				assert.Equal(t, expected.Address, actual.Address, "Incorrect address for token %s", token.Hex())
				assert.Zero(t, expected.Amount.Cmp(actual.Amount), "Incorrect amount for token %s", token.Hex())
			}
		})
	}
}

// TestExpireMaxTransferRecords provides comprehensive testing for the ExpireMaxTransferRecords function.
func TestExpireMaxTransferRecords(t *testing.T) {
	t.Parallel() // This test suite can be run in parallel with others.

	// --- Test Fixtures ---
	token1 := common.HexToAddress("0x1")
	token2 := common.HexToAddress("0x2")
	token3 := common.HexToAddress("0x3")

	now := time.Now()

	// --- Test Cases ---
	testCases := []struct {
		name         string
		records      map[common.Address]MaxTransferRecord
		staleAfter   time.Duration
		expectedKeys []common.Address // Keys that should remain
		expiredKeys  []common.Address // Keys that should be removed
		description  string
	}{
		{
			name: "Happy Path - Expire some, keep some",
			records: map[common.Address]MaxTransferRecord{
				token1: {Time: now.Add(-5 * time.Second)},  // Stale
				token2: {Time: now.Add(-1 * time.Second)},  // Fresh
				token3: {Time: now.Add(-10 * time.Second)}, // Stale
			},
			staleAfter:   3 * time.Second,
			expectedKeys: []common.Address{token2},
			expiredKeys:  []common.Address{token1, token3},
			description:  "Should remove records older than the stale duration and keep fresh ones.",
		},
		{
			name: "Edge Case - All records are fresh",
			records: map[common.Address]MaxTransferRecord{
				token1: {Time: now},
				token2: {Time: now.Add(-1 * time.Second)},
			},
			staleAfter:   5 * time.Second,
			expectedKeys: []common.Address{token1, token2},
			expiredKeys:  []common.Address{},
			description:  "Should keep all records if none are older than the stale duration.",
		},
		{
			name: "Edge Case - All records are stale",
			records: map[common.Address]MaxTransferRecord{
				token1: {Time: now.Add(-10 * time.Minute)},
				token2: {Time: now.Add(-5 * time.Minute)},
			},
			staleAfter:   1 * time.Minute,
			expectedKeys: []common.Address{},
			expiredKeys:  []common.Address{token1, token2},
			description:  "Should return an empty map if all records are stale.",
		},
		{
			name:         "Edge Case - Input map is empty",
			records:      map[common.Address]MaxTransferRecord{},
			staleAfter:   1 * time.Hour,
			expectedKeys: []common.Address{},
			expiredKeys:  []common.Address{},
			description:  "Should return an empty map if the input map is empty.",
		},
		{
			name: "Boundary Condition - Time is exactly on the boundary",
			records: map[common.Address]MaxTransferRecord{
				// This record's age is exactly 2 seconds.
				// Since `time.Since() < staleAfter` is a strict less-than,
				// a record with age == staleAfter should be expired.
				token1: {Time: now.Add(-2 * time.Second)},
			},
			staleAfter:   2 * time.Second,
			expectedKeys: []common.Address{},
			expiredKeys:  []common.Address{token1},
			description:  "A record whose age is exactly the stale duration should be expired.",
		},
	}

	for _, tc := range testCases {
		tc := tc // Capture range variable for parallel execution.
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel() // Each sub-test can run in parallel.
			t.Log(tc.description)

			freshRecords := ExpireMaxTransferRecords(tc.records, tc.staleAfter)

			require.Len(t, freshRecords, len(tc.expectedKeys), "The number of fresh records is incorrect.")
			// Check that all expected (fresh) keys are present.
			for _, key := range tc.expectedKeys {
				_, ok := freshRecords[key]
				assert.True(t, ok, "Expected fresh key %s not found in result.", key.Hex())
			}
			// Check that all expired keys are absent.
			for _, key := range tc.expiredKeys {
				_, ok := freshRecords[key]
				assert.False(t, ok, "Expired key %s should not be in the result.", key.Hex())
			}
		})
	}
}
