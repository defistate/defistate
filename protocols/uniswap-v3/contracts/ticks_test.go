package contracts

import (
	"context"
	"errors"
	"math/big"
	"testing"

	ethclients "github.com/defistate/defistate/clients/eth-clients"
	uniswapv3abi "github.com/defistate/defistate/protocols/uniswap-v3/abi"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTickInfoProvider(t *testing.T) {
	// --- Setup ---
	multicallAddress := common.HexToAddress("0xca11bde05977b3631167028862be2a173976ca11")
	poolAddress := common.HexToAddress("0x88e6A0c2dDD26FEEb64F039a2c41296FcB3f5640")
	blockNumber := big.NewInt(1)
	const testBatchSize = 100
	const testMaxConcurrency = 10

	// --- Test Case 1: Successful fetch with dynamic mocking ---
	t.Run("successfully fetches info for multiple ticks", func(t *testing.T) {
		mockClient := ethclients.NewTestETHClient()
		ticksToRequest := []int64{199980, 200040, 200100}

		// Create a map to store mock data for easy lookup by tick index.
		mockData := map[int64][]interface{}{
			199980: {
				big.NewInt(100), big.NewInt(-100), big.NewInt(1), big.NewInt(2),
				big.NewInt(3), big.NewInt(4), uint32(5), true,
			},
			200040: { // Uninitialized tick
				big.NewInt(0), big.NewInt(0), big.NewInt(0), big.NewInt(0),
				big.NewInt(0), big.NewInt(0), uint32(0), false,
			},
			200100: {
				big.NewInt(200), big.NewInt(200), big.NewInt(6), big.NewInt(7),
				big.NewInt(8), big.NewInt(9), uint32(10), true,
			},
		}

		// Helper types for ABI unpacking.
		type multicallCall struct {
			Target   common.Address
			CallData []byte
		}
		int24Type, _ := abi.NewType("int24", "", nil)
		tickInputArgs := abi.Arguments{{Type: int24Type}}

		// Set the dynamic handler on the mock client.
		mockClient.SetCallContractHandler(func(ctx context.Context, msg ethereum.CallMsg, blockNumber *big.Int) ([]byte, error) {
			require.Equal(t, multicallAddress, *msg.To)
			method := uniswapv3abi.Multicall3ABI.Methods["tryAggregate"]
			unpackedInput, err := method.Inputs.Unpack(msg.Data[4:])
			require.NoError(t, err)

			incomingCalls := unpackedInput[1].([]struct {
				Target   common.Address "json:\"target\""
				CallData []uint8        "json:\"callData\""
			})
			batchResults := make([]multicallResult, len(incomingCalls))

			for i, call := range incomingCalls {
				unpackedTick, err := tickInputArgs.Unpack(call.CallData[4:])
				require.NoError(t, err)
				tickIndex := unpackedTick[0].(*big.Int).Int64()

				tickData, ok := mockData[tickIndex]
				require.True(t, ok, "received request for an unexpected tick: %d", tickIndex)

				returnData, err := uniswapv3abi.UniswapV3ABI.Methods["ticks"].Outputs.Pack(tickData...)
				require.NoError(t, err)
				batchResults[i] = multicallResult{Success: true, ReturnData: returnData}
			}
			return method.Outputs.Pack(batchResults)
		})

		// --- Action ---
		provider := NewTickInfoProvider(multicallAddress, testBatchSize, testMaxConcurrency)
		tickInfos, err := provider(context.Background(), poolAddress, ticksToRequest, mockClient, blockNumber)

		// --- Assertions ---
		require.NoError(t, err)
		require.Len(t, tickInfos, 2, "Should only return the 2 initialized ticks")

		// Verify data for the first tick. The order is not guaranteed due to concurrency.
		if tickInfos[0].Index != 199980 {
			tickInfos[0], tickInfos[1] = tickInfos[1], tickInfos[0] // Swap for consistent checking
		}
		assert.Equal(t, int64(199980), tickInfos[0].Index)
		assert.Equal(t, 0, big.NewInt(100).Cmp(tickInfos[0].LiquidityGross))
		assert.Equal(t, 0, big.NewInt(-100).Cmp(tickInfos[0].LiquidityNet))

		// Verify data for the second initialized tick.
		assert.Equal(t, int64(200100), tickInfos[1].Index)
		assert.Equal(t, 0, big.NewInt(200).Cmp(tickInfos[1].LiquidityGross))
		assert.Equal(t, 0, big.NewInt(200).Cmp(tickInfos[1].LiquidityNet))
	})

	// --- Test Case 2: Multicall RPC fails ---
	t.Run("returns error when multicall rpc fails", func(t *testing.T) {
		mockClient := ethclients.NewTestETHClient()
		mockClient.SetCallContractHandler(func(ctx context.Context, msg ethereum.CallMsg, blockNumber *big.Int) ([]byte, error) {
			return nil, errors.New("simulated rpc error")
		})
		provider := NewTickInfoProvider(multicallAddress, testBatchSize, testMaxConcurrency)
		_, err := provider(context.Background(), poolAddress, []int64{1, 2, 3}, mockClient, blockNumber)
		require.Error(t, err)
		require.ErrorIs(t, err, ErrTickInfoMulticallFailed)
	})
}
