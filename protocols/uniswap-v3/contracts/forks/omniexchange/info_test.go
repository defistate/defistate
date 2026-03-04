package omniexchange

import (
	"context"
	"fmt"
	"math/big"
	"testing"

	ethclients "github.com/defistate/defistate/clients/eth-clients"
	uniswapv3abi "github.com/defistate/defistate/protocols/uniswap-v3/abi"
	omniexchangeabi "github.com/defistate/defistate/protocols/uniswap-v3/abi/forks/omniexchange"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewBatchPoolInfoProvider_SixPools(t *testing.T) {
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

	packOmniSlot0 := func(price *big.Int, tick int64) []byte {
		out := []interface{}{price, big.NewInt(tick), uint16(0), uint16(0), uint16(0), uint32(0), true}
		data, _ := omniexchangeabi.OmniExchangeABI.Methods["slot0"].Outputs.Pack(out...)
		return data
	}

	packUint256 := func(val *big.Int) []byte {
		data, _ := omniexchangeabi.OmniExchangeABI.Methods["liquidity"].Outputs.Pack(val)
		return data
	}

	t.Run("successfully fetches data for 6 pools concurrently", func(t *testing.T) {
		mockClient.SetCallContractHandler(func(ctx context.Context, msg ethereum.CallMsg, _ *big.Int) ([]byte, error) {
			args, err := uniswapv3abi.Multicall3ABI.Methods["tryAggregate"].Inputs.Unpack(msg.Data[4:])
			require.NoError(t, err)

			calls := args[1].([]struct {
				Target   common.Address `json:"target"`
				CallData []byte         `json:"callData"`
			})
			require.Len(t, calls, 18) // 6 pools * 3 calls

			results := make([]Multicall2Result, 18)
			for i := 0; i < 6; i++ {
				base := i * 3
				// Unique data per pool based on index
				price := big.NewInt(int64(1000 + i))
				tick := int64(i)
				liq := big.NewInt(int64(1000000 * (i + 1)))
				fee := big.NewInt(int64(500 * (i + 1)))

				results[base] = Multicall2Result{Success: true, ReturnData: packOmniSlot0(price, tick)}
				results[base+1] = Multicall2Result{Success: true, ReturnData: packUint256(liq)}
				results[base+2] = Multicall2Result{Success: true, ReturnData: packUint256(fee)}
			}

			return uniswapv3abi.Multicall3ABI.Methods["tryAggregate"].Outputs.Pack(results)
		})

		provider := NewBatchPoolInfoProvider(multicallAddress)
		ticks, liqs, prices, fees, errs := provider(context.Background(), pools, getClient, blockNumber)

		require.Len(t, ticks, 6)
		require.Len(t, errs, 6)

		for i := 0; i < 6; i++ {
			assert.NoError(t, errs[i], "Pool %d should not have error", i)
			assert.Equal(t, int64(i), ticks[i], "Tick mismatch at index %d", i)
			assert.Equal(t, uint64(1000000*(i+1)), liqs[i].Uint64(), "Liquidity mismatch at index %d", i)
			assert.Equal(t, uint64(500*(i+1)), fees[i], "Fee mismatch at index %d", i)
			assert.Equal(t, uint64(1000+i), prices[i].Uint64(), "Price mismatch at index %d", i)
		}
	})
}
