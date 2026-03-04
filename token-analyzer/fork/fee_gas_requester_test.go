package fork

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	fork "github.com/defistate/defistate/fork/anvil-forker"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockRPCService implements the backend logic for our test RPC server.
type MockRPCService struct {
	// handler is a function that defines how the mock service responds to requests.
	// The rpc server will inject the context as the first argument.
	handler func(ctx context.Context, req *fork.SimulateTokenTransferRequest) (*fork.SimulateTokenTransferResponse, error)
}

// SimulateTokenTransfer is the method that the RPC server will expose. It delegates
// its logic to the handler function. The context is automatically passed by the server.
func (m *MockRPCService) SimulateTokenTransfer(ctx context.Context, req *fork.SimulateTokenTransferRequest) (*fork.SimulateTokenTransferResponse, error) {
	if m.handler == nil {
		return nil, errors.New("mock handler not implemented")
	}
	return m.handler(ctx, req)
}

// setupTestServer creates and starts a real HTTP server with a mock RPC service.
func setupTestServer(t *testing.T, handler func(context.Context, *fork.SimulateTokenTransferRequest) (*fork.SimulateTokenTransferResponse, error)) string {
	t.Helper()
	server := rpc.NewServer()
	err := server.RegisterName("fork", &MockRPCService{handler: handler})
	require.NoError(t, err, "Failed to register mock service")

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err, "Failed to find free port")

	httpServer := &http.Server{Handler: server}
	go func() {
		if err := httpServer.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Printf("HTTP server error: %v\n", err)
		}
	}()

	t.Cleanup(func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
	})

	return "http://" + listener.Addr().String()
}

func TestERC20FeeAndGasRequester_RequestAll(t *testing.T) {
	// --- Common Arrange for all sub-tests ---
	const maxConcurrency = 4
	const numTokens = 20 // Using more tokens to better test concurrency
	tokensByHolder := make(map[common.Address]common.Address)
	expectedResults := make(map[common.Address]fork.SimulateTokenTransferResponse)
	errorToken := common.BigToAddress(big.NewInt(int64(numTokens / 2)))
	expectedErrorMsg := "simulated RPC error for token"

	for i := 0; i < numTokens; i++ {
		tokenAddr := common.BigToAddress(big.NewInt(int64(i)))
		holderAddr := common.BigToAddress(big.NewInt(int64(i + numTokens)))
		tokensByHolder[tokenAddr] = holderAddr
		expectedResults[tokenAddr] = fork.SimulateTokenTransferResponse{
			Gas:                     uint(5000 + i),
			FeeOnTransferPercentage: uint(i % 3),
		}
	}

	t.Run("happy path with partial failure and concurrency check", func(t *testing.T) {
		var activeRequests int32
		var maxActiveRequests int32

		handler := func(ctx context.Context, req *fork.SimulateTokenTransferRequest) (*fork.SimulateTokenTransferResponse, error) {
			currentActive := atomic.AddInt32(&activeRequests, 1)
			if currentActive > atomic.LoadInt32(&maxActiveRequests) {
				atomic.StoreInt32(&maxActiveRequests, currentActive)
			}
			defer atomic.AddInt32(&activeRequests, -1)

			if req.Token == errorToken {
				return nil, errors.New(expectedErrorMsg)
			}
			time.Sleep(10 * time.Millisecond) // Simulate network latency
			resp, ok := expectedResults[req.Token]
			if !ok {
				return nil, fmt.Errorf("unexpected token: %s", req.Token)
			}
			return &resp, nil
		}

		serverURL := setupTestServer(t, handler)
		requester, err := NewERC20FeeAndGasRequester(serverURL, maxConcurrency)
		require.NoError(t, err)

		results, err := requester.RequestAll(context.Background(), tokensByHolder)

		require.NoError(t, err, "RequestAll should not return a top-level error for partial failures")
		require.Len(t, results, numTokens, "Should have a result for every requested token")
		assert.LessOrEqual(t, atomic.LoadInt32(&maxActiveRequests), int32(maxConcurrency), "Concurrency limit should be respected")

		for tokenAddr := range tokensByHolder {
			result, ok := results[tokenAddr]
			require.True(t, ok, "Result missing for token %s", tokenAddr)

			if tokenAddr == errorToken {
				require.Error(t, result.Error, "Expected an error for the failing token")
				assert.Contains(t, result.Error.Error(), expectedErrorMsg)
			} else {
				require.NoError(t, result.Error)
				expected := expectedResults[tokenAddr]
				assert.Equal(t, float64(expected.FeeOnTransferPercentage), result.Fee)
				assert.Equal(t, uint64(expected.Gas), result.Gas)
			}
		}
	})

	t.Run("with pre-flight context cancellation", func(t *testing.T) {
		serverURL := setupTestServer(t, nil) // Handler not needed
		requester, err := NewERC20FeeAndGasRequester(serverURL, maxConcurrency)
		require.NoError(t, err)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err = requester.RequestAll(ctx, tokensByHolder)
		require.Error(t, err, "RequestAll should return an error if context is cancelled pre-flight")
		assert.Equal(t, context.Canceled, err)
	})

	t.Run("with mid-flight context cancellation", func(t *testing.T) {
		handler := func(ctx context.Context, req *fork.SimulateTokenTransferRequest) (*fork.SimulateTokenTransferResponse, error) {
			select {
			case <-time.After(5 * time.Second):
				return nil, errors.New("test timeout waiting for cancellation")
			case <-ctx.Done(): // Correctly check the context passed into the handler.
				return nil, ctx.Err()
			}
		}

		serverURL := setupTestServer(t, handler)
		requester, err := NewERC20FeeAndGasRequester(serverURL, maxConcurrency)
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		results, err := requester.RequestAll(ctx, tokensByHolder)

		require.NoError(t, err, "A mid-flight cancellation should not produce a top-level error")
		for _, result := range results {
			require.Error(t, result.Error)
			assert.ErrorIs(t, result.Error, context.DeadlineExceeded)
		}
	})

	t.Run("with empty input", func(t *testing.T) {
		serverURL := setupTestServer(t, nil) // Handler not needed
		requester, err := NewERC20FeeAndGasRequester(serverURL, maxConcurrency)
		require.NoError(t, err)

		results, err := requester.RequestAll(context.Background(), make(map[common.Address]common.Address))
		require.NoError(t, err)
		assert.Empty(t, results)
	})
}
