package block

import (
	"context"
	"errors"

	"github.com/ethereum/go-ethereum/core/types"
)

// Logger defines a standard interface for structured, leveled logging.
type Logger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
}

// FanOutBlockConfig holds the configuration for the FanOutBlock component.
type FanOutBlockConfig struct {
	// Input is the source channel to read blocks from. Required.
	Input <-chan *types.Block
	// Outputs is a slice of channels to which blocks will be sent. Required.
	Outputs []chan *types.Block
	// OutputTags is a slice of strings corresponding to each output channel,
	// used for logging purposes. Must be the same length as Outputs. Required.
	OutputTags []string
	// Logger is a structured logger for logging component activity. Required.
	Logger Logger
}

// Validate checks that the configuration is valid and has all required fields.
func (c *FanOutBlockConfig) Validate() error {
	if c.Input == nil {
		return errors.New("config: input channel cannot be nil")
	}
	if len(c.Outputs) == 0 {
		return errors.New("config: must have at least one output channel")
	}
	if len(c.Outputs) != len(c.OutputTags) {
		return errors.New("config: outputs and outputTags must have the same length")
	}
	if c.Logger == nil {
		return errors.New("config: logger cannot be nil")
	}
	return nil
}

// FanOutBlock validates the config and starts a background goroutine to fan out
// blocks from the input channel to all output channels.
//
// It returns an error immediately if the configuration is invalid. Otherwise, it
// returns nil and begins operating in the background. The lifecycle of the
// goroutine is managed by the provided context.
func FanOutBlock(ctx context.Context, cfg FanOutBlockConfig) error {
	// First, validate the configuration.
	if err := cfg.Validate(); err != nil {
		return err
	}

	// Start the fan-out goroutine.
	go func() {
		// When this goroutine exits, ensure all output channels are closed.
		defer func() {
			for _, ch := range cfg.Outputs {
				close(ch)
			}
		}()

		for {
			select {
			case <-ctx.Done():
				cfg.Logger.Info("Fan-out goroutine: context canceled, shutting down.")
				return

			case block, ok := <-cfg.Input:
				if !ok {
					cfg.Logger.Info("Fan-out goroutine: input channel closed, shutting down.")
					return
				}

				for i, ch := range cfg.Outputs {
					tag := cfg.OutputTags[i]
					select {
					case ch <- block:
						// Block was sent successfully.
					case <-ctx.Done():
						cfg.Logger.Info("Fan-out goroutine: context canceled during send, shutting down.")
						return
					default:
						cfg.Logger.Warn("output channel blocked, skipping send", "tag", tag)
					}
				}
			}
		}
	}()

	return nil
}
