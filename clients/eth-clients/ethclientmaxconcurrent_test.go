package ethclients

import (
	"context"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewETHClientWithMaxConcurrentCalls(t *testing.T) {
	testETHClient := NewTestETHClient()

	t.Run("valid max value", func(t *testing.T) {
		wrapper, err := NewETHClientWithMaxConcurrentCalls(testETHClient, 5)
		assert.NoError(t, err)
		assert.NotNil(t, wrapper)
		assert.NotNil(t, wrapper.semaphore)
		assert.Equal(t, 5, cap(wrapper.semaphore), "semaphore capacity should match max value")
	})

	t.Run("invalid max value", func(t *testing.T) {
		wrapper, err := NewETHClientWithMaxConcurrentCalls(testETHClient, 0)
		assert.Error(t, err)
		assert.Nil(t, wrapper)

		wrapper, err = NewETHClientWithMaxConcurrentCalls(testETHClient, -1)
		assert.Error(t, err)
		assert.Nil(t, wrapper)
	})
}

func TestETHClientWithMaxConcurrentCalls_PassThrough(t *testing.T) {
	testETHClient := NewTestETHClient()
	wrapper, err := NewETHClientWithMaxConcurrentCalls(testETHClient, 1) // max=1 is fine here
	require.NoError(t, err)

	t.Run("Close", func(t *testing.T) {
		// Reset state
		testETHClient.closeCalled = false
		assert.False(t, testETHClient.CloseCalled())

		wrapper.Close()
		assert.True(t, testETHClient.CloseCalled(), "underlying client's Close should have been called")
	})

	t.Run("SendTransaction", func(t *testing.T) {
		var handlerCalled bool
		testETHClient.SetSendTransactionHandler(func(ctx context.Context, tx *types.Transaction) error {
			handlerCalled = true
			return nil
		})

		_ = wrapper.SendTransaction(context.Background(), nil)
		assert.True(t, handlerCalled, "underlying client's SendTransaction should have been called")
	})
}

func TestETHClientWithMaxConcurrentCalls_Semaphore(t *testing.T) {
	t.Run("successful call acquires and releases semaphore", func(t *testing.T) {
		testETHClient := NewTestETHClient()
		wrapper, err := NewETHClientWithMaxConcurrentCalls(testETHClient, 1)
		require.NoError(t, err)

		var handlerCalled bool
		expectedResult := []byte("success")
		testETHClient.SetCallContractHandler(func(ctx context.Context, msg ethereum.CallMsg, blockNumber *big.Int) ([]byte, error) {
			handlerCalled = true
			return expectedResult, nil
		})

		res, err := wrapper.CallContract(context.Background(), ethereum.CallMsg{}, nil)

		assert.NoError(t, err)
		assert.Equal(t, expectedResult, res)
		assert.True(t, handlerCalled)
		// Semaphore should be empty after the call
		assert.Len(t, wrapper.semaphore, 0, "semaphore should be released after call")
	})

	t.Run("concurrency is limited by semaphore", func(t *testing.T) {
		maxConcurrent := 2
		testETHClient := NewTestETHClient()
		wrapper, err := NewETHClientWithMaxConcurrentCalls(testETHClient, maxConcurrent)
		require.NoError(t, err)

		// This handler will block until we send a value to the unblock channel
		unblock := make(chan struct{})
		testETHClient.SetCallContractHandler(func(ctx context.Context, msg ethereum.CallMsg, blockNumber *big.Int) ([]byte, error) {
			<-unblock // Wait for the signal
			return []byte("done"), nil
		})

		var wg sync.WaitGroup
		// Start 'maxConcurrent' calls that will block, filling the semaphore
		for i := 0; i < maxConcurrent; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, _ = wrapper.CallContract(context.Background(), ethereum.CallMsg{}, nil)
			}()
		}

		// Give the goroutines a moment to acquire the semaphore
		time.Sleep(50 * time.Millisecond)
		assert.Len(t, wrapper.semaphore, maxConcurrent, "semaphore should be full")

		// This call should block because the semaphore is full
		callBlocked := true
		callReturned := make(chan struct{})
		go func() {
			_, _ = wrapper.CallContract(context.Background(), ethereum.CallMsg{}, nil)
			callBlocked = false
			close(callReturned)
		}()

		// Check that the call is still blocked after a short time
		time.Sleep(50 * time.Millisecond)
		assert.True(t, callBlocked, "third call should be blocked by the full semaphore")

		// Unblock one of the first calls
		close(unblock)

		// Wait for the third call to return
		<-callReturned
		assert.False(t, callBlocked, "third call should have unblocked and returned")

		// Wait for the first two calls to finish
		wg.Wait()
		assert.Len(t, wrapper.semaphore, 0, "semaphore should be empty after all calls complete")
	})

	t.Run("blocked call is canceled by context", func(t *testing.T) {
		testETHClient := NewTestETHClient()
		wrapper, err := NewETHClientWithMaxConcurrentCalls(testETHClient, 1)
		require.NoError(t, err)

		// This handler blocks forever unless canceled
		handlerCalled := make(chan struct{}, 1)
		testETHClient.SetCallContractHandler(func(ctx context.Context, msg ethereum.CallMsg, blockNumber *big.Int) ([]byte, error) {
			handlerCalled <- struct{}{}
			<-ctx.Done() // Block until context is canceled
			return nil, ctx.Err()
		})

		// Fill the single semaphore slot
		go wrapper.CallContract(context.Background(), ethereum.CallMsg{}, nil)
		<-handlerCalled // Wait for the handler to be called, confirming semaphore is acquired

		// This call will block, waiting for the semaphore
		ctx, cancel := context.WithCancel(context.Background())
		var callErr error
		callReturned := make(chan struct{})
		go func() {
			_, callErr = wrapper.CallContract(ctx, ethereum.CallMsg{}, nil)
			close(callReturned)
		}()

		// Give it a moment to block
		time.Sleep(50 * time.Millisecond)

		// Now cancel the context
		cancel()

		// The call should immediately return with a context error
		<-callReturned
		assert.Error(t, callErr)
		assert.ErrorIs(t, callErr, context.Canceled)
	})

	t.Run("semaphore is released on panic", func(t *testing.T) {
		testETHClient := NewTestETHClient()
		wrapper, err := NewETHClientWithMaxConcurrentCalls(testETHClient, 1)
		require.NoError(t, err)

		testETHClient.SetCallContractHandler(func(ctx context.Context, msg ethereum.CallMsg, blockNumber *big.Int) ([]byte, error) {
			panic("uh oh")
		})

		// We expect a panic, so we need to recover from it
		assert.Panics(t, func() {
			_, _ = wrapper.CallContract(context.Background(), ethereum.CallMsg{}, nil)
		})

		// The most important part: the semaphore should be empty
		assert.Len(t, wrapper.semaphore, 0, "semaphore should be released even if underlying call panics")
	})
}
