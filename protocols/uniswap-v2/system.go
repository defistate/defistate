package uniswapv2

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"sync/atomic"
	"time"

	ethclients "github.com/defistate/defistate/clients/eth-clients"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	DefaultPruneInactivePoolsTickerDuration = 5 * time.Minute
)

// Logger defines a standard interface for structured, leveled logging,
// compatible with the standard library's slog.
type Logger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
}

// --- Function Type Definitions for Dependencies ---

// These named types create a clear, maintainable contract for the system's dependencies.

type GetClientFunc func() (ethclients.ETHClient, error)
type InBlockedListFunc func(poolAddr common.Address) bool
type PoolInitializerFunc func(ctx context.Context, poolAddr []common.Address, getClient func() (ethclients.ETHClient, error)) (token0, token1 []common.Address, poolType []uint8, feeBps []uint16, reserve0, reserve1 []*big.Int, errs []error)
type DiscoverPoolsFunc func([]types.Log) ([]common.Address, error)
type UpdatedInBlockFunc func([]types.Log) (pools []common.Address, reserve0, reserve1 []*big.Int, err error)
type GetReservesFunc func(ctx context.Context, poolAddrs []common.Address, getClient func() (ethclients.ETHClient, error), blockNumber *big.Int) (reserve0, reserve1 []*big.Int, errs []error)
type AddressToIDFunc func(common.Address) (uint64, error)
type IDToAddressFunc func(uint64) (common.Address, error)
type RegisterPoolFunc func(token0, token1, poolAddr common.Address) (poolID uint64, err error)
type RegisterPoolsFunc func(token0s, token1s, poolAddrs []common.Address) (poolIDS []uint64, error []error)
type OnDeletePoolsFunc func(poolIDs []uint64) []error
type ErrorHandlerFunc func(err error)
type TestBloomFunc func(types.Bloom) bool

// Config holds all the dependencies and settings for the UniswapV2System.
// Using a configuration struct makes initialization cleaner and more extensible.
type Config struct {
	SystemName          string
	PrometheusReg       prometheus.Registerer
	NewBlockEventer     chan *types.Block
	GetClient           GetClientFunc
	PoolInitializer     PoolInitializerFunc
	DiscoverPools       DiscoverPoolsFunc
	UpdatedInBlock      UpdatedInBlockFunc
	GetReserves         GetReservesFunc
	TokenAddressToID    AddressToIDFunc
	PoolAddressToID     AddressToIDFunc
	PoolIDToAddress     IDToAddressFunc
	RegisterPool        RegisterPoolFunc
	RegisterPools       RegisterPoolsFunc
	ErrorHandler        ErrorHandlerFunc
	OnDeletePools       OnDeletePoolsFunc
	TestBloom           TestBloomFunc
	FilterTopics        [][]common.Hash
	InitFrequency       time.Duration
	MaxInactiveDuration time.Duration
	Logger              Logger
}

// validate checks that all essential fields in the Config are provided.
func (c *Config) validate() error {
	if c.SystemName == "" {
		return errors.New("sSystemName is required")
	}
	if c.NewBlockEventer == nil {
		return errors.New("NewBlockEventer channel is required")
	}
	if c.GetClient == nil {
		return errors.New("GetClient function is required")
	}

	if c.PoolInitializer == nil {
		return errors.New("PoolInitializer function is required")
	}
	if c.DiscoverPools == nil {
		return errors.New("DiscoverPools function is required")
	}
	if c.UpdatedInBlock == nil {
		return errors.New("UpdatedInBlock function is required")
	}
	if c.GetReserves == nil {
		return errors.New("GetReserves function is required")
	}
	if c.TokenAddressToID == nil {
		return errors.New("TokenAddressToID function is required")
	}
	if c.PoolAddressToID == nil {
		return errors.New("PoolAddressToID function is required")
	}
	if c.PoolIDToAddress == nil {
		return errors.New("PoolIDToAddress function is required")
	}
	if c.RegisterPool == nil {
		return errors.New("RegisterPool function is required")
	}
	if c.RegisterPools == nil {
		return errors.New("RegisterPools function is required")
	}
	if c.ErrorHandler == nil {
		return errors.New("ErrorHandler function is required")
	}
	if c.TestBloom == nil {
		return errors.New("TestBloom function is required")
	}
	if c.OnDeletePools == nil {
		return errors.New("OnDeletePools is required")
	}
	if c.MaxInactiveDuration == 0 {
		return errors.New("MaxInactiveDuration is required")
	}
	if len(c.FilterTopics) == 0 {
		return errors.New("FilterTopics are required for performance")
	}

	return nil
}

// UniswapV2System is the main orchestrator that connects the data registry
// to the live blockchain. It handles block events, discovers and updates pools,
// and manages state with thread-safety.
type UniswapV2System struct {
	systemName          string
	newBlockEventer     chan *types.Block
	getClient           GetClientFunc
	poolInitializer     PoolInitializerFunc
	discoverPools       DiscoverPoolsFunc
	updatedInBlock      UpdatedInBlockFunc
	getReserves         GetReservesFunc
	tokenAddressToID    AddressToIDFunc
	poolAddressToID     AddressToIDFunc
	registerPool        RegisterPoolFunc
	registerPools       RegisterPoolsFunc
	poolIDToAddress     IDToAddressFunc
	cachedView          atomic.Pointer[[]PoolView]
	lastUpdatedAtBlock  atomic.Uint64
	errorHandler        ErrorHandlerFunc
	testBloom           TestBloomFunc
	filterTopics        [][]common.Hash // Store topics for use in handleNewBlock
	initFrequency       time.Duration
	maxInactiveDuration time.Duration
	pendingInit         map[common.Address]struct{}
	onDeletePools       OnDeletePoolsFunc
	mu                  sync.RWMutex
	registry            *UniswapV2Registry
	metrics             *Metrics
	logger              Logger
}

// NewUniswapV2System constructs a new, live system with an empty initial state.
func NewUniswapV2System(ctx context.Context, cfg *Config) (*UniswapV2System, error) {
	registry := NewUniswapV2Registry()
	return newUniswapV2System(ctx, cfg, registry)
}

// NewUniswapV2SystemFromViews constructs a new, live system from a snapshot of pool views.
// This is the primary entry point for rehydrating the system's state on startup.
func NewUniswapV2SystemFromViews(ctx context.Context, cfg *Config, views []PoolView) (*UniswapV2System, error) {
	registry := NewUniswapV2RegistryFromViews(views)
	return newUniswapV2System(ctx, cfg, registry)
}

// newUniswapV2System constructs and returns a new, fully initialized system.
// It starts all background goroutines, making it a self-contained, "live" service upon creation.
func newUniswapV2System(ctx context.Context, cfg *Config, registry *UniswapV2Registry) (*UniswapV2System, error) {
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid uniswapv2 system configuration: %w", err)
	}

	metrics := NewMetrics(cfg.PrometheusReg, cfg.SystemName)

	system := &UniswapV2System{
		systemName:       cfg.SystemName,
		newBlockEventer:  cfg.NewBlockEventer,
		getClient:        cfg.GetClient,
		poolInitializer:  cfg.PoolInitializer,
		discoverPools:    cfg.DiscoverPools,
		updatedInBlock:   cfg.UpdatedInBlock,
		getReserves:      cfg.GetReserves,
		tokenAddressToID: cfg.TokenAddressToID,
		poolAddressToID:  cfg.PoolAddressToID,
		poolIDToAddress:  cfg.PoolIDToAddress,
		registerPool:     cfg.RegisterPool,
		registerPools:    cfg.RegisterPools,
		errorHandler: func(err error) {
			errorType := determineErrorType(err)
			cfg.Logger.Error("internal error", "system", cfg.SystemName, "type", errorType, "error", err)
			metrics.ErrorsTotal.WithLabelValues(errorType).Inc()

			// 3. Call the user's external handler.
			cfg.ErrorHandler(err)
		},
		testBloom:           cfg.TestBloom,
		filterTopics:        cfg.FilterTopics,
		initFrequency:       cfg.InitFrequency,
		onDeletePools:       cfg.OnDeletePools,
		registry:            registry,
		pendingInit:         make(map[common.Address]struct{}),
		lastUpdatedAtBlock:  atomic.Uint64{},
		maxInactiveDuration: cfg.MaxInactiveDuration,
		metrics:             metrics,
		logger:              cfg.Logger,
	}

	// store initial view
	initialView := viewRegistry(registry)
	system.cachedView.Store(&initialView)
	system.logger.Info("uniswap v2 system started", "system", system.systemName)
	go system.listenBlockEventer(ctx)
	go system.startPoolInitializer(ctx)
	go system.pruneInactivePools(
		ctx,
		time.NewTicker(DefaultPruneInactivePoolsTickerDuration),
		system.maxInactiveDuration,
	)

	return system, nil
}

// listenBlockEventer is the main event loop for the system.
func (s *UniswapV2System) listenBlockEventer(ctx context.Context) {
	for {
		select {
		case b := <-s.newBlockEventer:
			timer := prometheus.NewTimer(s.metrics.BlockProcessingDur.WithLabelValues())

			if !s.testBloom(b.Bloom()) {
				s.lastUpdatedAtBlock.Store(b.NumberU64())
				s.metrics.LastProcessedBlock.WithLabelValues().Set(float64(b.NumberU64()))
				timer.ObserveDuration()
				continue
			}
			if err := s.handleNewBlock(ctx, b); err != nil {
				s.errorHandler(err)
			}
			timer.ObserveDuration()
		case <-ctx.Done():
			s.logger.Info("stopping due to context cancellation.")
			return
		}
	}
}

// startPoolInitializer is a background process that periodically initializes pools from the pending queue.
func (s *UniswapV2System) startPoolInitializer(ctx context.Context) {
	if s.initFrequency <= 0 {
		return
	}
	ticker := time.NewTicker(s.initFrequency)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.runPendingInitializations(ctx)
		case <-ctx.Done():
			return
		}
	}
}

// runPendingInitializations drains the pending queue and processes the new pools in a batch.
func (s *UniswapV2System) runPendingInitializations(ctx context.Context) {
	timer := prometheus.NewTimer(s.metrics.PoolInitDur.WithLabelValues())
	defer timer.ObserveDuration()

	var poolsToInit []common.Address
	func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if len(s.pendingInit) > 0 {
			poolsToInit = make([]common.Address, 0, len(s.pendingInit))
			for addr := range s.pendingInit {
				poolsToInit = append(poolsToInit, addr)
			}
			s.pendingInit = make(map[common.Address]struct{})
		}
	}()

	s.metrics.PendingInitQueueSize.WithLabelValues().Set(0)
	if len(poolsToInit) == 0 {
		return
	}

	s.logger.Info("running pool initializer", "count", len(poolsToInit))

	token0s, token1s, poolTypes, feeBps, reserve0s, reserve1s, errs := s.poolInitializer(ctx, poolsToInit, s.getClient)

	var initErrors []error
	var successfulInits int
	func() {
		s.mu.Lock()
		defer s.mu.Unlock()

		initErrors = s.applyInitializations(0, poolsToInit, token0s, token1s, poolTypes, feeBps, reserve0s, reserve1s, errs)
		successfulInits = len(poolsToInit) - len(initErrors)

		if successfulInits > 0 {
			s.updateCachedView()
		}
	}()

	if successfulInits > 0 {
		s.logger.Info(
			"successfully initialized new pools",
			"count", successfulInits,
			"failed", len(initErrors),
		)
		s.metrics.PoolsInitialized.WithLabelValues().Add(float64(successfulInits))
	}
	for _, e := range initErrors {
		s.errorHandler(e)
	}
}

func (s *UniswapV2System) pruneInactivePools(ctx context.Context, ticker *time.Ticker, maxInactiveDurUnix time.Duration) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			poolsToDelete := []uint64{}
			s.mu.Lock()
			poolsLastUpdatedAt := getLastUpdatedAtMap(s.registry)
			s.mu.Unlock()
			now := time.Now()
			for id, lastUpdatedAt := range poolsLastUpdatedAt {
				if lastUpdatedAt > 0 {
					if now.Sub(time.Unix(int64(lastUpdatedAt), 0)) > maxInactiveDurUnix {
						poolsToDelete = append(poolsToDelete, id)
					}
				}
			}
			errs := s.deletePools(poolsToDelete)
			for _, err := range errs {
				if err != nil {
					s.errorHandler(err)
				}
			}
		}
	}
}

// handleNewBlock processes a single block, performs fast synchronous updates,
// and queues slow pool initializations for asynchronous processing.
func (s *UniswapV2System) handleNewBlock(ctx context.Context, b *types.Block) error {
	blockNum := b.NumberU64()
	start := time.Now()
	defer func() {
		s.logger.Info("processed new block", "blockNumber", blockNum, "tx_count", len(b.Transactions()), "duration", time.Since(start))
	}()

	// load cached view
	prevView := *s.cachedView.Load()
	poolAddrs := make([]common.Address, 0, len(prevView))
	for _, view := range prevView {
		addr, err := s.poolIDToAddress(view.ID)
		if err != nil {
			s.errorHandler(&PrunerError{PoolID: view.ID, Err: fmt.Errorf("could not get address: %w", err)})
			continue
		}
		poolAddrs = append(poolAddrs, addr)
	}

	var (
		capturedErrors  []error
		validPoolsAddrs = make([]common.Address, 0, len(poolAddrs))
		validR0         = make([]*big.Int, 0, len(poolAddrs))
		validR1         = make([]*big.Int, 0, len(poolAddrs))
	)

	reserve0s, reserve1s, errs := s.getReserves(ctx, poolAddrs, s.getClient, b.Number())
	for i := 0; i < len(poolAddrs); i++ {
		if errs[i] != nil {
			capturedErrors = append(capturedErrors, errs[i])
			continue
		}
		validPoolsAddrs = append(validPoolsAddrs, poolAddrs[i])
		validR0 = append(validR0, reserve0s[i])
		validR1 = append(validR1, reserve1s[i])
	}

	func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		updateErrors := s.applyUpdates(blockNum, validPoolsAddrs, validR0, validR1)
		capturedErrors = append(capturedErrors, updateErrors...)
		s.lastUpdatedAtBlock.Store(blockNum)
		s.updateCachedView()
	}()

	// handle block logs in go routine
	go func() {
		err := s.handleBlockLogs(ctx, b)
		if err != nil {
			s.errorHandler(err)
		}
	}()
	s.metrics.LastProcessedBlock.WithLabelValues().Set(float64(blockNum))
	for _, e := range capturedErrors {
		s.errorHandler(e)
	}
	return nil
}

// handles discovery and tracks pool updates
func (s *UniswapV2System) handleBlockLogs(ctx context.Context, b *types.Block) error {
	blockNum := b.NumberU64()
	client, err := s.getClient()
	if err != nil {
		return fmt.Errorf("block %d: failed to get eth client: %w", blockNum, err)
	}

	filterStart := time.Now()
	logs, err := s.getLogs(ctx, client, b)
	s.logger.Info("filter logs rpc call completed", "blockNumber", blockNum, "duration", time.Since(filterStart))
	if err != nil {
		return fmt.Errorf("block %d: failed to filter logs: %w", blockNum, err)
	}

	updatedInBlock, _, _, err := s.updatedInBlock(logs)
	if err != nil {
		return &SystemError{BlockNumber: blockNum, Err: fmt.Errorf("failed to find pools updated in block: %w", err)}
	}

	discoveredPoolAddrs, err := s.discoverPools(logs)
	if err != nil {
		return &SystemError{BlockNumber: blockNum, Err: fmt.Errorf("failed to discover pools: %w", err)}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	blockTime := b.Header().Time
	for _, poolAddr := range updatedInBlock {
		poolID, err := s.poolAddressToID(poolAddr)
		if err != nil {
			continue // Pool not in registry, possibly pruned or pending initialization.
		}

		if !hasPool(poolID, s.registry) {
			continue // ensure pool belongs to this system
		}
		// let it be know that this pool was updated at
		setLastUpdatedAt(poolID, blockTime, s.registry)
	}

	newPendingCount := 0
	for _, poolAddr := range discoveredPoolAddrs {
		if _, err := s.poolAddressToID(poolAddr); err == nil {
			//pool already known
			continue
		}
		if _, exists := s.pendingInit[poolAddr]; !exists {
			s.pendingInit[poolAddr] = struct{}{}
			newPendingCount++
		}
	}

	if newPendingCount > 0 {
		s.metrics.PendingInitQueueSize.WithLabelValues().Add(float64(newPendingCount))
	}
	s.logger.Info(
		"discovered new pools in block",
		"blockNumber", blockNum,
		"count", len(discoveredPoolAddrs),
	)
	return nil
}

// applyUpdates handles updating reserves for existing pools. This method must be called within a write lock.
func (s *UniswapV2System) applyUpdates(blockNumber uint64, updatedPoolAddrs []common.Address, updatedReserve0s, updatedReserve1s []*big.Int) []error {
	var capturedErrors []error
	for i, poolAddr := range updatedPoolAddrs {
		poolID, err := s.poolAddressToID(poolAddr)
		if err != nil {
			continue // Pool not in registry, possibly pruned or pending initialization.
		}

		if !hasPool(poolID, s.registry) {
			continue // ensure pool belongs to this system
		}

		if err = updatePool(updatedReserve0s[i], updatedReserve1s[i], poolID, s.registry); err != nil {
			capturedErrors = append(capturedErrors, &UpdateError{
				SystemError: SystemError{BlockNumber: blockNumber, Err: err},
				PoolAddress: poolAddr,
				PoolID:      poolID,
				Reserve0:    updatedReserve0s[i],
				Reserve1:    updatedReserve1s[i],
			})
		}
	}
	return capturedErrors
}

// applyInitializations handles adding newly discovered pools to the registry. This method must be called within a write lock.
func (s *UniswapV2System) applyInitializations(
	blockNumber uint64,
	unknownPools, initToken0s, initToken1s []common.Address,
	initPoolTypes []uint8,
	initFeeBps []uint16,
	initReserve0s, initReserve1s []*big.Int,
	initErrs []error,
) []error {
	if len(unknownPools) == 0 {
		return nil
	}

	var capturedErrors []error

	// Filter out pools that failed the initial RPC call and prepare data for the valid ones.
	var validPools, validT0s, validT1s []common.Address
	var validTypes []uint8
	var validFees []uint16
	var validR0s, validR1s []*big.Int
	// Map from the index in the 'valid' slices back to the original index in 'unknownPools'.
	originalIndices := make(map[int]int)

	for i, poolAddr := range unknownPools {
		if initErrs[i] != nil {
			capturedErrors = append(capturedErrors, &InitializationError{
				SystemError: SystemError{BlockNumber: blockNumber, Err: initErrs[i]},
				PoolAddress: poolAddr,
			})
			continue
		}
		originalIndices[len(validPools)] = i
		validPools = append(validPools, poolAddr)
		validT0s = append(validT0s, initToken0s[i])
		validT1s = append(validT1s, initToken1s[i])
		validTypes = append(validTypes, initPoolTypes[i])
		validFees = append(validFees, initFeeBps[i])
		validR0s = append(validR0s, initReserve0s[i])
		validR1s = append(validR1s, initReserve1s[i])
	}

	if len(validPools) == 0 {
		return capturedErrors
	}

	// Step 1: Register all valid pools in a single batch operation.
	newPoolIDs, regErrs := s.registerPools(validT0s, validT1s, validPools)

	// Keep track of which pools failed registration so we don't try to add them locally.
	failedRegistration := make(map[int]bool)

	for i, regErr := range regErrs {
		if regErr != nil {
			poolAddr := validPools[i]
			capturedErrors = append(capturedErrors, &RegistrationError{
				InitializationError: InitializationError{
					SystemError: SystemError{BlockNumber: blockNumber, Err: regErr},
					PoolAddress: poolAddr,
				},
				Token0Address: validT0s[i],
				Token1Address: validT1s[i],
			})
			failedRegistration[i] = true
		}
	}

	// Step 2: Now, iterate through the valid pools again and add the ones that were successfully registered.
	for i, poolAddr := range validPools {
		if failedRegistration[i] {
			continue
		}

		poolID := newPoolIDs[i]
		if err := addPool(validT0s[i], validT1s[i], poolAddr, validTypes[i], validFees[i], s.tokenAddressToID, s.poolAddressToID, s.registry); err != nil {
			capturedErrors = append(capturedErrors, &InitializationError{SystemError: SystemError{BlockNumber: blockNumber, Err: err}, PoolAddress: poolAddr})
			continue
		}

		if err := updatePool(validR0s[i], validR1s[i], poolID, s.registry); err != nil {
			capturedErrors = append(capturedErrors, &InitializationError{
				SystemError: SystemError{BlockNumber: blockNumber, Err: fmt.Errorf("failed to set initial reserves: %w", err)},
				PoolAddress: poolAddr,
			})
		}
	}

	if len(capturedErrors) > 0 {
		return capturedErrors
	}

	return nil
}

// appropriate manager-level method.
func (s *UniswapV2System) deletePools(poolIDs []uint64) []error {
	if len(poolIDs) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	errs := make([]error, len(poolIDs))
	hasChanged := false
	hasErrors := false

	deletedPools := []uint64{}
	for i, poolID := range poolIDs {
		err := deletePool(poolID, s.registry)
		if err != nil {
			errs[i] = err
			hasErrors = true
		} else {
			hasChanged = true
			deletedPools = append(deletedPools, poolID)
		}
	}

	if hasChanged {
		// After any modification to the registry, the cached view must be updated.
		s.updateCachedView()
	}

	// we ignore the errors for now
	s.onDeletePools(deletedPools)

	if hasErrors {
		return errs
	}

	return nil
}

func (s *UniswapV2System) getLogs(ctx context.Context, client ethclients.ETHClient, block *types.Block) ([]types.Log, error) {
	blockNumber := block.Number()
	query := ethereum.FilterQuery{
		FromBlock: blockNumber,
		ToBlock:   blockNumber,
		Topics:    s.filterTopics,
	}

	logs, err := client.FilterLogs(ctx, query)
	if err != nil {
		return nil, err // For genuine RPC errors, fail immediately.
	}

	return logs, nil
}

// updateCachedView generates a fresh view from the registry and atomically updates the pointer.
// This method MUST be called from within a write lock (s.mu.Lock).
func (s *UniswapV2System) updateCachedView() {
	newView := viewRegistry(s.registry)
	s.cachedView.Store(&newView)
	s.metrics.PoolsInRegistry.WithLabelValues().Set(float64(len(newView)))
}

// View returns a copy of the latest registry view. This operation is lock-free.
func (s *UniswapV2System) View() []PoolView {
	viewPtr := s.cachedView.Load()
	if viewPtr == nil {
		return nil
	}
	view := *viewPtr
	viewCopy := make([]PoolView, len(view))
	copy(viewCopy, view)
	return viewCopy
}

// LastUpdatedAtBlock returns the block number of the last successfully processed block.
func (s *UniswapV2System) LastUpdatedAtBlock() uint64 {
	return s.lastUpdatedAtBlock.Load()
}
