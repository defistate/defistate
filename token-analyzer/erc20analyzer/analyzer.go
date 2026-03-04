package erc20analyzer

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"time"

	token "github.com/defistate/defistate/protocols/erc20-token-system"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

var (
	DefaultFeeAndGasUpdateFrequency = 5 * time.Minute
	DefaultMinTokenUpdateInterval   = 60 * time.Minute
	DefaultInitializeRequestTimeout = 20 * time.Second
)

// Logger defines a standard interface for structured, leveled logging,
// compatible with the standard library's slog.
type Logger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
}

// Config holds the dependencies required to initialize an Analyzer.
type Config struct {
	NewBlockEventer          <-chan *types.Block
	TokenStore               TokenStore
	TokenInitializer         TokenInitializer
	BlockExtractor           BlockExtractor
	TokenHolderAnalyzer      TokenHolderAnalyzer
	FeeAndGasRequester       FeeAndGasRequester
	FeeAndGasUpdateFrequency time.Duration
	MinTokenUpdateInterval   time.Duration
	ErrorHandler             func(error)
	Logger                   Logger
}

func (c *Config) applyDefaults() {
	if c.FeeAndGasUpdateFrequency == 0 {
		c.FeeAndGasUpdateFrequency = DefaultFeeAndGasUpdateFrequency
	}
	if c.MinTokenUpdateInterval == 0 {
		c.MinTokenUpdateInterval = DefaultMinTokenUpdateInterval
	}

	if c.Logger == nil {
		c.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
}

func (c *Config) validate() error {
	if c.TokenStore == nil {
		return errors.New("a TokenStore implementation is a required dependency")
	}
	if c.TokenInitializer == nil {
		return errors.New("a TokenInitializer is a required dependency")
	}
	if c.BlockExtractor == nil {
		return errors.New("a BlockExtractor implementation is a required dependency")
	}
	if c.NewBlockEventer == nil {
		return errors.New("NewBlockEventer channel is required")
	}
	if c.TokenHolderAnalyzer == nil {
		return errors.New("a TokenHolderAnalyzer implementation is a required dependency")
	}
	if c.FeeAndGasRequester == nil {
		return errors.New("a FeeAndGasRequester implementation is a required dependency")
	}
	if c.ErrorHandler == nil {
		return errors.New("an ErrorHandler is a required dependency")
	}
	return nil
}

// Analyzer is the central orchestrator for processing token data.
type Analyzer struct {
	tokenStore             TokenStore
	tokenInitializer       TokenInitializer
	tokenHolderAnalyzer    TokenHolderAnalyzer
	errorHandler           func(error)
	minTokenUpdateInterval time.Duration

	mu                 sync.RWMutex
	lastFeeAndGasCheck map[common.Address]time.Time
	logger             Logger
}

// NewAnalyzer creates and starts a new Analyzer instance.
func NewAnalyzer(ctx context.Context, cfg Config) (*Analyzer, error) {
	cfg.applyDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	cfg.Logger.Info("ERC20 Analyzer starting up",
		"fee_update_frequency", cfg.FeeAndGasUpdateFrequency,
		"token_update_interval", cfg.MinTokenUpdateInterval,
	)

	analyzer := &Analyzer{
		tokenStore:             cfg.TokenStore,
		tokenInitializer:       cfg.TokenInitializer,
		tokenHolderAnalyzer:    cfg.TokenHolderAnalyzer,
		errorHandler:           cfg.ErrorHandler,
		minTokenUpdateInterval: cfg.MinTokenUpdateInterval,
		lastFeeAndGasCheck:     make(map[common.Address]time.Time),
		logger:                 cfg.Logger,
	}

	go cfg.BlockExtractor.Run(
		ctx,
		cfg.NewBlockEventer,
		analyzer.blockLogsHandler,
		analyzer.errorHandler,
	)

	ticker := time.NewTicker(cfg.FeeAndGasUpdateFrequency)
	go analyzer.updateTokenFeeAndGasLoop(ctx, ticker, cfg.FeeAndGasRequester)

	return analyzer, nil
}

func (a *Analyzer) blockLogsHandler(ctx context.Context, logs []types.Log) error {
	if len(logs) > 0 {
		a.logger.Debug("Received new block logs for analysis", "log_count", len(logs))
	}
	a.tokenHolderAnalyzer.Update(logs)
	return nil
}

func (a *Analyzer) updateTokenFeeAndGasLoop(ctx context.Context, ticker *time.Ticker, requester FeeAndGasRequester) {
	defer ticker.Stop()
	a.logger.Info("starting fee and gas update loop")
	defer a.logger.Info("stopping fee and gas update loop")

	a.performFeeAndGasUpdate(ctx, requester) // Initial run

	for {
		select {
		case <-ticker.C:
			a.performFeeAndGasUpdate(ctx, requester)
		case <-ctx.Done():
			return
		}
	}
}

func (a *Analyzer) performFeeAndGasUpdate(ctx context.Context, requester FeeAndGasRequester) {
	a.logger.Debug("starting fee and gas update cycle")
	tokensToRequest := a.filterTokensRequiringUpdate()
	if len(tokensToRequest) == 0 {
		a.logger.Debug("no tokens require a fee and gas update")
		return
	}

	results, err := requester.RequestAll(ctx, tokensToRequest)
	if err != nil {
		a.errorHandler(err)
		return
	}

	a.updateTokens(ctx, results)
}

func (a *Analyzer) filterTokensRequiringUpdate() map[common.Address]common.Address {
	knownHolders := a.tokenHolderAnalyzer.TokenByMaxKnownHolder()
	if len(knownHolders) == 0 {
		return nil
	}
	tokensToRequest := make(map[common.Address]common.Address)
	a.mu.RLock()
	defer a.mu.RUnlock()
	for token, holder := range knownHolders {
		if lastCheck, ok := a.lastFeeAndGasCheck[token]; !ok || time.Since(lastCheck) > a.minTokenUpdateInterval {
			tokensToRequest[token] = holder
		}
	}
	return tokensToRequest
}

func (a *Analyzer) updateTokens(ctx context.Context, results map[common.Address]FeeAndGasResult) {
	for tokenAddr, result := range results {
		if result.Error != nil {
			a.errorHandler(result.Error)
			continue
		}

		tokenView, err := a.tokenStore.GetTokenByAddress(tokenAddr)
		if err != nil {
			// If token doesn't exist, initialize it.
			a.logger.Info("new token detected. Initializing.", "token_address", tokenAddr)
			initCtx, cancel := context.WithTimeout(ctx, DefaultInitializeRequestTimeout)
			initializedToken, err := a.tokenInitializer.Initialize(initCtx, tokenAddr, a.tokenStore)
			cancel()
			if err != nil {
				a.errorHandler(err)
				continue // If initialization fails, skip this token.
			}

			tokenView = initializedToken
		}

		// Now that we have the tokenView, either from Get or Initialize, update it.
		if err := a.tokenStore.UpdateToken(tokenView.ID, result.Fee, result.Gas); err != nil {
			a.errorHandler(err)
			continue // If update fails, don't update timestamp, so it will be retried.
		}

		// On successful update, lock and record the time.
		a.mu.Lock()
		a.lastFeeAndGasCheck[tokenAddr] = time.Now()
		a.mu.Unlock()
		a.logger.Debug("successfully updated token fee and gas", "token_address", tokenAddr)
	}
}

func (a *Analyzer) View() []token.TokenView {
	return a.tokenStore.View()
}
