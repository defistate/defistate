package app

import (
	"bytes"
	"context"
	"encoding/binary"
	"math/big"
	"strings"
	"time"

	ethclients "github.com/defistate/defistate/clients/eth-clients"
	"github.com/defistate/defistate/engine"
	token "github.com/defistate/defistate/protocols/erc20-token-system"
	uniswapv2 "github.com/defistate/defistate/protocols/uniswap-v2"
	uniswapv3 "github.com/defistate/defistate/protocols/uniswap-v3"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

// helpers
var (
	UniswapV2SwapType = uint8(5)
	UniswapV3SwapType = uint8(6)
	erc20ABI          abi.ABI
	swapABI           abi.ABI
)

func init() {
	var err error
	erc20ABI, err = abi.JSON(strings.NewReader(`[{"anonymous":false,"inputs":[{"indexed":true,"internalType":"address","name":"owner","type":"address"},{"indexed":true,"internalType":"address","name":"spender","type":"address"},{"indexed":false,"internalType":"uint256","name":"value","type":"uint256"}],"name":"Approval","type":"event"},{"anonymous":false,"inputs":[{"indexed":true,"internalType":"address","name":"from","type":"address"},{"indexed":true,"internalType":"address","name":"to","type":"address"},{"indexed":false,"internalType":"uint256","name":"value","type":"uint256"}],"name":"Transfer","type":"event"},{"inputs":[{"internalType":"address","name":"owner","type":"address"},{"internalType":"address","name":"spender","type":"address"}],"name":"allowance","outputs":[{"internalType":"uint256","name":"","type":"uint256"}],"stateMutability":"view","type":"function"},{"inputs":[{"internalType":"address","name":"spender","type":"address"},{"internalType":"uint256","name":"value","type":"uint256"}],"name":"approve","outputs":[{"internalType":"bool","name":"","type":"bool"}],"stateMutability":"nonpayable","type":"function"},{"inputs":[{"internalType":"address","name":"owner","type":"address"}],"name":"balanceOf","outputs":[{"internalType":"uint256","name":"","type":"uint256"}],"stateMutability":"view","type":"function"},{"inputs":[],"name":"decimals","outputs":[{"internalType":"uint8","name":"","type":"uint8"}],"stateMutability":"view","type":"function"},{"inputs":[],"name":"name","outputs":[{"internalType":"string","name":"","type":"string"}],"stateMutability":"view","type":"function"},{"inputs":[],"name":"symbol","outputs":[{"internalType":"string","name":"","type":"string"}],"stateMutability":"view","type":"function"},{"inputs":[],"name":"totalSupply","outputs":[{"internalType":"uint256","name":"","type":"uint256"}],"stateMutability":"view","type":"function"},{"inputs":[{"internalType":"address","name":"to","type":"address"},{"internalType":"uint256","name":"value","type":"uint256"}],"name":"transfer","outputs":[{"internalType":"bool","name":"","type":"bool"}],"stateMutability":"nonpayable","type":"function"},{"inputs":[{"internalType":"address","name":"from","type":"address"},{"internalType":"address","name":"to","type":"address"},{"internalType":"uint256","name":"value","type":"uint256"}],"name":"transferFrom","outputs":[{"internalType":"bool","name":"","type":"bool"}],"stateMutability":"nonpayable","type":"function"}]`))
	if err != nil {
		panic(err)
	}

	swapABI, err = abi.JSON(strings.NewReader(`[{"inputs":[{"internalType":"address","name":"_WETH","type":"address"}],"stateMutability":"nonpayable","type":"constructor"},{"stateMutability":"nonpayable","type":"fallback"},{"inputs":[{"internalType":"bytes","name":"data","type":"bytes"}],"name":"conditional","outputs":[{"internalType":"uint256","name":"","type":"uint256"}],"stateMutability":"nonpayable","type":"function"},{"inputs":[{"internalType":"enumSwapTypes","name":"swapType","type":"uint8"}],"name":"getCalcSwapContract","outputs":[{"internalType":"address","name":"","type":"address"}],"stateMutability":"view","type":"function"},{"inputs":[{"internalType":"address","name":"_token","type":"address"}],"name":"getCash","outputs":[],"stateMutability":"nonpayable","type":"function"},{"inputs":[{"internalType":"enumSwapTypes","name":"swapType","type":"uint8"}],"name":"getConditionalContract","outputs":[{"internalType":"address","name":"","type":"address"}],"stateMutability":"view","type":"function"},{"inputs":[{"internalType":"enumSwapTypes","name":"swapType","type":"uint8"}],"name":"getSwapContract","outputs":[{"internalType":"address","name":"","type":"address"}],"stateMutability":"view","type":"function"},{"inputs":[{"internalType":"enumSwapTypes","name":"swapType","type":"uint8"},{"internalType":"address","name":"contractAddress","type":"address"}],"name":"setCalcSwapContract","outputs":[],"stateMutability":"nonpayable","type":"function"},{"inputs":[{"internalType":"enumSwapTypes","name":"swapType","type":"uint8"},{"internalType":"address","name":"contractAddress","type":"address"}],"name":"setConditionalContract","outputs":[],"stateMutability":"nonpayable","type":"function"},{"inputs":[{"internalType":"enumSwapTypes","name":"swapType","type":"uint8"},{"internalType":"address","name":"contractAddress","type":"address"}],"name":"setSwapContract","outputs":[],"stateMutability":"nonpayable","type":"function"},{"inputs":[{"internalType":"uint256","name":"initialAmount","type":"uint256"},{"internalType":"bytes","name":"swapData","type":"bytes"},{"internalType":"address","name":"tokenOutAddress","type":"address"},{"internalType":"address","name":"receiver","type":"address"}],"name":"swap","outputs":[{"internalType":"uint256","name":"","type":"uint256"}],"stateMutability":"nonpayable","type":"function"},{"inputs":[{"internalType":"uint256","name":"initialAmount","type":"uint256"},{"internalType":"bytes","name":"swapData","type":"bytes"},{"internalType":"address","name":"tokenOutAddress","type":"address"},{"internalType":"address","name":"receiver","type":"address"},{"internalType":"uint256","name":"minOut","type":"uint256"}],"name":"swapWithMinOut","outputs":[{"internalType":"uint256","name":"","type":"uint256"}],"stateMutability":"nonpayable","type":"function"},{"inputs":[{"internalType":"uint256","name":"initialAmount","type":"uint256"},{"internalType":"address","name":"tokenInAdress","type":"address"},{"internalType":"address","name":"tokenOutAddress","type":"address"},{"internalType":"address","name":"receiver","type":"address"},{"internalType":"bytes","name":"swapData","type":"bytes"}],"name":"transferFromAndSwap","outputs":[],"stateMutability":"nonpayable","type":"function"},{"stateMutability":"payable","type":"receive"}]`))
	if err != nil {
		panic(err)
	}

}

// internal checks specific to our platform.
// this is just an example impl

func willTransferTokenIn(schema engine.ProtocolSchema) bool {
	switch schema {
	case uniswapv2.UniswapV2ProtocolSchema:
		return true
	case uniswapv3.UniswapV3ProtocolSchema:
		return true
	default:
		return false
	}
}

func requiresTransferTokenOut(schema engine.ProtocolSchema) bool {
	switch schema {
	case uniswapv2.UniswapV2ProtocolSchema:
		return true
	case uniswapv3.UniswapV3ProtocolSchema:
		return false
	default:
		return false
	}
}

func generateUniswapV2SwapDataAndGas(
	pathIndex int,
	tokenIn token.TokenView,
	tokenOut token.TokenView,
	previousPoolWillTransferTokenIn bool,
	nextPoolRequiresTransferTokenOut bool,
	swapCaller common.Address,
	nextPoolAddress common.Address,
	pool uniswapv2.Pool,
) (swapData []byte, gas *big.Int, err error) {

	tokensHaveFee := true
	doTransferIn := pathIndex == 0 || (pathIndex > 0 && !previousPoolWillTransferTokenIn)

	doTransferToNextPool := false
	nextPool := common.Address{}
	if nextPoolAddress == (common.Address{}) {
		// we can transfer to back to the caller
		// since this is the last swap
		doTransferToNextPool = true
		nextPool = swapCaller
	} else if nextPoolRequiresTransferTokenOut {
		doTransferToNextPool = true
		nextPool = nextPoolAddress
	}

	var data = new(bytes.Buffer)
	data.Write(pool.Address.Bytes())
	data.Write(nextPool.Bytes())
	data.Write(tokenIn.Address.Bytes())
	data.Write(tokenOut.Address.Bytes())
	// isToken0
	if tokenIn.ID == pool.IDs.Token0 {
		data.WriteByte(uint8(1))
	} else {
		data.WriteByte(uint8(0))
	}

	// is mod = false
	data.WriteByte(uint8(0))
	if tokensHaveFee {
		data.WriteByte(uint8(1))
	} else {
		data.WriteByte(uint8(0))
	}
	if doTransferIn {
		data.WriteByte(uint8(1))
	} else {
		data.WriteByte(uint8(0))
	}
	if doTransferToNextPool {
		data.WriteByte(uint8(1))
	} else {
		data.WriteByte(uint8(0))
	}

	// set swap data
	swapData = data.Bytes() // should be 117 bytes total at this point

	// Prepend the swap data with the length as a uint16 (2 bytes) and then the swap type (1 byte)
	lengthBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(lengthBytes, uint16(len(swapData)))
	// First prepend the length, then the swap type
	swapData = append(lengthBytes, swapData...)
	swapData = append([]byte{UniswapV2SwapType}, swapData...)

	// a 10k gas buffer + token out gas for transfer
	gasInt64 := 10000 + int64(tokenOut.GasForTransfer)
	if doTransferIn {
		// tokenIn is also transferred
		gasInt64 += int64(tokenIn.GasForTransfer)
	}
	gas = big.NewInt(gasInt64)
	return swapData, gas, nil

}

func generateUniswapV3SwapDataAndGas(
	pathIndex int,
	tokenIn token.TokenView,
	tokenOut token.TokenView,
	previousPoolWillTransferTokenIn bool,
	nextPoolRequiresTransferTokenOut bool,
	swapCaller common.Address,
	nextPoolAddress common.Address,
	pool uniswapv3.Pool,
) (swapData []byte, gas *big.Int, err error) {
	tokensHaveFee := tokenIn.FeeOnTransferPercent > 0 || tokenOut.FeeOnTransferPercent > 0
	doTransferToNextPool := false
	nextPool := common.Address{}
	if nextPoolAddress == (common.Address{}) {
		// we can transfer to back to the caller
		// since this is the last swap
		doTransferToNextPool = true
		nextPool = swapCaller
	} else if nextPoolRequiresTransferTokenOut {
		doTransferToNextPool = true
		nextPool = nextPoolAddress
	}

	data := new(bytes.Buffer)
	data.Write(pool.Address.Bytes())
	data.Write(nextPool.Bytes())
	data.Write(tokenIn.Address.Bytes())
	data.Write(tokenOut.Address.Bytes())
	if tokenIn.ID == pool.IDs.Token0 {
		data.WriteByte(uint8(1))
	} else {
		data.WriteByte(uint8(0))
	}

	if tokensHaveFee {
		data.WriteByte(uint8(1))
	} else {
		data.WriteByte(uint8(0))
	}

	if doTransferToNextPool {
		data.WriteByte(uint8(1))
	} else {
		data.WriteByte(uint8(0))
	}

	// set swap data
	swapData = data.Bytes() // should be 115 bytes total at this point

	// Prepend the swap data with the length as a uint16 (2 bytes) and then the swap type (1 byte)
	lengthBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(lengthBytes, uint16(len(swapData)))
	// First prepend the length, then the swap type
	swapData = append(lengthBytes, swapData...)
	swapData = append([]byte{UniswapV3SwapType}, swapData...)

	// two transfers are done - one to the next pool and on for tokenIn
	gas = big.NewInt(10000 + int64(tokenIn.GasForTransfer) + int64(tokenOut.GasForTransfer))
	return
}

// mergeSwapData implements the MergeDataFunc type. It takes a slice of
// individual swap data byte slices and merges them into a single byte slice,
// prepended with the total number of non-empty data slices.
func mergeSwapData(allData [][]byte) []byte {
	var numberOfData uint8
	var mergedData []byte

	// First, iterate to count only the non-empty data slices.
	for _, data := range allData {
		if len(data) > 0 {
			numberOfData++
		}
	}

	// Prepend the number of data segments as the first byte.
	mergedData = append(mergedData, numberOfData)

	// Append all the individual, non-empty swap data slices.
	for _, data := range allData {
		if len(data) > 0 {
			mergedData = append(mergedData, data...)
		}
	}

	return mergedData
}

type Contract struct {
	Address common.Address
	ABI     abi.ABI
}

func getFunctionDataWithoutArgs(
	ctx context.Context,
	fnName string,
	pointer *interface{},
	contract *Contract,
	client ethclients.ETHClient,

) error {
	data, err := contract.ABI.Pack(fnName)
	if err != nil {
		return err
	}

	msg := ethereum.CallMsg{
		To:   &contract.Address,
		Data: data,
	}

	data, err = client.CallContract(ctx, msg, nil)

	if err != nil {
		return err
	}

	err = contract.ABI.UnpackIntoInterface(pointer, fnName, data)

	if err != nil {
		return err
	}

	return nil
}

func getFunctionDataWithArgs(
	ctx context.Context,
	fnName string,
	args []interface{},
	contract *Contract,
	client ethclients.ETHClient,

) ([]byte, error) {

	data, err := contract.ABI.Pack(fnName, args...)
	if err != nil {
		return []byte{}, err
	}
	msg := ethereum.CallMsg{
		To:   &contract.Address,
		Data: data,
	}

	data, err = client.CallContract(ctx, msg, nil)

	if err != nil {
		return []byte{}, err
	}

	return data, nil
}

func getERC20TokenAllowance(owner, spender, token common.Address, client ethclients.ETHClient) (*big.Int, error) {
	allowance := new(big.Int)
	funcName := "allowance"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	data, err := getFunctionDataWithArgs(
		ctx,
		funcName,
		[]interface{}{
			owner,
			spender,
		},
		&Contract{Address: token, ABI: erc20ABI},
		client,
	)

	if err != nil {
		return new(big.Int), err
	}

	err = erc20ABI.UnpackIntoInterface(&allowance, funcName, data)
	if err != nil {
		return new(big.Int), err
	}

	return allowance, nil
}

func getERC20ApproveData(
	spender common.Address,
	amount *big.Int,
) ([]byte, error) {
	if amount == nil {
		amount = new(big.Int)
	}

	data, err := erc20ABI.Pack("approve", spender, amount)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func getTransferFromAndSwapData(
	initialAmount *big.Int,
	tokenInAddress common.Address,
	tokenOutAddress common.Address,
	receiver common.Address,
	swapData []byte,
) ([]byte, error) {
	if initialAmount == nil {
		initialAmount = new(big.Int)
	}

	data, err := swapABI.Pack(
		"transferFromAndSwap",
		initialAmount,
		tokenInAddress,
		tokenOutAddress,
		receiver,
		swapData,
	)
	if err != nil {
		return nil, err
	}

	return data, nil
}
