package uniswapv3

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

// SystemError is a base type for errors originating from the UniswapV3System,
// typically associated with a specific block.
type SystemError struct {
	BlockNumber uint64
	Err         error
}

// Error provides a descriptive message, omitting the block number if it's not relevant.
func (e *SystemError) Error() string {
	if e.BlockNumber > 0 {
		return fmt.Sprintf("block %d: %v", e.BlockNumber, e.Err)
	}
	// This will be called by wrapping errors when BlockNumber is 0.
	if e.Err != nil {
		return e.Err.Error()
	}
	return ""
}

func (e *SystemError) Unwrap() error {
	return e.Err
}

// InitializationError indicates a failure during the initialization of a new pool.
type InitializationError struct {
	SystemError
	PoolAddress common.Address
}

// Error provides a descriptive message, distinguishing between background and block-specific errors.
func (e *InitializationError) Error() string {
	if e.BlockNumber == 0 {
		return fmt.Sprintf("initializer: failed to initialize pool %s: %v", e.PoolAddress.Hex(), e.Err)
	}
	return fmt.Sprintf("block %d: failed to initialize pool %s: %v", e.BlockNumber, e.PoolAddress.Hex(), e.Err)
}

// RegistrationError is a critical error when the system fails to register
// a new, valid pool with its upstream dependencies.
type RegistrationError struct {
	InitializationError
	Token0Address common.Address
	Token1Address common.Address
}

// Error provides a descriptive message, distinguishing between background and block-specific errors.
func (e *RegistrationError) Error() string {
	if e.BlockNumber == 0 {
		return fmt.Sprintf("critical initializer: failed to register new pool %s (tokens %s, %s): %v", e.PoolAddress.Hex(), e.Token0Address.Hex(), e.Token1Address.Hex(), e.Err)
	}
	return fmt.Sprintf("critical block %d: failed to register new pool %s (tokens %s, %s): %v", e.BlockNumber, e.PoolAddress.Hex(), e.Token0Address.Hex(), e.Token1Address.Hex(), e.Err)
}

// DataConsistencyError indicates a critical internal state mismatch,
// for example, failing to find a pool ID immediately after it was registered.
type DataConsistencyError struct {
	SystemError
	PoolAddress common.Address
	Details     string
}

// Error provides a descriptive message, distinguishing between background and block-specific errors.
func (e *DataConsistencyError) Error() string {
	if e.BlockNumber == 0 {
		return fmt.Sprintf("critical internal: data consistency error for pool %s: %s: %v", e.PoolAddress.Hex(), e.Details, e.Err)
	}
	return fmt.Sprintf("critical block %d: data consistency error for pool %s: %s: %v", e.BlockNumber, e.PoolAddress.Hex(), e.Details, e.Err)
}

// UpdateError indicates a failure to update the state (tick, liquidity, etc.) of a known, existing pool.
type UpdateError struct {
	SystemError
	PoolAddress  common.Address
	PoolID       uint64
	Tick         int64
	Liquidity    *big.Int
	SqrtPriceX96 *big.Int
}

// Error provides a descriptive message, distinguishing between background and block-specific errors.
func (e *UpdateError) Error() string {
	if e.BlockNumber == 0 {
		return fmt.Sprintf("reconciler: failed to update state for pool %s (id %d): %v", e.PoolAddress.Hex(), e.PoolID, e.Err)
	}
	return fmt.Sprintf("block %d: failed to update state for pool %s (id %d): %v", e.BlockNumber, e.PoolAddress.Hex(), e.PoolID, e.Err)
}

// TickIndexingError indicates a failure when interacting with the TickIndexer,
// such as adding or removing a pool for tracking. This is often a critical failure.
type TickIndexingError struct {
	SystemError
	PoolAddress common.Address
	PoolID      uint64
	Operation   string // e.g., "Add", "Remove", "Get"
}

// Error provides a descriptive message, distinguishing between background and block-specific errors.
func (e *TickIndexingError) Error() string {
	if e.BlockNumber == 0 {
		return fmt.Sprintf("critical internal: TickIndexer failed operation '%s' for pool %s (id %d): %v", e.Operation, e.PoolAddress.Hex(), e.PoolID, e.Err)
	}
	return fmt.Sprintf("critical block %d: TickIndexer failed operation '%s' for pool %s (id %d): %v", e.BlockNumber, e.Operation, e.PoolAddress.Hex(), e.PoolID, e.Err)
}

// PrunerError indicates a failure during the periodic pruning process.
// It doesn't embed SystemError as pruning is not tied to a specific block.
type PrunerError struct {
	Err    error
	PoolID uint64
}

func (e *PrunerError) Error() string {
	return fmt.Sprintf("pruner: failed to process pool ID %d: %v", e.PoolID, e.Err)
}

func (e *PrunerError) Unwrap() error {
	return e.Err
}

// ReconciliationError indicates a failure during the self-healing state resync process.
type ReconciliationError struct {
	SystemError
	PoolAddress common.Address
}

func (e *ReconciliationError) Error() string {
	// This error is always from the reconciler, so its message is already clear.
	return fmt.Sprintf("reconciler: failed to get slot0 for pool %s: %v", e.PoolAddress.Hex(), e.Err)
}

// determineErrorType inspects an error to classify it for metrics and logging.
func determineErrorType(err error) string {
	var regErr *RegistrationError
	var consistencyErr *DataConsistencyError
	var tickErr *TickIndexingError
	var initErr *InitializationError
	var updateErr *UpdateError
	var reconErr *ReconciliationError
	var prunerErr *PrunerError
	var sysErr *SystemError

	// Check from most specific to most general to ensure correct classification.
	if errors.As(err, &regErr) {
		return "critical_registration"
	} else if errors.As(err, &consistencyErr) {
		return "critical_consistency"
	} else if errors.As(err, &tickErr) {
		return "critical_tick_indexing"
	} else if errors.As(err, &initErr) {
		return "pool_initialization"
	} else if errors.As(err, &updateErr) {
		return "pool_update"
	} else if errors.As(err, &reconErr) {
		return "reconciliation"
	} else if errors.As(err, &prunerErr) {
		return "pruner"
	} else if errors.As(err, &sysErr) {
		return "system_internal"
	}

	return "unknown"
}
