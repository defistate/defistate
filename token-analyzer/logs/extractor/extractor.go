// Package logextractor provides a reusable component for extracting logs from
// an Ethereum-like blockchain.
//
// It is designed to be a generic data pipeline that listens for new block events,
// efficiently filters them, and delegates the handling of retrieved logs to a
// user-provided callback. This decouples the low-level concern of data fetching
// from the higher-level business logic that consumes the data.
package logextractor

import (
	"context"
	"errors"

	"github.com/defistate/defistate/token-analyzer/erc20analyzer"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/core/types"
)

var _ erc20analyzer.BlockExtractor = (*LiveExtractor)(nil)

// LiveExtractor implements the BlockExtractor interface using a live connection
// to an Ethereum node. It holds the unexported dependencies needed to perform
// the extraction.
type LiveExtractor struct {
	// testBloom is a function that performs a fast, preliminary check on a
	// block's bloom filter to see if it might contain desired logs, avoiding
	// a slow network call for blocks that definitely do not.
	testBloom func(types.Bloom) bool

	// getFilterer returns a function capable of fetching logs from the blockchain.
	// This approach allows for flexible connection management, such as using a
	// connection pool or establishing a fresh connection on-demand to enhance
	// resilience.
	getFilterer func() (func(context.Context, ethereum.FilterQuery) ([]types.Log, error), error)
}

// NewLiveExtractor is the constructor for a LiveExtractor.
// It validates and bundles the dependencies required for the extractor to function.
//
// Parameters:
//   - testBloom: A function for efficiently checking a block's bloom filter.
//   - getFilterer: A function that provides a logger filtering function, used to
//     abstract away the underlying Ethereum client connection.
func NewLiveExtractor(
	testBloom func(types.Bloom) bool,
	getFilterer func() (func(context.Context, ethereum.FilterQuery) ([]types.Log, error), error),
) (*LiveExtractor, error) {
	if testBloom == nil || getFilterer == nil {
		return nil, errors.New("a bloom tester and logger filterer are required")
	}
	return &LiveExtractor{
		testBloom:   testBloom,
		getFilterer: getFilterer,
	}, nil
}

// Run starts and manages the main event loop for the LiveExtractor.
// It listens for new blocks, filters them, fetches logs, and passes them to the
// appropriate handler.
func (le *LiveExtractor) Run(
	ctx context.Context,
	eventer <-chan *types.Block,
	logsHandler func(context.Context, []types.Log) error,
	errHandler func(error),
) {
	// Start the main event loop, which will run until the context is cancelled.
	for {
		select {
		// A new block has been received from the eventer channel.
		case block := <-eventer:
			if block == nil {
				errHandler(errors.New("received nil block from eventer channel"))
				continue
			}

			// First, perform a fast, local check on the bloom filter. If it doesn't
			// match, we can skip this block entirely and avoid a slow network call.
			if !le.testBloom(block.Bloom()) {
				continue
			}

			// If the bloom filter matches, prepare a precise query to fetch logs
			// for this specific block.
			blockHash := block.Hash()
			query := ethereum.FilterQuery{
				BlockHash: &blockHash,
			}

			// Obtain a logger filtering function. This might involve fetching a client
			// from a connection pool, ensuring our connection is fresh.
			filterer, err := le.getFilterer()
			if err != nil {
				errHandler(err)
				continue
			}

			// Execute the RPC call to fetch the logs.
			logs, err := filterer(ctx, query)
			if err != nil {
				errHandler(err)
				continue
			}

			// Delegate the handling of the retrieved logs to the provided callback.
			// The extractor's job is done; it makes no assumptions about what the
			// logs mean or whether an empty slice is significant.
			if err := logsHandler(ctx, logs); err != nil {
				errHandler(err)
			}

		// The context has been cancelled, signaling a graceful shutdown.
		case <-ctx.Done():
			// Exit the loop and allow the goroutine to terminate.
			return
		}
	}
}
