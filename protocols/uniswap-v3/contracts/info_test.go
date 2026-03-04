package contracts

import (
	"context"
	"math/big"
	"testing"

	ethclients "github.com/defistate/defistate/clients/eth-clients"
	uniswapv3abi "github.com/defistate/defistate/protocols/uniswap-v3/abi"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewBatchPoolInfoProvider_Multicall(t *testing.T) {
	// --- Setup ---
	mockClient := ethclients.NewTestETHClient()
	getClient := func() (ethclients.ETHClient, error) {
		return mockClient, nil
	}
	multicallAddress := common.HexToAddress("0xca11bde05977b3631167028862be2a173976ca11")
	blockNumber := big.NewInt(1)
	pool1 := common.HexToAddress("0x1111111111111111111111111111111111111111")
	pool2 := common.HexToAddress("0x2222222222222222222222222222222222222222")

	// Helper to pack Slot0 return data
	packSlot0 := func(price *big.Int, tick int64) []byte {
		out := []interface{}{
			price, big.NewInt(tick), uint16(0), uint16(0), uint16(0), uint8(0), true,
		}
		data, _ := uniswapv3abi.UniswapV3ABI.Methods["slot0"].Outputs.Pack(out...)
		return data
	}

	// Helper to pack uint256 (Liquidity/Fee)
	packUint256 := func(val *big.Int) []byte {
		data, _ := uniswapv3abi.UniswapV3ABI.Methods["liquidity"].Outputs.Pack(val)
		return data
	}

	t.Run("successfully fetches data for multiple pools via tryAggregate", func(t *testing.T) {
		// Mock Data for Pool 1
		p1Price, p1Tick, p1Liq, p1Fee := big.NewInt(1000), int64(10), big.NewInt(1000000), uint64(500)
		// Mock Data for Pool 2
		p2Price, p2Tick, p2Liq, p2Fee := big.NewInt(2000), int64(20), big.NewInt(2000000), uint64(3000)

		mockClient.SetCallContractHandler(func(ctx context.Context, msg ethereum.CallMsg, _ *big.Int) ([]byte, error) {
			require.Equal(t, multicallAddress, *msg.To)

			// Unpack inputs for tryAggregate(bool, Call[])
			args, err := uniswapv3abi.Multicall3ABI.Methods["tryAggregate"].Inputs.Unpack(msg.Data[4:])
			require.NoError(t, err)

			calls := args[1].([]struct {
				Target   common.Address `json:"target"`
				CallData []byte         `json:"callData"`
			})
			require.Len(t, calls, 6)

			results := []Multicall2Result{
				// Pool 1 triplet
				{Success: true, ReturnData: packSlot0(p1Price, p1Tick)},
				{Success: true, ReturnData: packUint256(p1Liq)},
				{Success: true, ReturnData: packUint256(new(big.Int).SetUint64(p1Fee))},
				// Pool 2 triplet
				{Success: true, ReturnData: packSlot0(p2Price, p2Tick)},
				{Success: true, ReturnData: packUint256(p2Liq)},
				{Success: true, ReturnData: packUint256(new(big.Int).SetUint64(p2Fee))},
			}

			return uniswapv3abi.Multicall3ABI.Methods["tryAggregate"].Outputs.Pack(results)
		})

		provider := NewBatchPoolInfoProvider(multicallAddress)
		ticks, liqs, prices, fees, errs := provider(context.Background(), []common.Address{pool1, pool2}, getClient, blockNumber)

		// --- Assertions ---
		require.Len(t, ticks, 2)

		// Pool 1 Validation
		assert.NoError(t, errs[0])
		assert.Equal(t, p1Tick, ticks[0])
		assert.Equal(t, 0, p1Liq.Cmp(liqs[0]), "Liquidity for pool 1 should match")
		assert.Equal(t, 0, p1Price.Cmp(prices[0]), "Price for pool 1 should match")
		assert.Equal(t, p1Fee, fees[0])

		// Pool 2 Validation
		assert.NoError(t, errs[1])
		assert.Equal(t, p2Tick, ticks[1])
		assert.Equal(t, 0, p2Liq.Cmp(liqs[1]), "Liquidity for pool 2 should match")
		assert.Equal(t, 0, p2Price.Cmp(prices[1]), "Price for pool 2 should match")
		assert.Equal(t, p2Fee, fees[1])
	})

	t.Run("handles partial failure where one pool reverts", func(t *testing.T) {
		mockClient.SetCallContractHandler(func(ctx context.Context, msg ethereum.CallMsg, _ *big.Int) ([]byte, error) {
			results := []Multicall2Result{
				// Pool 1 succeeds
				{Success: true, ReturnData: packSlot0(big.NewInt(1), 1)},
				{Success: true, ReturnData: packUint256(big.NewInt(100))},
				{Success: true, ReturnData: packUint256(big.NewInt(500))},
				// Pool 2 fails on slot0
				{Success: false, ReturnData: []byte{}},
				{Success: true, ReturnData: packUint256(big.NewInt(200))},
				{Success: true, ReturnData: packUint256(big.NewInt(3000))},
			}
			return uniswapv3abi.Multicall3ABI.Methods["tryAggregate"].Outputs.Pack(results)
		})

		provider := NewBatchPoolInfoProvider(multicallAddress)
		ticks, liqs, _, _, errs := provider(context.Background(), []common.Address{pool1, pool2}, getClient, blockNumber)

		// Pool 1 should be fine
		assert.NoError(t, errs[0])
		assert.Equal(t, int64(1), ticks[0])
		assert.Equal(t, uint64(100), liqs[0].Uint64())

		// Pool 2 should have the error we set in the production code
		assert.Error(t, errs[1])
		assert.Contains(t, errs[1].Error(), "reverted")
	})
}
