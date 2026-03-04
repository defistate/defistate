package reserves

import (
	"context"
	"math/big"
	"testing"

	ethclients "github.com/defistate/defistate/clients/eth-clients"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testMulticallAddr = common.HexToAddress("0xcA11bde05977b3631167028862bE2a173976CA11")

func reservesToBytes(r0, r1 *big.Int) []byte {
	// Uniswap V2 getReserves returns (uint112, uint112, uint32)
	// These are padded to 32-byte slots in the return data.
	data := make([]byte, 96)
	copy(data[32-len(r0.Bytes()):32], r0.Bytes())
	copy(data[64-len(r1.Bytes()):64], r1.Bytes())
	return data
}

func TestGetReserves_Multicall(t *testing.T) {
	pool1 := common.HexToAddress("0x1111111111111111111111111111111111111111")
	pool2 := common.HexToAddress("0x2222222222222222222222222222222222222222")
	res1 := []*big.Int{big.NewInt(100), big.NewInt(200)}
	res2 := []*big.Int{big.NewInt(300), big.NewInt(400)}

	client := ethclients.NewTestETHClient()
	getClient := func() (ethclients.ETHClient, error) {
		return client, nil
	}
	client.SetCallContractHandler(func(ctx context.Context, msg ethereum.CallMsg, _ *big.Int) ([]byte, error) {
		require.Equal(t, testMulticallAddr, *msg.To)

		// 1. Strip 4-byte selector to unpack inputs
		// tryAggregate(bool requireSuccess, Call[] calls)
		require.True(t, len(msg.Data) > 4)
		args, err := multicall3ABI.Methods["tryAggregate"].Inputs.Unpack(msg.Data[4:])
		require.NoError(t, err)

		// 2. Extract and cast the calls slice
		// In tryAggregate, requireSuccess is args[0] and calls is args[1]
		calls := args[1].([]struct {
			Target   common.Address `json:"target"`
			CallData []byte         `json:"callData"`
		})

		results := make([]Result, len(calls))
		for i, call := range calls {
			if call.Target == pool1 {
				results[i] = Result{Success: true, ReturnData: reservesToBytes(res1[0], res1[1])}
			} else if call.Target == pool2 {
				results[i] = Result{Success: true, ReturnData: reservesToBytes(res2[0], res2[1])}
			} else {
				results[i] = Result{Success: false}
			}
		}

		// 3. Return the packed Result slice (no selector)
		return multicall3ABI.Methods["tryAggregate"].Outputs.Pack(results)
	})

	// Run with 2 concurrent workers and chunk size of 100
	fetcher := NewGetReserves(2, 100, testMulticallAddr)
	r0s, r1s, errs := fetcher(context.Background(), []common.Address{pool1, pool2}, getClient, big.NewInt(1))

	// --- Assertions ---
	require.Len(t, r0s, 2)

	// Pool 1 checks
	assert.NoError(t, errs[0])
	assert.Equal(t, res1[0].Uint64(), r0s[0].Uint64())
	assert.Equal(t, res1[1].Uint64(), r1s[0].Uint64())

	// Pool 2 checks
	assert.NoError(t, errs[1])
	assert.Equal(t, res2[0].Uint64(), r0s[1].Uint64())
	assert.Equal(t, res2[1].Uint64(), r1s[1].Uint64())
}
