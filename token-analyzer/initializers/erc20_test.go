package initializers

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"testing"
	"time"

	ethclients "github.com/defistate/defistate/clients/eth-clients"
	token "github.com/defistate/defistate/protocols/erc20-token-system"
	"github.com/defistate/defistate/token-analyzer/abi"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockTokenStore provides a test implementation of the TokenStore interface.
type mockTokenStore struct {
	mu         sync.RWMutex
	addTokenCh chan struct {
		Addr         common.Address
		Name, Symbol string
		Decimals     uint8
	}
	errToReturn error
	idToReturn  uint64
}

func newMockTokenStore() *mockTokenStore {
	return &mockTokenStore{
		addTokenCh: make(chan struct {
			Addr         common.Address
			Name, Symbol string
			Decimals     uint8
		}, 1),
	}
}
func (m *mockTokenStore) AddToken(addr common.Address, name, symbol string, decimals uint8) (uint64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.errToReturn != nil {
		return 0, m.errToReturn
	}
	m.addTokenCh <- struct {
		Addr         common.Address
		Name, Symbol string
		Decimals     uint8
	}{addr, name, symbol, decimals}
	return m.idToReturn, nil
}
func (m *mockTokenStore) DeleteToken(idToDelete uint64) error                  { return nil }
func (m *mockTokenStore) UpdateToken(id uint64, fee float64, gas uint64) error { return nil }
func (m *mockTokenStore) View() []token.TokenView                              { return nil }
func (m *mockTokenStore) GetTokenByID(id uint64) (token.TokenView, error) {
	return token.TokenView{}, nil
}
func (m *mockTokenStore) GetTokenByAddress(addr common.Address) (token.TokenView, error) {
	return token.TokenView{}, nil
}

func TestERC20Initializer_Initialize(t *testing.T) {
	// --- Common Arrange for all sub-tests ---
	tokenAddr := common.HexToAddress("0x1")
	expectedName := "Test Token"
	expectedSymbol := "TEST"
	expectedDecimals := uint8(18)
	expectedID := uint64(1)

	// Pre-calculate the method signature hashes to identify contract calls.
	nameData, _ := abi.ERC20ABI.Pack("name")
	symbolData, _ := abi.ERC20ABI.Pack("symbol")
	decimalsData, _ := abi.ERC20ABI.Pack("decimals")

	t.Run("happy path - successful initialization", func(t *testing.T) {
		// Arrange
		testETHClient := ethclients.NewTestETHClient()
		store := newMockTokenStore()
		store.idToReturn = expectedID

		// Set the handler to return the correct data for each method call.
		testETHClient.SetCallContractHandler(func(ctx context.Context, msg ethereum.CallMsg, blockNumber *big.Int) ([]byte, error) {
			switch {
			case string(msg.Data) == string(nameData):
				return abi.ERC20ABI.Methods["name"].Outputs.Pack(expectedName)
			case string(msg.Data) == string(symbolData):
				return abi.ERC20ABI.Methods["symbol"].Outputs.Pack(expectedSymbol)
			case string(msg.Data) == string(decimalsData):
				return abi.ERC20ABI.Methods["decimals"].Outputs.Pack(expectedDecimals)
			}
			return nil, fmt.Errorf("unexpected contract call: %x", msg.Data)
		})

		getClient := func() (ethclients.ETHClient, error) { return testETHClient, nil }
		initializer, err := NewERC20Initializer(getClient)
		require.NoError(t, err)

		// Act
		resultView, err := initializer.Initialize(context.Background(), tokenAddr, store)

		// Assert
		require.NoError(t, err)
		assert.Equal(t, expectedID, resultView.ID)
		assert.Equal(t, tokenAddr, resultView.Address)
		assert.Equal(t, expectedName, resultView.Name)
		assert.Equal(t, expectedSymbol, resultView.Symbol)
		assert.Equal(t, expectedDecimals, resultView.Decimals)

		// Assert that AddToken was called on the store with the correct data.
		select {
		case added := <-store.addTokenCh:
			assert.Equal(t, tokenAddr, added.Addr)
			assert.Equal(t, expectedName, added.Name)
			assert.Equal(t, expectedSymbol, added.Symbol)
			assert.Equal(t, expectedDecimals, added.Decimals)
		case <-time.After(50 * time.Millisecond):
			t.Fatal("timed out waiting for store.AddToken to be called")
		}
	})

	t.Run("failure path - name call fails", func(t *testing.T) {
		// Arrange
		testETHClient := ethclients.NewTestETHClient()
		store := newMockTokenStore()
		expectedErr := errors.New("rpc error: name call failed")

		testETHClient.SetCallContractHandler(func(ctx context.Context, msg ethereum.CallMsg, blockNumber *big.Int) ([]byte, error) {
			if string(msg.Data) == string(nameData) {
				return nil, expectedErr
			}
			return nil, nil // Return success for other calls
		})

		getClient := func() (ethclients.ETHClient, error) { return testETHClient, nil }
		initializer, _ := NewERC20Initializer(getClient)

		// Act
		_, err := initializer.Initialize(context.Background(), tokenAddr, store)

		// Assert
		require.Error(t, err)
		assert.ErrorIs(t, err, expectedErr)
	})

	t.Run("failure path - AddToken fails", func(t *testing.T) {
		// Arrange
		testETHClient := ethclients.NewTestETHClient()
		store := newMockTokenStore()
		expectedErr := errors.New("db error: failed to add token")
		store.errToReturn = expectedErr

		testETHClient.SetCallContractHandler(func(ctx context.Context, msg ethereum.CallMsg, blockNumber *big.Int) ([]byte, error) {
			switch {
			case string(msg.Data) == string(nameData):
				return abi.ERC20ABI.Methods["name"].Outputs.Pack(expectedName)
			case string(msg.Data) == string(symbolData):
				return abi.ERC20ABI.Methods["symbol"].Outputs.Pack(expectedSymbol)
			case string(msg.Data) == string(decimalsData):
				return abi.ERC20ABI.Methods["decimals"].Outputs.Pack(expectedDecimals)
			}
			return nil, fmt.Errorf("unexpected contract call: %x", msg.Data)
		})

		getClient := func() (ethclients.ETHClient, error) { return testETHClient, nil }
		initializer, _ := NewERC20Initializer(getClient)

		// Act
		_, err := initializer.Initialize(context.Background(), tokenAddr, store)

		// Assert
		require.Error(t, err)
		assert.ErrorIs(t, err, expectedErr)
	})

	t.Run("context cancelled during call", func(t *testing.T) {
		// Arrange
		testETHClient := ethclients.NewTestETHClient()
		store := newMockTokenStore()

		ctx, cancel := context.WithCancel(context.Background())

		testETHClient.SetCallContractHandler(func(ctx context.Context, msg ethereum.CallMsg, blockNumber *big.Int) ([]byte, error) {
			// Simulate a long-running call that gets cancelled.
			cancel() // Cancel the context from inside the mock call
			<-ctx.Done()
			return nil, ctx.Err()
		})

		getClient := func() (ethclients.ETHClient, error) { return testETHClient, nil }
		initializer, _ := NewERC20Initializer(getClient)

		// Act
		_, err := initializer.Initialize(ctx, tokenAddr, store)

		// Assert
		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
	})
}
