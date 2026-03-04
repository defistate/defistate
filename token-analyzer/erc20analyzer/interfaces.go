package erc20analyzer

import (
	"context"

	token "github.com/defistate/defistate/protocols/erc20-token-system"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// FeeAndGasResult holds the outcome of a fee/gas request for a single token.
// It includes an Error field to allow for partial success in batch operations.
type FeeAndGasResult struct {
	Fee   float64
	Gas   uint64
	Error error // Holds an error if this specific request failed.
}

// FeeAndGasRequester defines the contract for a component that can fetch
// fee and gas data for a batch of tokens.
type FeeAndGasRequester interface {
	// RequestAll takes a map of tokens to their respective holders and returns
	// a map of tokens to their fee/gas results. It should handle requests
	// concurrently for efficiency.
	RequestAll(
		ctx context.Context,
		tokensByHolder map[common.Address]common.Address,
	) (map[common.Address]FeeAndGasResult, error)
}

// TokenStore defines the interface for a system that stores and manages
// ERC20 token data. This allows the analyzer to remain decoupled from the
// specific database or storage implementation.
type TokenStore interface {
	AddToken(addr common.Address, name, symbol string, decimals uint8) (uint64, error)
	DeleteToken(idToDelete uint64) error
	UpdateToken(id uint64, fee float64, gas uint64) error
	View() []token.TokenView
	GetTokenByID(id uint64) (token.TokenView, error)
	GetTokenByAddress(addr common.Address) (token.TokenView, error)
}

// TokenHolderAnalyzer defines the contract for any component that can analyze logs
// to determine the primary holder of a token based on its own internal strategy.
type TokenHolderAnalyzer interface {
	// Update processes a new batch of logs to update its internal state.
	Update(logs []types.Log)

	// TokenByMaxKnownHolder returns a map of the current "max holder" for each token.
	TokenByMaxKnownHolder() map[common.Address]common.Address
}

// BlockExtractor defines the contract for a service that extracts logs.
// It provides a standardized way to start, run, and shut down a logger extraction
// process, abstracting away the specific implementation details.
type BlockExtractor interface {
	// Run starts the main logger extraction process.
	//
	// It is a blocking call that continuously listens for new blocks on the
	// eventer channel and handles them. It is designed to be run in a separate
	// goroutine.
	//
	// The process can be gracefully shut down by canceling the provided context.
	//
	// Parameters:
	//   - ctx: The context for managing the lifecycle of the Run loop.
	//   - eventer: A read-only channel that provides new blocks to be processed.
	//   - logsHandler: A callback function that is invoked with any logs found
	//     in a block that matches the filtering criteria.
	//   - errHandler: A callback function for handling any non-terminal errors
	//     that occur during the process.
	Run(
		ctx context.Context,
		eventer <-chan *types.Block,
		logsHandler func(context.Context, []types.Log) error,
		errHandler func(error),
	)
}

type TokenInitializer interface {
	Initialize(ctx context.Context, tokenAddress common.Address, store TokenStore) (token.TokenView, error)
}
