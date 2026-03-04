package ethclients

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
)

// ETHClient defines the methods exposed by an Ethereum RPC client.
// This interface abstracts the concrete implementation (*ethclient.Client)
// allowing for easier testing and dependency injection.
type ETHClient interface {
	// Close closes the underlying RPC connection.
	Close()
	// Client gets the underlying RPC client.
	Client() *rpc.Client

	// === Blockchain Access ===

	// ChainID retrieves the current chain ID for transaction replay protection.
	ChainID(ctx context.Context) (*big.Int, error)
	// BlockByHash returns the given full block.
	BlockByHash(ctx context.Context, hash common.Hash) (*types.Block, error)
	// BlockByNumber returns a block from the current canonical chain. If number is nil, the
	// latest known block is returned.
	BlockByNumber(ctx context.Context, number *big.Int) (*types.Block, error)
	// BlockNumber returns the most recent block number.
	BlockNumber(ctx context.Context) (uint64, error)
	// PeerCount returns the number of p2p peers.
	PeerCount(ctx context.Context) (uint64, error)
	// BlockReceipts returns the receipts of a given block number or hash.
	BlockReceipts(ctx context.Context, blockNrOrHash rpc.BlockNumberOrHash) ([]*types.Receipt, error)
	// HeaderByHash returns the block header with the given hash.
	HeaderByHash(ctx context.Context, hash common.Hash) (*types.Header, error)
	// HeaderByNumber returns a block header from the current canonical chain. If number is
	// nil, the latest known header is returned.
	HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error)
	// TransactionByHash returns the transaction with the given hash.
	TransactionByHash(ctx context.Context, hash common.Hash) (tx *types.Transaction, isPending bool, err error)
	// TransactionSender returns the sender address of the given transaction.
	TransactionSender(ctx context.Context, tx *types.Transaction, block common.Hash, index uint) (common.Address, error)
	// TransactionCount returns the total number of transactions in the given block.
	TransactionCount(ctx context.Context, blockHash common.Hash) (uint, error)
	// TransactionInBlock returns a single transaction at index in the given block.
	TransactionInBlock(ctx context.Context, blockHash common.Hash, index uint) (*types.Transaction, error)
	// TransactionReceipt returns the receipt of a transaction by transaction hash.
	TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error)
	// SyncProgress retrieves the current progress of the sync algorithm.
	SyncProgress(ctx context.Context) (*ethereum.SyncProgress, error)
	// SubscribeNewHead subscribes to notifications about the current blockchain head.
	SubscribeNewHead(ctx context.Context, ch chan<- *types.Header) (ethereum.Subscription, error)

	// === State Access ===

	// NetworkID returns the network ID.
	NetworkID(ctx context.Context) (*big.Int, error)
	// BalanceAt returns the wei balance of the given account at a specific block number.
	BalanceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (*big.Int, error)
	// BalanceAtHash returns the wei balance of the given account at a specific block hash.
	BalanceAtHash(ctx context.Context, account common.Address, blockHash common.Hash) (*big.Int, error)
	// StorageAt returns the value of key in the contract storage of the given account at a specific block number.
	StorageAt(ctx context.Context, account common.Address, key common.Hash, blockNumber *big.Int) ([]byte, error)
	// StorageAtHash returns the value of key in the contract storage of the given account at a specific block hash.
	StorageAtHash(ctx context.Context, account common.Address, key common.Hash, blockHash common.Hash) ([]byte, error)
	// CodeAt returns the contract code of the given account at a specific block number.
	CodeAt(ctx context.Context, account common.Address, blockNumber *big.Int) ([]byte, error)
	// CodeAtHash returns the contract code of the given account at a specific block hash.
	CodeAtHash(ctx context.Context, account common.Address, blockHash common.Hash) ([]byte, error)
	// NonceAt returns the account nonce of the given account at a specific block number.
	NonceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (uint64, error)
	// NonceAtHash returns the account nonce of the given account at a specific block hash.
	NonceAtHash(ctx context.Context, account common.Address, blockHash common.Hash) (uint64, error)

	// === Filters ===

	// FilterLogs executes a filter query.
	FilterLogs(ctx context.Context, q ethereum.FilterQuery) ([]types.Log, error)
	// SubscribeFilterLogs subscribes to the results of a streaming filter query.
	SubscribeFilterLogs(ctx context.Context, q ethereum.FilterQuery, ch chan<- types.Log) (ethereum.Subscription, error)

	// === Pending State ===

	// PendingBalanceAt returns the wei balance of the given account in the pending state.
	PendingBalanceAt(ctx context.Context, account common.Address) (*big.Int, error)
	// PendingStorageAt returns the value of key in the contract storage of the given account in the pending state.
	PendingStorageAt(ctx context.Context, account common.Address, key common.Hash) ([]byte, error)
	// PendingCodeAt returns the contract code of the given account in the pending state.
	PendingCodeAt(ctx context.Context, account common.Address) ([]byte, error)
	// PendingNonceAt returns the account nonce of the given account in the pending state.
	PendingNonceAt(ctx context.Context, account common.Address) (uint64, error)
	// PendingTransactionCount returns the total number of transactions in the pending state.
	PendingTransactionCount(ctx context.Context) (uint, error)

	// === Contract Calling ===

	// CallContract executes a message call transaction at a specific block number.
	CallContract(ctx context.Context, msg ethereum.CallMsg, blockNumber *big.Int) ([]byte, error)
	// CallContractAtHash executes a message call transaction at a specific block hash.
	CallContractAtHash(ctx context.Context, msg ethereum.CallMsg, blockHash common.Hash) ([]byte, error)
	// PendingCallContract executes a message call transaction using the pending state.
	PendingCallContract(ctx context.Context, msg ethereum.CallMsg) ([]byte, error)
	// SuggestGasPrice retrieves the currently suggested gas price.
	SuggestGasPrice(ctx context.Context) (*big.Int, error)
	// SuggestGasTipCap retrieves the currently suggested gas tip cap.
	SuggestGasTipCap(ctx context.Context) (*big.Int, error)
	// FeeHistory retrieves the fee market history.
	FeeHistory(ctx context.Context, blockCount uint64, lastBlock *big.Int, rewardPercentiles []float64) (*ethereum.FeeHistory, error)
	// EstimateGas tries to estimate the gas needed to execute a specific transaction.
	EstimateGas(ctx context.Context, msg ethereum.CallMsg) (uint64, error)
	// SendTransaction injects a signed transaction into the pending pool.
	SendTransaction(ctx context.Context, tx *types.Transaction) error
}
