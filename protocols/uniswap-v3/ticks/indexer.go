package ticks

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	ethclients "github.com/defistate/defistate/clients/eth-clients"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/prometheus/client_golang/prometheus"
)

// Logger defines a standard interface for structured, leveled logging,
// compatible with the standard library's slog.
type Logger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
}

// Package-level errors for consistent error handling.
var (
	ErrPoolNotFound  = errors.New("pool not tracked by indexer")
	ErrPoolExists    = errors.New("pool already tracked by indexer")
	ErrNilDependency = errors.New("a function dependency cannot be nil")
)

// updateRequest encapsulates the data needed to update the state for multiple pools.
type updateRequest struct {
	pools       []common.Address
	newDatas    [][]TickInfo
	blockNumber uint64
}

type pendingInit struct {
	address common.Address
	spacing uint64
}

type TickIndexer struct {
	id          []uint64
	address     []common.Address
	ticks       [][]TickInfo
	bitmap      []Bitmap
	spacing     []uint64
	lastUpdates []uint64

	// dependencies
	getClient           func() (ethclients.ETHClient, error)
	getTickBitmap       func(ctx context.Context, pools []common.Address, spacing []uint64, blockNumber *big.Int) ([]Bitmap, []error)
	getInitializedTicks func(ctx context.Context, pools []common.Address, bitmaps []Bitmap, spacing []uint64, blockNumber *big.Int) (infos [][]TickInfo, errs []error)
	getTicks            func(ctx context.Context, pools []common.Address, ticks [][]int64, blockNumber *big.Int) ([][]TickInfo, []error)
	updatedInBlock      func(logs []types.Log) (pools []common.Address, updatedTicks [][]int64, err error)
	errorHandler        func(error)
	testBloomFunc       func(types.Bloom) bool
	filterTopics        [][]common.Hash

	// Mappings for efficient lookups
	idToIndex      map[uint64]int
	addressToIndex map[common.Address]int

	// Queues and background routine management
	pendingInit        map[uint64]*pendingInit
	pendingUpdates     chan updateRequest
	initFrequency      time.Duration
	resyncFrequency    time.Duration
	updateFrequency    time.Duration
	lastUpdatedAtBlock atomic.Uint64

	logMaxRetries int
	logRetryDelay time.Duration

	mu sync.RWMutex
	// Observability
	metrics *indexerMetrics

	logger Logger
}

type Bitmap map[int16]*big.Int

// TickView provides a safe, external representation of a single pool's state.
type TickView struct {
	ID    uint64     `json:"id"`
	Ticks []TickInfo `json:"ticks"`
}

// Config holds all the dependencies and settings for creating a new TickIndexer.
type Config struct {
	SystemName          string
	Registry            prometheus.Registerer
	NewBlockEventer     chan *types.Block
	GetClient           func() (ethclients.ETHClient, error)
	GetTickBitmap       func(ctx context.Context, pools []common.Address, spacing []uint64, blockNumber *big.Int) ([]Bitmap, []error)
	GetInitializedTicks func(ctx context.Context, pools []common.Address, bitmaps []Bitmap, spacing []uint64, blockNumber *big.Int) (infos [][]TickInfo, errs []error)
	GetTicks            func(ctx context.Context, pools []common.Address, ticks [][]int64, blockNumber *big.Int) ([][]TickInfo, []error)
	UpdatedInBlock      func(logs []types.Log) (pools []common.Address, updatedTicks [][]int64, err error)
	ErrorHandler        func(error)
	TestBloomFunc       func(types.Bloom) bool
	FilterTopics        [][]common.Hash
	InitFrequency       time.Duration
	ResyncFrequency     time.Duration
	UpdateFrequency     time.Duration
	LogMaxRetries       int
	LogRetryDelay       time.Duration
	Logger              Logger
}

func (cfg *Config) validate() error {
	// --- Dependency Validation ---
	if cfg.Registry == nil {
		return fmt.Errorf("prometheus registry cannot be nil")
	}
	if cfg.GetClient == nil {
		return fmt.Errorf("GetClient dependency: %w", ErrNilDependency)
	}
	if cfg.GetTickBitmap == nil {
		return fmt.Errorf("GetTickBitmap dependency: %w", ErrNilDependency)
	}
	if cfg.GetInitializedTicks == nil {
		return fmt.Errorf("GetInitializedTicks dependency: %w", ErrNilDependency)
	}
	if cfg.GetTicks == nil {
		return fmt.Errorf("GetTicks dependency: %w", ErrNilDependency)
	}
	if cfg.UpdatedInBlock == nil {
		return fmt.Errorf("UpdatedInBlock dependency: %w", ErrNilDependency)
	}
	if cfg.ErrorHandler == nil {
		return fmt.Errorf("ErrorHandler dependency: %w", ErrNilDependency)
	}
	if cfg.TestBloomFunc == nil {
		return fmt.Errorf("TestBloomFunc dependency: %w", ErrNilDependency)
	}
	if len(cfg.FilterTopics) == 0 {
		return errors.New("filter topics are required for performance")
	}

	return nil
}

// it is crucial that these fetchers are optimized to run and return faster than new blocks are processed
// else we must reduce or reconciliation frequency
func NewTickIndexer(
	ctx context.Context,
	cfg *Config,
) (*TickIndexer, error) {
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid uniswapv2 system configuration: %w", err)
	}

	// Create and register metrics with the provided registry.
	metrics := newIndexerMetrics(cfg.Registry, cfg.SystemName)

	tickIndexer := &TickIndexer{
		id:                  make([]uint64, 0),
		address:             make([]common.Address, 0),
		ticks:               make([][]TickInfo, 0),
		bitmap:              make([]Bitmap, 0),
		idToIndex:           make(map[uint64]int),
		addressToIndex:      make(map[common.Address]int),
		pendingInit:         make(map[uint64]*pendingInit),
		pendingUpdates:      make(chan updateRequest, 100), // Buffered channel
		getClient:           cfg.GetClient,
		getTickBitmap:       cfg.GetTickBitmap,
		getInitializedTicks: cfg.GetInitializedTicks,
		getTicks:            cfg.GetTicks,
		updatedInBlock:      cfg.UpdatedInBlock,
		errorHandler: func(err error) {
			errorType := "unknown"
			var initErr *InitializationError
			var blockErr *BlockProcessingError
			var dropErr *UpdateDroppedError
			var resyncErr *ResyncError

			if errors.As(err, &dropErr) {
				errorType = "update_dropped"
			} else if errors.As(err, &blockErr) {
				errorType = "block_processing"
			} else if errors.As(err, &initErr) {
				errorType = "initialization"
			} else if errors.As(err, &resyncErr) {
				errorType = "resync"
			}

			cfg.Logger.Error("tick indexer error occurred", "type", errorType, "error", err)
			metrics.errorsTotal.WithLabelValues(errorType).Inc()
			cfg.ErrorHandler(err) // Call the original handler
		},

		testBloomFunc:   cfg.TestBloomFunc,
		filterTopics:    cfg.FilterTopics,
		initFrequency:   cfg.InitFrequency,
		resyncFrequency: cfg.ResyncFrequency,
		updateFrequency: cfg.UpdateFrequency,
		metrics:         metrics,
		logMaxRetries:   cfg.LogMaxRetries,
		logRetryDelay:   cfg.LogRetryDelay,
		logger:          cfg.Logger,
	}

	// Launch the background goroutines.
	go tickIndexer.listenBlockEventer(ctx, cfg.NewBlockEventer)
	go tickIndexer.startInitializer(ctx)
	go tickIndexer.startUpdater(ctx)
	go tickIndexer.startResyncer(ctx)

	// In NewTickIndexer, after starting goroutines...
	tickIndexer.logger.Info(
		"tick indexer started successfully",
		"init_frequency", cfg.InitFrequency,
		"resync_frequency", cfg.ResyncFrequency,
		"update_frequency", cfg.UpdateFrequency,
	)

	return tickIndexer, nil
}

// listenBlockEventer is the primary consumer of new blocks. It performs a bloom
// filter check before handing off the block for more expensive processing.
func (ti *TickIndexer) listenBlockEventer(ctx context.Context, newBlockEventer chan *types.Block) {
	for {
		select {
		case block := <-newBlockEventer:
			if block == nil {
				continue
			}
			timer := prometheus.NewTimer(ti.metrics.blockProcessingDuration.WithLabelValues())
			blockNum := block.NumberU64()
			ti.logger.Debug("new block received by tick indexer", "block", blockNum)
			// Test the block's bloom filter. If it passes, process the block.
			if ti.testBloomFunc(block.Bloom()) {
				ti.logger.Debug("block passed bloom filter", "block", blockNum)
				if err := ti.handleNewBlock(ctx, block); err != nil {
					// On error, we do not advance the block counter, allowing for potential retries.
					ti.errorHandler(err)
					timer.ObserveDuration()
					continue // Skip setting the last updated block
				}
			}

			// The block was processed successfully, either because it contained updates
			// which were handled, or because the bloom filter test failed (meaning
			// there were no relevant updates). In both cases, we are now up-to-date
			// with this block.
			ti.setLastUpdatedAtBlock(blockNum)
			timer.ObserveDuration()
			ti.metrics.lastProcessedBlock.Set(float64(blockNum)) // METRIC: update last processed block

		case <-ctx.Done():
			return
		}
	}
}

// startInitializer periodically checks for and processes pools in the pendingInit queue.
func (ti *TickIndexer) startInitializer(ctx context.Context) {
	ticker := time.NewTicker(ti.initFrequency)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// --- Step 1: Create a batch of pools to process WITHOUT clearing the main queue yet. ---
			var batchIDs []uint64
			var batchPending []*pendingInit

			ti.mu.RLock() // Use a read lock to safely copy the pending items.
			if len(ti.pendingInit) > 0 {
				for id, pending := range ti.pendingInit {
					batchIDs = append(batchIDs, id)
					batchPending = append(batchPending, pending)
				}
			}
			ti.mu.RUnlock()

			if len(batchIDs) == 0 {
				continue
			}

			// --- Step 2: Process the batch. This function now returns the IDs of successfully processed pools. ---
			successfulIDs := ti.initializePools(ctx, batchIDs, batchPending)

			// --- Step 3: Remove ONLY the successful pools from the pending queue. ---
			if len(successfulIDs) > 0 {
				ti.mu.Lock()
				for _, id := range successfulIDs {
					delete(ti.pendingInit, id)
				}
				ti.logger.Info(
					"finished pool initialization run",
					"processed", len(batchIDs),
					"successful", len(successfulIDs),
				)
				ti.mu.Unlock()
			}

		case <-ctx.Done():
			return
		}
	}
}

// startUpdater periodically drains the pendingUpdates channel and applies them to the state.
func (ti *TickIndexer) startUpdater(ctx context.Context) {
	ticker := time.NewTicker(ti.updateFrequency)
	defer ticker.Stop()
	for {
		select {
		case update := <-ti.pendingUpdates:
			// METRIC: On successful receive, decrement the queue depth gauge.
			ti.logger.Debug(
				"applying tick updates",
				"pool_count", len(update.pools),
			)
			ti.metrics.pendingUpdatesQueue.Dec()
			ti.mu.Lock()
			ti.applyTickUpdates(update.pools, update.newDatas, update.blockNumber)
			ti.mu.Unlock()
		case <-ctx.Done():
			return
		}
	}
}

// startResyncer periodically re-fetches the state of tracked pools to correct
// any state drift that may have occurred from missed events.
func (ti *TickIndexer) startResyncer(ctx context.Context) {
	ticker := time.NewTicker(ti.resyncFrequency)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			ti.resyncPools(ctx)
		case <-ctx.Done():
			return
		}
	}
}

// getLogsWithRetry attempts to fetch logs for a specific block, using a
// high-frequency polling strategy to account for potential node indexing delays.
//
// This function is called only after a block's bloom filter has passed our
// test, meaning we expect relevant logs to be present. If the initial query
// returns an empty slice, it retries up to `defaultLogMaxRetries` times
// before concluding the block has no relevant logs.
//
// It is crucial that this function does not fail to retrieve logs when they exist for the block
// Failing to retrieve logs happen when the node has not fully indexed logs for that block
func (ti *TickIndexer) getLogsWithRetry(ctx context.Context, client ethclients.ETHClient, block *types.Block) ([]types.Log, error) {
	blockHash := block.Hash()
	blockNumber := block.Number()
	query := ethereum.FilterQuery{
		FromBlock: blockNumber,
		ToBlock:   blockNumber,
		Topics:    ti.filterTopics,
	}
	// maxAttempts is 1 + the s.logMaxRetries value
	// we will try to fetch logs at least 1.
	maxAttempts := 1 + ti.logMaxRetries
	for i := range maxAttempts {

		attempt := i + 1
		logs, err := client.FilterLogs(ctx, query)
		if err != nil {
			return nil, err // For genuine RPC errors, fail immediately.
		}

		// If logs are found, we have succeeded.
		if len(logs) > 0 {
			return logs, nil
		}

		// If logs are empty, it might be a race condition (node might still be processing the block)
		// we can retry if attempt < maxAttempts

		if attempt < maxAttempts {
			select {
			case <-time.After(ti.logRetryDelay):
				ti.logger.Debug("retrying fetch for block", "block", block.NumberU64(), "attempt", attempt)
				continue
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
	}

	// If all retries are exhausted, assume no relevant logs exist.
	ti.logger.Warn("no relevant logs found for block after all retries", "block", block.NumberU64(), "hash", blockHash.Hex())
	return []types.Log{}, nil // Return an empty slice, not an error.
}

// handleNewBlock processes events from a single block, fetches required data,
// and queues the updates for the updater goroutine.
func (ti *TickIndexer) handleNewBlock(ctx context.Context, b *types.Block) error {
	blockNum := b.NumberU64()
	client, err := ti.getClient()
	if err != nil {
		return &BlockProcessingError{BlockNumber: blockNum, BaseError: BaseError{Err: fmt.Errorf("failed to get eth client: %w", err)}}
	}

	logs, err := ti.getLogsWithRetry(ctx, client, b)
	if err != nil {
		return &BlockProcessingError{BlockNumber: blockNum, BaseError: BaseError{Err: fmt.Errorf("failed to filter logs with retry: %w", err)}}
	}

	// Create a snapshot of currently tracked pools to avoid repeated locking.
	ti.mu.RLock()
	trackedPools := make(map[common.Address]struct{}, len(ti.addressToIndex))
	for addr := range ti.addressToIndex {
		trackedPools[addr] = struct{}{}
	}
	ti.mu.RUnlock()
	poolsToFetch := make(map[common.Address]struct{})
	ticksToFetchByPool := make(map[common.Address][]int64)

	updatedPoolAddrs, updatedTickIndexes, err := ti.updatedInBlock(logs)
	if err != nil {
		ti.errorHandler(fmt.Errorf("block %d: error parsing updated pools from logs: %w", blockNum, err))
	} else {
		for i, updatedPoolAddr := range updatedPoolAddrs {
			// Check if the pool is tracked by this indexer instance before fetching (lock-free).
			if _, isTracked := trackedPools[updatedPoolAddr]; isTracked {
				poolsToFetch[updatedPoolAddr] = struct{}{}
				ticksToFetchByPool[updatedPoolAddr] = append(ticksToFetchByPool[updatedPoolAddr], updatedTickIndexes[i]...)
			}
		}
	}

	if len(poolsToFetch) == 0 {
		return nil
	}

	// Convert maps to slices for the batch RPC call.
	finalPoolsToFetch := make([]common.Address, 0, len(poolsToFetch))
	finalTicksToFetch := make([][]int64, 0, len(poolsToFetch))
	for addr := range poolsToFetch {
		finalPoolsToFetch = append(finalPoolsToFetch, addr)
		finalTicksToFetch = append(finalTicksToFetch, ticksToFetchByPool[addr])
	}

	newTickData, errs := ti.getTicks(ctx, finalPoolsToFetch, finalTicksToFetch, b.Number())
	for _, e := range errs {
		if e != nil {
			ti.errorHandler(fmt.Errorf("block %d: error getting updated tick data: %w", blockNum, e))
		}
	}
	select {
	case ti.pendingUpdates <- updateRequest{
		pools:       finalPoolsToFetch,
		newDatas:    newTickData,
		blockNumber: blockNum,
	}:
		// METRIC: On successful send, increment the queue depth gauge.
		ti.metrics.pendingUpdatesQueue.Inc()
	default:
		// Use the new custom error type here
		err := &UpdateDroppedError{
			BlockProcessingError: BlockProcessingError{
				BlockNumber: blockNum,
				BaseError: BaseError{
					Err: fmt.Errorf("tick update channel is full, dropping update"),
				},
			},
		}
		ti.errorHandler(err)
		ti.metrics.updatesDroppedTotal.Inc()
	}

	ti.logger.Info(
		"found tick updates in block",
		"block", blockNum,
		"pools_with_updates", len(finalPoolsToFetch),
	)
	return nil
}

// applyTickUpdates applies fetched tick data to the main state. It correctly handles
// updating existing ticks, inserting new ticks, and removing uninitialized ticks.
// This must be called within a write lock.
func (ti *TickIndexer) applyTickUpdates(pools []common.Address, newDatasByPool [][]TickInfo, blockNumber uint64) {
	for i, poolAddr := range pools {
		poolIndex, ok := ti.addressToIndex[poolAddr]
		if !ok {
			continue // This pool is not being tracked.
		}

		for _, updatedTick := range newDatasByPool[i] {
			poolTicks := ti.ticks[poolIndex]
			ti.lastUpdates[poolIndex] = blockNumber
			// Use binary search to efficiently find the potential position of the tick.
			foundIndex := sort.Search(len(poolTicks), func(k int) bool {
				return poolTicks[k].Index >= updatedTick.Index
			})

			// Case 1: The tick exists at the found index.
			if foundIndex < len(poolTicks) && poolTicks[foundIndex].Index == updatedTick.Index {
				// Check if the tick has become uninitialized.
				if updatedTick.LiquidityGross.Cmp(big.NewInt(0)) == 0 {
					// REMOVE the tick from the slice.
					ti.logger.Debug("removing zero-liquidity tick", "pool", poolAddr.Hex(), "tick_index", updatedTick.Index)
					ti.ticks[poolIndex] = append(ti.ticks[poolIndex][:foundIndex], ti.ticks[poolIndex][foundIndex+1:]...)
				} else {
					// UPDATE the existing tick.
					ti.logger.Debug("updating existing tick", "pool", poolAddr.Hex(), "tick_index", updatedTick.Index)
					ti.ticks[poolIndex][foundIndex] = updatedTick
				}
			} else { // Case 2: The tick does not exist.
				// If the tick has liquidity, it's a new tick that needs to be inserted.
				if updatedTick.LiquidityGross.Cmp(big.NewInt(0)) > 0 {
					// INSERT the new tick at the correct sorted position.
					ti.logger.Debug("inserting new tick", "pool", poolAddr.Hex(), "tick_index", updatedTick.Index)
					ti.ticks[poolIndex] = append(ti.ticks[poolIndex], TickInfo{})
					copy(ti.ticks[poolIndex][foundIndex+1:], ti.ticks[poolIndex][foundIndex:])
					ti.ticks[poolIndex][foundIndex] = updatedTick
				}
				// If LiquidityGross is 0 and the tick doesn't exist, we do nothing.
			}
		}
	}
}

// initializePools fetches the full initial state for a batch of new pools and
// integrates them into the indexer's main state.
//
// CRITICAL CHANGE: It now returns a slice of uint64 containing the IDs of the
// pools that were successfully initialized and added to the indexer's state.
func (ti *TickIndexer) initializePools(ctx context.Context, batchIDs []uint64, batchPending []*pendingInit) []uint64 {
	// --- Perform heavy data fetching outside of the lock ---
	// Step 1: Fetch initial bitmaps for all pools in the batch.
	batchAddresses := make([]common.Address, len(batchPending))
	batchSpacings := make([]uint64, len(batchPending))

	for i, b := range batchPending {
		batchAddresses[i] = b.address
		batchSpacings[i] = b.spacing
	}

	bitmaps, bitmapErrs := ti.getTickBitmap(ctx, batchAddresses, batchSpacings, nil) //block number intentionally set as nil @todo verify whether we should specify block.
	for _, err := range bitmapErrs {
		if err != nil {
			ti.errorHandler(&InitializationError{BaseError{
				Err: fmt.Errorf("failed to get tick bitmap: %w", err),
			}})
		}
	}

	// --- Step 2: Filter out any pools that had errors before the next call ---
	successfulPools := make([]common.Address, 0, len(batchAddresses))
	successfulBitmaps := make([]Bitmap, 0, len(batchAddresses))
	successfulSpacings := make([]uint64, 0, len(batchAddresses))
	addressToOriginalIndex := make(map[common.Address]int, len(batchAddresses))

	for i, addr := range batchAddresses {
		if bitmapErrs == nil || bitmapErrs[i] == nil {
			successfulPools = append(successfulPools, addr)
			successfulBitmaps = append(successfulBitmaps, bitmaps[i])
			successfulSpacings = append(successfulSpacings, batchSpacings[i])
			addressToOriginalIndex[addr] = i
		}
	}

	// --- Step 3: Fetch initialized ticks ONLY for the successful pools ---
	var initializedTicksForSuccesses [][]TickInfo
	var tickInfoErrsForSuccesses []error

	if len(successfulPools) > 0 {
		initializedTicksForSuccesses, tickInfoErrsForSuccesses = ti.getInitializedTicks(ctx, successfulPools, successfulBitmaps, successfulSpacings, nil) //block number intentionally set as nil @todo verify whether we should specify block.
		for _, err := range tickInfoErrsForSuccesses {
			if err != nil {
				ti.errorHandler(fmt.Errorf("initializer: failed to get initialized ticks: %w", err))
			}
		}
	}

	// --- Step 4: Map the results from the filtered call back to their original positions ---
	allTickInfos := make([][]TickInfo, len(batchAddresses))
	allTickInfoErrs := make([]error, len(batchAddresses))

	for i, poolData := range initializedTicksForSuccesses {
		poolAddr := successfulPools[i]
		originalIndex := addressToOriginalIndex[poolAddr]
		allTickInfos[originalIndex] = poolData
		if tickInfoErrsForSuccesses != nil {
			allTickInfoErrs[originalIndex] = tickInfoErrsForSuccesses[i]
		}
	}

	// This slice will hold the IDs of pools that are successfully added.
	var successfulIDs []uint64

	// --- Step 5: Acquire write lock to integrate the successful results into the main state ---
	ti.mu.Lock()
	defer ti.mu.Unlock()

	for i, id := range batchIDs {
		// If an error occurred for this index 'i' at any point, we skip it.
		// It will remain in the pendingInit queue to be retried.
		if (bitmapErrs != nil && bitmapErrs[i] != nil) || (allTickInfoErrs[i] != nil) {
			continue
		}

		addr := batchAddresses[i]
		spacing := batchSpacings[i]
		if _, exists := ti.idToIndex[id]; exists {
			// This pool was somehow added by another routine, which is unexpected but safe to skip.
			// We still consider it a "success" for the purpose of removing it from the pending queue.
			successfulIDs = append(successfulIDs, id)
			continue
		}

		index := len(ti.id)
		ti.idToIndex[id] = index
		ti.addressToIndex[addr] = index
		ti.id = append(ti.id, id)
		ti.address = append(ti.address, addr)
		ti.spacing = append(ti.spacing, spacing)
		ti.lastUpdates = append(ti.lastUpdates, 0)

		poolTicks := allTickInfos[i]
		sort.Slice(poolTicks, func(k, l int) bool {
			return poolTicks[k].Index < poolTicks[l].Index
		})

		ti.ticks = append(ti.ticks, poolTicks)
		ti.bitmap = append(ti.bitmap, bitmaps[i])

		// The pool was successfully added, so record its ID.
		ti.logger.Info("successfully initialized pool", "pool_id", id, "pool_address", addr.Hex())
		successfulIDs = append(successfulIDs, id)
	}

	return successfulIDs
}

// resyncPools fetches the current on-chain state for all tracked pools and replaces the local state.
func (ti *TickIndexer) resyncPools(ctx context.Context) {
	ti.logger.Info("starting tick state resync process")
	timer := prometheus.NewTimer(ti.metrics.resyncDuration.WithLabelValues())
	defer timer.ObserveDuration()

	// Phase 1: Get a snapshot of all tracked pools under a read lock.
	ti.mu.RLock()
	addresses := make([]common.Address, len(ti.address))
	spacings := make([]uint64, len(ti.address))
	lastUpdates := make([]uint64, len(ti.address))
	// Create a deep copy of the current ticks to compare against later.
	prevTicks := make([][]TickInfo, len(ti.ticks))
	for i, tickSlice := range ti.ticks {
		prevTicks[i] = make([]TickInfo, len(tickSlice))
		copy(prevTicks[i], tickSlice)
	}
	copy(addresses, ti.address)
	copy(spacings, ti.spacing)
	copy(lastUpdates, ti.lastUpdates)

	ti.logger.Info("starting tick state resync for all pools", "pool_count", len(addresses))
	ti.mu.RUnlock()

	if len(addresses) == 0 {
		return
	}

	// Phase 2: Fetch the complete, correct on-chain state (lock-free).
	currentBitmaps, bitmapErrs := ti.getTickBitmap(ctx, addresses, spacings, nil)                    //block number intentionally set as nil @todo verify whether we should specify block.
	freshTickData, tickErrs := ti.getInitializedTicks(ctx, addresses, currentBitmaps, spacings, nil) //block number intentionally set as nil @todo verify whether we should specify block.

	// sort ticks
	for _, freshTicks := range freshTickData {
		sort.Slice(freshTicks, func(k, l int) bool {
			return freshTicks[k].Index < freshTicks[l].Index
		})
	}

	// create map to store  indexes with tick drifts
	hasTickDrifts := make(map[int]struct{}, len(addresses))
	for i := range addresses {
		if (bitmapErrs != nil && bitmapErrs[i] != nil) || (tickErrs != nil && tickErrs[i] != nil) {
			continue
		}
		if !areTickSlicesEqual(prevTicks[i], freshTickData[i]) {
			hasTickDrifts[i] = struct{}{}
		}
	}

	// Phase 3: Acquire a write lock and replace the old state, with a safety check.
	ti.mu.Lock()
	defer ti.mu.Unlock()

	for i, addr := range addresses {
		poolIndex, ok := ti.addressToIndex[addr]
		if !ok {
			continue // Pool was removed during the resync process.
		}

		// If either fetch failed for this pool, skip the update to avoid corrupting state.
		if (bitmapErrs != nil && bitmapErrs[i] != nil) || (tickErrs != nil && tickErrs[i] != nil) {
			ti.errorHandler(&ResyncError{BaseError{
				Err: fmt.Errorf("failed to fetch full state for pool %s during resync", addr.Hex()),
			}})
			continue
		}

		// --- CRITICAL SAFETY CHECK ---
		// Compare the current state with the snapshot we took before the RPC calls.
		// @todo
		// this check isn't complete (because last update might have been for just one tick and not all)
		// fix this by tagging each tick update with a block number
		if lastUpdates[i] != ti.lastUpdates[poolIndex] {
			// The state was updated by a real-time block event while we were fetching.
			// The real-time data is fresher, so we discard our stale resync data for this pool.
			continue
		}

		// Sort the freshly fetched ticks by index.
		freshTicks := freshTickData[i]

		// --- Check for actual state drift before applying the update ---
		_, tickDriftDetected := hasTickDrifts[i]
		if tickDriftDetected {
			ti.logger.Warn(
				"state drift detected during resync, applying correction",
				"pool_address", addr.Hex(),
				"old_tick_count", len(prevTicks[i]),
				"new_tick_count", len(freshTicks),
			)
			// Atomically replace the old data with the new, correct data.
			ti.ticks[poolIndex] = freshTicks
			ti.bitmap[poolIndex] = currentBitmaps[i]
			ti.metrics.resyncPoolsCorrectedTotal.Inc() // METRIC: increment corrected ticks during resync
		}
	}

	ti.logger.Info("tick state resync complete", "pool_count", len(addresses))
}

// areTickSlicesEqual is a helper to compare two slices of TickInfo for equality.
func areTickSlicesEqual(a, b []TickInfo) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Index != b[i].Index ||
			a[i].LiquidityGross.Cmp(b[i].LiquidityGross) != 0 ||
			a[i].LiquidityNet.Cmp(b[i].LiquidityNet) != 0 {
			return false
		}
	}
	return true
}

// setLastUpdatedAtBlock safely updates the last processed block number.
func (ti *TickIndexer) setLastUpdatedAtBlock(blockNum uint64) {
	ti.lastUpdatedAtBlock.Store(blockNum)
}

// Add adds a new pool to the pending initialization queue.
func (ti *TickIndexer) Add(pool uint64, address common.Address, spacing uint64) error {
	ti.mu.Lock()
	defer ti.mu.Unlock()
	ti.logger.Info("adding pool to pending initialization", "pool_id", pool, "address", address.Hex())
	if _, exists := ti.idToIndex[pool]; exists {
		return ErrPoolExists
	}
	ti.pendingInit[pool] = &pendingInit{
		address: address,
		spacing: spacing,
	}
	ti.metrics.trackedPoolsTotal.Inc() // METRIC: increment total pools

	return nil
}

// Remove stops tracking a pool and removes all its associated data.
func (ti *TickIndexer) Remove(pool uint64) error {
	ti.mu.Lock()
	defer ti.mu.Unlock()
	indexToRemove, exists := ti.idToIndex[pool]
	if !exists {
		return ErrPoolNotFound
	}

	addr := ti.address[indexToRemove]
	ti.logger.Info("removing pool from tick indexer", "pool_id", pool, "address", addr.Hex())
	lastIndex := len(ti.id) - 1
	if indexToRemove != lastIndex {
		lastPoolID := ti.id[lastIndex]
		lastPoolAddress := ti.address[lastIndex]
		lastPoolSpacing := ti.spacing[lastIndex]
		lastUpdates := ti.lastUpdates[lastIndex]
		ti.id[indexToRemove] = lastPoolID
		ti.address[indexToRemove] = lastPoolAddress
		ti.spacing[indexToRemove] = lastPoolSpacing
		ti.lastUpdates[indexToRemove] = lastUpdates
		ti.ticks[indexToRemove] = ti.ticks[lastIndex]
		ti.bitmap[indexToRemove] = ti.bitmap[lastIndex]
		ti.idToIndex[lastPoolID] = indexToRemove
		ti.addressToIndex[lastPoolAddress] = indexToRemove
	}
	delete(ti.idToIndex, pool)
	delete(ti.addressToIndex, addr)
	ti.id = ti.id[:lastIndex]
	ti.address = ti.address[:lastIndex]
	ti.spacing = ti.spacing[:lastIndex]
	ti.lastUpdates = ti.lastUpdates[:lastIndex]
	ti.ticks = ti.ticks[:lastIndex]
	ti.bitmap = ti.bitmap[:lastIndex]
	ti.metrics.trackedPoolsTotal.Dec() // METRIC: decrement total pools

	return nil
}

// View returns a deep copy snapshot of the state of all tracked pools.
func (ti *TickIndexer) View() []TickView {
	ti.mu.RLock()
	defer ti.mu.RUnlock()
	views := make([]TickView, 0, len(ti.id))
	for i, poolID := range ti.id {
		sourceTicks := ti.ticks[i]
		ticksCopy := make([]TickInfo, len(sourceTicks))
		for j, tick := range sourceTicks {
			ticksCopy[j] = TickInfo{
				Index:          tick.Index,
				LiquidityGross: new(big.Int).Set(tick.LiquidityGross),
				LiquidityNet:   new(big.Int).Set(tick.LiquidityNet),
			}
		}
		views = append(views, TickView{ID: poolID, Ticks: ticksCopy})
	}
	return views
}

func (ti *TickIndexer) LastUpdatedAtBlock() uint64 {
	return ti.lastUpdatedAtBlock.Load()
}

// Get returns a deep copy of the tick information for a specific pool.
func (ti *TickIndexer) Get(pool uint64) ([]TickInfo, error) {
	ti.mu.RLock()
	defer ti.mu.RUnlock()
	index, exists := ti.idToIndex[pool]
	if !exists {
		return nil, ErrPoolNotFound
	}
	sourceTicks := ti.ticks[index]
	ticksCopy := make([]TickInfo, len(sourceTicks))
	for i, tick := range sourceTicks {
		ticksCopy[i] = TickInfo{
			Index:          tick.Index,
			LiquidityGross: new(big.Int).Set(tick.LiquidityGross),
			LiquidityNet:   new(big.Int).Set(tick.LiquidityNet),
		}
	}
	return ticksCopy, nil
}
