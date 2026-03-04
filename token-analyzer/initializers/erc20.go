// Package initializers provides concrete implementations for the various
// dependency interfaces defined in the erc20analyzer package.
package initializers

import (
	"context"
	"errors"
	"fmt"
	"sync"

	ethclients "github.com/defistate/defistate/clients/eth-clients"
	token "github.com/defistate/defistate/protocols/erc20-token-system"
	"github.com/defistate/defistate/token-analyzer/abi"
	"github.com/defistate/defistate/token-analyzer/erc20analyzer"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
)

// ERC20Initializer is a concrete implementation of the TokenInitializer interface.
// It uses a live Ethereum client connection to fetch token metadata.
type ERC20Initializer struct {
	getClient func() (ethclients.ETHClient, error)
}

// Statically verify that *ERC20Initializer implements the interface.
// This will now fail until the TokenInitializer interface is updated.
var _ erc20analyzer.TokenInitializer = (*ERC20Initializer)(nil)

// NewERC20Initializer creates a new initializer that uses the provided getClient function.
func NewERC20Initializer(getClient func() (ethclients.ETHClient, error)) (*ERC20Initializer, error) {
	if getClient == nil {
		return nil, errors.New("getClient function cannot be nil")
	}
	return &ERC20Initializer{
		getClient: getClient,
	}, nil
}

// Initialize fetches a token's name, symbol, and decimals from the blockchain,
// adds the token to the provided store, and returns the resulting TokenView.
// It respects the provided context for timeouts and cancellation.
func (i *ERC20Initializer) Initialize(ctx context.Context, tokenAddress common.Address, store erc20analyzer.TokenStore) (token.TokenView, error) {
	client, err := i.getClient()
	if err != nil {
		return token.TokenView{}, err
	}
	if client == nil {
		return token.TokenView{}, errors.New("getClient returned a nil client")
	}

	var name, symbol string
	var decimals uint8
	var errName, errSymbol, errDecimals error
	var wg sync.WaitGroup
	wg.Add(3)

	go func() {
		defer wg.Done()
		name, errName = i.fetchName(ctx, client, tokenAddress)
	}()

	go func() {
		defer wg.Done()
		symbol, errSymbol = i.fetchSymbol(ctx, client, tokenAddress)
	}()

	go func() {
		defer wg.Done()
		decimals, errDecimals = i.fetchDecimals(ctx, client, tokenAddress)
	}()

	wg.Wait()

	if errName != nil {
		return token.TokenView{}, fmt.Errorf("failed to fetch name for token %s: %w", tokenAddress.Hex(), errName)
	}
	if errSymbol != nil {
		return token.TokenView{}, fmt.Errorf("failed to fetch symbol for token %s: %w", tokenAddress.Hex(), errSymbol)
	}
	if errDecimals != nil {
		return token.TokenView{}, fmt.Errorf("failed to fetch decimals for token %s: %w", tokenAddress.Hex(), errDecimals)
	}

	// Add the new token to the store.
	id, err := store.AddToken(tokenAddress, name, symbol, decimals)
	if err != nil {
		return token.TokenView{}, fmt.Errorf("failed to add token %s to store: %w", tokenAddress.Hex(), err)
	}

	// Construct and return the TokenView.
	return token.TokenView{
		ID:       id,
		Address:  tokenAddress,
		Name:     name,
		Symbol:   symbol,
		Decimals: decimals,
	}, nil
}

// fetchName calls the 'name' method on the ERC20 contract.
func (i *ERC20Initializer) fetchName(ctx context.Context, client ethclients.ETHClient, addr common.Address) (string, error) {
	data, err := abi.ERC20ABI.Pack("name")
	if err != nil {
		return "", fmt.Errorf("failed to pack data for name: %w", err)
	}

	msg := ethereum.CallMsg{To: &addr, Data: data}
	resultBytes, err := client.CallContract(ctx, msg, nil)
	if err != nil {
		return "", err
	}

	var out string
	if err := abi.ERC20ABI.UnpackIntoInterface(&out, "name", resultBytes); err != nil {
		return "", fmt.Errorf("failed to unpack name: %w", err)
	}
	return out, nil
}

// fetchSymbol calls the 'symbol' method on the ERC20 contract.
func (i *ERC20Initializer) fetchSymbol(ctx context.Context, client ethclients.ETHClient, addr common.Address) (string, error) {
	data, err := abi.ERC20ABI.Pack("symbol")
	if err != nil {
		return "", fmt.Errorf("failed to pack data for symbol: %w", err)
	}

	msg := ethereum.CallMsg{To: &addr, Data: data}
	resultBytes, err := client.CallContract(ctx, msg, nil)
	if err != nil {
		return "", err
	}

	var out string
	if err := abi.ERC20ABI.UnpackIntoInterface(&out, "symbol", resultBytes); err != nil {
		return "", fmt.Errorf("failed to unpack symbol: %w", err)
	}
	return out, nil
}

// fetchDecimals calls the 'decimals' method on the ERC20 contract.
func (i *ERC20Initializer) fetchDecimals(ctx context.Context, client ethclients.ETHClient, addr common.Address) (uint8, error) {
	data, err := abi.ERC20ABI.Pack("decimals")
	if err != nil {
		return 0, fmt.Errorf("failed to pack data for decimals: %w", err)
	}

	msg := ethereum.CallMsg{To: &addr, Data: data}
	resultBytes, err := client.CallContract(ctx, msg, nil)
	if err != nil {
		return 0, err
	}

	var out uint8
	if err := abi.ERC20ABI.UnpackIntoInterface(&out, "decimals", resultBytes); err != nil {
		return 0, fmt.Errorf("failed to unpack decimals: %w", err)
	}
	return out, nil
}
