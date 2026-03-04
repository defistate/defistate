package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/defistate/defistate/differ"
	"github.com/defistate/defistate/engine"
	"github.com/ethereum/go-ethereum/rpc"
)

const (
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

// StateEngine defines the interface required by the StateStreamer.
type StateEngine interface {
	Subscribe() (sub *engine.Subscription, unsubscribe func())
}

// StateDiffer defines the interface for the differ component.
type StateDiffer interface {
	Diff(prev, new *engine.State) (*differ.StateDiff, error)
}

// Config holds the dependencies and configuration for the StateStreamer.
type Config struct {
	Engine            StateEngine
	Differ            StateDiffer
	Logger            Logger
	FullStateInterval uint64
}

// validate checks if the configuration is valid.
func (c *Config) validate() error {
	if c.Engine == nil {
		return errors.New("config: Engine cannot be nil")
	}
	if c.Differ == nil {
		return errors.New("config: Differ cannot be nil")
	}
	if c.Logger == nil {
		return errors.New("config: Logger cannot be nil")
	}
	if c.FullStateInterval < 2 {
		return errors.New("config: FullStateInterval must be 2 or greater to enable diffing")
	}
	return nil
}

// StateStreamer acts as the RPC service layer.
type StateStreamer struct {
	engine            StateEngine
	differ            StateDiffer
	logger            Logger
	fullStateInterval uint64
}

// NewStateStreamer creates a new instance of the streamer layer from a config.
func NewStateStreamer(cfg *Config) (*StateStreamer, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return &StateStreamer{
		engine:            cfg.Engine,
		differ:            cfg.Differ,
		logger:            cfg.Logger,
		fullStateInterval: cfg.FullStateInterval,
	}, nil
}

// SubscriptionEvent is the wrapper object sent to clients.
type SubscriptionEvent struct {
	Type    string `json:"type"`
	Payload any    `json:"payload"`
	// NEW: Add a timestamp for when the event was sent by the server.
	// We use int64 (Unix nanoseconds) for precision and to avoid JSON time formatting issues.
	SentAt int64 `json:"sentAt"`
}

const (
	EventTypeFull = "full"
	EventTypeDiff = "diff"
)

// SubscribeStateStream handles the subscription request with the new, more resilient diffing logic.
func (streamer *StateStreamer) SubscribeStateStream(ctx context.Context) (*rpc.Subscription, error) {
	notifier, supported := rpc.NotifierFromContext(ctx)
	if !supported {
		return nil, rpc.ErrNotificationsUnsupported
	}

	rpcSub := notifier.CreateSubscription()

	go func() {
		engineSub, unsubscribe := streamer.engine.Subscribe()
		defer unsubscribe()

		// --- State for this specific subscriber ---
		var prevState *engine.State
		var blocksSinceFullState uint64 = streamer.fullStateInterval
		var previousStateHadError bool // Track the error state of the last processed state.

		for {
			select {
			case state, ok := <-engineSub.C():
				if !ok {
					return
				}

				var event *SubscriptionEvent
				currentStateHasError := state.HasErrors()

				// --- Decision Logic ---
				// Send a full state if:
				// 1. It's the first state for this subscriber.
				// 2. There was a gap in state
				// 3. The periodic interval has been reached.
				// 4. The CURRENT state contains a subsystem error.
				// 5. The PREVIOUS state had an error (to force a resync on the next good state).
				if prevState == nil || (prevState.Block.Number.Uint64()+uint64(1)) != state.Block.Number.Uint64() || blocksSinceFullState >= streamer.fullStateInterval || currentStateHasError || previousStateHadError {
					event = &SubscriptionEvent{
						Type:    EventTypeFull,
						Payload: state,
						SentAt:  time.Now().UnixNano(),
					}

					blocksSinceFullState = 1 // Reset counter
				} else {
					// Otherwise, if the state is healthy and sequential, calculate and send a diff.
					diff, err := streamer.differ.Diff(prevState, state)
					if err != nil {
						// Log the error so we know why the diff failed (e.g. schema mismatch)
						streamer.logger.Error("Failed to calculate state diff",
							"error", err,
							"from_block", prevState.Block.Number,
							"to_block", state.Block.Number,
						)
						// Skip sending this event.
						// Note: We do NOT update 'prevState' below, so the next block will try
						// to diff against the *same* 'prevState' again.
						continue
					}
					event = &SubscriptionEvent{
						Type:    EventTypeDiff,
						Payload: diff,
						SentAt:  time.Now().UnixNano(),
					}
					// We only need the last SENT full state to calculate the next diff.
					blocksSinceFullState++
				}

				// After processing, update the error state for the next iteration.
				previousStateHadError = currentStateHasError
				// update lastFullState
				prevState = state
				_ = notifier.Notify(rpcSub.ID, event)

			case <-rpcSub.Err():
				return
			case <-engineSub.Done():
				return
			}
		}
	}()

	return rpcSub, nil
}

// StartStreamer creates and starts the WebSocket RPC server.
// It now correctly takes the fully constructed StateStreamer instance.
func StartStreamer(ctx context.Context, streamer *StateStreamer, port int, origins []string) (<-chan error, error) {
	server := rpc.NewServer()

	if err := server.RegisterName(RpcNamespace, streamer); err != nil {
		return nil, fmt.Errorf("failed to register streamer: %v", err)
	}

	wsHandler := server.WebsocketHandler(origins)
	addr := fmt.Sprintf(":%d", port)
	httpServer := &http.Server{
		Addr:    addr,
		Handler: wsHandler,
	}

	errChan := make(chan error, 1)

	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- err
		}
	}()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
	}()

	streamer.logger.Info("json-rpc streamer started successfully", "addr", addr)
	return errChan, nil
}
