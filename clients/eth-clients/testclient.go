package ethclients

import (
	"context"
	"errors"
	"math/big"
	"sync"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
)

// TestETHClient provides a mock implementation of the ETHClient interface.
// It defaults to returning nil/zero values but allows registering custom handler
// functions for specific methods using Set...Handler methods.
type TestETHClient struct {
	mu sync.RWMutex // Protect concurrent access to handlers

	// --- Handler Fields ---
	// Define a function field for each method in the ETHClient interface.
	// These fields will store the custom handler functions provided by tests.

	// CloseFunc func() // Close doesn't return, simple tracking might be better if needed
	ClientFunc func() *rpc.Client

	// Blockchain Access Handlers
	ChainIDFunc            func(ctx context.Context) (*big.Int, error)
	BlockByHashFunc        func(ctx context.Context, hash common.Hash) (*types.Block, error)
	BlockByNumberFunc      func(ctx context.Context, number *big.Int) (*types.Block, error)
	BlockNumberFunc        func(ctx context.Context) (uint64, error)
	PeerCountFunc          func(ctx context.Context) (uint64, error)
	BlockReceiptsFunc      func(ctx context.Context, blockNrOrHash rpc.BlockNumberOrHash) ([]*types.Receipt, error)
	HeaderByHashFunc       func(ctx context.Context, hash common.Hash) (*types.Header, error)
	HeaderByNumberFunc     func(ctx context.Context, number *big.Int) (*types.Header, error)
	TransactionByHashFunc  func(ctx context.Context, hash common.Hash) (tx *types.Transaction, isPending bool, err error)
	TransactionSenderFunc  func(ctx context.Context, tx *types.Transaction, block common.Hash, index uint) (common.Address, error)
	TransactionCountFunc   func(ctx context.Context, blockHash common.Hash) (uint, error)
	TransactionInBlockFunc func(ctx context.Context, blockHash common.Hash, index uint) (*types.Transaction, error)
	TransactionReceiptFunc func(ctx context.Context, txHash common.Hash) (*types.Receipt, error)
	SyncProgressFunc       func(ctx context.Context) (*ethereum.SyncProgress, error)
	SubscribeNewHeadFunc   func(ctx context.Context, ch chan<- *types.Header) (ethereum.Subscription, error)

	// State Access Handlers
	NetworkIDFunc     func(ctx context.Context) (*big.Int, error)
	BalanceAtFunc     func(ctx context.Context, account common.Address, blockNumber *big.Int) (*big.Int, error)
	BalanceAtHashFunc func(ctx context.Context, account common.Address, blockHash common.Hash) (*big.Int, error)
	StorageAtFunc     func(ctx context.Context, account common.Address, key common.Hash, blockNumber *big.Int) ([]byte, error)
	StorageAtHashFunc func(ctx context.Context, account common.Address, key common.Hash, blockHash common.Hash) ([]byte, error)
	CodeAtFunc        func(ctx context.Context, account common.Address, blockNumber *big.Int) ([]byte, error)
	CodeAtHashFunc    func(ctx context.Context, account common.Address, blockHash common.Hash) ([]byte, error)
	NonceAtFunc       func(ctx context.Context, account common.Address, blockNumber *big.Int) (uint64, error)
	NonceAtHashFunc   func(ctx context.Context, account common.Address, blockHash common.Hash) (uint64, error)

	// Filters Handlers
	FilterLogsFunc          func(ctx context.Context, q ethereum.FilterQuery) ([]types.Log, error)
	SubscribeFilterLogsFunc func(ctx context.Context, q ethereum.FilterQuery, ch chan<- types.Log) (ethereum.Subscription, error)

	// Pending State Handlers
	PendingBalanceAtFunc        func(ctx context.Context, account common.Address) (*big.Int, error)
	PendingStorageAtFunc        func(ctx context.Context, account common.Address, key common.Hash) ([]byte, error)
	PendingCodeAtFunc           func(ctx context.Context, account common.Address) ([]byte, error)
	PendingNonceAtFunc          func(ctx context.Context, account common.Address) (uint64, error)
	PendingTransactionCountFunc func(ctx context.Context) (uint, error)

	// Contract Calling Handlers
	CallContractFunc        func(ctx context.Context, msg ethereum.CallMsg, blockNumber *big.Int) ([]byte, error)
	CallContractAtHashFunc  func(ctx context.Context, msg ethereum.CallMsg, blockHash common.Hash) ([]byte, error)
	PendingCallContractFunc func(ctx context.Context, msg ethereum.CallMsg) ([]byte, error)
	SuggestGasPriceFunc     func(ctx context.Context) (*big.Int, error)
	SuggestGasTipCapFunc    func(ctx context.Context) (*big.Int, error)
	FeeHistoryFunc          func(ctx context.Context, blockCount uint64, lastBlock *big.Int, rewardPercentiles []float64) (*ethereum.FeeHistory, error)
	EstimateGasFunc         func(ctx context.Context, msg ethereum.CallMsg) (uint64, error)
	SendTransactionFunc     func(ctx context.Context, tx *types.Transaction) error

	// --- Internal state for methods without return values if needed ---
	closeCalled bool
}

// NewTestETHClient creates a new TestETHClient mock instance.
func NewTestETHClient() *TestETHClient {
	return &TestETHClient{}
}

// --- Handler Setter Methods ---
// Provide public methods to set the handler function for each interface method.

func (m *TestETHClient) SetClientHandler(f func() *rpc.Client) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ClientFunc = f
}

func (m *TestETHClient) SetChainIDHandler(f func(ctx context.Context) (*big.Int, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ChainIDFunc = f
}

func (m *TestETHClient) SetBlockByHashHandler(f func(ctx context.Context, hash common.Hash) (*types.Block, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.BlockByHashFunc = f
}

func (m *TestETHClient) SetBlockByNumberHandler(f func(ctx context.Context, number *big.Int) (*types.Block, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.BlockByNumberFunc = f
}

func (m *TestETHClient) SetBlockNumberHandler(f func(ctx context.Context) (uint64, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.BlockNumberFunc = f
}

func (m *TestETHClient) SetPeerCountHandler(f func(ctx context.Context) (uint64, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.PeerCountFunc = f
}

func (m *TestETHClient) SetBlockReceiptsHandler(f func(ctx context.Context, blockNrOrHash rpc.BlockNumberOrHash) ([]*types.Receipt, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.BlockReceiptsFunc = f
}

func (m *TestETHClient) SetHeaderByHashHandler(f func(ctx context.Context, hash common.Hash) (*types.Header, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.HeaderByHashFunc = f
}

func (m *TestETHClient) SetHeaderByNumberHandler(f func(ctx context.Context, number *big.Int) (*types.Header, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.HeaderByNumberFunc = f
}

func (m *TestETHClient) SetTransactionByHashHandler(f func(ctx context.Context, hash common.Hash) (tx *types.Transaction, isPending bool, err error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.TransactionByHashFunc = f
}

func (m *TestETHClient) SetTransactionSenderHandler(f func(ctx context.Context, tx *types.Transaction, block common.Hash, index uint) (common.Address, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.TransactionSenderFunc = f
}

func (m *TestETHClient) SetTransactionCountHandler(f func(ctx context.Context, blockHash common.Hash) (uint, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.TransactionCountFunc = f
}

func (m *TestETHClient) SetTransactionInBlockHandler(f func(ctx context.Context, blockHash common.Hash, index uint) (*types.Transaction, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.TransactionInBlockFunc = f
}

func (m *TestETHClient) SetTransactionReceiptHandler(f func(ctx context.Context, txHash common.Hash) (*types.Receipt, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.TransactionReceiptFunc = f
}

func (m *TestETHClient) SetSyncProgressHandler(f func(ctx context.Context) (*ethereum.SyncProgress, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.SyncProgressFunc = f
}

func (m *TestETHClient) SetSubscribeNewHeadHandler(f func(ctx context.Context, ch chan<- *types.Header) (ethereum.Subscription, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.SubscribeNewHeadFunc = f
}

func (m *TestETHClient) SetNetworkIDHandler(f func(ctx context.Context) (*big.Int, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.NetworkIDFunc = f
}

func (m *TestETHClient) SetBalanceAtHandler(f func(ctx context.Context, account common.Address, blockNumber *big.Int) (*big.Int, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.BalanceAtFunc = f
}

func (m *TestETHClient) SetBalanceAtHashHandler(f func(ctx context.Context, account common.Address, blockHash common.Hash) (*big.Int, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.BalanceAtHashFunc = f
}

func (m *TestETHClient) SetStorageAtHandler(f func(ctx context.Context, account common.Address, key common.Hash, blockNumber *big.Int) ([]byte, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.StorageAtFunc = f
}

func (m *TestETHClient) SetStorageAtHashHandler(f func(ctx context.Context, account common.Address, key common.Hash, blockHash common.Hash) ([]byte, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.StorageAtHashFunc = f
}

func (m *TestETHClient) SetCodeAtHandler(f func(ctx context.Context, account common.Address, blockNumber *big.Int) ([]byte, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.CodeAtFunc = f
}

func (m *TestETHClient) SetCodeAtHashHandler(f func(ctx context.Context, account common.Address, blockHash common.Hash) ([]byte, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.CodeAtHashFunc = f
}

func (m *TestETHClient) SetNonceAtHandler(f func(ctx context.Context, account common.Address, blockNumber *big.Int) (uint64, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.NonceAtFunc = f
}

func (m *TestETHClient) SetNonceAtHashHandler(f func(ctx context.Context, account common.Address, blockHash common.Hash) (uint64, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.NonceAtHashFunc = f
}

func (m *TestETHClient) SetFilterLogsHandler(f func(ctx context.Context, q ethereum.FilterQuery) ([]types.Log, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.FilterLogsFunc = f
}

func (m *TestETHClient) SetSubscribeFilterLogsHandler(f func(ctx context.Context, q ethereum.FilterQuery, ch chan<- types.Log) (ethereum.Subscription, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.SubscribeFilterLogsFunc = f
}

func (m *TestETHClient) SetPendingBalanceAtHandler(f func(ctx context.Context, account common.Address) (*big.Int, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.PendingBalanceAtFunc = f
}

func (m *TestETHClient) SetPendingStorageAtHandler(f func(ctx context.Context, account common.Address, key common.Hash) ([]byte, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.PendingStorageAtFunc = f
}

func (m *TestETHClient) SetPendingCodeAtHandler(f func(ctx context.Context, account common.Address) ([]byte, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.PendingCodeAtFunc = f
}

func (m *TestETHClient) SetPendingNonceAtHandler(f func(ctx context.Context, account common.Address) (uint64, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.PendingNonceAtFunc = f
}

func (m *TestETHClient) SetPendingTransactionCountHandler(f func(ctx context.Context) (uint, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.PendingTransactionCountFunc = f
}

func (m *TestETHClient) SetCallContractHandler(f func(ctx context.Context, msg ethereum.CallMsg, blockNumber *big.Int) ([]byte, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.CallContractFunc = f
}

func (m *TestETHClient) SetCallContractAtHashHandler(f func(ctx context.Context, msg ethereum.CallMsg, blockHash common.Hash) ([]byte, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.CallContractAtHashFunc = f
}

func (m *TestETHClient) SetPendingCallContractHandler(f func(ctx context.Context, msg ethereum.CallMsg) ([]byte, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.PendingCallContractFunc = f
}

func (m *TestETHClient) SetSuggestGasPriceHandler(f func(ctx context.Context) (*big.Int, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.SuggestGasPriceFunc = f
}

func (m *TestETHClient) SetSuggestGasTipCapHandler(f func(ctx context.Context) (*big.Int, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.SuggestGasTipCapFunc = f
}

func (m *TestETHClient) SetFeeHistoryHandler(f func(ctx context.Context, blockCount uint64, lastBlock *big.Int, rewardPercentiles []float64) (*ethereum.FeeHistory, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.FeeHistoryFunc = f
}

func (m *TestETHClient) SetEstimateGasHandler(f func(ctx context.Context, msg ethereum.CallMsg) (uint64, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.EstimateGasFunc = f
}

func (m *TestETHClient) SetSendTransactionHandler(f func(ctx context.Context, tx *types.Transaction) error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.SendTransactionFunc = f
}

// --- ETHClient Interface Method Implementations ---
// Each method now checks if a handler is registered and calls it,
// otherwise returns the default nil/zero value.

func (m *TestETHClient) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closeCalled = true
	// Note: CloseFunc isn't used here, as Close has no return.
	// Tests can check m.CloseCalled() if needed.
}

// CloseCalled returns true if Close() was invoked.
func (m *TestETHClient) CloseCalled() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.closeCalled
}

func (m *TestETHClient) Client() *rpc.Client {
	m.mu.RLock()
	handler := m.ClientFunc
	m.mu.RUnlock()
	if handler != nil {
		return handler()
	}
	return nil // Default
}

func (m *TestETHClient) ChainID(ctx context.Context) (*big.Int, error) {
	m.mu.RLock()
	handler := m.ChainIDFunc
	m.mu.RUnlock()
	if handler != nil {
		return handler(ctx)
	}
	return nil, nil // Default
}

func (m *TestETHClient) BlockByHash(ctx context.Context, hash common.Hash) (*types.Block, error) {
	m.mu.RLock()
	handler := m.BlockByHashFunc
	m.mu.RUnlock()
	if handler != nil {
		return handler(ctx, hash)
	}
	return nil, nil // Default
}

func (m *TestETHClient) BlockByNumber(ctx context.Context, number *big.Int) (*types.Block, error) {
	m.mu.RLock()
	handler := m.BlockByNumberFunc
	m.mu.RUnlock()
	if handler != nil {
		return handler(ctx, number)
	}
	return nil, nil // Default
}

func (m *TestETHClient) BlockNumber(ctx context.Context) (uint64, error) {
	m.mu.RLock()
	handler := m.BlockNumberFunc
	m.mu.RUnlock()
	if handler != nil {
		return handler(ctx)
	}
	return 0, nil // Default
}

func (m *TestETHClient) PeerCount(ctx context.Context) (uint64, error) {
	m.mu.RLock()
	handler := m.PeerCountFunc
	m.mu.RUnlock()
	if handler != nil {
		return handler(ctx)
	}
	return 0, nil // Default
}

func (m *TestETHClient) BlockReceipts(ctx context.Context, blockNrOrHash rpc.BlockNumberOrHash) ([]*types.Receipt, error) {
	m.mu.RLock()
	handler := m.BlockReceiptsFunc
	m.mu.RUnlock()
	if handler != nil {
		return handler(ctx, blockNrOrHash)
	}
	return nil, nil // Default
}

func (m *TestETHClient) HeaderByHash(ctx context.Context, hash common.Hash) (*types.Header, error) {
	m.mu.RLock()
	handler := m.HeaderByHashFunc
	m.mu.RUnlock()
	if handler != nil {
		return handler(ctx, hash)
	}
	return nil, nil // Default
}

func (m *TestETHClient) HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error) {
	m.mu.RLock()
	handler := m.HeaderByNumberFunc
	m.mu.RUnlock()
	if handler != nil {
		return handler(ctx, number)
	}
	return nil, nil // Default
}

func (m *TestETHClient) TransactionByHash(ctx context.Context, hash common.Hash) (tx *types.Transaction, isPending bool, err error) {
	m.mu.RLock()
	handler := m.TransactionByHashFunc
	m.mu.RUnlock()
	if handler != nil {
		return handler(ctx, hash)
	}
	return nil, false, nil // Default
}

func (m *TestETHClient) TransactionSender(ctx context.Context, tx *types.Transaction, block common.Hash, index uint) (common.Address, error) {
	m.mu.RLock()
	handler := m.TransactionSenderFunc
	m.mu.RUnlock()
	if handler != nil {
		return handler(ctx, tx, block, index)
	}
	return common.Address{}, nil // Default
}

func (m *TestETHClient) TransactionCount(ctx context.Context, blockHash common.Hash) (uint, error) {
	m.mu.RLock()
	handler := m.TransactionCountFunc
	m.mu.RUnlock()
	if handler != nil {
		return handler(ctx, blockHash)
	}
	return 0, nil // Default
}

func (m *TestETHClient) TransactionInBlock(ctx context.Context, blockHash common.Hash, index uint) (*types.Transaction, error) {
	m.mu.RLock()
	handler := m.TransactionInBlockFunc
	m.mu.RUnlock()
	if handler != nil {
		return handler(ctx, blockHash, index)
	}
	return nil, nil // Default
}

func (m *TestETHClient) TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error) {
	m.mu.RLock()
	handler := m.TransactionReceiptFunc
	m.mu.RUnlock()
	if handler != nil {
		return handler(ctx, txHash)
	}
	return nil, nil // Default
}

func (m *TestETHClient) SyncProgress(ctx context.Context) (*ethereum.SyncProgress, error) {
	m.mu.RLock()
	handler := m.SyncProgressFunc
	m.mu.RUnlock()
	if handler != nil {
		return handler(ctx)
	}
	return nil, nil // Default
}

func (m *TestETHClient) SubscribeNewHead(ctx context.Context, ch chan<- *types.Header) (ethereum.Subscription, error) {
	m.mu.RLock()
	handler := m.SubscribeNewHeadFunc
	m.mu.RUnlock()
	if handler != nil {
		return handler(ctx, ch)
	}
	// Default: return a non-nil, non-functional subscription
	return nil, errors.New("unhandled")
}

func (m *TestETHClient) NetworkID(ctx context.Context) (*big.Int, error) {
	m.mu.RLock()
	handler := m.NetworkIDFunc
	m.mu.RUnlock()
	if handler != nil {
		return handler(ctx)
	}
	return nil, nil // Default
}

func (m *TestETHClient) BalanceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (*big.Int, error) {
	m.mu.RLock()
	handler := m.BalanceAtFunc
	m.mu.RUnlock()
	if handler != nil {
		return handler(ctx, account, blockNumber)
	}
	return nil, nil // Default
}

func (m *TestETHClient) BalanceAtHash(ctx context.Context, account common.Address, blockHash common.Hash) (*big.Int, error) {
	m.mu.RLock()
	handler := m.BalanceAtHashFunc
	m.mu.RUnlock()
	if handler != nil {
		return handler(ctx, account, blockHash)
	}
	return nil, nil // Default
}

func (m *TestETHClient) StorageAt(ctx context.Context, account common.Address, key common.Hash, blockNumber *big.Int) ([]byte, error) {
	m.mu.RLock()
	handler := m.StorageAtFunc
	m.mu.RUnlock()
	if handler != nil {
		return handler(ctx, account, key, blockNumber)
	}
	return nil, nil // Default
}

func (m *TestETHClient) StorageAtHash(ctx context.Context, account common.Address, key common.Hash, blockHash common.Hash) ([]byte, error) {
	m.mu.RLock()
	handler := m.StorageAtHashFunc
	m.mu.RUnlock()
	if handler != nil {
		return handler(ctx, account, key, blockHash)
	}
	return nil, nil // Default
}

func (m *TestETHClient) CodeAt(ctx context.Context, account common.Address, blockNumber *big.Int) ([]byte, error) {
	m.mu.RLock()
	handler := m.CodeAtFunc
	m.mu.RUnlock()
	if handler != nil {
		return handler(ctx, account, blockNumber)
	}
	return nil, nil // Default
}

func (m *TestETHClient) CodeAtHash(ctx context.Context, account common.Address, blockHash common.Hash) ([]byte, error) {
	m.mu.RLock()
	handler := m.CodeAtHashFunc
	m.mu.RUnlock()
	if handler != nil {
		return handler(ctx, account, blockHash)
	}
	return nil, nil // Default
}

func (m *TestETHClient) NonceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (uint64, error) {
	m.mu.RLock()
	handler := m.NonceAtFunc
	m.mu.RUnlock()
	if handler != nil {
		return handler(ctx, account, blockNumber)
	}
	return 0, nil // Default
}

func (m *TestETHClient) NonceAtHash(ctx context.Context, account common.Address, blockHash common.Hash) (uint64, error) {
	m.mu.RLock()
	handler := m.NonceAtHashFunc
	m.mu.RUnlock()
	if handler != nil {
		return handler(ctx, account, blockHash)
	}
	return 0, nil // Default
}

func (m *TestETHClient) FilterLogs(ctx context.Context, q ethereum.FilterQuery) ([]types.Log, error) {
	m.mu.RLock()
	handler := m.FilterLogsFunc
	m.mu.RUnlock()
	if handler != nil {
		return handler(ctx, q)
	}
	return nil, nil // Default
}

func (m *TestETHClient) SubscribeFilterLogs(ctx context.Context, q ethereum.FilterQuery, ch chan<- types.Log) (ethereum.Subscription, error) {
	m.mu.RLock()
	handler := m.SubscribeFilterLogsFunc
	m.mu.RUnlock()
	if handler != nil {
		return handler(ctx, q, ch)
	}
	// Default: return a non-nil, non-functional subscription
	return nil, errors.New("unhandled")
}

func (m *TestETHClient) PendingBalanceAt(ctx context.Context, account common.Address) (*big.Int, error) {
	m.mu.RLock()
	handler := m.PendingBalanceAtFunc
	m.mu.RUnlock()
	if handler != nil {
		return handler(ctx, account)
	}
	return nil, nil // Default
}

func (m *TestETHClient) PendingStorageAt(ctx context.Context, account common.Address, key common.Hash) ([]byte, error) {
	m.mu.RLock()
	handler := m.PendingStorageAtFunc
	m.mu.RUnlock()
	if handler != nil {
		return handler(ctx, account, key)
	}
	return nil, nil // Default
}

func (m *TestETHClient) PendingCodeAt(ctx context.Context, account common.Address) ([]byte, error) {
	m.mu.RLock()
	handler := m.PendingCodeAtFunc
	m.mu.RUnlock()
	if handler != nil {
		return handler(ctx, account)
	}
	return nil, nil // Default
}

func (m *TestETHClient) PendingNonceAt(ctx context.Context, account common.Address) (uint64, error) {
	m.mu.RLock()
	handler := m.PendingNonceAtFunc
	m.mu.RUnlock()
	if handler != nil {
		return handler(ctx, account)
	}
	return 0, nil // Default
}

func (m *TestETHClient) PendingTransactionCount(ctx context.Context) (uint, error) {
	m.mu.RLock()
	handler := m.PendingTransactionCountFunc
	m.mu.RUnlock()
	if handler != nil {
		return handler(ctx)
	}
	return 0, nil // Default
}

func (m *TestETHClient) CallContract(ctx context.Context, msg ethereum.CallMsg, blockNumber *big.Int) ([]byte, error) {
	m.mu.RLock()
	handler := m.CallContractFunc
	m.mu.RUnlock()
	if handler != nil {
		return handler(ctx, msg, blockNumber)
	}
	return nil, nil // Default
}

func (m *TestETHClient) CallContractAtHash(ctx context.Context, msg ethereum.CallMsg, blockHash common.Hash) ([]byte, error) {
	m.mu.RLock()
	handler := m.CallContractAtHashFunc
	m.mu.RUnlock()
	if handler != nil {
		return handler(ctx, msg, blockHash)
	}
	return nil, nil // Default
}

func (m *TestETHClient) PendingCallContract(ctx context.Context, msg ethereum.CallMsg) ([]byte, error) {
	m.mu.RLock()
	handler := m.PendingCallContractFunc
	m.mu.RUnlock()
	if handler != nil {
		return handler(ctx, msg)
	}
	return nil, nil // Default
}

func (m *TestETHClient) SuggestGasPrice(ctx context.Context) (*big.Int, error) {
	m.mu.RLock()
	handler := m.SuggestGasPriceFunc
	m.mu.RUnlock()
	if handler != nil {
		return handler(ctx)
	}
	return nil, nil // Default
}

func (m *TestETHClient) SuggestGasTipCap(ctx context.Context) (*big.Int, error) {
	m.mu.RLock()
	handler := m.SuggestGasTipCapFunc
	m.mu.RUnlock()
	if handler != nil {
		return handler(ctx)
	}
	return nil, nil // Default
}

func (m *TestETHClient) FeeHistory(ctx context.Context, blockCount uint64, lastBlock *big.Int, rewardPercentiles []float64) (*ethereum.FeeHistory, error) {
	m.mu.RLock()
	handler := m.FeeHistoryFunc
	m.mu.RUnlock()
	if handler != nil {
		return handler(ctx, blockCount, lastBlock, rewardPercentiles)
	}
	return nil, nil // Default
}

func (m *TestETHClient) EstimateGas(ctx context.Context, msg ethereum.CallMsg) (uint64, error) {
	m.mu.RLock()
	handler := m.EstimateGasFunc
	m.mu.RUnlock()
	if handler != nil {
		return handler(ctx, msg)
	}
	return 0, nil // Default
}

func (m *TestETHClient) SendTransaction(ctx context.Context, tx *types.Transaction) error {
	m.mu.RLock()
	handler := m.SendTransactionFunc
	m.mu.RUnlock()
	if handler != nil {
		return handler(ctx, tx)
	}
	return nil // Default
}

type TestSubscription struct {
	unsubscribe func()
	err         func() <-chan error
}

func (s *TestSubscription) Unsubscribe() {
	s.unsubscribe()
}

func (s *TestSubscription) Err() <-chan error {
	return s.err()
}
func NewTestSubscription(unsubscribe func(), err func() <-chan error) *TestSubscription {
	return &TestSubscription{
		unsubscribe: unsubscribe,
		err:         err,
	}
}
