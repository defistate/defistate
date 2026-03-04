package ethclients

import (
	"context"
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
)

// @todo
// generics for semaphore acquire and release

type ETHClientWithMaxConcurrentCalls struct {
	ethClient ETHClient
	semaphore chan struct{}
}

func NewETHClientWithMaxConcurrentCalls(client ETHClient, max int) (*ETHClientWithMaxConcurrentCalls, error) {
	if max <= 0 {
		return nil, errors.New("max concurrent calls must be greater than 0")
	}
	return &ETHClientWithMaxConcurrentCalls{
		ethClient: client,
		semaphore: make(chan struct{}, max),
	}, nil
}

// CallContract is the most called and the slowest method
// it is the most important to limit the number of concurrent calls
func (c *ETHClientWithMaxConcurrentCalls) CallContract(ctx context.Context, msg ethereum.CallMsg, blockNumber *big.Int) ([]byte, error) {
	select {
	case c.semaphore <- struct{}{}:
		defer func() { <-c.semaphore }()
		return c.ethClient.CallContract(ctx, msg, blockNumber)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Close closes the underlying RPC connection.
func (c *ETHClientWithMaxConcurrentCalls) Close() {
	c.ethClient.Close()
}

// Client gets the underlying RPC client.
func (c *ETHClientWithMaxConcurrentCalls) Client() (_ *rpc.Client) {
	return c.ethClient.Client()
}

// ChainID retrieves the current chain ID for transaction replay protection.
func (c *ETHClientWithMaxConcurrentCalls) ChainID(ctx context.Context) (*big.Int, error) {
	select {
	case c.semaphore <- struct{}{}:
		defer func() { <-c.semaphore }()
		return c.ethClient.ChainID(ctx)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// BlockByHash returns the given full block.
func (c *ETHClientWithMaxConcurrentCalls) BlockByHash(ctx context.Context, hash common.Hash) (*types.Block, error) {
	select {
	case c.semaphore <- struct{}{}:
		defer func() { <-c.semaphore }()
		return c.ethClient.BlockByHash(ctx, hash)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// BlockByNumber returns a block from the current canonical chain. If number is nil, the
// latest known block is returned.
func (c *ETHClientWithMaxConcurrentCalls) BlockByNumber(ctx context.Context, number *big.Int) (*types.Block, error) {
	select {
	case c.semaphore <- struct{}{}:
		defer func() { <-c.semaphore }()
		return c.ethClient.BlockByNumber(ctx, number)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// BlockNumber returns the most recent block number.
func (c *ETHClientWithMaxConcurrentCalls) BlockNumber(ctx context.Context) (uint64, error) {
	select {
	case c.semaphore <- struct{}{}:
		defer func() { <-c.semaphore }()
		return c.ethClient.BlockNumber(ctx)
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

// PeerCount returns the number of p2p peers.
func (c *ETHClientWithMaxConcurrentCalls) PeerCount(ctx context.Context) (uint64, error) {
	select {
	case c.semaphore <- struct{}{}:
		defer func() { <-c.semaphore }()
		return c.ethClient.PeerCount(ctx)
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

// BlockReceipts returns the receipts of a given block number or hash.
func (c *ETHClientWithMaxConcurrentCalls) BlockReceipts(ctx context.Context, blockNrOrHash rpc.BlockNumberOrHash) ([]*types.Receipt, error) {
	select {
	case c.semaphore <- struct{}{}:
		defer func() { <-c.semaphore }()
		return c.ethClient.BlockReceipts(ctx, blockNrOrHash)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// HeaderByHash returns the block header with the given hash.
func (c *ETHClientWithMaxConcurrentCalls) HeaderByHash(ctx context.Context, hash common.Hash) (*types.Header, error) {
	select {
	case c.semaphore <- struct{}{}:
		defer func() { <-c.semaphore }()
		return c.ethClient.HeaderByHash(ctx, hash)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// HeaderByNumber returns a block header from the current canonical chain. If number is
// nil, the latest known header is returned.
func (c *ETHClientWithMaxConcurrentCalls) HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error) {
	select {
	case c.semaphore <- struct{}{}:
		defer func() { <-c.semaphore }()
		return c.ethClient.HeaderByNumber(ctx, number)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// TransactionByHash returns the transaction with the given hash.
func (c *ETHClientWithMaxConcurrentCalls) TransactionByHash(ctx context.Context, hash common.Hash) (tx *types.Transaction, isPending bool, err error) {
	select {
	case c.semaphore <- struct{}{}:
		defer func() { <-c.semaphore }()
		return c.ethClient.TransactionByHash(ctx, hash)
	case <-ctx.Done():
		return nil, false, ctx.Err()
	}
}

// TransactionSender returns the sender address of the given transaction.
func (c *ETHClientWithMaxConcurrentCalls) TransactionSender(ctx context.Context, tx *types.Transaction, block common.Hash, index uint) (common.Address, error) {
	select {
	case c.semaphore <- struct{}{}:
		defer func() { <-c.semaphore }()
		return c.ethClient.TransactionSender(ctx, tx, block, index)
	case <-ctx.Done():
		return common.Address{}, ctx.Err()
	}
}

// TransactionCount returns the total number of transactions in the given block.
func (c *ETHClientWithMaxConcurrentCalls) TransactionCount(ctx context.Context, blockHash common.Hash) (uint, error) {
	select {
	case c.semaphore <- struct{}{}:
		defer func() { <-c.semaphore }()
		return c.ethClient.TransactionCount(ctx, blockHash)
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

// TransactionInBlock returns a single transaction at index in the given block.
func (c *ETHClientWithMaxConcurrentCalls) TransactionInBlock(ctx context.Context, blockHash common.Hash, index uint) (*types.Transaction, error) {
	select {
	case c.semaphore <- struct{}{}:
		defer func() { <-c.semaphore }()
		return c.ethClient.TransactionInBlock(ctx, blockHash, index)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// TransactionReceipt returns the receipt of a transaction by transaction hash.
func (c *ETHClientWithMaxConcurrentCalls) TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error) {
	select {
	case c.semaphore <- struct{}{}:
		defer func() { <-c.semaphore }()
		return c.ethClient.TransactionReceipt(ctx, txHash)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// SyncProgress retrieves the current progress of the sync algorithm.
func (c *ETHClientWithMaxConcurrentCalls) SyncProgress(ctx context.Context) (*ethereum.SyncProgress, error) {
	select {
	case c.semaphore <- struct{}{}:
		defer func() { <-c.semaphore }()
		return c.ethClient.SyncProgress(ctx)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// SubscribeNewHead subscribes to notifications about the current blockchain head.
func (c *ETHClientWithMaxConcurrentCalls) SubscribeNewHead(ctx context.Context, ch chan<- *types.Header) (ethereum.Subscription, error) {
	select {
	case c.semaphore <- struct{}{}:
		defer func() { <-c.semaphore }()
		return c.ethClient.SubscribeNewHead(ctx, ch)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// NetworkID returns the network ID.
func (c *ETHClientWithMaxConcurrentCalls) NetworkID(ctx context.Context) (*big.Int, error) {
	select {
	case c.semaphore <- struct{}{}:
		defer func() { <-c.semaphore }()
		return c.ethClient.NetworkID(ctx)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// BalanceAt returns the wei balance of the given account at a specific block number.
func (c *ETHClientWithMaxConcurrentCalls) BalanceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (*big.Int, error) {
	select {
	case c.semaphore <- struct{}{}:
		defer func() { <-c.semaphore }()
		return c.ethClient.BalanceAt(ctx, account, blockNumber)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// BalanceAtHash returns the wei balance of the given account at a specific block hash.
func (c *ETHClientWithMaxConcurrentCalls) BalanceAtHash(ctx context.Context, account common.Address, blockHash common.Hash) (*big.Int, error) {
	select {
	case c.semaphore <- struct{}{}:
		defer func() { <-c.semaphore }()
		return c.ethClient.BalanceAtHash(ctx, account, blockHash)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// StorageAt returns the value of key in the contract storage of the given account at a specific block number.
func (c *ETHClientWithMaxConcurrentCalls) StorageAt(ctx context.Context, account common.Address, key common.Hash, blockNumber *big.Int) ([]byte, error) {
	select {
	case c.semaphore <- struct{}{}:
		defer func() { <-c.semaphore }()
		return c.ethClient.StorageAt(ctx, account, key, blockNumber)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// StorageAtHash returns the value of key in the contract storage of the given account at a specific block hash.
func (c *ETHClientWithMaxConcurrentCalls) StorageAtHash(ctx context.Context, account common.Address, key common.Hash, blockHash common.Hash) ([]byte, error) {
	select {
	case c.semaphore <- struct{}{}:
		defer func() { <-c.semaphore }()
		return c.ethClient.StorageAtHash(ctx, account, key, blockHash)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// CodeAt returns the contract code of the given account at a specific block number.
func (c *ETHClientWithMaxConcurrentCalls) CodeAt(ctx context.Context, account common.Address, blockNumber *big.Int) ([]byte, error) {
	select {
	case c.semaphore <- struct{}{}:
		defer func() { <-c.semaphore }()
		return c.ethClient.CodeAt(ctx, account, blockNumber)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// CodeAtHash returns the contract code of the given account at a specific block hash.
func (c *ETHClientWithMaxConcurrentCalls) CodeAtHash(ctx context.Context, account common.Address, blockHash common.Hash) ([]byte, error) {
	select {
	case c.semaphore <- struct{}{}:
		defer func() { <-c.semaphore }()
		return c.ethClient.CodeAtHash(ctx, account, blockHash)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// NonceAt returns the account nonce of the given account at a specific block number.
func (c *ETHClientWithMaxConcurrentCalls) NonceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (uint64, error) {
	select {
	case c.semaphore <- struct{}{}:
		defer func() { <-c.semaphore }()
		return c.ethClient.NonceAt(ctx, account, blockNumber)
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

// NonceAtHash returns the account nonce of the given account at a specific block hash.
func (c *ETHClientWithMaxConcurrentCalls) NonceAtHash(ctx context.Context, account common.Address, blockHash common.Hash) (uint64, error) {
	select {
	case c.semaphore <- struct{}{}:
		defer func() { <-c.semaphore }()
		return c.ethClient.NonceAtHash(ctx, account, blockHash)
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

// FilterLogs executes a filter query.
func (c *ETHClientWithMaxConcurrentCalls) FilterLogs(ctx context.Context, q ethereum.FilterQuery) ([]types.Log, error) {
	select {
	case c.semaphore <- struct{}{}:
		defer func() { <-c.semaphore }()
		return c.ethClient.FilterLogs(ctx, q)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// SubscribeFilterLogs subscribes to the results of a streaming filter query.
func (c *ETHClientWithMaxConcurrentCalls) SubscribeFilterLogs(ctx context.Context, q ethereum.FilterQuery, ch chan<- types.Log) (ethereum.Subscription, error) {
	select {
	case c.semaphore <- struct{}{}:
		defer func() { <-c.semaphore }()
		return c.ethClient.SubscribeFilterLogs(ctx, q, ch)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// PendingBalanceAt returns the wei balance of the given account in the pending state.
func (c *ETHClientWithMaxConcurrentCalls) PendingBalanceAt(ctx context.Context, account common.Address) (*big.Int, error) {
	select {
	case c.semaphore <- struct{}{}:
		defer func() { <-c.semaphore }()
		return c.ethClient.PendingBalanceAt(ctx, account)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// PendingStorageAt returns the value of key in the contract storage of the given account in the pending state.
func (c *ETHClientWithMaxConcurrentCalls) PendingStorageAt(ctx context.Context, account common.Address, key common.Hash) ([]byte, error) {
	select {
	case c.semaphore <- struct{}{}:
		defer func() { <-c.semaphore }()
		return c.ethClient.PendingStorageAt(ctx, account, key)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// PendingCodeAt returns the contract code of the given account in the pending state.
func (c *ETHClientWithMaxConcurrentCalls) PendingCodeAt(ctx context.Context, account common.Address) ([]byte, error) {
	select {
	case c.semaphore <- struct{}{}:
		defer func() { <-c.semaphore }()
		return c.ethClient.PendingCodeAt(ctx, account)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// PendingNonceAt returns the account nonce of the given account in the pending state.
func (c *ETHClientWithMaxConcurrentCalls) PendingNonceAt(ctx context.Context, account common.Address) (uint64, error) {
	select {
	case c.semaphore <- struct{}{}:
		defer func() { <-c.semaphore }()
		return c.ethClient.PendingNonceAt(ctx, account)
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

// PendingTransactionCount returns the total number of transactions in the pending state.
func (c *ETHClientWithMaxConcurrentCalls) PendingTransactionCount(ctx context.Context) (uint, error) {
	select {
	case c.semaphore <- struct{}{}:
		defer func() { <-c.semaphore }()
		return c.ethClient.PendingTransactionCount(ctx)
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

// CallContractAtHash executes a message call transaction at a specific block hash.
func (c *ETHClientWithMaxConcurrentCalls) CallContractAtHash(ctx context.Context, msg ethereum.CallMsg, blockHash common.Hash) ([]byte, error) {
	select {
	case c.semaphore <- struct{}{}:
		defer func() { <-c.semaphore }()
		return c.ethClient.CallContractAtHash(ctx, msg, blockHash)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// PendingCallContract executes a message call transaction using the pending state.
func (c *ETHClientWithMaxConcurrentCalls) PendingCallContract(ctx context.Context, msg ethereum.CallMsg) ([]byte, error) {
	select {
	case c.semaphore <- struct{}{}:
		defer func() { <-c.semaphore }()
		return c.ethClient.PendingCallContract(ctx, msg)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// SuggestGasPrice retrieves the currently suggested gas price.
func (c *ETHClientWithMaxConcurrentCalls) SuggestGasPrice(ctx context.Context) (*big.Int, error) {
	select {
	case c.semaphore <- struct{}{}:
		defer func() { <-c.semaphore }()
		return c.ethClient.SuggestGasPrice(ctx)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// SuggestGasTipCap retrieves the currently suggested gas tip cap.
func (c *ETHClientWithMaxConcurrentCalls) SuggestGasTipCap(ctx context.Context) (*big.Int, error) {
	select {
	case c.semaphore <- struct{}{}:
		defer func() { <-c.semaphore }()
		return c.ethClient.SuggestGasTipCap(ctx)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// FeeHistory retrieves the fee market history.
func (c *ETHClientWithMaxConcurrentCalls) FeeHistory(ctx context.Context, blockCount uint64, lastBlock *big.Int, rewardPercentiles []float64) (*ethereum.FeeHistory, error) {
	select {
	case c.semaphore <- struct{}{}:
		defer func() { <-c.semaphore }()
		return c.ethClient.FeeHistory(ctx, blockCount, lastBlock, rewardPercentiles)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// EstimateGas tries to estimate the gas needed to execute a specific transaction.
func (c *ETHClientWithMaxConcurrentCalls) EstimateGas(ctx context.Context, msg ethereum.CallMsg) (uint64, error) {
	select {
	case c.semaphore <- struct{}{}:
		defer func() { <-c.semaphore }()
		return c.ethClient.EstimateGas(ctx, msg)
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

// SendTransaction injects a signed transaction into the pending pool.
func (c *ETHClientWithMaxConcurrentCalls) SendTransaction(ctx context.Context, tx *types.Transaction) error {
	// User decided not to slow down SendTransaction with the semaphore
	return c.ethClient.SendTransaction(ctx, tx)
}
