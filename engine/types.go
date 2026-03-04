package engine

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

type ProtocolName string
type ProtocolID string

// ProtocolSchem defines the decode contract for a protocol's data
type ProtocolSchema string

type ProtocolMeta struct {
	Name ProtocolName `json:"name"`           // human label
	Tags []string     `json:"tags,omitempty"` // "dex", "fork", etc.
}

type ProtocolState struct {
	Meta ProtocolMeta `json:"meta"`

	// what is the current block of the protocol's data?
	SyncedBlockNumber *uint64 `json:"syncedBlockNumber,omitempty"`

	// Schema is the decode contract for Data.
	// Example:
	// "defistate/uniswap-v2-system/PoolView@v1"
	Schema ProtocolSchema `json:"schema"`

	// Data is the protocol view, shaped by Schema.
	Data any `json:"data,omitempty"`

	// Error is populated if this protocol is out-of-sync or failed for this block.
	Error string `json:"error,omitempty"`
}

type Protocol interface {
	View() (data any, schema ProtocolSchema, err error)

	// Meta returns stable identity metadata for this protocol instance.
	Meta() ProtocolMeta

	// Schema returns the protocol's schema for data decoding
	// think of the schema as the protocol's data identity
	Schema() ProtocolSchema
}

type BlockSynchronizedProtocol interface {
	Protocol
	// LastUpdatedAtBlock returns the highest block number for which the
	// protocol's internal state is fully synchronized.
	LastUpdatedAtBlock() uint64
}

// Logger defines a standard interface for structured, leveled logging,
// compatible with the standard library's slog.
type Logger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
}

func (state *State) HasErrors() bool {
	// Check protocol-level errors
	for _, pr := range state.Protocols {
		if pr.Error != "" {
			return true
		}
	}
	return false
}

// BlockSummary contains only the essential block information for clients.
type BlockSummary struct {
	Number      *big.Int    `json:"number"`
	Hash        common.Hash `json:"hash"`
	Timestamp   uint64      `json:"timestamp"`
	ReceivedAt  int64       `json:"receivedAt"` // The Unix nanosecond timestamp when the engine started processing the block.
	GasUsed     uint64      `json:"gasUsed"`
	GasLimit    uint64      `json:"gasLimit"`
	StateRoot   common.Hash `json:"stateRoot"`
	TxHash      common.Hash `json:"txHash"`
	ReceiptHash common.Hash `json:"receiptHash"`
}

// State is the main data structure broadcast to subscribers.
type State struct {
	ChainID   uint64                       `json:"chainId"`
	Timestamp uint64                       `json:"timestamp"`
	Block     BlockSummary                 `json:"block"`
	Protocols map[ProtocolID]ProtocolState `json:"protocols"`
}

// Subscription represents a subscription to the engine's event stream.
type Subscription struct {
	// done is the internal channel that will be closed when the engine shuts down.
	done <-chan struct{}

	c <-chan *State
}

// C provides a public getter for the engineView c chan
// getter for the event channel and bridging its concrete type.
func (s *Subscription) C() <-chan *State {
	return s.c
}

// Done provides a public getter for the shutdown signal channel.
func (s *Subscription) Done() <-chan struct{} {
	return s.done
}

func NewSubscription(c <-chan *State, done <-chan struct{}) *Subscription {
	return &Subscription{
		c:    c,
		done: done,
	}
}
