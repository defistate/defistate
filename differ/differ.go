package differ

import (
	"errors"
	"fmt"
	"time"

	"github.com/defistate/defistate/engine"
	"github.com/prometheus/client_golang/prometheus"
)

// --- Config and Main Struct ---
type ProtocolDiffer func(old, new any) (diff any, err error)

// StateDifferConfig holds all the individual differ functions and dependencies.
type StateDifferConfig struct {
	// One differ per schema (data contract), not per protocol identity.
	ProtocolDiffers map[engine.ProtocolSchema]ProtocolDiffer
	Registry        prometheus.Registerer // Now required for metrics.
	Logger          Logger                // Now required for logging.
}

// validate checks if the configuration is valid, ensuring required dependencies are present.
func (c *StateDifferConfig) validate() error {
	if c.Registry == nil {
		return errors.New("config: Registry cannot be nil")
	}
	if c.Logger == nil {
		return errors.New("config: Logger cannot be nil")
	}
	return nil
}

// StateDiffer is the main differ engine, now with metrics and logging.
type StateDiffer struct {
	metrics         *Metrics
	logger          Logger
	protocolDiffers map[engine.ProtocolSchema]ProtocolDiffer
}

// NewStateDiffer constructs a new differ from a configuration, returning an error if the config is invalid.
func NewStateDiffer(cfg *StateDifferConfig) (*StateDiffer, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	protocolDiffers := make(map[engine.ProtocolSchema]ProtocolDiffer, len(cfg.ProtocolDiffers))
	for protocolID, protocolDiffer := range cfg.ProtocolDiffers {
		protocolDiffers[protocolID] = protocolDiffer
	}

	return &StateDiffer{
		metrics:         NewMetrics(cfg.Registry),
		logger:          cfg.Logger,
		protocolDiffers: protocolDiffers,
	}, nil
}

// Diff is the main orchestrator method. It now operates under the guarantee that
// it will only receive valid, error-free views to compare.
func (d *StateDiffer) Diff(old, new *engine.State) (*StateDiff, error) {
	totalTimer := prometheus.NewTimer(d.metrics.diffDuration.WithLabelValues())
	defer totalTimer.ObserveDuration()

	// we still ensure old and new views have no errors
	if old.HasErrors() || new.HasErrors() {
		return nil, errors.New("StateDiffer received view with error!")
	}

	protocolDiffs := make(map[engine.ProtocolID]ProtocolDiff)
	for protocolID, newProtocolState := range new.Protocols {
		oldProtocolState, ok := old.Protocols[protocolID]
		if !ok {
			return nil, fmt.Errorf("protocolID %s does not exist in old state", protocolID)
		}

		differFunc, exists := d.protocolDiffers[newProtocolState.Schema]
		if !exists {
			return nil, fmt.Errorf("no differ registered for schema %q", newProtocolState.Schema)
		}
		diffData, err := differFunc(oldProtocolState.Data, newProtocolState.Data)
		if err != nil {
			return nil, err
		}

		diff := ProtocolDiff{
			Meta:              newProtocolState.Meta,
			SyncedBlockNumber: newProtocolState.SyncedBlockNumber,
			Schema:            newProtocolState.Schema,
			Data:              diffData,
		}

		protocolDiffs[protocolID] = diff
	}

	stateDiff := &StateDiff{
		Timestamp: uint64(time.Now().UnixNano()),
		FromBlock: old.Block.Number.Uint64(),
		ToBlock:   new.Block,
		Protocols: protocolDiffs,
	}

	return stateDiff, nil
}
