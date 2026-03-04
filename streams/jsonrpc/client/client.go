package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	differ "github.com/defistate/defistate/differ"
	"github.com/defistate/defistate/engine"
	"github.com/ethereum/go-ethereum/rpc"
)

// Constants for reconnection logic
const (
	initialReconnectDelay = 1 * time.Second
	maxReconnectDelay     = 30 * time.Second

	// RpcNamespace is the namespace under which the streamer is registered.
	RpcNamespace                  = "defi"
	StateStreamSubscriptionMethod = "subscribeStateStream"
)

// Logger defines a standard interface for structured, leveled logging.
type Logger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
}

// StatePatcherFunc defines the function signature for a method that safely applies
// a diff to a previous state.
type StatePatcherFunc func(prevState *engine.State, diff *differ.StateDiff) (newState *engine.State, err error)

type DecoderFunc func(schema engine.ProtocolSchema, data json.RawMessage) (any, error)

// Config holds the configuration for the client.
type Config struct {
	URL              string
	Logger           Logger
	BufferSize       uint
	StatePatcher     StatePatcherFunc
	StateDecoder     DecoderFunc
	StateDiffDecoder DecoderFunc
}

// validate checks if the configuration is valid.
func (c *Config) validate() error {
	if c.URL == "" {
		return errors.New("config: URL is required")
	}
	if c.BufferSize < 1 {
		return errors.New("config: BufferSize must be greater than 0")
	}
	if c.Logger == nil {
		return errors.New("config: Logger is required")
	}
	if c.StatePatcher == nil {
		return errors.New("config: StatePatcher is required")
	}
	if c.StateDecoder == nil {
		return errors.New("config: StateDecoder is required")
	}
	if c.StateDiffDecoder == nil {
		return errors.New("config: StateDiffDecoder is required")
	}
	return nil
}

// SubscriptionEvent is the wrapper object received from the server.
type SubscriptionEvent struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
	SentAt  int64           `json:"sentAt"`
}

// -----------------------------------------------------------------------------
// StreamProcessor
// -----------------------------------------------------------------------------

// StreamProcessor handles the business logic of parsing events, maintaining
// the latest state, applying diffs, and broadcasting updates.
// It is decoupled from the networking layer.
type StreamProcessor struct {
	lastState        *engine.State
	statePatcher     StatePatcherFunc
	stateDecoder     DecoderFunc
	stateDiffDecoder DecoderFunc
	stateCh          chan *engine.State
	logger           Logger
}

// NewStreamProcessor creates a pure logic processor without networking.
func NewStreamProcessor(
	logger Logger,
	bufferSize uint,
	statePatcher StatePatcherFunc,
	stateDecoder DecoderFunc,
	stateDiffDecoder DecoderFunc,
) *StreamProcessor {
	return &StreamProcessor{
		logger:           logger,
		stateCh:          make(chan *engine.State, bufferSize),
		statePatcher:     statePatcher,
		stateDecoder:     stateDecoder,
		stateDiffDecoder: stateDiffDecoder,
	}
}

// State returns a read-only channel for receiving new states.
func (sp *StreamProcessor) State() <-chan *engine.State {
	return sp.stateCh
}

// ProcessMessage accepts a raw JSON message (from WS, File, or JS), processes it,
// and updates the internal state.
func (sp *StreamProcessor) ProcessMessage(rawData json.RawMessage) error {
	processingStart := time.Now()
	var event SubscriptionEvent

	if err := json.Unmarshal(rawData, &event); err != nil {
		return fmt.Errorf("failed to unmarshal subscription event: %w", err)
	}

	switch event.Type {
	case "full":
		return sp.handleFullState(event, processingStart)
	case "diff":
		return sp.handleDiff(event, processingStart)
	default:
		return fmt.Errorf("Received unknown event type: %s", event.Type)
	}
}

func (sp *StreamProcessor) handleFullState(event SubscriptionEvent, start time.Time) error {
	var cState clientState
	if err := json.Unmarshal(event.Payload, &cState); err != nil {
		return fmt.Errorf("failed to unmarshal full state payload: %w", err)
	}

	// init state
	state := engine.State{
		ChainID:   cState.ChainID,
		Timestamp: cState.Timestamp,
		Block:     cState.Block,
		Protocols: map[engine.ProtocolID]engine.ProtocolState{},
	}

	for pID, protocolState := range cState.Protocols {
		// @note - crucial error handling!
		// it makes sense to warn and skip processing block if any protocol has an error
		// rather than allow for inconsitent states downstream
		if protocolState.Error != "" {
			return fmt.Errorf("protocol %s errored for block %d: %s", pID, cState.Block.Number.Uint64(), protocolState.Error)
		}
		typedData, err := sp.stateDecoder(protocolState.Schema, protocolState.Data)
		if err != nil {
			return fmt.Errorf("failed to decode state for protocol %s: %w", pID, err)
		}

		state.Protocols[pID] = engine.ProtocolState{
			Meta:              protocolState.Meta,
			SyncedBlockNumber: protocolState.SyncedBlockNumber,
			Schema:            protocolState.Schema,
			Data:              typedData,
			Error:             protocolState.Error,
		}
	}

	processingDur := time.Since(start)
	sp.logMetrics(&state, processingDur, event.SentAt, "full")

	sp.storeState(&state)
	sp.stateCh <- &state
	return nil
}

func (sp *StreamProcessor) handleDiff(event SubscriptionEvent, start time.Time) error {
	var cDiff clientStateDiff
	if err := json.Unmarshal(event.Payload, &cDiff); err != nil {
		return fmt.Errorf("failed to unmarshal diff payload: %w", err)
	}

	if sp.lastState == nil {
		return fmt.Errorf("received diff before full state; from_block: %d, to_block: %d", cDiff.FromBlock, cDiff.ToBlock.Number)
	}

	diff := differ.StateDiff{
		FromBlock: cDiff.FromBlock,
		ToBlock:   cDiff.ToBlock,
		Timestamp: cDiff.Timestamp,
		Protocols: make(map[engine.ProtocolID]differ.ProtocolDiff),
	}

	for pID, protocolDiff := range cDiff.Protocols {
		// @note - crucial error handling!
		// it makes sense to warn and skip processing block if any protocol has an error
		// rather than allow for inconsitent states downstream
		if protocolDiff.Error != "" {
			return fmt.Errorf("protocol %s errored for block %d: %s", pID, cDiff.ToBlock.Number.Uint64(), protocolDiff.Error)
		}

		typedData, err := sp.stateDiffDecoder(protocolDiff.Schema, protocolDiff.Data)
		if err != nil {
			return fmt.Errorf("failed to decode diff data for protocol %s: %w", pID, err)
		}

		diff.Protocols[pID] = differ.ProtocolDiff{
			Meta:              protocolDiff.Meta,
			SyncedBlockNumber: protocolDiff.SyncedBlockNumber,
			Schema:            protocolDiff.Schema,
			Data:              typedData,
			Error:             protocolDiff.Error,
		}
	}

	lastBlockNum := sp.lastState.Block.Number.Uint64()
	if diff.FromBlock != lastBlockNum {
		sp.logger.Warn(
			"Received out-of-order diff; state may be out of sync. Discarding.",
			"last_known_block", lastBlockNum,
			"diff_from_block", diff.FromBlock,
			"diff_to_block", diff.ToBlock.Number,
		)
		return nil // Non-fatal, just ignored
	}

	newState, err := sp.statePatcher(sp.lastState, &diff)
	if err != nil {
		return fmt.Errorf("failed to patch state: %w", err)
	}

	newState.Timestamp = diff.Timestamp

	processingDur := time.Since(start)
	sp.logMetrics(newState, processingDur, event.SentAt, "diff")

	sp.storeState(newState)
	sp.stateCh <- newState
	return nil
}

func (sp *StreamProcessor) storeState(state *engine.State) {
	sp.lastState = state
}

func (sp *StreamProcessor) logMetrics(state *engine.State, processingDur time.Duration, sentAt int64, stateType string) {
	if state == nil {
		return
	}

	clientFinishTime := time.Now()
	blockTimestamp := time.Unix(int64(state.Block.Timestamp), 0)
	clientStartTime := clientFinishTime.Add(-processingDur)
	serverFinishTime := time.Unix(0, sentAt)

	transportTime := clientStartTime.Sub(serverFinishTime)
	totalLatency := clientFinishTime.Sub(blockTimestamp)
	serverProcessingMs := serverFinishTime.Sub(time.Unix(0, state.Block.ReceivedAt)).Milliseconds()

	errorCount := 0
	for _, p := range state.Protocols {
		if p.Error != "" {
			errorCount++
		}
	}

	sp.logger.Debug("State Processed",
		"block", state.Block.Number,
		"type", stateType,
		"protocols", len(state.Protocols),
		"errors", errorCount,
		"latency_total_ms", totalLatency.Milliseconds(),
		"latency_transport_ms", transportTime.Milliseconds(),
		"latency_proc_ms", processingDur.Milliseconds(),
		"latency_server_ms", serverProcessingMs,
	)
}

// -----------------------------------------------------------------------------
// Client (Networking Wrapper)
// -----------------------------------------------------------------------------

// Client manages the connection and uses StreamProcessor for logic.
type Client struct {
	processor *StreamProcessor
	errCh     chan error
	logger    Logger
}

// NewClient creates a new client with networking enabled.
func NewClient(ctx context.Context, cfg Config) (*Client, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	processor := NewStreamProcessor(
		cfg.Logger,
		cfg.BufferSize,
		cfg.StatePatcher,
		cfg.StateDecoder,
		cfg.StateDiffDecoder,
	)

	client := &Client{
		processor: processor,
		errCh:     make(chan error, 1),
		logger:    cfg.Logger,
	}

	go client.run(ctx, cfg.URL)
	return client, nil
}

// State delegates to the processor's state channel.
func (c *Client) State() <-chan *engine.State {
	return c.processor.State()
}

// Err returns a read-only channel for receiving fatal (unrecoverable) errors.
func (c *Client) Err() <-chan error {
	return c.errCh
}

// run handles the networking lifecycle and feeds data to the processor.
func (c *Client) run(ctx context.Context, url string) {
	// Note: We do NOT close c.processor.stateCh here because the processor owns it,
	// but the client owns the lifecycle. Ideally we close it when we strictly stop run.
	defer close(c.errCh)
	reconnectDelay := initialReconnectDelay

	for {
		if ctx.Err() != nil {
			c.logger.Info("Client context canceled, shutting down.")
			return
		}

		c.logger.Info("Attempting to connect to RPC server", "url", url)
		rpcClient, err := rpc.DialContext(ctx, url)
		if err != nil {
			c.logger.Error("Failed to connect to RPC server, will retry...", "error", err, "delay", reconnectDelay)
			time.Sleep(reconnectDelay)
			reconnectDelay = min(reconnectDelay*2, maxReconnectDelay)
			continue
		}

		c.logger.Info("successfully connected to RPC server.")
		reconnectDelay = initialReconnectDelay

		err = c.subscribeAndProcess(ctx, rpcClient)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				c.logger.Info("Context canceled, shutting down.")
				return
			}
			c.logger.Error("Subscription failed, will reconnect...", "error", err, "delay", reconnectDelay)
			time.Sleep(reconnectDelay)
			reconnectDelay = min(reconnectDelay*2, maxReconnectDelay)
		}
	}
}

func (c *Client) subscribeAndProcess(ctx context.Context, rpcClient *rpc.Client) error {
	defer rpcClient.Close()

	rawCh := make(chan json.RawMessage)
	sub, err := rpcClient.Subscribe(ctx, RpcNamespace, rawCh, StateStreamSubscriptionMethod)
	if err != nil {
		return fmt.Errorf("failed to subscribe: %w", err)
	}
	defer sub.Unsubscribe()

	c.logger.Info("successfully subscribed. Waiting for data...")
	for {
		select {
		case rawData := <-rawCh:
			// Delegate logic to the processor
			if err := c.processor.ProcessMessage(rawData); err != nil {
				c.logger.Error("Error processing message", "error", err)
			}
		case err := <-sub.Err():
			return err
		case <-ctx.Done():
			c.logger.Info("Context cancelled, stopping subscription.")
			return ctx.Err()
		}
	}
}

func min(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
