package aerodrome

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/big"
	"testing"

	ethclients "github.com/defistate/defistate/clients/eth-clients"
	uniswapv3abi "github.com/defistate/defistate/protocols/uniswap-v3/abi"
	aerodromeabi "github.com/defistate/defistate/protocols/uniswap-v3/abi/forks/aerodrome"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPoolInitializer(t *testing.T) {
	// --- Setup ---
	mockClient := ethclients.NewTestETHClient()
	multicallAddress := common.HexToAddress("0xca11bde05977b3631167028862be2a173976ca11")
	poolAddress := common.HexToAddress("0x88e6A0c2dDD26FEEb64F039a2c41296FcB3f5640") // USDC/WETH 0.05%
	blockNumber := big.NewInt(1)
	// Declare shared struct types for use in all sub-tests.
	type multicallCall struct {
		Target   common.Address
		CallData []byte
	}
	type multicallResult struct {
		Success    bool
		ReturnData []byte
	}

	// --- Test Case 1: Successful fetch ---
	t.Run("successfully fetches all pool info via multicall", func(t *testing.T) {
		// --- Mock Data Setup ---
		mockToken0 := common.HexToAddress("0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48")
		mockToken1 := common.HexToAddress("0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2")
		mockFee := uint64(500)
		mockTickSpacing := uint64(10)
		mockTick := int64(200000)
		mockLiquidity := big.NewInt(123456789012345)
		mockSqrtPriceX96 := new(big.Int)
		mockSqrtPriceX96.SetString("1461345844772434763942354344", 10)

		// 1. Prepare call and return data for each function
		callNames := []string{"token0", "token1", "fee", "tickSpacing", "slot0", "liquidity"}
		mockReturnValues := []interface{}{
			mockToken0,
			mockToken1,
			big.NewInt(int64(mockFee)),
			big.NewInt(int64(mockTickSpacing)),
			// The full 6-field struct for slot0
			[]interface{}{mockSqrtPriceX96, big.NewInt(mockTick), uint16(0), uint16(0), uint16(0), true},
			mockLiquidity,
		}

		expectedMulticallCalls := make([]multicallCall, len(callNames))
		mockMulticallResults := make([]multicallResult, len(callNames))

		for i, name := range callNames {
			callData, err := aerodromeabi.AerodromeABI.Pack(name)
			require.NoError(t, err)
			expectedMulticallCalls[i] = multicallCall{Target: poolAddress, CallData: callData}

			var returnData []byte
			// Handle the multi-value return from slot0 differently
			if name == "slot0" {
				slot0Values := mockReturnValues[i].([]interface{})
				returnData, err = aerodromeabi.AerodromeABI.Methods[name].Outputs.Pack(slot0Values...)
			} else {
				returnData, err = aerodromeabi.AerodromeABI.Methods[name].Outputs.Pack(mockReturnValues[i])
			}
			require.NoError(t, err)
			mockMulticallResults[i] = multicallResult{Success: true, ReturnData: returnData}
		}

		// 2. Prepare the final batched multicall request and response
		multicallInput, err := uniswapv3abi.Multicall3ABI.Pack("tryAggregate", false, expectedMulticallCalls)
		require.NoError(t, err)
		multicallOutput, err := uniswapv3abi.Multicall3ABI.Methods["tryAggregate"].Outputs.Pack(mockMulticallResults)
		require.NoError(t, err)

		// 3. Set the mock handler
		mockClient.SetCallContractHandler(func(ctx context.Context, msg ethereum.CallMsg, blockNumber *big.Int) ([]byte, error) {
			if msg.To == nil || msg.To.Hex() != multicallAddress.Hex() {
				return nil, fmt.Errorf("unexpected call to %s", msg.To.Hex())
			}
			if bytes.Equal(msg.Data, multicallInput) {
				return multicallOutput, nil
			}
			return nil, fmt.Errorf("unexpected call data %x", msg.Data)
		})

		// --- Action ---
		provider := NewPoolInitializer(multicallAddress)
		token0, token1, fee, tickSpacing, tick, liquidity, sqrtPriceX96, err := provider(context.Background(), poolAddress, mockClient, blockNumber)

		// --- Assertions ---
		require.NoError(t, err)
		assert.Equal(t, mockToken0.Hex(), token0.Hex(), "Token0 should match")
		assert.Equal(t, mockToken1.Hex(), token1.Hex(), "Token1 should match")
		assert.Equal(t, mockFee, fee, "Fee should match")
		assert.Equal(t, mockTickSpacing, tickSpacing, "TickSpacing should match")
		assert.Equal(t, mockTick, tick, "Tick should match")
		assert.Equal(t, 0, mockLiquidity.Cmp(liquidity), "Liquidity should match")
		assert.Equal(t, 0, mockSqrtPriceX96.Cmp(sqrtPriceX96), "SqrtPriceX96 should match")
	})

	// --- Test Case 2: Multicall RPC itself fails ---
	t.Run("returns error when multicall rpc fails", func(t *testing.T) {
		// --- Mock Setup ---
		mockClient.SetCallContractHandler(func(ctx context.Context, msg ethereum.CallMsg, blockNumber *big.Int) ([]byte, error) {
			return nil, errors.New("simulated rpc error")
		})

		// --- Action ---
		provider := NewPoolInitializer(multicallAddress)
		_, _, _, _, _, _, _, err := provider(context.Background(), poolAddress, mockClient, blockNumber)

		// --- Assertions ---
		require.Error(t, err)
		require.ErrorIs(t, err, ErrPoolInitMulticallFailed)
	})

	// --- Test Case 3: A sub-call fails ---
	t.Run("returns error when a subcall fails", func(t *testing.T) {
		// --- Mock Setup ---
		// FIX: Provide valid, packed return data for the calls that are supposed to succeed.
		// The error was caused by the provider failing to unpack invalid data (`0x01`)
		// before it even got to the intentionally failed call.
		token0Return, err := aerodromeabi.AerodromeABI.Methods["token0"].Outputs.Pack(common.Address{})
		require.NoError(t, err)
		token1Return, err := aerodromeabi.AerodromeABI.Methods["token1"].Outputs.Pack(common.Address{})
		require.NoError(t, err)

		// Create a valid response for all calls except one.
		mockMulticallResults := []multicallResult{
			{Success: true, ReturnData: token0Return},
			{Success: true, ReturnData: token1Return},
			{Success: false, ReturnData: []byte{}}, // Make the 'fee' call fail.
			{Success: true, ReturnData: []byte{}},  // The rest don't matter as the code will exit.
			{Success: true, ReturnData: []byte{}},
			{Success: true, ReturnData: []byte{}},
		}

		multicallOutput, err := uniswapv3abi.Multicall3ABI.Methods["tryAggregate"].Outputs.Pack(mockMulticallResults)
		require.NoError(t, err)

		mockClient.SetCallContractHandler(func(ctx context.Context, msg ethereum.CallMsg, blockNumber *big.Int) ([]byte, error) {
			// We don't need to check the input here, just return the crafted failure response.
			return multicallOutput, nil
		})

		// --- Action ---
		provider := NewPoolInitializer(multicallAddress)
		_, _, _, _, _, _, _, err = provider(context.Background(), poolAddress, mockClient, blockNumber)

		// --- Assertions ---
		require.Error(t, err)
		require.ErrorIs(t, err, ErrInvalidPoolInitResponse)
		assert.Contains(t, err.Error(), "fee call failed")
	})
}
