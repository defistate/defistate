package ticks

import "fmt"

// BaseError is a base type for errors originating from the TickIndexer.
type BaseError struct {
	Err error
}

func (e *BaseError) Unwrap() error { return e.Err }
func (e *BaseError) Error() string { return e.Err.Error() }

// InitializationError indicates a failure during the initialization of new pools.
type InitializationError struct {
	BaseError
}

// Error provides a clear prefix for errors from the background initializer.
func (e *InitializationError) Error() string {
	return fmt.Sprintf("initializer: %v", e.Err)
}

// BlockProcessingError indicates a failure during the processing of a new block.
type BlockProcessingError struct {
	BaseError
	BlockNumber uint64
}

func (e *BlockProcessingError) Error() string {
	return fmt.Sprintf("block %d: %v", e.BlockNumber, e.Err)
}

// UpdateDroppedError indicates a real-time update was dropped due to a full queue.
type UpdateDroppedError struct {
	BlockProcessingError
}

// ResyncError indicates a failure during the state reconciliation process.
type ResyncError struct {
	BaseError
}

// Error provides a clear prefix for errors from the background resyncer.
func (e *ResyncError) Error() string {
	return fmt.Sprintf("resyncer: %v", e.Err)
}
