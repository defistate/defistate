package aerodrome

import (
	"context"
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

func TestNewBatchPoolInfoProvider_Aerodrome(t *testing.T) {
	// --- Setup ---
	mockClient := ethclients.NewTestETHClient()
	getClient := func() (ethclients.ETHClient, error) {
		return mockClient, nil
	}
	multicallAddress := common.HexToAddress("0xca11bde05977b3631167028862be2a173976ca11")
	blockNumber := big.NewInt(1)
	pool1 := common.HexToAddress("0x1111111111111111111111111111111111111111")
	pool2 := common.HexToAddress("0x2222222222222222222222222222222222222222")

	// Helper to pack Aerodrome Slot0 return data
	// Aerodrome Slot0: (uint160 sqrtPriceX96, int24 tick, uint16 observationIndex,
	// uint16 observationCardinality, uint16 observationCardinalityNext, bool unlocked)
	packAerodromeSlot0 := func(price *big.Int, tick int64) []byte {
		out := []interface{}{
			price, big.NewInt(tick), uint16(0), uint16(0), uint16(0), true,
		}
		data, _ := aerodromeabi.AerodromeABI.Methods["slot0"].Outputs.Pack(out...)
		return data
	}

	// Helper to pack uint256 (Liquidity/Fee) ensuring 32-byte alignment
	packUint256 := func(val *big.Int) []byte {
		data, _ := aerodromeabi.AerodromeABI.Methods["liquidity"].Outputs.Pack(val)
		return data
	}

	t.Run("successfully fetches data for multiple Aerodrome pools", func(t *testing.T) {
		// Mock Data
		p1Price, p1Tick, p1Liq, p1Fee := big.NewInt(1000), int64(10), big.NewInt(1000000), uint64(500)
		p2Price, p2Tick, p2Liq, p2Fee := big.NewInt(2000), int64(20), big.NewInt(2000000), uint64(3000)

		mockClient.SetCallContractHandler(func(ctx context.Context, msg ethereum.CallMsg, _ *big.Int) ([]byte, error) {
			require.Equal(t, multicallAddress, *msg.To)

			// tryAggregate(bool requireSuccess, Call[] calls)
			args, err := uniswapv3abi.Multicall3ABI.Methods["tryAggregate"].Inputs.Unpack(msg.Data[4:])
			require.NoError(t, err)

			calls := args[1].([]struct {
				Target   common.Address `json:"target"`
				CallData []byte         `json:"callData"`
			})
			require.Len(t, calls, 6)

			results := []Multicall2Result{
				// Pool 1 triplet
				{Success: true, ReturnData: packAerodromeSlot0(p1Price, p1Tick)},
				{Success: true, ReturnData: packUint256(p1Liq)},
				{Success: true, ReturnData: packUint256(new(big.Int).SetUint64(p1Fee))},
				// Pool 2 triplet
				{Success: true, ReturnData: packAerodromeSlot0(p2Price, p2Tick)},
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
		assert.Equal(t, 0, p1Liq.Cmp(liqs[0]))
		assert.Equal(t, p1Fee, fees[0])

		// Pool 2 Validation
		assert.NoError(t, errs[1])
		assert.Equal(t, 0, p2Price.Cmp(prices[1]))
		assert.Equal(t, p2Fee, fees[1])
	})

	t.Run("handles Aerodrome subcall failure", func(t *testing.T) {
		mockClient.SetCallContractHandler(func(ctx context.Context, msg ethereum.CallMsg, _ *big.Int) ([]byte, error) {
			results := []Multicall2Result{
				// Pool 1 succeeds
				{Success: true, ReturnData: packAerodromeSlot0(big.NewInt(1), 1)},
				{Success: true, ReturnData: packUint256(big.NewInt(100))},
				{Success: true, ReturnData: packUint256(big.NewInt(500))},
				// Pool 2 fails
				{Success: false, ReturnData: []byte{}},
				{Success: true, ReturnData: packUint256(big.NewInt(200))},
				{Success: true, ReturnData: packUint256(big.NewInt(3000))},
			}
			return uniswapv3abi.Multicall3ABI.Methods["tryAggregate"].Outputs.Pack(results)
		})

		provider := NewBatchPoolInfoProvider(multicallAddress)
		_, _, _, _, errs := provider(context.Background(), []common.Address{pool1, pool2}, getClient, blockNumber)

		assert.NoError(t, errs[0])
		assert.Error(t, errs[1])
		assert.Contains(t, errs[1].Error(), "reverted")
	})
}
