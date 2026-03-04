package pancakeswap

import (
	"context"
	"fmt"
	"math/big"
	"testing"

	ethclients "github.com/defistate/defistate/clients/eth-clients"
	uniswapv3abi "github.com/defistate/defistate/protocols/uniswap-v3/abi"
	pancakeswapv3abi "github.com/defistate/defistate/protocols/uniswap-v3/abi/forks/pancakeswap"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewBatchPoolInfoProvider_PancakeSwap(t *testing.T) {
	// --- Setup ---
	mockClient := ethclients.NewTestETHClient()
	getClient := func() (ethclients.ETHClient, error) {
		return mockClient, nil
	}
	multicallAddress := common.HexToAddress("0xca11bde05977b3631167028862be2a173976ca11")
	blockNumber := big.NewInt(1)
	// Generate 6 mock pool addresses
	pools := make([]common.Address, 6)
	for i := 0; i < 6; i++ {
		pools[i] = common.HexToAddress(fmt.Sprintf("0x%d%d%d%d%d%d%d%d%d%d%d%d%d%d%d%d%d%d%d%d", i, i, i, i, i, i, i, i, i, i, i, i, i, i, i, i, i, i, i, i))
	}

	// Helper to pack PancakeSwap Slot0 return data
	packPancakeSlot0 := func(price *big.Int, tick int64) []byte {
		out := []interface{}{
			price, big.NewInt(tick), uint16(0), uint16(0), uint16(0), uint32(0), true,
		}
		data, _ := pancakeswapv3abi.PancakeswapV3ABI.Methods["slot0"].Outputs.Pack(out...)
		return data
	}

	// Helper to pack uint256 (Liquidity/Fee) ensuring 32-byte alignment
	packUint256 := func(val *big.Int) []byte {
		data, _ := pancakeswapv3abi.PancakeswapV3ABI.Methods["liquidity"].Outputs.Pack(val)
		return data
	}

	t.Run("successfully fetches data for 6 pools including high-bit liquidity", func(t *testing.T) {
		// Mock a "Max Uint128" liquidity value.
		// If parsed as signed int128, this would be -1.
		maxUint128, _ := new(big.Int).SetString("340282366920938463463374607431768211455", 10)

		mockClient.SetCallContractHandler(func(ctx context.Context, msg ethereum.CallMsg, _ *big.Int) ([]byte, error) {
			require.Equal(t, multicallAddress, *msg.To)

			// tryAggregate(bool requireSuccess, Call[] calls)
			args, err := uniswapv3abi.Multicall3ABI.Methods["tryAggregate"].Inputs.Unpack(msg.Data[4:])
			require.NoError(t, err)

			calls := args[1].([]struct {
				Target   common.Address `json:"target"`
				CallData []byte         `json:"callData"`
			})
			require.Len(t, calls, 18) // 6 pools * 3 calls

			results := make([]Multicall2Result, 18)
			for i := 0; i < 6; i++ {
				baseIdx := i * 3

				// We use the "negative-looking" value for the first pool
				liqVal := big.NewInt(int64(1000000 * (i + 1)))
				if i == 0 {
					liqVal = maxUint128
				}

				results[baseIdx] = Multicall2Result{Success: true, ReturnData: packPancakeSlot0(big.NewInt(1000), 10)}
				results[baseIdx+1] = Multicall2Result{Success: true, ReturnData: packUint256(liqVal)}
				results[baseIdx+2] = Multicall2Result{Success: true, ReturnData: packUint256(big.NewInt(2500))}
			}

			return uniswapv3abi.Multicall3ABI.Methods["tryAggregate"].Outputs.Pack(results)
		})

		provider := NewBatchPoolInfoProvider(multicallAddress)
		ticks, liqs, _, fees, errs := provider(context.Background(), pools, getClient, blockNumber)

		// --- Assertions ---
		require.Len(t, ticks, 6)

		// Validate Pool 0 (The high-bit liquidity pool)
		assert.NoError(t, errs[0])
		assert.Equal(t, 0, maxUint128.Cmp(liqs[0]), "High-bit liquidity should be parsed as a large positive uint128, not negative")
		assert.True(t, liqs[0].Sign() > 0, "Liquidity must be positive")

		// Validate Pool 5 (Standard pool)
		assert.NoError(t, errs[5])
		assert.Equal(t, uint64(2500), fees[5])
		assert.Equal(t, uint64(6000000), liqs[5].Uint64())
	})

	t.Run("handles partial batch failure", func(t *testing.T) {
		mockClient.SetCallContractHandler(func(ctx context.Context, msg ethereum.CallMsg, _ *big.Int) ([]byte, error) {
			results := make([]Multicall2Result, 18)
			for i := 0; i < 6; i++ {
				baseIdx := i * 3
				// Force pool index 3 to fail
				success := i != 3
				results[baseIdx] = Multicall2Result{Success: success, ReturnData: packPancakeSlot0(big.NewInt(1), 1)}
				results[baseIdx+1] = Multicall2Result{Success: success, ReturnData: packUint256(big.NewInt(1))}
				results[baseIdx+2] = Multicall2Result{Success: success, ReturnData: packUint256(big.NewInt(1))}
			}
			return uniswapv3abi.Multicall3ABI.Methods["tryAggregate"].Outputs.Pack(results)
		})

		provider := NewBatchPoolInfoProvider(multicallAddress)
		_, _, _, _, errs := provider(context.Background(), pools, getClient, blockNumber)

		assert.NoError(t, errs[0])
		assert.Error(t, errs[3], "Pool at index 3 should have failed")
		assert.Contains(t, errs[3].Error(), "reverted")
		assert.NoError(t, errs[5])
	})
}
