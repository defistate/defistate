package engine

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	DefaultBlockQueueSize         = 100
	DefaultPollSyncInterval       = 100 * time.Millisecond
	DefaultMaxWaitUntilSync       = 500 * time.Millisecond
	DefaultSubscriptionChanBuffer = 10
)

// --------------------------------------------------------------------------------
// --- Configuration and Main Struct ---
// --------------------------------------------------------------------------------

type ErrorHandlerFunc func(err error)

// Config holds all dependencies and settings for creating a new Engine.
type Config struct {
	// Required Dependencies
	ChainID         *big.Int
	NewBlockEventer chan *types.Block

	//  Block-Unaware DeFi Protocols
	Protocols                  map[ProtocolID]Protocol
	BlockSynchronizedProtocols map[ProtocolID]BlockSynchronizedProtocol

	// Behavior Configuration
	PollSyncInterval time.Duration
	MaxWaitUntilSync time.Duration
	BlockQueueSize   int

	ErrorHandler ErrorHandlerFunc
	Logger       Logger
	Registry     prometheus.Registerer
}

func (cfg *Config) validate() error {
	if cfg.ChainID == nil {
		return errors.New("config: ChainID cannot be nil")
	}
	if cfg.NewBlockEventer == nil {
		return errors.New("config: NewBlockEventer cannot be nil")
	}

	if cfg.Registry == nil {
		return errors.New("config: Registry cannot be nil")
	}

	for id, p := range cfg.Protocols {
		if p == nil {
			return fmt.Errorf("config: Protocols[%q] is nil", id)
		}
	}
	for id, p := range cfg.BlockSynchronizedProtocols {
		if p == nil {
			return fmt.Errorf("config: BlockSynchronizedProtocols[%q] is nil", id)
		}
		if _, exists := cfg.Protocols[id]; exists {
			return fmt.Errorf("config: protocol %q registered in both Protocols and BlockSynchronizedProtocols", id)
		}
	}

	return nil
}

// Engine functions as a reliable eventing pipeline.
type Engine struct {
	chainID              *big.Int
	pollSyncInterval     time.Duration
	maxWaitUntilSync     time.Duration
	lastProcessedBlock   uint64
	errorHandler         ErrorHandlerFunc
	logger               Logger
	blockProcessingQueue chan *types.Block
	viewEventer          chan *State
	shutdownSignal       chan struct{}
	lastView             atomic.Pointer[State] // New: Cache for the last view
	subscribers          []chan *State
	subscribersMu        sync.Mutex
	metrics              *Metrics

	//@todo refactor Protocol by addition of the LastUpdatedAtBlock method.
	// protocols contains only protocols that do not expose a LastUpdatedAtBlock method
	protocols map[ProtocolID]Protocol
	// blockSynchronizedProtocols contains protocols that expose a LastUpdatedAtBlock method
	blockSynchronizedProtocols map[ProtocolID]BlockSynchronizedProtocol
}

// NewEngine constructs and returns a new, fully initialized engine.
func NewEngine(ctx context.Context, cfg *Config) (*Engine, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	metrics := NewMetrics(cfg.Registry)

	logger := cfg.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(os.Stdout, nil))
	}

	errorHandler := cfg.ErrorHandler
	if errorHandler == nil {
		errorHandler = func(err error) {
			logger.Error("Engine encountered an error", "error", err)
			metrics.ErrorsTotal.WithLabelValues("unhandled").Inc()
		}
	}

	queueSize := cfg.BlockQueueSize
	if queueSize <= 0 {
		queueSize = DefaultBlockQueueSize
	}

	pollSyncInterval := cfg.PollSyncInterval
	if pollSyncInterval <= 0 {
		pollSyncInterval = DefaultPollSyncInterval
	}

	maxWaitUntilSync := cfg.MaxWaitUntilSync
	if maxWaitUntilSync <= 0 {
		maxWaitUntilSync = DefaultMaxWaitUntilSync
	}

	// Copy to freeze configuration (avoid external mutation after startup).
	protocols := make(map[ProtocolID]Protocol, len(cfg.Protocols))
	for protocolID, protocol := range cfg.Protocols {
		if protocol == nil {
			return nil, fmt.Errorf("config: protocols[%q] cannot be nil", protocolID)
		}
		protocols[protocolID] = protocol
	}

	// Copy to freeze configuration (avoid external mutation after startup).
	blockSynchronizedProtocols := make(map[ProtocolID]BlockSynchronizedProtocol, len(cfg.BlockSynchronizedProtocols))
	for protocolID, protocol := range cfg.BlockSynchronizedProtocols {
		if protocol == nil {
			return nil, fmt.Errorf("config: protocols[%q] cannot be nil", protocolID)
		}
		blockSynchronizedProtocols[protocolID] = protocol
	}

	engine := &Engine{
		chainID:                    cfg.ChainID,
		pollSyncInterval:           pollSyncInterval,
		maxWaitUntilSync:           maxWaitUntilSync,
		errorHandler:               errorHandler,
		logger:                     logger,
		blockProcessingQueue:       make(chan *types.Block, queueSize),
		viewEventer:                make(chan *State),
		shutdownSignal:             make(chan struct{}),
		metrics:                    metrics,
		protocols:                  protocols,
		blockSynchronizedProtocols: blockSynchronizedProtocols,
	}

	engine.lastView.Store(nil) // Initialize the last view cache

	lenBlockSynchronizedProtocols := len(engine.blockSynchronizedProtocols)
	lenProtocols := len(engine.protocols)
	engine.metrics.ActiveProtocols.Set(float64(lenProtocols + lenBlockSynchronizedProtocols))

	engine.logger.Info("Engine starting up",
		"protocols", lenProtocols+lenBlockSynchronizedProtocols,
		"poll_sync_interval", pollSyncInterval,
		"max_wait_until_sync", maxWaitUntilSync,
	)

	go engine.feedBlockQueue(ctx, cfg.NewBlockEventer)
	go engine.runEventLoop(ctx)
	go engine.broadcastViews(ctx)

	return engine, nil
}

// --------------------------------------------------------------------------------
// --- Public Methods ---
// --------------------------------------------------------------------------------

// Subscribe allows a consumer to receive the stream of State events.
func (engine *Engine) Subscribe() (*Subscription, func()) {
	engine.subscribersMu.Lock()
	defer engine.subscribersMu.Unlock()

	subChan := make(chan *State, DefaultSubscriptionChanBuffer)
	engine.subscribers = append(engine.subscribers, subChan)
	engine.metrics.ActiveSubscribers.Set(float64(len(engine.subscribers)))
	engine.logger.Info("New subscriber connected", "total_subscribers", len(engine.subscribers))

	subscription := NewSubscription(subChan, engine.shutdownSignal)

	// --- New Logic: Send initial view ---
	// Launch a goroutine to send the initial view. This avoids blocking the Subscribe call.
	if lastView := engine.lastView.Load(); lastView != nil {
		go func() {
			// Use a non-blocking send. If the subscriber's channel is full
			// at this moment, the initial view is dropped.
			select {
			case subChan <- lastView:
				engine.logger.Debug("Sent initial cached view to new subscriber", "block_number", lastView.Block.Number)
			case <-subscription.done:
				// Engine is shutting down, abort send.
			default:
				// This case is hit if the subscriber's channel buffer is full.
				engine.logger.Warn("Failed to send initial view to new subscriber; channel is full or consumer not ready.", "block_number", lastView.Block.Number)
				engine.metrics.ErrorsTotal.WithLabelValues("subscriber_initial_view_dropped").Inc()
			}
		}()
	}

	unsubscribe := func() {
		engine.subscribersMu.Lock()
		defer engine.subscribersMu.Unlock()

		targetIndex := -1
		for i, ch := range engine.subscribers {
			if ch == subChan {
				targetIndex = i
				break
			}
		}

		if targetIndex != -1 {
			engine.subscribers[targetIndex] = engine.subscribers[len(engine.subscribers)-1]
			engine.subscribers = engine.subscribers[:len(engine.subscribers)-1]
			close(subChan)
			engine.metrics.ActiveSubscribers.Set(float64(len(engine.subscribers)))
			engine.logger.Info("Subscriber unsubscribed", "total_subscribers", len(engine.subscribers))
		}
	}

	return subscription, unsubscribe
}

// LastProcessedBlock returns the number of the last block for which a view was successfully generated.
func (engine *Engine) LastProcessedBlock() uint64 {
	return atomic.LoadUint64(&engine.lastProcessedBlock)
}

// --------------------------------------------------------------------------------
// --- Core Pipeline Goroutines ---
// --------------------------------------------------------------------------------

func (engine *Engine) feedBlockQueue(ctx context.Context, newBlockEventer <-chan *types.Block) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			close(engine.blockProcessingQueue)
			return
		case block, ok := <-newBlockEventer:
			if !ok {
				engine.logger.Info("Upstream block eventer channel closed.")
				close(engine.blockProcessingQueue)
				return
			}
			if block != nil {
				engine.blockProcessingQueue <- block
				engine.metrics.BlockQueueDepth.Set(float64(len(engine.blockProcessingQueue)))
			}
		case <-ticker.C:
			engine.metrics.BlockQueueDepth.Set(float64(len(engine.blockProcessingQueue)))
		}
	}
}

// runEventLoop is the core engine. It now waits for protocols before building the view.
func (engine *Engine) runEventLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			close(engine.viewEventer)
			return
		case block, ok := <-engine.blockProcessingQueue:
			if !ok {
				engine.logger.Info("Block processing queue closed, shutting down event loop.")
				close(engine.viewEventer)
				return
			}

			// Capture the timestamp as soon as processing for this block begins.
			receivedAt := time.Now().UnixNano()

			procTimer := prometheus.NewTimer(engine.metrics.BlockProcessingDur)
			blockNum := block.NumberU64()
			engine.logger.Debug("Processing block", "block_number", blockNum)

			engine.waitForBlockSynchronizedProtocols(ctx, block)

			view := engine.buildState(block, receivedAt)
			engine.viewEventer <- view
			engine.lastView.Store(view) // Store the latest successfully generated view.

			atomic.StoreUint64(&engine.lastProcessedBlock, blockNum)
			engine.metrics.LastProcessedBlock.Set(float64(blockNum))

			procTimer.ObserveDuration()
			engine.logger.Info("Event for block sent to broadcaster", "block_number", blockNum)
		}
	}
}

func (engine *Engine) broadcastViews(ctx context.Context) {
	// Defer closing the shutdown signal to ensure all subscribers are notified when this goroutine exits.
	defer close(engine.shutdownSignal)

	for {
		var view *State
		var ok bool

		select {
		case <-ctx.Done():
			engine.logger.Info("Context cancelled, shutting down broadcaster.")
			return
		case view, ok = <-engine.viewEventer:
			if !ok {
				engine.logger.Info("View eventer channel closed, shutting down broadcaster.")
				return
			}
		}

		engine.subscribersMu.Lock()
		for _, subChan := range engine.subscribers {
			select {
			case subChan <- view:
			default:
				engine.errorHandler(fmt.Errorf("subscriber channel is full for block %d, dropping view", view.Block.Number))
				engine.metrics.ErrorsTotal.WithLabelValues("subscriber_channel_full").Inc()
			}
		}
		engine.subscribersMu.Unlock()
	}
}

// --------------------------------------------------------------------------------
// --- Helper Methods ---
// --------------------------------------------------------------------------------

// waitForBlockSynchronizedProtocols polls until all block synchronized protocols have processed the target block or a timeout occurs.
func (engine *Engine) waitForBlockSynchronizedProtocols(ctx context.Context, targetBlock *types.Block) {
	syncTimer := prometheus.NewTimer(engine.metrics.SyncDur)
	defer syncTimer.ObserveDuration()

	timer := time.NewTimer(engine.maxWaitUntilSync)
	defer timer.Stop()

	ticker := time.NewTicker(engine.pollSyncInterval)
	defer ticker.Stop()

	targetBlockNum := targetBlock.NumberU64()

	if engine.areProtocolsSynced(targetBlockNum) {
		return
	}

	for {
		select {
		case <-timer.C:
			err := fmt.Errorf("timed out after %v waiting for protocols to sync to block %d", engine.maxWaitUntilSync, targetBlockNum)
			engine.errorHandler(err)
			engine.metrics.ErrorsTotal.WithLabelValues("sync_timeout").Inc()
			engine.logger.Warn("Protocol sync timed out", "block_number", targetBlockNum, "timeout", engine.maxWaitUntilSync)
			return

		case <-ticker.C:
			if engine.areProtocolsSynced(targetBlockNum) {
				return // Success, all protocols are synced.
			}
		case <-ctx.Done():
			return
		}
	}
}

// areProtocolsSynced only checks the block-synchronized protocols
func (engine *Engine) areProtocolsSynced(targetBlock uint64) bool {
	timer := prometheus.NewTimer(engine.metrics.SyncCheckDur)
	defer timer.ObserveDuration()

	for _, protocol := range engine.blockSynchronizedProtocols {
		if protocol.LastUpdatedAtBlock() < targetBlock {
			return false
		}
	}
	return true
}

// buildState constructs the final view, checking each protocol individually
// and populating the result with either data or an error.
// buildState constructs the final state for a given block and populates each protocol result.
// The engine owns out-of-sync detection: protocols are considered valid for this event only
// if LastUpdatedAtBlock() >= targetBlockNum.
func (engine *Engine) buildState(b *types.Block, receivedAt int64) *State {
	buildTimer := prometheus.NewTimer(engine.metrics.StateBuildDur)
	defer buildTimer.ObserveDuration()

	targetBlockNum := b.NumberU64()

	ProtocolStates := make(map[ProtocolID]ProtocolState, len(engine.protocols)+len(engine.blockSynchronizedProtocols))

	// --- Process Block-Unaware Protocols ---
	for protocolID, protocol := range engine.protocols {
		protocolMeta := protocol.Meta()

		ProtocolState := ProtocolState{
			Meta: protocolMeta,
		}

		data, schema, err := protocol.View()
		if err != nil {
			ProtocolState.Error = err.Error()
			engine.metrics.ErrorsTotal.WithLabelValues("protocol_view_error").Inc()
			ProtocolStates[protocolID] = ProtocolState
			continue
		}

		if schema == "" {
			ProtocolState.Error = "protocol returned empty schema"
			engine.metrics.ErrorsTotal.WithLabelValues("protocol_empty_schema").Inc()
			ProtocolStates[protocolID] = ProtocolState
			continue
		}

		ProtocolState.Schema = schema
		ProtocolState.Data = data
		ProtocolStates[protocolID] = ProtocolState
	}

	// --- Process Block-Aware Protocols ---
	for protocolID, protocol := range engine.blockSynchronizedProtocols {
		protocolMeta := protocol.Meta()
		syncedBlockNumber := protocol.LastUpdatedAtBlock()

		ProtocolState := ProtocolState{
			Meta:              protocolMeta,
			SyncedBlockNumber: &syncedBlockNumber,
			Schema:            protocol.Schema(),
		}

		// Engine-owned sync semantics: if behind, emit uniform error and omit data.
		if syncedBlockNumber < targetBlockNum {
			ProtocolState.Error = fmt.Sprintf(
				"protocol out of sync: synced=%d target=%d",
				syncedBlockNumber,
				targetBlockNum,
			)
			engine.metrics.ErrorsTotal.WithLabelValues("protocol_out_of_sync").Inc()
			ProtocolStates[protocolID] = ProtocolState
			continue
		}

		data, _, err := protocol.View()
		if err != nil {
			ProtocolState.Error = err.Error()
			engine.metrics.ErrorsTotal.WithLabelValues("protocol_view_error").Inc()
			ProtocolStates[protocolID] = ProtocolState
			continue
		}

		ProtocolState.Data = data
		ProtocolStates[protocolID] = ProtocolState
	}

	// --- Block Summary ---
	header := b.Header()
	blockSummary := BlockSummary{
		Number:      b.Number(),
		Hash:        b.Hash(),
		Timestamp:   b.Time(),
		ReceivedAt:  receivedAt,
		GasUsed:     header.GasUsed,
		GasLimit:    header.GasLimit,
		StateRoot:   header.Root,
		TxHash:      header.TxHash,
		ReceiptHash: header.ReceiptHash,
	}

	return &State{
		ChainID:   engine.chainID.Uint64(),
		Block:     blockSummary,
		Timestamp: uint64(time.Now().UnixNano()),
		Protocols: ProtocolStates,
	}
}

// @note questions to answer
// who detects and handles reorgs?
// protocols or the engine?
// engine detects
// protocols detect and handle.
