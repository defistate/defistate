package differ

import "github.com/defistate/defistate/engine"

// Logger defines a standard interface for structured, leveled logging.
type Logger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
}

type ProtocolDiff struct {
	Meta engine.ProtocolMeta `json:"meta"`

	// what is the current block of the protocol's data?
	SyncedBlockNumber *uint64 `json:"syncedBlockNumber,omitempty"`

	// Schema is the decode contract for Data.
	// Examples:
	// "defistate/uniswapv2/poolView@v1"
	// "defistate/uniswapv3/poolView@v1"
	// "defistate/curve/poolView@v1"
	Schema engine.ProtocolSchema `json:"schema"`

	// Data is the protocol diff, shaped by Schema.
	Data any `json:"data,omitempty"`

	// Error is populated if this protocol is out-of-sync or failed for this block.
	Error string `json:"error,omitempty"`
}

// --
// engineDiffView represents a summary of changes FromBlock to ToBlock.
type StateDiff struct {
	Timestamp uint64                             `json:"timestamp"`
	FromBlock uint64                             `json:"fromBlock"`
	ToBlock   engine.BlockSummary                `json:"toBlock"`
	Protocols map[engine.ProtocolID]ProtocolDiff `json:"protocols"`
}
