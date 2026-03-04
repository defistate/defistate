package fork

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"sync"

	fork "github.com/defistate/defistate/fork/anvil-forker"
	"github.com/defistate/defistate/token-analyzer/erc20analyzer"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rpc"
)

// ERC20FeeAndGasRequester implements the FeeAndGasRequester interface using
// a direct RPC connection. It manages concurrent requests using a semaphore.
type ERC20FeeAndGasRequester struct {
	rpc       *rpc.Client
	semaphore chan struct{}
}

// Statically verify that *ERC20FeeAndGasRequester implements the interface.
var _ erc20analyzer.FeeAndGasRequester = (*ERC20FeeAndGasRequester)(nil)

// NewERC20FeeAndGasRequester creates a new requester with a connection to the
// given RPC URL and a specific limit on concurrent requests.
func NewERC20FeeAndGasRequester(
	url string,
	maxConcurrentRequests int,
) (*ERC20FeeAndGasRequester, error) {
	rpc, err := rpc.Dial(url)
	if err != nil {
		return nil, err
	}

	if maxConcurrentRequests < 1 {
		maxConcurrentRequests = 1
	}

	requester := &ERC20FeeAndGasRequester{
		rpc:       rpc,
		semaphore: make(chan struct{}, maxConcurrentRequests),
	}

	return requester, nil
}

// generateRandomAddress creates a new, cryptographically secure random address.
func generateRandomAddress() (common.Address, error) {
	addressBytes := make([]byte, 20)
	if _, err := rand.Read(addressBytes); err != nil {
		return common.Address{}, fmt.Errorf("failed to generate random receiver address: %w", err)
	}
	return common.BytesToAddress(addressBytes), nil
}

// requestOne executes the logic for fetching the fee and gas for a single
// token-holder pair by calling a custom fork RPC method.
func (requester *ERC20FeeAndGasRequester) requestOne(ctx context.Context, token, holder common.Address) (erc20analyzer.FeeAndGasResult, error) {
	// For a realistic simulation, we generate a new random receiver address
	// for each transfer. This avoids special handling of the zero address.
	receiver, err := generateRandomAddress()
	if err != nil {
		return erc20analyzer.FeeAndGasResult{}, err
	}

	// Call the custom fork RPC method to simulate the transfer.
	var resp fork.SimulateTokenTransferResponse
	err = requester.rpc.CallContext(ctx, &resp, "fork_simulateTokenTransfer", fork.SimulateTokenTransferRequest{
		Token:    token,
		Holder:   holder,
		Receiver: receiver,
	})

	if err != nil {
		return erc20analyzer.FeeAndGasResult{}, err
	}

	// check if there is a response level error
	if resp.Error != "" {
		return erc20analyzer.FeeAndGasResult{}, errors.New(resp.Error)
	}

	return erc20analyzer.FeeAndGasResult{
		Fee: float64(resp.FeeOnTransferPercentage),
		Gas: uint64(resp.Gas),
	}, nil
}

// RequestAll orchestrates fetching fee and gas data for all provided token-holder
// pairs concurrently. It allows for partial success, returning results for all
// tokens, with failed requests indicated by a non-nil Error field in the result.
func (requester *ERC20FeeAndGasRequester) RequestAll(
	ctx context.Context,
	tokensByHolder map[common.Address]common.Address,
) (map[common.Address]erc20analyzer.FeeAndGasResult, error) {

	// Pre-flight check for catastrophic failure.
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		results = make(map[common.Address]erc20analyzer.FeeAndGasResult, len(tokensByHolder))
	)

	for token, holder := range tokensByHolder {
		// Stop queueing new jobs if context is cancelled.
		if ctx.Err() != nil {
			break
		}

		wg.Add(1)
		// Acquire a semaphore slot HERE, before spawning the goroutine.
		// This throttles the creation of goroutines to the configured limit.
		requester.semaphore <- struct{}{}

		go func(token, holder common.Address) {
			defer wg.Done()
			// Release the semaphore slot when the goroutine finishes.
			defer func() { <-requester.semaphore }()

			// Still check context error for a fast exit path.
			if ctx.Err() != nil {
				mu.Lock()
				results[token] = erc20analyzer.FeeAndGasResult{Error: ctx.Err()}
				mu.Unlock()
				return
			}

			// Make the actual request for a single item.
			result, err := requester.requestOne(ctx, token, holder)

			mu.Lock()
			if err != nil {
				// Store per-item errors in the result struct.
				results[token] = erc20analyzer.FeeAndGasResult{Error: err}
			} else {
				results[token] = result
			}
			mu.Unlock()

		}(token, holder)
	}

	wg.Wait()

	// After waiting, ensure any tasks that were never started because the
	// context was cancelled are also marked with an error.
	if ctx.Err() != nil {
		for token := range tokensByHolder {
			if _, exists := results[token]; !exists {
				results[token] = erc20analyzer.FeeAndGasResult{Error: ctx.Err()}
			}
		}
	}

	return results, nil
}
