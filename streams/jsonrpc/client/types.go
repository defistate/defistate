package client

import (
	"encoding/json"

	"github.com/defistate/defistate/engine"
)

// clientState mirrors engine.State but strictly types the Data field as RawMessage.
// This prevents the Go JSON decoder from unmarshaling into map[string]interface{}.
type clientState struct {
	ChainID   uint64                                    `json:"chainId"`
	Timestamp uint64                                    `json:"timestamp"`
	Block     engine.BlockSummary                       `json:"block"`
	Protocols map[engine.ProtocolID]clientProtocolState `json:"protocols"`
}

type clientProtocolState struct {
	Meta              engine.ProtocolMeta   `json:"meta"`
	SyncedBlockNumber *uint64               `json:"syncedBlockNumber,omitempty"`
	Schema            engine.ProtocolSchema `json:"schema"`
	Error             string                `json:"error,omitempty"`

	// Data is kept as raw bytes. We decode this later using the specific Schema.
	Data json.RawMessage `json:"data,omitempty"`
}

type clientProtocolStateDiff struct {
	Meta              engine.ProtocolMeta   `json:"meta"`
	SyncedBlockNumber *uint64               `json:"syncedBlockNumber,omitempty"`
	Schema            engine.ProtocolSchema `json:"schema"`
	Error             string                `json:"error,omitempty"`

	// Data is kept as raw bytes. We decode this later using the specific Schema.
	Data json.RawMessage `json:"data,omitempty"`
}

// clientProtocolStateDiff mirrors differ.StateDiff but keeps the protocol diffs as raw bytes.
type clientStateDiff struct {
	FromBlock uint64                                        `json:"fromBlock"`
	ToBlock   engine.BlockSummary                           `json:"toBlock"`
	Timestamp uint64                                        `json:"timestamp"`
	Protocols map[engine.ProtocolID]clientProtocolStateDiff `json:"protocols"`
}
