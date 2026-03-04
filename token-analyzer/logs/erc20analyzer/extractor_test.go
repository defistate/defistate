package erc20analyzer

import (
	"math/big"
	"testing"

	"github.com/defistate/defistate/token-analyzer/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExtractMaxSingleTransfer verifies the logic for finding the single largest transaction.
func TestExtractMaxSingleTransfer(t *testing.T) {
	t.Parallel()

	// --- Test Fixtures (scoped to this test) ---
	var (
		token1  = common.HexToAddress("0x1")
		walletA = common.HexToAddress("0xA")
		walletB = common.HexToAddress("0xB")
		walletC = common.HexToAddress("0xC")
	)

	// --- Helper Functions for Test ---
	createTransferLog := func(token, from, to common.Address, amount int64) types.Log {
		val := new(big.Int).SetInt64(amount)
		return types.Log{
			Address: token,
			Topics: []common.Hash{
				abi.ERC20ABI.Events["Transfer"].ID,
				common.BytesToHash(from.Bytes()),
				common.BytesToHash(to.Bytes()),
			},
			Data: common.LeftPadBytes(val.Bytes(), 32),
		}
	}
	allowAll := func(common.Address) bool { return true }
	allowOnlyWalletA := func(addr common.Address) bool { return addr == walletA }

	testCases := []struct {
		name          string
		logs          []types.Log
		isAllowedFunc func(common.Address) bool
		expectedMap   map[common.Address]MaxTransferRecord
		description   string
	}{
		{
			name:          "Happy Path - New max value updates record",
			logs:          []types.Log{createTransferLog(token1, walletA, walletC, 500), createTransferLog(token1, walletB, walletC, 1000)},
			isAllowedFunc: allowAll,
			expectedMap:   map[common.Address]MaxTransferRecord{token1: {Address: walletB, Amount: big.NewInt(1000)}},
			description:   "Should correctly update the max transferrer when a larger transfer occurs.",
		},
		{
			name:          "Edge Case - Tie in max value",
			logs:          []types.Log{createTransferLog(token1, walletA, walletC, 1000), createTransferLog(token1, walletB, walletC, 1000)},
			isAllowedFunc: allowAll,
			expectedMap:   map[common.Address]MaxTransferRecord{token1: {Address: walletA, Amount: big.NewInt(1000)}},
			description:   "In a tie, the first address to transfer the max amount should be kept.",
		},
		{
			name: "Filtering - Ignores disallowed address",
			logs: []types.Log{
				createTransferLog(token1, walletA, walletC, 100),   // Allowed
				createTransferLog(token1, walletB, walletC, 99999), // Disallowed, should be ignored
			},
			isAllowedFunc: allowOnlyWalletA,
			expectedMap:   map[common.Address]MaxTransferRecord{token1: {Address: walletA, Amount: big.NewInt(100)}},
			description:   "Should ignore a larger transfer from a disallowed address.",
		},
		{
			name:          "Input - Empty logger slice",
			logs:          []types.Log{},
			isAllowedFunc: allowAll,
			expectedMap:   map[common.Address]MaxTransferRecord{},
			description:   "Should return an empty map for empty logger input.",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			t.Log(tc.description)
			actualMap := ExtractMaxSingleTransfer(tc.logs, tc.isAllowedFunc)
			verifyMaxTransferRecordMapTestHelper(t, tc.expectedMap, actualMap)
		})
	}
}

// TestExtractMaxTotalVolumeTransferrer verifies the logic for finding the highest total volume sender.
func TestExtractMaxTotalVolumeTransferrer(t *testing.T) {
	t.Parallel()

	// --- Test Fixtures (scoped to this test) ---
	var (
		token1  = common.HexToAddress("0x1")
		token2  = common.HexToAddress("0x2")
		walletA = common.HexToAddress("0xA")
		walletB = common.HexToAddress("0xB")
		walletC = common.HexToAddress("0xC")
	)

	// --- Helper Functions for Test ---
	createTransferLog := func(token, from, to common.Address, amount int64) types.Log {
		val := new(big.Int).SetInt64(amount)
		return types.Log{
			Address: token,
			Topics: []common.Hash{
				abi.ERC20ABI.Events["Transfer"].ID,
				common.BytesToHash(from.Bytes()),
				common.BytesToHash(to.Bytes()),
			},
			Data: common.LeftPadBytes(val.Bytes(), 32),
		}
	}
	allowAll := func(common.Address) bool { return true }
	allowOnlyAandC := func(addr common.Address) bool { return addr == walletA || addr == walletC }

	testCases := []struct {
		name          string
		logs          []types.Log
		isAllowedFunc func(common.Address) bool
		expectedMap   map[common.Address]MaxTransferRecord
		description   string
	}{
		{
			name: "Happy Path - Sums transfers and finds max volume",
			logs: []types.Log{
				createTransferLog(token1, walletA, walletC, 100),
				createTransferLog(token1, walletB, walletC, 400),
				createTransferLog(token1, walletA, walletC, 301), // walletA total is now 401
				createTransferLog(token2, walletC, walletA, 999),
			},
			isAllowedFunc: allowAll,
			expectedMap: map[common.Address]MaxTransferRecord{
				token1: {Address: walletA, Amount: big.NewInt(401)},
				token2: {Address: walletC, Amount: big.NewInt(999)},
			},
			description: "Should sum transfers and identify the address with the highest total volume.",
		},
		{
			name: "Filtering - Ignores volume from disallowed address",
			logs: []types.Log{
				createTransferLog(token1, walletA, walletC, 100),   // Allowed: A total = 100
				createTransferLog(token1, walletB, walletA, 99999), // Disallowed, volume ignored
				createTransferLog(token1, walletC, walletA, 101),   // Allowed: C total = 101
				createTransferLog(token1, walletA, walletC, 50),    // Allowed: A total = 150 (new max)
			},
			isAllowedFunc: allowOnlyAandC,
			expectedMap:   map[common.Address]MaxTransferRecord{token1: {Address: walletA, Amount: big.NewInt(150)}},
			description:   "Should completely ignore transfers from a disallowed address when summing volume.",
		},
		{
			name:          "Input - Empty logger slice",
			logs:          []types.Log{},
			isAllowedFunc: allowAll,
			expectedMap:   map[common.Address]MaxTransferRecord{},
			description:   "Should return an empty map for empty logger input.",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			t.Log(tc.description)
			actualMap := ExtractMaxTotalVolumeTransferrer(tc.logs, tc.isAllowedFunc)
			verifyMaxTransferRecordMapTestHelper(t, tc.expectedMap, actualMap)
		})
	}
}

// FuzzExtractMaxSingleTransfer ensures the function can gracefully handle arbitrary data.
func FuzzExtractMaxSingleTransfer(f *testing.F) {
	f.Add(common.HexToAddress("0x1").Bytes(), common.HexToAddress("0xA").Bytes(), int64(100))
	f.Fuzz(func(t *testing.T, addrBytes, fromBytes []byte, amount int64) {
		logger := types.Log{
			Address: common.BytesToAddress(addrBytes),
			Topics: []common.Hash{
				abi.ERC20ABI.Events["Transfer"].ID,
				common.BytesToHash(fromBytes),
				common.HexToHash("0x0"),
			},
			Data: new(big.Int).SetInt64(amount).Bytes(),
		}
		// Test will fail if the function panics. We use a simple "allow all" filter.
		ExtractMaxSingleTransfer([]types.Log{logger}, func(common.Address) bool { return true })
	})
}

// FuzzExtractMaxTotalVolumeTransferrer ensures the summation logic can gracefully handle arbitrary data.
func FuzzExtractMaxTotalVolumeTransferrer(f *testing.F) {
	f.Add(common.HexToAddress("0x1").Bytes(), common.HexToAddress("0xA").Bytes(), int64(100))
	f.Fuzz(func(t *testing.T, addrBytes, fromBytes []byte, amount int64) {
		logger := types.Log{
			Address: common.BytesToAddress(addrBytes),
			Topics: []common.Hash{
				abi.ERC20ABI.Events["Transfer"].ID,
				common.BytesToHash(fromBytes),
				common.HexToHash("0x0"),
			},
			Data: new(big.Int).SetInt64(amount).Bytes(),
		}
		// Test will fail if the function panics. We use a simple "allow all" filter.
		ExtractMaxTotalVolumeTransferrer([]types.Log{logger}, func(common.Address) bool { return true })
	})
}

// verifyMaxTransferRecordMapTestHelper remains unchanged.
func verifyMaxTransferRecordMapTestHelper(t *testing.T, expected, actual map[common.Address]MaxTransferRecord) {
	t.Helper()
	require.Len(t, actual, len(expected), "The number of tokens in the result map is incorrect.")
	for expectedToken, expectedTransferrer := range expected {
		actualTransferrer, ok := actual[expectedToken]
		require.True(t, ok, "Expected token %s was not found in the result.", expectedToken.Hex())
		assert.Equal(t, expectedTransferrer.Address, actualTransferrer.Address, "Incorrect max transferrer address for token %s", expectedToken.Hex())
		require.NotNil(t, actualTransferrer.Amount, "Actual amount should not be nil for token %s", expectedToken.Hex())
		assert.Zero(t, expectedTransferrer.Amount.Cmp(actualTransferrer.Amount), "Incorrect max transfer amount for token %s", expectedToken.Hex())
		assert.False(t, actualTransferrer.Time.IsZero(), "Timestamp was not set for token %s", expectedToken.Hex())
	}
}

// TestExtractMaxSingleReceiver verifies the logic for finding the single largest transaction to a receiver.
func TestExtractMaxSingleReceiver(t *testing.T) {
	t.Parallel()

	// --- Test Fixtures (scoped to this test) ---
	var (
		token1  = common.HexToAddress("0x1")
		walletA = common.HexToAddress("0xA")
		walletB = common.HexToAddress("0xB")
		walletC = common.HexToAddress("0xC")
	)

	// --- Helper Functions for Test ---
	createTransferLog := func(token, from, to common.Address, amount int64) types.Log {
		val := new(big.Int).SetInt64(amount)
		return types.Log{
			Address: token,
			Topics: []common.Hash{
				abi.ERC20ABI.Events["Transfer"].ID,
				common.BytesToHash(from.Bytes()),
				common.BytesToHash(to.Bytes()),
			},
			Data: common.LeftPadBytes(val.Bytes(), 32),
		}
	}
	allowAll := func(common.Address) bool { return true }
	allowOnlyWalletC := func(addr common.Address) bool { return addr == walletC }

	testCases := []struct {
		name          string
		logs          []types.Log
		isAllowedFunc func(common.Address) bool
		expectedMap   map[common.Address]MaxTransferRecord
		description   string
	}{
		{
			name:          "Happy Path - New max value updates record",
			logs:          []types.Log{createTransferLog(token1, walletA, walletC, 500), createTransferLog(token1, walletA, walletB, 1000)},
			isAllowedFunc: allowAll,
			expectedMap:   map[common.Address]MaxTransferRecord{token1: {Address: walletB, Amount: big.NewInt(1000)}},
			description:   "Should correctly identify the receiver of the largest single transfer.",
		},
		{
			name:          "Edge Case - Tie in max value",
			logs:          []types.Log{createTransferLog(token1, walletA, walletC, 1000), createTransferLog(token1, walletA, walletB, 1000)},
			isAllowedFunc: allowAll,
			expectedMap:   map[common.Address]MaxTransferRecord{token1: {Address: walletC, Amount: big.NewInt(1000)}},
			description:   "In a tie, the first address to receive the max amount should be kept.",
		},
		{
			name: "Filtering - Ignores disallowed receiver",
			logs: []types.Log{
				createTransferLog(token1, walletA, walletC, 100),   // Allowed
				createTransferLog(token1, walletA, walletB, 99999), // Disallowed, should be ignored
			},
			isAllowedFunc: allowOnlyWalletC,
			expectedMap:   map[common.Address]MaxTransferRecord{token1: {Address: walletC, Amount: big.NewInt(100)}},
			description:   "Should ignore a larger transfer to a disallowed address.",
		},
		{
			name:          "Input - Empty logger slice",
			logs:          []types.Log{},
			isAllowedFunc: allowAll,
			expectedMap:   map[common.Address]MaxTransferRecord{},
			description:   "Should return an empty map for empty logger input.",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			t.Log(tc.description)
			actualMap := ExtractMaxSingleReceiver(tc.logs, tc.isAllowedFunc)
			verifyMaxTransferRecordMapTestHelper(t, tc.expectedMap, actualMap)
		})
	}
}

// TestExtractMaxTotalVolumeReceiver verifies the logic for finding the highest total volume receiver.
func TestExtractMaxTotalVolumeReceiver(t *testing.T) {
	t.Parallel()

	// --- Test Fixtures (scoped to this test) ---
	var (
		token1  = common.HexToAddress("0x1")
		token2  = common.HexToAddress("0x2")
		walletA = common.HexToAddress("0xA")
		walletB = common.HexToAddress("0xB")
		walletC = common.HexToAddress("0xC")
	)

	// --- Helper Functions for Test ---
	createTransferLog := func(token, from, to common.Address, amount int64) types.Log {
		val := new(big.Int).SetInt64(amount)
		return types.Log{
			Address: token,
			Topics: []common.Hash{
				abi.ERC20ABI.Events["Transfer"].ID,
				common.BytesToHash(from.Bytes()),
				common.BytesToHash(to.Bytes()),
			},
			Data: common.LeftPadBytes(val.Bytes(), 32),
		}
	}
	allowAll := func(common.Address) bool { return true }
	allowOnlyAandC := func(addr common.Address) bool { return addr == walletA || addr == walletC }

	testCases := []struct {
		name          string
		logs          []types.Log
		isAllowedFunc func(common.Address) bool
		expectedMap   map[common.Address]MaxTransferRecord
		description   string
	}{
		{
			name: "Happy Path - Sums transfers and finds max volume receiver",
			logs: []types.Log{
				createTransferLog(token1, walletA, walletC, 100), // C total = 100
				createTransferLog(token1, walletA, walletB, 400), // B total = 400
				createTransferLog(token1, walletA, walletC, 301), // C total = 401 (new max)
				createTransferLog(token2, walletB, walletA, 999), // A total = 999
			},
			isAllowedFunc: allowAll,
			expectedMap: map[common.Address]MaxTransferRecord{
				token1: {Address: walletC, Amount: big.NewInt(401)},
				token2: {Address: walletA, Amount: big.NewInt(999)},
			},
			description: "Should sum received transfers and identify the address with the highest total volume.",
		},
		{
			name: "Filtering - Ignores volume to disallowed address",
			logs: []types.Log{
				createTransferLog(token1, walletB, walletA, 100),   // Allowed: A total = 100
				createTransferLog(token1, walletA, walletB, 99999), // Disallowed, volume ignored
				createTransferLog(token1, walletB, walletC, 101),   // Allowed: C total = 101
				createTransferLog(token1, walletB, walletA, 50),    // Allowed: A total = 150 (new max)
			},
			isAllowedFunc: allowOnlyAandC,
			expectedMap:   map[common.Address]MaxTransferRecord{token1: {Address: walletA, Amount: big.NewInt(150)}},
			description:   "Should completely ignore transfers to a disallowed address when summing volume.",
		},
		{
			name:          "Input - Empty logger slice",
			logs:          []types.Log{},
			isAllowedFunc: allowAll,
			expectedMap:   map[common.Address]MaxTransferRecord{},
			description:   "Should return an empty map for empty logger input.",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			t.Log(tc.description)
			actualMap := ExtractMaxTotalVolumeReceiver(tc.logs, tc.isAllowedFunc)
			verifyMaxTransferRecordMapTestHelper(t, tc.expectedMap, actualMap)
		})
	}
}

// FuzzExtractMaxSingleReceiver ensures the function can gracefully handle arbitrary data.
func FuzzExtractMaxSingleReceiver(f *testing.F) {
	f.Add(common.HexToAddress("0x1").Bytes(), common.HexToAddress("0xA").Bytes(), int64(100))
	f.Fuzz(func(t *testing.T, addrBytes, toBytes []byte, amount int64) {
		logger := types.Log{
			Address: common.BytesToAddress(addrBytes),
			Topics: []common.Hash{
				abi.ERC20ABI.Events["Transfer"].ID,
				common.HexToHash("0x0"), // from
				common.BytesToHash(toBytes),
			},
			Data: new(big.Int).SetInt64(amount).Bytes(),
		}
		// Test will fail if the function panics.
		ExtractMaxSingleReceiver([]types.Log{logger}, func(common.Address) bool { return true })
	})
}

// FuzzExtractMaxTotalVolumeReceiver ensures the summation logic can gracefully handle arbitrary data.
func FuzzExtractMaxTotalVolumeReceiver(f *testing.F) {
	f.Add(common.HexToAddress("0x1").Bytes(), common.HexToAddress("0xA").Bytes(), int64(100))
	f.Fuzz(func(t *testing.T, addrBytes, toBytes []byte, amount int64) {
		logger := types.Log{
			Address: common.BytesToAddress(addrBytes),
			Topics: []common.Hash{
				abi.ERC20ABI.Events["Transfer"].ID,
				common.HexToHash("0x0"), // from
				common.BytesToHash(toBytes),
			},
			Data: new(big.Int).SetInt64(amount).Bytes(),
		}
		// Test will fail if the function panics.
		ExtractMaxTotalVolumeReceiver([]types.Log{logger}, func(common.Address) bool { return true })
	})
}
