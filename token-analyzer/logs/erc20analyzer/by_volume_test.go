package erc20analyzer

import (
	"context"
	"math/big"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/defistate/defistate/token-analyzer/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Shared Test Fixtures & Helpers ---

var (
	testToken1  = common.HexToAddress("0x1")
	testToken2  = common.HexToAddress("0x2")
	testWalletA = common.HexToAddress("0xA")
	testWalletB = common.HexToAddress("0xB")
	testWalletC = common.HexToAddress("0xC")
	testWalletD = common.HexToAddress("0xD")
)

// createTransferLog is a shared helper to build a valid ERC20 Transfer logger for tests.
func createTransferLog(t *testing.T, token, from, to common.Address, amount int64) types.Log {
	t.Helper() // Mark this as a test helper function.
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

// TestVolumeAnalyzer_Lifecycle provides a comprehensive, end-to-end test for the
// VolumeAnalyzer component. It verifies the entire lifecycle in a sequential flow.
func TestVolumeAnalyzer_Lifecycle(t *testing.T) {
	// --- Test Configuration ---
	expiryCheckFrequency := 50 * time.Millisecond
	recordStaleDuration := 100 * time.Millisecond

	// --- Step 1: Initialization ---
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // Ensure shutdown is called at the end of the test.

	cfg := Config{
		ExpiryCheckFrequency: expiryCheckFrequency,
		RecordStaleDuration:  recordStaleDuration,
		IsAllowedAddress:     func(common.Address) bool { return true },
	}
	analyzer, err := NewVolumeAnalyzer(ctx, cfg)
	require.NoError(t, err)
	require.NotNil(t, analyzer, "Constructor should not return nil")

	// --- Step 2: Initial Update and Verification ---
	t.Log("Lifecycle Step: Initial Update")
	initialLogs := []types.Log{
		createTransferLog(t, testToken1, testWalletA, testWalletC, 100),
		createTransferLog(t, testToken1, testWalletB, testWalletC, 500), // B is max
		createTransferLog(t, testToken1, testWalletA, testWalletC, 200), // A's total is 300
	}
	analyzer.Update(initialLogs)

	holders := analyzer.TokenByMaxKnownHolder()
	require.Len(t, holders, 1, "Should have one token record after initial update")
	assert.Equal(t, testWalletB, holders[testToken1], "WalletB should be the initial max holder")

	// --- Step 3: Filtering Logic Verification ---
	t.Log("Lifecycle Step: Verifying Filtering Logic")
	filterCtx, filterCancel := context.WithCancel(context.Background())
	defer filterCancel()

	// Create a new analyzer with a filter that only allows wallet A
	filterCfg := Config{
		ExpiryCheckFrequency: expiryCheckFrequency,
		RecordStaleDuration:  recordStaleDuration,
		IsAllowedAddress:     func(addr common.Address) bool { return addr == testWalletA },
	}
	filteredAnalyzer, err := NewVolumeAnalyzer(filterCtx, filterCfg)
	require.NoError(t, err)
	filterLogs := []types.Log{
		createTransferLog(t, testToken1, testWalletA, testWalletC, 100), // Allowed
		createTransferLog(t, testToken1, testWalletB, testWalletC, 999), // Disallowed
	}
	filteredAnalyzer.Update(filterLogs)
	filteredHolders := filteredAnalyzer.TokenByMaxKnownHolder()
	require.Len(t, filteredHolders, 1, "Filtered analyzer should have one record")
	assert.Equal(t, testWalletA, filteredHolders[testToken1], "Only WalletA should be considered due to the filter")

	// --- Step 4: Record Reset Verification ---
	t.Log("Lifecycle Step: Verifying Record Reset")
	waitDuration := recordStaleDuration + expiryCheckFrequency

	// The record should NOT be deleted. Instead, its amount should be reset to zero.
	assert.Eventually(t, func() bool {
		record, ok := analyzer.RecordByToken(testToken1)
		require.True(t, ok, "Record for testToken1 should not be deleted")
		// Check if the amount is now zero
		return record.Amount.Cmp(big.NewInt(0)) == 0
	}, waitDuration*2, expiryCheckFrequency/2, "Record amount should be reset to zero")

	// Verify the holder address is preserved
	holders = analyzer.TokenByMaxKnownHolder()
	require.Len(t, holders, 1, "Holders map should still contain one record")
	assert.Equal(t, testWalletB, holders[testToken1], "Holder address should be preserved after reset")

	// --- Step 5: State Update After Expiry Verification ---
	t.Log("Lifecycle Step: Verifying Update After Expiry")
	secondLogs := []types.Log{
		createTransferLog(t, testToken1, testWalletC, testWalletD, 9999),
	}
	analyzer.Update(secondLogs)

	holdersAfterUpdate := analyzer.TokenByMaxKnownHolder()
	require.Len(t, holdersAfterUpdate, 1, "Should have one token record after second update")
	assert.Equal(t, testWalletC, holdersAfterUpdate[testToken1], "WalletC should be the new max holder")

	// --- Step 6: Graceful Shutdown Verification ---
	t.Log("Lifecycle Step: Verifying Graceful Shutdown via context cancellation")
	cancel() // Trigger shutdown
	// The test will hang if the goroutine doesn't exit, and the race detector will catch issues.
}

// TestVolumeAnalyzer_ConcurrencyAndStress puts the VolumeAnalyzer under heavy load.
func TestVolumeAnalyzer_ConcurrencyAndStress(t *testing.T) {
	// --- Test Configuration ---
	expiryCheckFrequency := 200 * time.Millisecond
	recordStaleDuration := 1 * time.Second

	// --- Step 1: Initialization ---
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := Config{
		ExpiryCheckFrequency: expiryCheckFrequency,
		RecordStaleDuration:  recordStaleDuration,
		IsAllowedAddress:     func(common.Address) bool { return true }, // Allow all for stress test
	}
	analyzer, err := NewVolumeAnalyzer(ctx, cfg)
	require.NoError(t, err)
	require.NotNil(t, analyzer, "Constructor should not return nil")

	// --- Step 2: Concurrent Reads and Writes ---
	var wg sync.WaitGroup
	numGoroutines := 50
	updatesPerGoroutine := 20
	wg.Add(numGoroutines)

	t.Logf("Starting %d goroutines, each performing %d updates...", numGoroutines, updatesPerGoroutine)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < updatesPerGoroutine; j++ {
				logs := []types.Log{
					createTransferLog(t, testToken1, testWalletA, testWalletC, 10),
					createTransferLog(t, testToken1, testWalletB, testWalletD, 20),
					createTransferLog(t, testToken2, testWalletC, testWalletD, 5),
				}
				analyzer.Update(logs)
				time.Sleep(time.Duration(rand.Intn(5)) * time.Millisecond)
			}
		}()
	}

	wg.Wait()

	// --- Step 3: Final State Verification ---
	t.Log("All updates complete. Verifying final state...")
	finalHolders := analyzer.TokenByMaxKnownHolder()

	expectedHolderToken1 := testWalletB
	expectedHolderToken2 := testWalletC

	require.Len(t, finalHolders, 2, "Should have records for exactly two tokens")
	assert.Equal(t, expectedHolderToken1, finalHolders[testToken1], "Incorrect final max holder for token 1")
	assert.Equal(t, expectedHolderToken2, finalHolders[testToken2], "Incorrect final max holder for token 2")

	// --- Step 4: Verify Reset Still Works Under Load ---
	t.Logf("Waiting for all records to become stale (waiting > %s)...", recordStaleDuration)

	assert.Eventually(t, func() bool {
		record1, ok1 := analyzer.RecordByToken(testToken1)
		record2, ok2 := analyzer.RecordByToken(testToken2)

		// Both records must exist and have their amounts reset to zero
		return ok1 && ok2 &&
			record1.Amount.Cmp(big.NewInt(0)) == 0 &&
			record2.Amount.Cmp(big.NewInt(0)) == 0
	}, recordStaleDuration*2, expiryCheckFrequency/4, "All record amounts should be reset to zero")

	// Verify the final map length is still correct
	finalHolders = analyzer.TokenByMaxKnownHolder()
	require.Len(t, finalHolders, 2, "Holders map should still contain two records")
}
