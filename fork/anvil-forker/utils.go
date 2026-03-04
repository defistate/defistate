package fork

import (
	"context"
	"fmt"
	"log"
	"math/big"
	"strings"
	"time"

	ethclients "github.com/defistate/defistate/clients/eth-clients"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

const erc20TransferABI = `[{"anonymous":false,"inputs":[{"indexed":true,"internalType":"address","name":"owner","type":"address"},{"indexed":true,"internalType":"address","name":"spender","type":"address"},{"indexed":false,"internalType":"uint256","name":"value","type":"uint256"}],"name":"Approval","type":"event"},{"anonymous":false,"inputs":[{"indexed":true,"internalType":"address","name":"from","type":"address"},{"indexed":true,"internalType":"address","name":"to","type":"address"},{"indexed":false,"internalType":"uint256","name":"value","type":"uint256"}],"name":"Transfer","type":"event"},{"inputs":[{"internalType":"address","name":"owner","type":"address"},{"internalType":"address","name":"spender","type":"address"}],"name":"allowance","outputs":[{"internalType":"uint256","name":"","type":"uint256"}],"stateMutability":"view","type":"function"},{"inputs":[{"internalType":"address","name":"spender","type":"address"},{"internalType":"uint256","name":"value","type":"uint256"}],"name":"approve","outputs":[{"internalType":"bool","name":"","type":"bool"}],"stateMutability":"nonpayable","type":"function"},{"inputs":[{"internalType":"address","name":"owner","type":"address"}],"name":"balanceOf","outputs":[{"internalType":"uint256","name":"","type":"uint256"}],"stateMutability":"view","type":"function"},{"inputs":[],"name":"decimals","outputs":[{"internalType":"uint8","name":"","type":"uint8"}],"stateMutability":"view","type":"function"},{"inputs":[],"name":"name","outputs":[{"internalType":"string","name":"","type":"string"}],"stateMutability":"view","type":"function"},{"inputs":[],"name":"symbol","outputs":[{"internalType":"string","name":"","type":"string"}],"stateMutability":"view","type":"function"},{"inputs":[],"name":"totalSupply","outputs":[{"internalType":"uint256","name":"","type":"uint256"}],"stateMutability":"view","type":"function"},{"inputs":[{"internalType":"address","name":"to","type":"address"},{"internalType":"uint256","name":"value","type":"uint256"}],"name":"transfer","outputs":[{"internalType":"bool","name":"","type":"bool"}],"stateMutability":"nonpayable","type":"function"},{"inputs":[{"internalType":"address","name":"from","type":"address"},{"internalType":"address","name":"to","type":"address"},{"internalType":"uint256","name":"value","type":"uint256"}],"name":"transferFrom","outputs":[{"internalType":"bool","name":"","type":"bool"}],"stateMutability":"nonpayable","type":"function"}]`

var ERC20ABI abi.ABI

func init() {
	var err error
	ERC20ABI, err = abi.JSON(strings.NewReader(erc20TransferABI))
	if err != nil {
		log.Fatal(err)
	}

}

func getLatestBlockNumber(client ethclients.ETHClient) (uint64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	return client.BlockNumber(ctx)
}

func calcFeePercentage(sent, received *big.Int) uint {
	if sent.Cmp(big.NewInt(0)) == 0 {
		return 0
	}
	diff := new(big.Int).Sub(sent, received)
	percent := new(big.Int).Mul(diff, big.NewInt(100))
	percent.Div(percent, sent)
	return uint(percent.Uint64())
}

func max(a, b uint) uint {
	if a > b {
		return a
	}
	return b
}

func getERC20Balance(ctx context.Context, client ethclients.ETHClient, token common.Address, address common.Address) (*big.Int, error) {

	data, err := ERC20ABI.Pack("balanceOf", address)
	if err != nil {
		return nil, fmt.Errorf("pack error: %w", err)
	}
	msg := ethereum.CallMsg{
		To:   &token,
		Data: data,
	}

	data, err = client.CallContract(ctx, msg, nil)
	if err != nil {
		return nil, fmt.Errorf("call contract error: %w", err)
	}

	args, err := ERC20ABI.Unpack("balanceOf", data)
	if err != nil {
		return nil, fmt.Errorf("unpack error: %w", err)
	}

	return args[0].(*big.Int), nil

}

func transferERC20(
	from, to, token common.Address,
	amount *big.Int,
	sendTransaction func(from, token common.Address, data []byte) (common.Hash, error),
	getReceipt func(common.Hash) (*types.Receipt, error),
) (*types.Receipt, error) {
	data, err := ERC20ABI.Pack("transfer", to, amount)
	if err != nil {
		return nil, err
	}

	hash, err := sendTransaction(from, token, data)
	if err != nil {
		return nil, err
	}

	receipt, err := getReceipt(hash)
	if err != nil {
		return nil, err
	}

	return receipt, nil
}
