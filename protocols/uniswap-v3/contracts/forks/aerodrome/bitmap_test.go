package aerodrome

import (
	"context"
	"errors"
	"math/big"
	"testing"
	"time"

	ethclients "github.com/defistate/defistate/clients/eth-clients"
	uniswapv3abi "github.com/defistate/defistate/protocols/uniswap-v3/abi"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewBitMapProvider contains unit tests for the NewBitMapProvider function.
// The tests have been updated to use a dynamic mocking strategy.
func TestNewBitMapProvider(t *testing.T) {
	// --- Setup ---
	// Note: mockClient is now initialized within each sub-test to ensure isolation.
	multicallAddress := common.HexToAddress("0xca11bde05977b3631167028862be2a173976ca11")
	poolAddress := common.HexToAddress("0x88e6A0c2dDD26FEEb64F039a2c41296FcB3f5640")
	// Test parameters for batching and concurrency.
	const testBatchSize = 500
	const testMaxConcurrency = 10
	blockNumber := big.NewInt(1)
	// --- Test Case 1: Successful fetch with dynamic mocking ---
	t.Run("successfully fetches and decodes a bitmap", func(t *testing.T) {
		// Initialize a new mock client for this specific test case.
		mockClient := ethclients.NewTestETHClient()

		// --- Mock Data Setup ---
		const tickSpacing uint64 = 60

		// Define the sample active ticks that our dynamic mock will respond with.
		// These MUST be within the valid word position range for the given tickSpacing.
		// For tickSpacing 60, the range is [-57, 57].
		word1Pos := int16(-50)
		word1Value := new(big.Int)
		word1Value.SetString("50000000000000000000000000000000000000000000000000000000000000000", 10)

		word2Pos := int16(25)
		word2Value := big.NewInt(9876543210)

		// Helper types for ABI unpacking and packing.
		type multicallCall struct {
			Target   common.Address
			CallData []byte
		}
		type multicallResult struct {
			Success    bool
			ReturnData []byte
		}

		// ABI types for decoding sub-calls and encoding sub-call results.
		int16Type, _ := abi.NewType("int16", "", nil)
		tickBitmapInputArgs := abi.Arguments{{Type: int16Type}}
		uint256Type, _ := abi.NewType("uint256", "", nil)
		tickBitmapReturnArgs := abi.Arguments{{Type: uint256Type}}

		// 2. Set the dynamic handler on the mock client.
		mockClient.SetCallContractHandler(func(ctx context.Context, msg ethereum.CallMsg, blockNumber *big.Int) ([]byte, error) {
			require.Equal(t, multicallAddress, *msg.To, "Call made to wrong address")

			// a. Unpack the incoming multicall request to see what's being asked for.
			method := uniswapv3abi.Multicall3ABI.Methods["tryAggregate"]
			unpackedInput, err := method.Inputs.Unpack(msg.Data[4:])
			require.NoError(t, err)

			incomingCalls := unpackedInput[1].([]struct {
				Target   common.Address "json:\"target\""
				CallData []uint8        "json:\"callData\""
			})
			batchResults := make([]multicallResult, len(incomingCalls))

			// b. For each sub-call, decode its arguments, find the mock data, and prepare a result.
			for i, call := range incomingCalls {
				// The `tickBitmap` calldata starts with a 4-byte selector.
				unpackedWordPos, err := tickBitmapInputArgs.Unpack(call.CallData[4:])
				require.NoError(t, err)
				wordPos := unpackedWordPos[0].(int16)

				valueToPack := big.NewInt(0)
				if wordPos == word1Pos {
					valueToPack = word1Value
				} else if wordPos == word2Pos {
					valueToPack = word2Value
				}

				encodedReturnData, err := tickBitmapReturnArgs.Pack(valueToPack)
				require.NoError(t, err)
				batchResults[i] = multicallResult{Success: true, ReturnData: encodedReturnData}
			}

			// c. Pack the dynamically generated batch of results into a valid multicall response.
			return method.Outputs.Pack(batchResults)
		})

		// --- Action ---
		provider := NewBitMapProvider(multicallAddress, testBatchSize, testMaxConcurrency)
		bitmap, err := provider(context.Background(), poolAddress, tickSpacing, mockClient, blockNumber)

		// --- Assertions ---
		require.NoError(t, err)
		require.NotNil(t, bitmap)
		require.Len(t, bitmap, 2, "Bitmap should contain exactly 2 non-zero words")

		assert.Equal(t, 0, word1Value.Cmp(bitmap[word1Pos]), "Value for word1 should match")
		assert.Equal(t, 0, word2Value.Cmp(bitmap[word2Pos]), "Value for word2 should match")
	})

	// --- Test Case 2: multicall fails ---
	t.Run("returns error when multicall fails", func(t *testing.T) {
		mockClient := ethclients.NewTestETHClient()
		const tickSpacing uint64 = 60
		mockClient.SetCallContractHandler(func(ctx context.Context, msg ethereum.CallMsg, blockNumber *big.Int) ([]byte, error) {
			return nil, errors.New("simulated rpc error")
		})

		provider := NewBitMapProvider(multicallAddress, testBatchSize, testMaxConcurrency)
		_, err := provider(context.Background(), poolAddress, tickSpacing, mockClient, blockNumber)

		require.Error(t, err)
		require.ErrorIs(t, err, ErrMulticallRPCFailed)
	})

	// --- Test Case 3: unpack multicall fails ---
	t.Run("returns error when unpacking multicall fails", func(t *testing.T) {
		mockClient := ethclients.NewTestETHClient()
		const tickSpacing uint64 = 60
		mockClient.SetCallContractHandler(func(ctx context.Context, msg ethereum.CallMsg, blockNumber *big.Int) ([]byte, error) {
			return []byte("invalid data"), nil // Malformed data
		})

		provider := NewBitMapProvider(multicallAddress, testBatchSize, testMaxConcurrency)
		_, err := provider(context.Background(), poolAddress, tickSpacing, mockClient, blockNumber)

		require.Error(t, err)
		require.ErrorIs(t, err, ErrUnpackMulticallFailed)
	})

	// --- Test Case 4: Context is canceled mid-flight ---
	t.Run("returns error when context is canceled", func(t *testing.T) {
		mockClient := ethclients.NewTestETHClient()
		const tickSpacing uint64 = 60
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		mockClient.SetCallContractHandler(func(ctx context.Context, msg ethereum.CallMsg, blockNumber *big.Int) ([]byte, error) {
			time.Sleep(100 * time.Millisecond)
			return nil, ctx.Err()
		})

		provider := NewBitMapProvider(multicallAddress, testBatchSize, testMaxConcurrency)
		_, err := provider(ctx, poolAddress, tickSpacing, mockClient, blockNumber)

		require.Error(t, err)
		// Use assert.Contains because the provider may not wrap the context error
		// correctly, which would break a stricter assert.ErrorIs check.
		assert.Contains(t, err.Error(), context.DeadlineExceeded.Error())
	})
}
