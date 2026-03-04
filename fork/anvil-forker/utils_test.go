package fork

import (
	"context"
	"encoding/hex"
	"errors"
	"math/big"
	"testing"

	ethclients "github.com/defistate/defistate/clients/eth-clients"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/assert"
)

func TestGetLatestBlockNumber(t *testing.T) {
	client := ethclients.NewTestETHClient()
	expected := uint64(64)

	client.SetBlockNumberHandler(func(ctx context.Context) (uint64, error) {
		return expected, nil
	})

	actual, err := getLatestBlockNumber(client)
	assert.NoError(t, err)
	assert.Equal(t, expected, actual)
}

func TestCalcFeePercentage(t *testing.T) {
	cases := []struct {
		sent, received string
		expected       uint
	}{
		{"1000", "900", 10},
		{"1000", "1000", 0},
		{"1000", "0", 100},
	}

	for _, tc := range cases {
		t.Run(tc.sent+"_"+tc.received, func(t *testing.T) {
			sent, _ := new(big.Int).SetString(tc.sent, 10)
			received, _ := new(big.Int).SetString(tc.received, 10)
			result := calcFeePercentage(sent, received)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestGetERC20Balance(t *testing.T) {
	client := ethclients.NewTestETHClient()
	expected := big.NewInt(1e17)

	client.SetCallContractHandler(func(ctx context.Context, msg ethereum.CallMsg, blockNumber *big.Int) ([]byte, error) {
		sig := hex.EncodeToString(msg.Data)
		if len(sig) < 8 || sig[:8] != "70a08231" {
			return nil, errors.New("unexpected method signature")
		}

		uint256Ty, _ := abi.NewType("uint256", "uint256", nil)
		args := abi.Arguments{{Type: uint256Ty}}
		return args.Pack(expected)
	})

	token := common.HexToAddress("0x00102")
	user := common.HexToAddress("0xdeadbeef")

	actual, err := getERC20Balance(context.Background(), client, token, user)

	assert.NoError(t, err)
	assert.EqualValues(t, expected, actual)
}

func TestTransferERC20(t *testing.T) {
	expectedHash := common.HexToHash("0xdeadbeef")
	expectedReceipt := &types.Receipt{TxHash: expectedHash}

	sendTransactionFn := func(from, token common.Address, data []byte) (common.Hash, error) {
		return expectedHash, nil
	}

	getReceiptFn := func(common.Hash) (*types.Receipt, error) {
		return expectedReceipt, nil
	}

	from := common.Address{}
	to := common.Address{}
	token := common.Address{}
	amount := big.NewInt(1000)

	actualReceipt, err := transferERC20(
		from,
		to,
		token,
		amount,
		sendTransactionFn,
		getReceiptFn,
	)

	assert.Nil(t, err, "expected no error")
	assert.NotNil(t, actualReceipt, "expected reciept")
}
