package abi

import (
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// ERC20ABI is the parsed ABI for the standard ERC-20 token interface.
var ERC20ABI abi.ABI

func init() {
	const erc20ABIJSON = `
	[
		{
			"anonymous": false,
			"inputs": [
				{ "indexed": true, "internalType": "address", "name": "owner", "type": "address" },
				{ "indexed": true, "internalType": "address", "name": "spender", "type": "address" },
				{ "indexed": false, "internalType": "uint256", "name": "value", "type": "uint256" }
			],
			"name": "Approval",
			"type": "event"
		},
		{
			"anonymous": false,
			"inputs": [
				{ "indexed": true, "internalType": "address", "name": "from", "type": "address" },
				{ "indexed": true, "internalType": "address", "name": "to", "type": "address" },
				{ "indexed": false, "internalType": "uint256", "name": "value", "type": "uint256" }
			],
			"name": "Transfer",
			"type": "event"
		},
		{
			"inputs": [
				{ "internalType": "address", "name": "owner", "type": "address" },
				{ "internalType": "address", "name": "spender", "type": "address" }
			],
			"name": "allowance",
			"outputs": [
				{ "internalType": "uint256", "name": "", "type": "uint256" }
			],
			"stateMutability": "view",
			"type": "function"
		},
		{
			"inputs": [
				{ "internalType": "address", "name": "spender", "type": "address" },
				{ "internalType": "uint256", "name": "value", "type": "uint256" }
			],
			"name": "approve",
			"outputs": [
				{ "internalType": "bool", "name": "", "type": "bool" }
			],
			"stateMutability": "nonpayable",
			"type": "function"
		},
		{
			"inputs": [
				{ "internalType": "address", "name": "owner", "type": "address" }
			],
			"name": "balanceOf",
			"outputs": [
				{ "internalType": "uint256", "name": "", "type": "uint256" }
			],
			"stateMutability": "view",
			"type": "function"
		},
		{
			"inputs": [],
			"name": "decimals",
			"outputs": [
				{ "internalType": "uint8", "name": "", "type": "uint8" }
			],
			"stateMutability": "view",
			"type": "function"
		},
		{
			"inputs": [],
			"name": "name",
			"outputs": [
				{ "internalType": "string", "name": "", "type": "string" }
			],
			"stateMutability": "view",
			"type": "function"
		},
		{
			"inputs": [],
			"name": "symbol",
			"outputs": [
				{ "internalType": "string", "name": "", "type": "string" }
			],
			"stateMutability": "view",
			"type": "function"
		},
		{
			"inputs": [],
			"name": "totalSupply",
			"outputs": [
				{ "internalType": "uint256", "name": "", "type": "uint256" }
			],
			"stateMutability": "view",
			"type": "function"
		},
		{
			"inputs": [
				{ "internalType": "address", "name": "to", "type": "address" },
				{ "internalType": "uint256", "name": "value", "type": "uint256" }
			],
			"name": "transfer",
			"outputs": [
				{ "internalType": "bool", "name": "", "type": "bool" }
			],
			"stateMutability": "nonpayable",
			"type": "function"
		},
		{
			"inputs": [
				{ "internalType": "address", "name": "from", "type": "address" },
				{ "internalType": "address", "name": "to", "type": "address" },
				{ "internalType": "uint256", "name": "value", "type": "uint256" }
			],
			"name": "transferFrom",
			"outputs": [
				{ "internalType": "bool", "name": "", "type": "bool" }
			],
			"stateMutability": "nonpayable",
			"type": "function"
		}
	]`

	var err error
	ERC20ABI, err = abi.JSON(strings.NewReader(erc20ABIJSON))
	if err != nil {
		panic("failed to parse ERC20 ABI: " + err.Error())
	}
}

// ParseERC20TransferEvent checks if a logger is an ERC20 Transfer event and, if so,
// parses the sender, recipient, and value from it.
//
// It returns an error if the logger is not a valid Transfer event.
func ParseERC20TransferEvent(logger types.Log) (sender, recipient common.Address, value *big.Int, err error) {
	// First, check if the logger's topic signature matches the ERC20 Transfer event signature.
	// An ERC20 Transfer event will have exactly 3 topics:
	// Topics[0]: Event Signature (the hash of "Transfer(address,address,uint256)")
	// Topics[1]: The indexed `from` address
	// Topics[2]: The indexed `to` address
	if len(logger.Topics) != 3 || logger.Topics[0] != ERC20ABI.Events["Transfer"].ID {
		return common.Address{}, common.Address{}, nil, errors.New("logger is not a valid ERC20 Transfer event")
	}

	// Indexed 'address' parameters are stored as 32-byte values in the topics.
	// We can convert them directly to a common.Address.
	sender = common.BytesToAddress(logger.Topics[1].Bytes())
	recipient = common.BytesToAddress(logger.Topics[2].Bytes())

	// The non-indexed 'value' is stored in the logger's Data field.
	// We need to use the ABI to unpack this data.
	// The event name to look for in the ABI is "Transfer".
	var transferEvent struct {
		Value *big.Int
	}
	err = ERC20ABI.UnpackIntoInterface(&transferEvent, "Transfer", logger.Data)
	if err != nil {
		return common.Address{}, common.Address{}, nil, fmt.Errorf("failed to unpack logger data: %w", err)
	}

	value = transferEvent.Value

	return sender, recipient, value, nil
}
