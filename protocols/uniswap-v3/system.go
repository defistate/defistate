package uniswapv3

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"sync/atomic"
	"time"

	ethclients "github.com/defistate/defistate/clients/eth-clients"
	"github.com/defistate/defistate/protocols/uniswap-v3/ticks"
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

type IDs struct {
	Pool   uint64 `json:"pool"`
	Token0 uint64 `json:"token0"`
	Token1 uint64 `json:"token1"`
}

// Pool is the user-facing, fully resolved struct.
// It uses standard types (common.Address) for ease of use.
type Pool struct {
	Protocol     string           `json:"protocol"` // e.g., "uniswap_v2", "sushiswap"
	IDs          IDs              `json:"ids"`
	Address      common.Address   `json:"address"`
	Token0       common.Address   `json:"token0"`
	Token1       common.Address   `json:"token1"`
	Fee          uint64           `json:"fee"`
	TickSpacing  uint64           `json:"tickSpacing"`
	Liquidity    *big.Int         `json:"liquidity"`
	SqrtPriceX96 *big.Int         `json:"sqrtPriceX96"`
	Tick         int64            `json:"tick"`
	Ticks        []ticks.TickInfo `json:"ticks"`
}

// PoolView is the fully enriched view of a pool, combining the minimal
// core data with the detailed tick liquidity information.
type PoolView struct {
	PoolViewMinimal `json:",inline"`
	Ticks           []ticks.TickInfo `json:"ticks"`
}

// TickIndexer defines the interface for a system that tracks detailed tick liquidity data.
type TickIndexer interface {
	Add(pool uint64, address common.Address, spacing uint64) error
	Get(pool uint64) ([]ticks.TickInfo, error)
	LastUpdatedAtBlock() uint64
	Remove(pool uint64) error
	View() []ticks.TickView
}

// --- Function Type Definitions for Dependencies ---

type GetClientFunc func() (ethclients.ETHClient, error)
type InBlockedListFunc func(poolAddr common.Address) bool

// PoolInitializerFunc defines the function signature for initializing a batch of pools.
// It is the responsibility of the implementer to handle the batch efficiently,
// including managing the concurrency of requests to the Ethereum client and handling
// potential rate-limiting or timeout issues when processing a large number of addresses.
type PoolInitializerFunc func(
	ctx context.Context,
	poolAddr []common.Address,
	getClient GetClientFunc,
	blockNumber *big.Int,
) (
	token0, token1 []common.Address,
	fee []uint64,
	tickSpacing []uint64,
	tick []int64,
	liquidity []*big.Int,
	sqrtPriceX96 []*big.Int,
	errs []error,
)
type DiscoverPoolsFunc func([]types.Log) ([]common.Address, error)

// ideally, the SwapsInBlockFunc should handle Swap, Mint and Burn logs
type SwapsInBlockFunc func(logs []types.Log) (
	pools []common.Address,
	ticks []int64,
	liquidities []*big.Int,
	sqrtPricesX96 []*big.Int,
	err error,
)

type MintsAndBurnsInBlockFunc func(logs []types.Log) (pools []common.Address)

type GetPoolInfoFunc func(ctx context.Context, poolAddrs []common.Address, getClient GetClientFunc, blockNumber *big.Int) (
	ticks []int64,
	liquidities []*big.Int,
	sqrtPricesX96 []*big.Int,
	fees []uint64,
	errs []error,
)

type AddressToIDFunc func(common.Address) (uint64, error)
type IDToAddressFunc func(uint64) (common.Address, error)
type RegisterPoolFunc func(token0, token1, poolAddr common.Address) (poolID uint64, err error)
type RegisterPoolsFunc func(token0s, token1s, poolAddrs []common.Address) (poolIDs []uint64, errs []error)
type ErrorHandlerFunc func(err error)
type OnDeletePoolsFunc func(poolIDs []uint64) []error

// ideally, the TestBloomFunc should test for Swap, Mint and Burn logs
type TestBloomFunc func(types.Bloom) bool

// Config struct to hold all dependencies for the UniswapV3System.
type Config struct {
	SystemName           string
	PrometheusReg        prometheus.Registerer
	NewBlockEventer      chan *types.Block
	GetClient            GetClientFunc
	PoolInitializer      PoolInitializerFunc
	DiscoverPools        DiscoverPoolsFunc
	SwapsInBlock         SwapsInBlockFunc
	MintsAndBurnsInBlock MintsAndBurnsInBlockFunc
	GetPoolInfo          GetPoolInfoFunc
	TokenAddressToID     AddressToIDFunc
	PoolAddressToID      AddressToIDFunc
	PoolIDToAddress      IDToAddressFunc
	RegisterPool         RegisterPoolFunc
	RegisterPools        RegisterPoolsFunc
	ErrorHandler         ErrorHandlerFunc
	TestBloom            TestBloomFunc
	FilterTopics         [][]common.Hash // Optional: Injected topics for logger filtering for performance.
	TickIndexer          TickIndexer
	InitFrequency        time.Duration
	MaxInactiveDuration  time.Duration
	OnDeletePools        OnDeletePoolsFunc
	Logger               Logger
}

func (cfg *Config) validate() error {
	if cfg.PrometheusReg == nil {
		return errors.New("config.PrometheusReg cannot be nil")
	}
	if cfg.NewBlockEventer == nil {
		return errors.New("config.NewBlockEventer cannot be nil")
	}
	if cfg.GetClient == nil {
		return errors.New("config.GetClient cannot be nil")
	}

	if cfg.PoolInitializer == nil {
		return errors.New("config.PoolInitializer cannot be nil")
	}
	if cfg.DiscoverPools == nil {
		return errors.New("config.DiscoverPools cannot be nil")
	}
	if cfg.SwapsInBlock == nil {
		return errors.New("config.SwapsInBlock cannot be nil")
	}
	if cfg.MintsAndBurnsInBlock == nil {
		return errors.New("config.MintsAndBurnsInBlock cannot be nil")
	}
	if cfg.TokenAddressToID == nil {
		return errors.New("config.TokenAddressToID cannot be nil")
	}
	if cfg.PoolAddressToID == nil {
		return errors.New("config.PoolAddressToID cannot be nil")
	}
	if cfg.PoolIDToAddress == nil {
		return errors.New("config.PoolIDToAddress cannot be nil")
	}
	if cfg.RegisterPool == nil {
		return errors.New("config.RegisterPool cannot be nil")
	}
	if cfg.RegisterPools == nil {
		return errors.New("config.RegisterPools cannot be nil")
	}
	if cfg.ErrorHandler == nil {
		return errors.New("config.ErrorHandler cannot be nil")
	}
	if cfg.TestBloom == nil {
		return errors.New("config.TestBloom cannot be nil")
	}
	if cfg.TickIndexer == nil {
		return errors.New("config.TickIndexer cannot be nil")
	}
	if len(cfg.FilterTopics) == 0 {
		return errors.New("config.FilterTopics are required for performance")
	}
	if cfg.GetPoolInfo == nil {
		return errors.New("config.GetPoolInfo cannot be nil")
	}
	if cfg.OnDeletePools == nil {
		return errors.New("config.OnDeletePools is required")
	}
	if cfg.MaxInactiveDuration == 0 {
		return errors.New("config.MaxInactiveDuration cannot be zero")
	}

	return nil
}

// UniswapV3System is the main orchestrator that connects the data registry
// to the live blockchain. It handles block events, discovers and updates pools,
// and manages state with thread-safety.
type UniswapV3System struct {
	systemName           string
	newBlockEventer      chan *types.Block
	getClient            GetClientFunc
	poolInitializer      PoolInitializerFunc
	discoverPools        DiscoverPoolsFunc
	swapsInBlock         SwapsInBlockFunc
	mintsAndBurnsInBlock MintsAndBurnsInBlockFunc
	getPoolInfo          GetPoolInfoFunc
	tokenAddressToID     AddressToIDFunc
	poolAddressToID      AddressToIDFunc
	registerPool         RegisterPoolFunc
	registerPools        RegisterPoolsFunc
	poolIDToAddress      IDToAddressFunc
	cachedView           atomic.Pointer[[]PoolView]
	lastUpdatedAtBlock   atomic.Uint64
	errorHandler         ErrorHandlerFunc
	testBloom            TestBloomFunc
	filterTopics         [][]common.Hash // Optional: Injected topics for logger filtering for performance.
	initFrequency        time.Duration
	maxInactiveDuration  time.Duration
	onDeletePools        OnDeletePoolsFunc

	pendingInit map[common.Address]struct{}
	mu          sync.RWMutex
	registry    *UniswapV3Registry
	tickIndexer TickIndexer
	metrics     *Metrics
	logger      Logger
}

// NewUniswapV3System constructs and returns a new, fully initialized system.
func NewUniswapV3System(ctx context.Context, cfg *Config) (*UniswapV3System, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	metrics := NewMetrics(cfg.PrometheusReg, cfg.SystemName)

	system := &UniswapV3System{
		systemName:           cfg.SystemName,
		newBlockEventer:      cfg.NewBlockEventer,
		getClient:            cfg.GetClient,
		poolInitializer:      cfg.PoolInitializer,
		discoverPools:        cfg.DiscoverPools,
		swapsInBlock:         cfg.SwapsInBlock,
		mintsAndBurnsInBlock: cfg.MintsAndBurnsInBlock,
		getPoolInfo:          cfg.GetPoolInfo,
		tokenAddressToID:     cfg.TokenAddressToID,
		poolAddressToID:      cfg.PoolAddressToID,
		poolIDToAddress:      cfg.PoolIDToAddress,
		registerPool:         cfg.RegisterPool,
		registerPools:        cfg.RegisterPools,
		testBloom:            cfg.TestBloom,
		filterTopics:         cfg.FilterTopics,
		initFrequency:        cfg.InitFrequency,
		maxInactiveDuration:  cfg.MaxInactiveDuration,
		onDeletePools:        cfg.OnDeletePools,
		registry:             NewUniswapV3Registry(),
		pendingInit:          make(map[common.Address]struct{}),
		lastUpdatedAtBlock:   atomic.Uint64{},
		tickIndexer:          cfg.TickIndexer,
		metrics:              metrics,
		errorHandler: func(err error) {
			errorType := determineErrorType(err)
			cfg.Logger.Error("internal error", "system", cfg.SystemName, "type", errorType, "error", err)
			metrics.ErrorsTotal.WithLabelValues(errorType).Inc()
			cfg.ErrorHandler(err)

		},

		logger: cfg.Logger,
	}

	emptyView := make([]PoolView, 0)
	system.cachedView.Store(&emptyView)
	system.logger.Info("uniswap v3 system started", "system", system.systemName)

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
func (s *UniswapV3System) listenBlockEventer(ctx context.Context) {
	for {
		select {
		case b := <-s.newBlockEventer:
			timer := prometheus.NewTimer(s.metrics.BlockProcessingDur.WithLabelValues())

			if !s.testBloom(b.Bloom()) {
				s.logger.Debug("block bloom filter test failed, skipping detailed processing", "block", b.NumberU64())
				s.setLastUpdatedAtBlock(b.NumberU64())
				s.metrics.LastProcessedBlock.WithLabelValues().Set(float64(b.NumberU64()))
				timer.ObserveDuration()
				continue
			}
			if err := s.handleNewBlock(ctx, b); err != nil {
				s.errorHandler(err)
			} else {
				s.metrics.LastProcessedBlock.WithLabelValues().Set(float64(b.NumberU64()))
			}
			timer.ObserveDuration()
		case <-ctx.Done():
			return
		}
	}
}

// startPoolInitializer is a background process that periodically initializes pools from the pending queue.
func (s *UniswapV3System) startPoolInitializer(ctx context.Context) {
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

func (s *UniswapV3System) pruneInactivePools(ctx context.Context, ticker *time.Ticker, maxInactiveDurUnix time.Duration) {
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

// handleNewBlock processes a single block's events.
func (s *UniswapV3System) handleNewBlock(ctx context.Context, b *types.Block) error {
	blockNum := b.NumberU64()
	s.logger.Debug("processing new block", "blockNumber", blockNum, "tx_count", len(b.Transactions()))

	prevView := *s.cachedView.Load()
	poolsToFetch := make(map[common.Address]uint64)

	for _, pool := range prevView {
		addr, err := s.poolIDToAddress(pool.ID)
		if err != nil {
			// highly unlikely to error here
			s.errorHandler(err)
			continue
		}
		poolsToFetch[addr] = pool.ID
	}

	var fetchedPoolAddrs []common.Address
	var fetchedTicks []int64
	var fetchedLiqs []*big.Int
	var fetchedSqrtPrices []*big.Int
	var fetchedFees []uint64
	var capturedErrors []error
	if len(poolsToFetch) > 0 {
		addrsToFetch := make([]common.Address, 0, len(poolsToFetch))
		for addr := range poolsToFetch {
			addrsToFetch = append(addrsToFetch, addr)
		}

		freshTicks, freshLiqs, freshSqrtPrices, fees, errs := s.getPoolInfo(ctx, addrsToFetch, s.getClient, b.Number())
		for i, addr := range addrsToFetch {
			if errs[i] != nil {
				capturedErrors = append(capturedErrors, errs[i])
				continue
			}
			fetchedPoolAddrs = append(fetchedPoolAddrs, addr)
			fetchedTicks = append(fetchedTicks, freshTicks[i])
			fetchedLiqs = append(fetchedLiqs, freshLiqs[i])
			fetchedSqrtPrices = append(fetchedSqrtPrices, freshSqrtPrices[i])
			fetchedFees = append(fetchedFees, fees[i])
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.updatePools(fetchedPoolAddrs, fetchedFees, fetchedTicks, fetchedLiqs, fetchedSqrtPrices, blockNum)
	s.setLastUpdatedAtBlock(blockNum)
	s.updateCachedView()

	// handle block logs in go routine
	go func() {
		err := s.handleBlockLogs(ctx, b)
		if err != nil {
			s.errorHandler(err)
		}
	}()

	for _, e := range capturedErrors {
		s.errorHandler(e)
	}

	return nil
}

// handles discovery and tracks pool updates
func (s *UniswapV3System) handleBlockLogs(ctx context.Context, b *types.Block) error {
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

	updatedInBlock := map[common.Address]struct{}{}

	addrs, _, _, _, err := s.swapsInBlock(logs)
	if err != nil {
		return &SystemError{BlockNumber: blockNum, Err: fmt.Errorf("failed to find pools updated in block: %w", err)}
	}
	for _, addr := range addrs {
		updatedInBlock[addr] = struct{}{}
	}

	addrs = s.mintsAndBurnsInBlock(logs)

	for _, addr := range addrs {
		updatedInBlock[addr] = struct{}{}
	}

	discoveredPoolAddrs, err := s.discoverPools(logs)
	if err != nil {
		return &SystemError{BlockNumber: blockNum, Err: fmt.Errorf("failed to discover pools: %w", err)}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	blockTime := b.Header().Time
	for poolAddr := range updatedInBlock {
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

// runPendingInitializations drains the pending queue and processes new pools.
func (s *UniswapV3System) runPendingInitializations(ctx context.Context) {
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

	s.logger.Info("starting pending pool initialization run", "pool_count", len(poolsToInit))
	s.metrics.PendingInitQueueSize.WithLabelValues().Set(0)

	if len(poolsToInit) == 0 {
		return
	}

	token0s, token1s, fees, tickSpacings, ticks, liquidities, sqrtPrices, errs := s.poolInitializer(ctx, poolsToInit, s.getClient, nil)

	var initErrors []error
	var successfulInits int
	func() {
		s.mu.Lock()
		defer s.mu.Unlock()

		initErrors = s.applyInitializations(0, poolsToInit, token0s, token1s, fees, tickSpacings, ticks, liquidities, sqrtPrices, errs)
		successfulInits = len(poolsToInit) - len(initErrors)

		if successfulInits > 0 {
			s.updateCachedView()
		}
	}()

	if successfulInits > 0 {
		s.metrics.PoolsInitialized.WithLabelValues().Add(float64(successfulInits))
	}
	for _, e := range initErrors {
		s.errorHandler(e)
	}
}

// applyInitializations handles adding newly discovered pools to the registry. This method must be called within a write lock.
func (s *UniswapV3System) applyInitializations(
	blockNumber uint64,
	unknownPools, initToken0s, initToken1s []common.Address,
	initFees, initTickSpacings []uint64,
	initTicks []int64,
	initLiquidities, initSqrtPrices []*big.Int,
	initErrs []error,
) []error {
	if len(unknownPools) == 0 {
		return nil
	}

	var capturedErrors []error

	// Filter out pools that failed the initial RPC call and prepare data for the valid ones.
	var validPools, validT0s, validT1s []common.Address
	var validFees, validSpacings []uint64
	var validTicks []int64
	var validLiqs, validPrices []*big.Int

	for i, poolAddr := range unknownPools {
		if initErrs[i] != nil {
			capturedErrors = append(capturedErrors, &InitializationError{
				SystemError: SystemError{BlockNumber: blockNumber, Err: initErrs[i]},
				PoolAddress: poolAddr,
			})
			continue
		}
		validPools = append(validPools, poolAddr)
		validT0s = append(validT0s, initToken0s[i])
		validT1s = append(validT1s, initToken1s[i])
		validFees = append(validFees, initFees[i])
		validSpacings = append(validSpacings, initTickSpacings[i])
		validTicks = append(validTicks, initTicks[i])
		validLiqs = append(validLiqs, initLiquidities[i])
		validPrices = append(validPrices, initSqrtPrices[i])
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
		if err := s.tickIndexer.Add(poolID, poolAddr, validSpacings[i]); err != nil {
			capturedErrors = append(capturedErrors, &TickIndexingError{
				SystemError: SystemError{Err: err},
				PoolAddress: poolAddr,
				PoolID:      poolID,
				Operation:   "Add",
			})
			continue
		}

		token0ID, err := s.tokenAddressToID(validT0s[i])
		if err != nil {
			capturedErrors = append(capturedErrors, &DataConsistencyError{
				SystemError: SystemError{Err: err}, PoolAddress: poolAddr, Details: fmt.Sprintf("failed to get ID for token0 %s", validT0s[i].Hex()),
			})
			continue
		}
		token1ID, err := s.tokenAddressToID(validT1s[i])
		if err != nil {
			capturedErrors = append(capturedErrors, &DataConsistencyError{
				SystemError: SystemError{Err: err}, PoolAddress: poolAddr, Details: fmt.Sprintf("failed to get ID for token1 %s", validT1s[i].Hex()),
			})
			continue
		}

		if err := addPool(poolID, token0ID, token1ID, validFees[i], validSpacings[i], s.registry); err != nil {
			capturedErrors = append(capturedErrors, &InitializationError{SystemError: SystemError{BlockNumber: blockNumber, Err: err}, PoolAddress: poolAddr})
			continue
		}

		if err := updatePool(poolID, validFees[i], validTicks[i], validLiqs[i], validPrices[i], s.registry); err != nil {
			capturedErrors = append(capturedErrors, &InitializationError{
				SystemError: SystemError{BlockNumber: blockNumber, Err: fmt.Errorf("failed to set initial state: %w", err)},
				PoolAddress: poolAddr,
			})
		}
	}

	if len(capturedErrors) > 0 {
		return capturedErrors
	}

	return nil
}

// updatePools must be called within lock
func (s *UniswapV3System) updatePools(
	poolAddrs []common.Address,
	fees []uint64,
	ticks []int64,
	liquidities []*big.Int,
	sqrtPrices []*big.Int,

	blockNumber uint64,
) {
	for i, poolAddr := range poolAddrs {
		poolID, err := s.poolAddressToID(poolAddr)
		if err != nil {
			continue
		}

		if !hasPool(poolID, s.registry) {
			continue // ensure pool belongs to this system
		}

		err = updatePool(poolID, fees[i], ticks[i], liquidities[i], sqrtPrices[i], s.registry)
		if err != nil {
			s.errorHandler(&UpdateError{
				SystemError:  SystemError{BlockNumber: blockNumber, Err: err},
				PoolAddress:  poolAddr,
				PoolID:       poolID,
				Tick:         ticks[i],
				Liquidity:    liquidities[i],
				SqrtPriceX96: sqrtPrices[i],
			})
		}
	}
}

func (s *UniswapV3System) deletePools(poolIDs []uint64) []error {
	s.mu.Lock()
	defer s.mu.Unlock()

	errs := make([]error, len(poolIDs))
	var hasChanged bool
	var hasErrors bool
	var successfullyDeleted []uint64 // Track what actually gets deleted

	for i, poolID := range poolIDs {
		// First, attempt to delete from the main registry.
		err := deletePool(poolID, s.registry)
		if err != nil {
			errs[i] = err
			hasErrors = true
			continue
		}

		// If the primary deletion was successful, the state has changed.
		hasChanged = true
		successfullyDeleted = append(successfullyDeleted, poolID)

		// Now, attempt to remove from the tick indexer.
		err = s.tickIndexer.Remove(poolID)
		if err != nil {
			// Report this as an error, but the state has still changed.
			errs[i] = fmt.Errorf("pool %d deleted from registry but failed to remove from tick indexer: %w", poolID, err)
			hasErrors = true
		}
	}

	if hasChanged {
		// After any modification, the cached view must be updated.
		s.updateCachedView()
	}

	if len(successfullyDeleted) > 0 && s.onDeletePools != nil {
		s.onDeletePools(successfullyDeleted)
	}

	if hasErrors {
		return errs
	}

	return nil

}

// View returns a slice of all fully enriched pool views from the cache.
func (s *UniswapV3System) View() []PoolView {
	cachedSlice := s.cachedView.Load()
	if cachedSlice == nil || len(*cachedSlice) == 0 {
		return nil
	}

	currentView := *cachedSlice

	viewCopy := make([]PoolView, len(currentView))
	for i, v := range currentView {
		ticksCopy := make([]ticks.TickInfo, len(v.Ticks))
		for j, tick := range v.Ticks {
			ticksCopy[j] = ticks.TickInfo{
				Index:          tick.Index,
				LiquidityGross: new(big.Int).Set(tick.LiquidityGross),
				LiquidityNet:   new(big.Int).Set(tick.LiquidityNet),
			}
		}

		viewCopy[i] = PoolView{
			PoolViewMinimal: PoolViewMinimal{
				ID:           v.ID,
				Token0:       v.Token0,
				Token1:       v.Token1,
				Fee:          v.Fee,
				TickSpacing:  v.TickSpacing,
				Tick:         v.Tick,
				Liquidity:    new(big.Int).Set(v.Liquidity),
				SqrtPriceX96: new(big.Int).Set(v.SqrtPriceX96),
			},
			Ticks: ticksCopy,
		}
	}
	return viewCopy
}

// LastUpdatedAtBlock returns the block number of the last successfully processed block.
func (s *UniswapV3System) LastUpdatedAtBlock() uint64 {
	return s.lastUpdatedAtBlock.Load()
}

// setLastUpdatedAtBlock safely sets the last updated block number.
func (s *UniswapV3System) setLastUpdatedAtBlock(blockNum uint64) {
	s.lastUpdatedAtBlock.Store(blockNum)
}

// updateCachedView generates a fresh, fully enriched view and atomically updates the pointer.
func (s *UniswapV3System) updateCachedView() {
	// time cache updates
	timer := prometheus.NewTimer(s.metrics.UpdateCachedVeiwDur.WithLabelValues())
	defer timer.ObserveDuration()

	minimalViews := viewRegistry(s.registry)
	newView := make([]PoolView, 0, len(minimalViews))

	for _, minimalView := range minimalViews {
		tickInfo, err := s.tickIndexer.Get(minimalView.ID)
		if err != nil {
			poolAddr, _ := s.poolIDToAddress(minimalView.ID)
			s.errorHandler(&TickIndexingError{
				SystemError: SystemError{Err: err},
				PoolAddress: poolAddr,
				PoolID:      minimalView.ID,
				Operation:   "Get",
			})
			tickInfo = []ticks.TickInfo{}
		}

		fullView := PoolView{
			PoolViewMinimal: minimalView,
			Ticks:           tickInfo,
		}
		newView = append(newView, fullView)
	}

	s.cachedView.Store(&newView)
	s.metrics.PoolsInRegistry.WithLabelValues().Set(float64(len(newView)))
}

func (s *UniswapV3System) getLogs(ctx context.Context, client ethclients.ETHClient, block *types.Block) ([]types.Log, error) {
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
