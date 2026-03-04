package ethclients

import (
	"context"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"

	"github.com/stretchr/testify/assert"
)

var ErrTestCallerExpectsErr = errors.New("caller expects error")

func TestNewTestETHClient(t *testing.T) {
	testETHClient := NewTestETHClient()
	assert.NotNil(t, testETHClient)
}

func TestTestETHClient_SetClientHandler(t *testing.T) {
	testETHClient := NewTestETHClient()
	assert.NotNil(t, testETHClient)
	assert.Nil(t, testETHClient.Client(), "expected nil client before set")
	handler := func() *rpc.Client {
		return &rpc.Client{}
	}
	testETHClient.SetClientHandler(handler)
	assert.NotNil(t, testETHClient.Client(), "expected non nil client")
}

func TestTestETHClient_SetChainIDHandler(t *testing.T) {
	testETHClient := NewTestETHClient()
	assert.NotNil(t, testETHClient)

	t.Run("handler returns without error", func(t *testing.T) {
		expectedChainID := big.NewInt(1)
		handler := func(ctx context.Context) (*big.Int, error) {
			return expectedChainID, nil
		}

		testETHClient.SetChainIDHandler(handler)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		chainId, err := testETHClient.ChainID(ctx)
		assert.Nil(t, err)
		assert.True(t, chainId.Cmp(expectedChainID) == 0)
	})

	t.Run("handler returns with error", func(t *testing.T) {
		handler := func(ctx context.Context) (*big.Int, error) {
			return nil, errors.New("caller expects error")
		}

		testETHClient.SetChainIDHandler(handler)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		chainId, err := testETHClient.ChainID(ctx)
		assert.NotNil(t, err)
		assert.Nil(t, chainId)
	})
}

func TestTestETHClient_SetBlockByHashHandler(t *testing.T) {
	testETHClient := NewTestETHClient()
	assert.NotNil(t, testETHClient)
	t.Run("handler returns without error", func(t *testing.T) {
		expectedBlock := &types.Block{}
		handler := func(ctx context.Context, hash common.Hash) (*types.Block, error) {
			return expectedBlock, nil
		}
		testETHClient.SetBlockByHashHandler(handler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		block, err := testETHClient.BlockByHash(ctx, common.Hash{})
		assert.Nil(t, err)
		assert.NotNil(t, block)
		assert.Equal(t, block, expectedBlock)
	})

	t.Run("handler returns with error", func(t *testing.T) {
		handler := func(ctx context.Context, hash common.Hash) (*types.Block, error) {
			return nil, errors.New("caller expects error")
		}
		testETHClient.SetBlockByHashHandler(handler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		block, err := testETHClient.BlockByHash(ctx, common.Hash{})
		assert.NotNil(t, err)
		assert.Nil(t, block)
	})
}

func TestTestETHClient_SetBlockByNumberHandler(t *testing.T) {
	testETHClient := NewTestETHClient()
	assert.NotNil(t, testETHClient)

	t.Run("handler returns without error", func(t *testing.T) {
		expectedBlock := &types.Block{}
		handler := func(ctx context.Context, number *big.Int) (*types.Block, error) {
			return expectedBlock, nil
		}
		testETHClient.SetBlockByNumberHandler(handler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		block, err := testETHClient.BlockByNumber(ctx, big.NewInt(1))
		assert.Nil(t, err)
		assert.NotNil(t, block)
		assert.Equal(t, block, expectedBlock)
	})

	t.Run("handler returns with error", func(t *testing.T) {
		handler := func(ctx context.Context, number *big.Int) (*types.Block, error) {
			return nil, ErrTestCallerExpectsErr
		}
		testETHClient.SetBlockByNumberHandler(handler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		block, err := testETHClient.BlockByNumber(ctx, big.NewInt(1))
		assert.NotNil(t, err)
		assert.Nil(t, block)
	})
}

func TestTestETHClient_SetBlockNumberHandler(t *testing.T) {
	testETHClient := NewTestETHClient()
	assert.NotNil(t, testETHClient)
	t.Run("handler returns without error", func(t *testing.T) {
		expectedBlockNumber := uint64(100)
		handler := func(ctx context.Context) (uint64, error) {
			return expectedBlockNumber, nil
		}
		testETHClient.SetBlockNumberHandler(handler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		blockNumber, err := testETHClient.BlockNumber(ctx)
		assert.Nil(t, err)
		assert.Equal(t, blockNumber, expectedBlockNumber)
	})

	t.Run("handler returns with error", func(t *testing.T) {
		handler := func(ctx context.Context) (uint64, error) {
			return 0, ErrTestCallerExpectsErr
		}
		testETHClient.SetBlockNumberHandler(handler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		blockNumber, err := testETHClient.BlockNumber(ctx)
		assert.NotNil(t, err)
		assert.Equal(t, blockNumber, uint64(0))
	})

}

func TestTestETHClient_SetPeerCountHandler(t *testing.T) {
	testETHClient := NewTestETHClient()
	assert.NotNil(t, testETHClient)

	t.Run("handler returns without error", func(t *testing.T) {
		expectedPeerCount := uint64(36)
		handler := func(ctx context.Context) (uint64, error) {
			return expectedPeerCount, nil
		}
		testETHClient.SetPeerCountHandler(handler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		peerCount, err := testETHClient.PeerCount(ctx)
		assert.Nil(t, err)
		assert.Equal(t, peerCount, expectedPeerCount)

	})

	t.Run("handler returns with error", func(t *testing.T) {
		handler := func(ctx context.Context) (uint64, error) {
			return 0, ErrTestCallerExpectsErr
		}
		testETHClient.SetPeerCountHandler(handler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, err := testETHClient.PeerCount(ctx)
		assert.NotNil(t, err)
	})
}

func TestTestETHClient_SetBlockReceiptsHandler(t *testing.T) {
	testETHClient := NewTestETHClient()
	assert.NotNil(t, testETHClient)

	t.Run("handler returns without error", func(t *testing.T) {
		expectedReceipts := []*types.Receipt{&types.Receipt{}}
		handler := func(ctx context.Context, blockNrOrHash rpc.BlockNumberOrHash) ([]*types.Receipt, error) {
			return expectedReceipts, nil
		}
		testETHClient.SetBlockReceiptsHandler(handler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		blockReceipts, err := testETHClient.BlockReceipts(ctx, rpc.BlockNumberOrHash{})
		assert.Nil(t, err)
		assert.NotNil(t, blockReceipts)
		assert.Equal(t, blockReceipts, expectedReceipts)
	})

	t.Run("handler returns with error", func(t *testing.T) {
		handler := func(ctx context.Context, blockNrOrHash rpc.BlockNumberOrHash) ([]*types.Receipt, error) {
			return nil, ErrTestCallerExpectsErr
		}
		testETHClient.SetBlockReceiptsHandler(handler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		blockReceipts, err := testETHClient.BlockReceipts(ctx, rpc.BlockNumberOrHash{})
		assert.NotNil(t, err)
		assert.Nil(t, blockReceipts)
	})
}

func TestTestETHClient_SetHeaderByHashHandler(t *testing.T) {
	testETHClient := NewTestETHClient()
	assert.NotNil(t, testETHClient)

	t.Run("handler returns without error", func(t *testing.T) {
		expectedHeader := &types.Header{}
		handler := func(ctx context.Context, hash common.Hash) (*types.Header, error) {
			return expectedHeader, nil
		}
		testETHClient.SetHeaderByHashHandler(handler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		header, err := testETHClient.HeaderByHash(ctx, common.Hash{})
		assert.Nil(t, err)
		assert.Equal(t, header, expectedHeader)
	})

	t.Run("handler returns with error", func(t *testing.T) {
		handler := func(ctx context.Context, hash common.Hash) (*types.Header, error) {
			return nil, ErrTestCallerExpectsErr
		}
		testETHClient.SetHeaderByHashHandler(handler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		header, err := testETHClient.HeaderByHash(ctx, common.Hash{})
		assert.NotNil(t, err)
		assert.Nil(t, header)
	})
}

func TestTestETHClient_SetHeaderByNumberHandler(t *testing.T) {
	testETHClient := NewTestETHClient()
	assert.NotNil(t, testETHClient)

	t.Run("handler returns without error", func(t *testing.T) {
		expectedHeader := &types.Header{}
		handler := func(ctx context.Context, number *big.Int) (*types.Header, error) {
			return expectedHeader, nil
		}
		testETHClient.SetHeaderByNumberHandler(handler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		header, err := testETHClient.HeaderByNumber(ctx, big.NewInt(1))
		assert.Nil(t, err)
		assert.Equal(t, header, expectedHeader)
	})

	t.Run("handler returns with error", func(t *testing.T) {
		handler := func(ctx context.Context, number *big.Int) (*types.Header, error) {
			return nil, ErrTestCallerExpectsErr
		}
		testETHClient.SetHeaderByNumberHandler(handler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		header, err := testETHClient.HeaderByNumber(ctx, big.NewInt(1))
		assert.NotNil(t, err)
		assert.Nil(t, header)
	})
}

func TestTestETHClient_SetTransactionByHashHandler(t *testing.T) {
	testETHClient := NewTestETHClient()
	assert.NotNil(t, testETHClient)

	t.Run("handler returns without error", func(t *testing.T) {
		expectedTransaction := &types.Transaction{}
		expectedIsPending := true
		handler := func(ctx context.Context, hash common.Hash) (*types.Transaction, bool, error) {
			return expectedTransaction, expectedIsPending, nil
		}
		testETHClient.SetTransactionByHashHandler(handler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		transaction, isPending, err := testETHClient.TransactionByHash(ctx, common.Hash{})
		assert.Nil(t, err)
		assert.Equal(t, isPending, expectedIsPending)
		assert.Equal(t, transaction, expectedTransaction)
	})
	t.Run("handler returns with error", func(t *testing.T) {
		handler := func(ctx context.Context, hash common.Hash) (*types.Transaction, bool, error) {
			return nil, false, ErrTestCallerExpectsErr
		}
		testETHClient.SetTransactionByHashHandler(handler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		transaction, isPending, err := testETHClient.TransactionByHash(ctx, common.Hash{})
		assert.NotNil(t, err)
		assert.Equal(t, isPending, false)
		assert.Nil(t, transaction)
	})
}

func TestTestETHClient_SetTransactionSenderHandler(t *testing.T) {
	testETHClient := NewTestETHClient()
	assert.NotNil(t, testETHClient)

	t.Run("handler returns without error", func(t *testing.T) {
		expectedSender := common.HexToAddress("0x123")
		handler := func(ctx context.Context, tx *types.Transaction, block common.Hash, index uint) (common.Address, error) {
			return expectedSender, nil
		}
		testETHClient.SetTransactionSenderHandler(handler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		sender, err := testETHClient.TransactionSender(ctx, &types.Transaction{}, common.Hash{}, 0)
		assert.Nil(t, err)
		assert.Equal(t, sender, expectedSender)
	})

	t.Run("handler returns with error", func(t *testing.T) {
		handler := func(ctx context.Context, tx *types.Transaction, block common.Hash, index uint) (common.Address, error) {
			return common.Address{}, ErrTestCallerExpectsErr
		}
		testETHClient.SetTransactionSenderHandler(handler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		sender, err := testETHClient.TransactionSender(ctx, &types.Transaction{}, common.Hash{}, 0)
		assert.NotNil(t, err)
		assert.Equal(t, sender, common.Address{})
	})
}

func TestTestETHClient_SetTransactionCountHandler(t *testing.T) {
	testETHClient := NewTestETHClient()
	assert.NotNil(t, testETHClient)

	t.Run("handler returns without error", func(t *testing.T) {
		expectedCount := uint(10)
		handler := func(ctx context.Context, blockHash common.Hash) (uint, error) {
			return expectedCount, nil
		}
		testETHClient.SetTransactionCountHandler(handler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		count, err := testETHClient.TransactionCount(ctx, common.Hash{})
		assert.Nil(t, err)
		assert.Equal(t, count, expectedCount)
	})

	t.Run("handler returns with error", func(t *testing.T) {
		handler := func(ctx context.Context, blockHash common.Hash) (uint, error) {
			return 0, ErrTestCallerExpectsErr
		}
		testETHClient.SetTransactionCountHandler(handler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		count, err := testETHClient.TransactionCount(ctx, common.Hash{})
		assert.NotNil(t, err)
		assert.Equal(t, count, uint(0))
	})
}

func TestTestETHClient_SetTransactionInBlockHandler(t *testing.T) {
	testETHClient := NewTestETHClient()
	assert.NotNil(t, testETHClient)

	t.Run("handler returns without error", func(t *testing.T) {
		expectedTransaction := &types.Transaction{}
		handler := func(ctx context.Context, blockHash common.Hash, index uint) (*types.Transaction, error) {
			return expectedTransaction, nil
		}
		testETHClient.SetTransactionInBlockHandler(handler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		tx, err := testETHClient.TransactionInBlock(ctx, common.Hash{}, 0)
		assert.Nil(t, err)
		assert.Equal(t, tx, expectedTransaction)
	})

	t.Run("handler returns with error", func(t *testing.T) {
		handler := func(ctx context.Context, blockHash common.Hash, index uint) (*types.Transaction, error) {
			return nil, ErrTestCallerExpectsErr
		}
		testETHClient.SetTransactionInBlockHandler(handler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		tx, err := testETHClient.TransactionInBlock(ctx, common.Hash{}, 0)
		assert.NotNil(t, err)
		assert.Nil(t, tx)
	})
}

func TestTestETHClient_SetTransactionReceiptHandler(t *testing.T) {
	testETHClient := NewTestETHClient()
	assert.NotNil(t, testETHClient)

	t.Run("handler returns without error", func(t *testing.T) {
		expectedReceipt := &types.Receipt{}
		handler := func(ctx context.Context, txHash common.Hash) (*types.Receipt, error) {
			return expectedReceipt, nil
		}
		testETHClient.SetTransactionReceiptHandler(handler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		receipt, err := testETHClient.TransactionReceipt(ctx, common.Hash{})
		assert.Nil(t, err)
		assert.Equal(t, receipt, expectedReceipt)
	})

	t.Run("handler returns with error", func(t *testing.T) {
		handler := func(ctx context.Context, txHash common.Hash) (*types.Receipt, error) {
			return nil, ErrTestCallerExpectsErr
		}
		testETHClient.SetTransactionReceiptHandler(handler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		receipt, err := testETHClient.TransactionReceipt(ctx, common.Hash{})
		assert.NotNil(t, err)
		assert.Nil(t, receipt)
	})
}

func TestTestETHClient_SetSyncProgressHandler(t *testing.T) {
	testETHClient := NewTestETHClient()
	assert.NotNil(t, testETHClient)

	t.Run("handler returns without error", func(t *testing.T) {
		expectedProgress := &ethereum.SyncProgress{HighestBlock: 100}
		handler := func(ctx context.Context) (*ethereum.SyncProgress, error) {
			return expectedProgress, nil
		}
		testETHClient.SetSyncProgressHandler(handler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		progress, err := testETHClient.SyncProgress(ctx)
		assert.Nil(t, err)
		assert.Equal(t, progress, expectedProgress)
	})

	t.Run("handler returns with error", func(t *testing.T) {
		handler := func(ctx context.Context) (*ethereum.SyncProgress, error) {
			return nil, ErrTestCallerExpectsErr
		}
		testETHClient.SetSyncProgressHandler(handler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		progress, err := testETHClient.SyncProgress(ctx)
		assert.NotNil(t, err)
		assert.Nil(t, progress)
	})
}

func TestTestETHClient_SetSubscribeNewHeadHandler(t *testing.T) {
	testETHClient := NewTestETHClient()
	assert.NotNil(t, testETHClient)

	t.Run("handler returns without error", func(t *testing.T) {
		expectedSub := NewTestSubscription(func() {}, func() <-chan error { return nil })
		handler := func(ctx context.Context, ch chan<- *types.Header) (ethereum.Subscription, error) {
			return expectedSub, nil
		}
		testETHClient.SetSubscribeNewHeadHandler(handler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		sub, err := testETHClient.SubscribeNewHead(ctx, make(chan<- *types.Header))
		assert.Nil(t, err)
		assert.Equal(t, sub, expectedSub)
	})

	t.Run("handler returns with error", func(t *testing.T) {
		handler := func(ctx context.Context, ch chan<- *types.Header) (ethereum.Subscription, error) {
			return nil, ErrTestCallerExpectsErr
		}
		testETHClient.SetSubscribeNewHeadHandler(handler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		sub, err := testETHClient.SubscribeNewHead(ctx, make(chan<- *types.Header))
		assert.NotNil(t, err)
		assert.Nil(t, sub)
	})
}

func TestTestETHClient_SetNetworkIDHandler(t *testing.T) {
	testETHClient := NewTestETHClient()
	assert.NotNil(t, testETHClient)

	t.Run("handler returns without error", func(t *testing.T) {
		expectedNetworkID := big.NewInt(1)
		handler := func(ctx context.Context) (*big.Int, error) {
			return expectedNetworkID, nil
		}
		testETHClient.SetNetworkIDHandler(handler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		networkID, err := testETHClient.NetworkID(ctx)
		assert.Nil(t, err)
		assert.True(t, networkID.Cmp(expectedNetworkID) == 0)
	})

	t.Run("handler returns with error", func(t *testing.T) {
		handler := func(ctx context.Context) (*big.Int, error) {
			return nil, ErrTestCallerExpectsErr
		}
		testETHClient.SetNetworkIDHandler(handler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		networkID, err := testETHClient.NetworkID(ctx)
		assert.NotNil(t, err)
		assert.Nil(t, networkID)
	})
}

func TestTestETHClient_SetBalanceAtHandler(t *testing.T) {
	testETHClient := NewTestETHClient()
	assert.NotNil(t, testETHClient)

	t.Run("handler returns without error", func(t *testing.T) {
		expectedBalance := big.NewInt(1e18)
		handler := func(ctx context.Context, account common.Address, blockNumber *big.Int) (*big.Int, error) {
			return expectedBalance, nil
		}
		testETHClient.SetBalanceAtHandler(handler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		balance, err := testETHClient.BalanceAt(ctx, common.Address{}, nil)
		assert.Nil(t, err)
		assert.True(t, balance.Cmp(expectedBalance) == 0)
	})

	t.Run("handler returns with error", func(t *testing.T) {
		handler := func(ctx context.Context, account common.Address, blockNumber *big.Int) (*big.Int, error) {
			return nil, ErrTestCallerExpectsErr
		}
		testETHClient.SetBalanceAtHandler(handler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		balance, err := testETHClient.BalanceAt(ctx, common.Address{}, nil)
		assert.NotNil(t, err)
		assert.Nil(t, balance)
	})
}

func TestTestETHClient_SetStorageAtHandler(t *testing.T) {
	testETHClient := NewTestETHClient()
	assert.NotNil(t, testETHClient)

	t.Run("handler returns without error", func(t *testing.T) {
		expectedData := []byte("test")
		handler := func(ctx context.Context, account common.Address, key common.Hash, blockNumber *big.Int) ([]byte, error) {
			return expectedData, nil
		}
		testETHClient.SetStorageAtHandler(handler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		data, err := testETHClient.StorageAt(ctx, common.Address{}, common.Hash{}, nil)
		assert.Nil(t, err)
		assert.Equal(t, data, expectedData)
	})

	t.Run("handler returns with error", func(t *testing.T) {
		handler := func(ctx context.Context, account common.Address, key common.Hash, blockNumber *big.Int) ([]byte, error) {
			return nil, ErrTestCallerExpectsErr
		}
		testETHClient.SetStorageAtHandler(handler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		data, err := testETHClient.StorageAt(ctx, common.Address{}, common.Hash{}, nil)
		assert.NotNil(t, err)
		assert.Nil(t, data)
	})
}

func TestTestETHClient_SetCodeAtHandler(t *testing.T) {
	testETHClient := NewTestETHClient()
	assert.NotNil(t, testETHClient)

	t.Run("handler returns without error", func(t *testing.T) {
		expectedCode := []byte{0x60, 0x80, 0x60, 0x40, 0x52}
		handler := func(ctx context.Context, account common.Address, blockNumber *big.Int) ([]byte, error) {
			return expectedCode, nil
		}
		testETHClient.SetCodeAtHandler(handler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		code, err := testETHClient.CodeAt(ctx, common.Address{}, nil)
		assert.Nil(t, err)
		assert.Equal(t, code, expectedCode)
	})

	t.Run("handler returns with error", func(t *testing.T) {
		handler := func(ctx context.Context, account common.Address, blockNumber *big.Int) ([]byte, error) {
			return nil, ErrTestCallerExpectsErr
		}
		testETHClient.SetCodeAtHandler(handler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		code, err := testETHClient.CodeAt(ctx, common.Address{}, nil)
		assert.NotNil(t, err)
		assert.Nil(t, code)
	})
}

func TestTestETHClient_SetNonceAtHandler(t *testing.T) {
	testETHClient := NewTestETHClient()
	assert.NotNil(t, testETHClient)

	t.Run("handler returns without error", func(t *testing.T) {
		expectedNonce := uint64(5)
		handler := func(ctx context.Context, account common.Address, blockNumber *big.Int) (uint64, error) {
			return expectedNonce, nil
		}
		testETHClient.SetNonceAtHandler(handler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		nonce, err := testETHClient.NonceAt(ctx, common.Address{}, nil)
		assert.Nil(t, err)
		assert.Equal(t, nonce, expectedNonce)
	})

	t.Run("handler returns with error", func(t *testing.T) {
		handler := func(ctx context.Context, account common.Address, blockNumber *big.Int) (uint64, error) {
			return 0, ErrTestCallerExpectsErr
		}
		testETHClient.SetNonceAtHandler(handler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		nonce, err := testETHClient.NonceAt(ctx, common.Address{}, nil)
		assert.NotNil(t, err)
		assert.Equal(t, nonce, uint64(0))
	})
}

func TestTestETHClient_SetFilterLogsHandler(t *testing.T) {
	testETHClient := NewTestETHClient()
	assert.NotNil(t, testETHClient)

	t.Run("handler returns without error", func(t *testing.T) {
		expectedLogs := []types.Log{{}}
		handler := func(ctx context.Context, q ethereum.FilterQuery) ([]types.Log, error) {
			return expectedLogs, nil
		}
		testETHClient.SetFilterLogsHandler(handler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		logs, err := testETHClient.FilterLogs(ctx, ethereum.FilterQuery{})
		assert.Nil(t, err)
		assert.Equal(t, logs, expectedLogs)
	})

	t.Run("handler returns with error", func(t *testing.T) {
		handler := func(ctx context.Context, q ethereum.FilterQuery) ([]types.Log, error) {
			return nil, ErrTestCallerExpectsErr
		}
		testETHClient.SetFilterLogsHandler(handler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		logs, err := testETHClient.FilterLogs(ctx, ethereum.FilterQuery{})
		assert.NotNil(t, err)
		assert.Nil(t, logs)
	})
}

func TestTestETHClient_SetSubscribeFilterLogsHandler(t *testing.T) {
	testETHClient := NewTestETHClient()
	assert.NotNil(t, testETHClient)

	t.Run("handler returns without error", func(t *testing.T) {
		expectedSub := NewTestSubscription(func() {}, func() <-chan error { return nil })
		handler := func(ctx context.Context, q ethereum.FilterQuery, ch chan<- types.Log) (ethereum.Subscription, error) {
			return expectedSub, nil
		}
		testETHClient.SetSubscribeFilterLogsHandler(handler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		sub, err := testETHClient.SubscribeFilterLogs(ctx, ethereum.FilterQuery{}, make(chan<- types.Log))
		assert.Nil(t, err)
		assert.Equal(t, sub, expectedSub)
	})

	t.Run("handler returns with error", func(t *testing.T) {
		handler := func(ctx context.Context, q ethereum.FilterQuery, ch chan<- types.Log) (ethereum.Subscription, error) {
			return nil, ErrTestCallerExpectsErr
		}
		testETHClient.SetSubscribeFilterLogsHandler(handler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		sub, err := testETHClient.SubscribeFilterLogs(ctx, ethereum.FilterQuery{}, make(chan<- types.Log))
		assert.NotNil(t, err)
		assert.Nil(t, sub)
	})
}

func TestTestETHClient_SetPendingBalanceAtHandler(t *testing.T) {
	testETHClient := NewTestETHClient()
	assert.NotNil(t, testETHClient)

	t.Run("handler returns without error", func(t *testing.T) {
		expectedBalance := big.NewInt(1e18)
		handler := func(ctx context.Context, account common.Address) (*big.Int, error) {
			return expectedBalance, nil
		}
		testETHClient.SetPendingBalanceAtHandler(handler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		balance, err := testETHClient.PendingBalanceAt(ctx, common.Address{})
		assert.Nil(t, err)
		assert.True(t, balance.Cmp(expectedBalance) == 0)
	})

	t.Run("handler returns with error", func(t *testing.T) {
		handler := func(ctx context.Context, account common.Address) (*big.Int, error) {
			return nil, ErrTestCallerExpectsErr
		}
		testETHClient.SetPendingBalanceAtHandler(handler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		balance, err := testETHClient.PendingBalanceAt(ctx, common.Address{})
		assert.NotNil(t, err)
		assert.Nil(t, balance)
	})
}

func TestTestETHClient_SetPendingStorageAtHandler(t *testing.T) {
	testETHClient := NewTestETHClient()
	assert.NotNil(t, testETHClient)

	t.Run("handler returns without error", func(t *testing.T) {
		expectedData := []byte("pending test")
		handler := func(ctx context.Context, account common.Address, key common.Hash) ([]byte, error) {
			return expectedData, nil
		}
		testETHClient.SetPendingStorageAtHandler(handler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		data, err := testETHClient.PendingStorageAt(ctx, common.Address{}, common.Hash{})
		assert.Nil(t, err)
		assert.Equal(t, data, expectedData)
	})

	t.Run("handler returns with error", func(t *testing.T) {
		handler := func(ctx context.Context, account common.Address, key common.Hash) ([]byte, error) {
			return nil, ErrTestCallerExpectsErr
		}
		testETHClient.SetPendingStorageAtHandler(handler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		data, err := testETHClient.PendingStorageAt(ctx, common.Address{}, common.Hash{})
		assert.NotNil(t, err)
		assert.Nil(t, data)
	})
}

func TestTestETHClient_SetPendingCodeAtHandler(t *testing.T) {
	testETHClient := NewTestETHClient()
	assert.NotNil(t, testETHClient)

	t.Run("handler returns without error", func(t *testing.T) {
		expectedCode := []byte{0xde, 0xad, 0xbe, 0xef}
		handler := func(ctx context.Context, account common.Address) ([]byte, error) {
			return expectedCode, nil
		}
		testETHClient.SetPendingCodeAtHandler(handler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		code, err := testETHClient.PendingCodeAt(ctx, common.Address{})
		assert.Nil(t, err)
		assert.Equal(t, code, expectedCode)
	})

	t.Run("handler returns with error", func(t *testing.T) {
		handler := func(ctx context.Context, account common.Address) ([]byte, error) {
			return nil, ErrTestCallerExpectsErr
		}
		testETHClient.SetPendingCodeAtHandler(handler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		code, err := testETHClient.PendingCodeAt(ctx, common.Address{})
		assert.NotNil(t, err)
		assert.Nil(t, code)
	})
}

func TestTestETHClient_SetPendingNonceAtHandler(t *testing.T) {
	testETHClient := NewTestETHClient()
	assert.NotNil(t, testETHClient)

	t.Run("handler returns without error", func(t *testing.T) {
		expectedNonce := uint64(42)
		handler := func(ctx context.Context, account common.Address) (uint64, error) {
			return expectedNonce, nil
		}
		testETHClient.SetPendingNonceAtHandler(handler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		nonce, err := testETHClient.PendingNonceAt(ctx, common.Address{})
		assert.Nil(t, err)
		assert.Equal(t, nonce, expectedNonce)
	})

	t.Run("handler returns with error", func(t *testing.T) {
		handler := func(ctx context.Context, account common.Address) (uint64, error) {
			return 0, ErrTestCallerExpectsErr
		}
		testETHClient.SetPendingNonceAtHandler(handler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		nonce, err := testETHClient.PendingNonceAt(ctx, common.Address{})
		assert.NotNil(t, err)
		assert.Equal(t, nonce, uint64(0))
	})
}

func TestTestETHClient_SetPendingTransactionCountHandler(t *testing.T) {
	testETHClient := NewTestETHClient()
	assert.NotNil(t, testETHClient)

	t.Run("handler returns without error", func(t *testing.T) {
		expectedCount := uint(5)
		handler := func(ctx context.Context) (uint, error) {
			return expectedCount, nil
		}
		testETHClient.SetPendingTransactionCountHandler(handler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		count, err := testETHClient.PendingTransactionCount(ctx)
		assert.Nil(t, err)
		assert.Equal(t, count, expectedCount)
	})

	t.Run("handler returns with error", func(t *testing.T) {
		handler := func(ctx context.Context) (uint, error) {
			return 0, ErrTestCallerExpectsErr
		}
		testETHClient.SetPendingTransactionCountHandler(handler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		count, err := testETHClient.PendingTransactionCount(ctx)
		assert.NotNil(t, err)
		assert.Equal(t, count, uint(0))
	})
}

func TestTestETHClient_SetCallContractHandler(t *testing.T) {
	testETHClient := NewTestETHClient()
	assert.NotNil(t, testETHClient)

	t.Run("handler returns without error", func(t *testing.T) {
		expectedResult := []byte("result")
		handler := func(ctx context.Context, msg ethereum.CallMsg, blockNumber *big.Int) ([]byte, error) {
			return expectedResult, nil
		}
		testETHClient.SetCallContractHandler(handler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		result, err := testETHClient.CallContract(ctx, ethereum.CallMsg{}, nil)
		assert.Nil(t, err)
		assert.Equal(t, result, expectedResult)
	})

	t.Run("handler returns with error", func(t *testing.T) {
		handler := func(ctx context.Context, msg ethereum.CallMsg, blockNumber *big.Int) ([]byte, error) {
			return nil, ErrTestCallerExpectsErr
		}
		testETHClient.SetCallContractHandler(handler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		result, err := testETHClient.CallContract(ctx, ethereum.CallMsg{}, nil)
		assert.NotNil(t, err)
		assert.Nil(t, result)
	})
}

func TestTestETHClient_SetFeeHistoryHandler(t *testing.T) {
	testETHClient := NewTestETHClient()
	assert.NotNil(t, testETHClient)

	t.Run("handler returns without error", func(t *testing.T) {
		expectedFeeHistory := &ethereum.FeeHistory{}
		handler := func(ctx context.Context, blockCount uint64, lastBlock *big.Int, rewardPercentiles []float64) (*ethereum.FeeHistory, error) {
			return expectedFeeHistory, nil
		}
		testETHClient.SetFeeHistoryHandler(handler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		feeHistory, err := testETHClient.FeeHistory(ctx, 1, nil, nil)
		assert.Nil(t, err)
		assert.Equal(t, feeHistory, expectedFeeHistory)
	})

	t.Run("handler returns with error", func(t *testing.T) {
		handler := func(ctx context.Context, blockCount uint64, lastBlock *big.Int, rewardPercentiles []float64) (*ethereum.FeeHistory, error) {
			return nil, ErrTestCallerExpectsErr
		}
		testETHClient.SetFeeHistoryHandler(handler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		feeHistory, err := testETHClient.FeeHistory(ctx, 1, nil, nil)
		assert.NotNil(t, err)
		assert.Nil(t, feeHistory)
	})
}

func TestTestETHClient_SetEstimateGasHandler(t *testing.T) {
	testETHClient := NewTestETHClient()
	assert.NotNil(t, testETHClient)

	t.Run("handler returns without error", func(t *testing.T) {
		expectedGas := uint64(21000)
		handler := func(ctx context.Context, msg ethereum.CallMsg) (uint64, error) {
			return expectedGas, nil
		}
		testETHClient.SetEstimateGasHandler(handler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		gas, err := testETHClient.EstimateGas(ctx, ethereum.CallMsg{})
		assert.Nil(t, err)
		assert.Equal(t, gas, expectedGas)
	})

	t.Run("handler returns with error", func(t *testing.T) {
		handler := func(ctx context.Context, msg ethereum.CallMsg) (uint64, error) {
			return 0, ErrTestCallerExpectsErr
		}
		testETHClient.SetEstimateGasHandler(handler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		gas, err := testETHClient.EstimateGas(ctx, ethereum.CallMsg{})
		assert.NotNil(t, err)
		assert.Equal(t, gas, uint64(0))
	})
}

func TestTestETHClient_SetSendTransactionHandler(t *testing.T) {
	testETHClient := NewTestETHClient()
	assert.NotNil(t, testETHClient)

	t.Run("handler returns without error", func(t *testing.T) {
		handler := func(ctx context.Context, tx *types.Transaction) error {
			return nil
		}
		testETHClient.SetSendTransactionHandler(handler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err := testETHClient.SendTransaction(ctx, &types.Transaction{})
		assert.Nil(t, err)
	})

	t.Run("handler returns with error", func(t *testing.T) {
		handler := func(ctx context.Context, tx *types.Transaction) error {
			return ErrTestCallerExpectsErr
		}
		testETHClient.SetSendTransactionHandler(handler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err := testETHClient.SendTransaction(ctx, &types.Transaction{})
		assert.NotNil(t, err)
	})
}

func TestTestETHClient_SetBalanceAtHashHandler(t *testing.T) {
	testETHClient := NewTestETHClient()
	assert.NotNil(t, testETHClient)

	t.Run("handler returns without error", func(t *testing.T) {
		expectedBalance := big.NewInt(5e18)
		handler := func(ctx context.Context, account common.Address, blockHash common.Hash) (*big.Int, error) {
			return expectedBalance, nil
		}
		testETHClient.SetBalanceAtHashHandler(handler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		balance, err := testETHClient.BalanceAtHash(ctx, common.Address{}, common.Hash{})
		assert.Nil(t, err)
		assert.True(t, balance.Cmp(expectedBalance) == 0)
	})

	t.Run("handler returns with error", func(t *testing.T) {
		handler := func(ctx context.Context, account common.Address, blockHash common.Hash) (*big.Int, error) {
			return nil, ErrTestCallerExpectsErr
		}
		testETHClient.SetBalanceAtHashHandler(handler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		balance, err := testETHClient.BalanceAtHash(ctx, common.Address{}, common.Hash{})
		assert.NotNil(t, err)
		assert.Nil(t, balance)
	})
}

func TestTestETHClient_SetStorageAtHashHandler(t *testing.T) {
	testETHClient := NewTestETHClient()
	assert.NotNil(t, testETHClient)

	t.Run("handler returns without error", func(t *testing.T) {
		expectedData := []byte("storage at hash")
		handler := func(ctx context.Context, account common.Address, key common.Hash, blockHash common.Hash) ([]byte, error) {
			return expectedData, nil
		}
		testETHClient.SetStorageAtHashHandler(handler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		data, err := testETHClient.StorageAtHash(ctx, common.Address{}, common.Hash{}, common.Hash{})
		assert.Nil(t, err)
		assert.Equal(t, data, expectedData)
	})

	t.Run("handler returns with error", func(t *testing.T) {
		handler := func(ctx context.Context, account common.Address, key common.Hash, blockHash common.Hash) ([]byte, error) {
			return nil, ErrTestCallerExpectsErr
		}
		testETHClient.SetStorageAtHashHandler(handler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		data, err := testETHClient.StorageAtHash(ctx, common.Address{}, common.Hash{}, common.Hash{})
		assert.NotNil(t, err)
		assert.Nil(t, data)
	})
}

func TestTestETHClient_SetCodeAtHashHandler(t *testing.T) {
	testETHClient := NewTestETHClient()
	assert.NotNil(t, testETHClient)

	t.Run("handler returns without error", func(t *testing.T) {
		expectedCode := []byte{0xfe, 0xed, 0xfa, 0xce}
		handler := func(ctx context.Context, account common.Address, blockHash common.Hash) ([]byte, error) {
			return expectedCode, nil
		}
		testETHClient.SetCodeAtHashHandler(handler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		code, err := testETHClient.CodeAtHash(ctx, common.Address{}, common.Hash{})
		assert.Nil(t, err)
		assert.Equal(t, code, expectedCode)
	})

	t.Run("handler returns with error", func(t *testing.T) {
		handler := func(ctx context.Context, account common.Address, blockHash common.Hash) ([]byte, error) {
			return nil, ErrTestCallerExpectsErr
		}
		testETHClient.SetCodeAtHashHandler(handler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		code, err := testETHClient.CodeAtHash(ctx, common.Address{}, common.Hash{})
		assert.NotNil(t, err)
		assert.Nil(t, code)
	})
}

func TestTestETHClient_SetNonceAtHashHandler(t *testing.T) {
	testETHClient := NewTestETHClient()
	assert.NotNil(t, testETHClient)

	t.Run("handler returns without error", func(t *testing.T) {
		expectedNonce := uint64(99)
		handler := func(ctx context.Context, account common.Address, blockHash common.Hash) (uint64, error) {
			return expectedNonce, nil
		}
		testETHClient.SetNonceAtHashHandler(handler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		nonce, err := testETHClient.NonceAtHash(ctx, common.Address{}, common.Hash{})
		assert.Nil(t, err)
		assert.Equal(t, nonce, expectedNonce)
	})

	t.Run("handler returns with error", func(t *testing.T) {
		handler := func(ctx context.Context, account common.Address, blockHash common.Hash) (uint64, error) {
			return 0, ErrTestCallerExpectsErr
		}
		testETHClient.SetNonceAtHashHandler(handler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		nonce, err := testETHClient.NonceAtHash(ctx, common.Address{}, common.Hash{})
		assert.NotNil(t, err)
		assert.Equal(t, nonce, uint64(0))
	})
}

func TestTestETHClient_SetCallContractAtHashHandler(t *testing.T) {
	testETHClient := NewTestETHClient()
	assert.NotNil(t, testETHClient)

	t.Run("handler returns without error", func(t *testing.T) {
		expectedResult := []byte("result at hash")
		handler := func(ctx context.Context, msg ethereum.CallMsg, blockHash common.Hash) ([]byte, error) {
			return expectedResult, nil
		}
		testETHClient.SetCallContractAtHashHandler(handler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		result, err := testETHClient.CallContractAtHash(ctx, ethereum.CallMsg{}, common.Hash{})
		assert.Nil(t, err)
		assert.Equal(t, result, expectedResult)
	})

	t.Run("handler returns with error", func(t *testing.T) {
		handler := func(ctx context.Context, msg ethereum.CallMsg, blockHash common.Hash) ([]byte, error) {
			return nil, ErrTestCallerExpectsErr
		}
		testETHClient.SetCallContractAtHashHandler(handler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		result, err := testETHClient.CallContractAtHash(ctx, ethereum.CallMsg{}, common.Hash{})
		assert.NotNil(t, err)
		assert.Nil(t, result)
	})
}

func TestTestETHClient_SetPendingCallContractHandler(t *testing.T) {
	testETHClient := NewTestETHClient()
	assert.NotNil(t, testETHClient)

	t.Run("handler returns without error", func(t *testing.T) {
		expectedResult := []byte("pending result")
		handler := func(ctx context.Context, msg ethereum.CallMsg) ([]byte, error) {
			return expectedResult, nil
		}
		testETHClient.SetPendingCallContractHandler(handler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		result, err := testETHClient.PendingCallContract(ctx, ethereum.CallMsg{})
		assert.Nil(t, err)
		assert.Equal(t, result, expectedResult)
	})

	t.Run("handler returns with error", func(t *testing.T) {
		handler := func(ctx context.Context, msg ethereum.CallMsg) ([]byte, error) {
			return nil, ErrTestCallerExpectsErr
		}
		testETHClient.SetPendingCallContractHandler(handler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		result, err := testETHClient.PendingCallContract(ctx, ethereum.CallMsg{})
		assert.NotNil(t, err)
		assert.Nil(t, result)
	})
}

func TestTestETHClient_SetSuggestGasPriceHandler(t *testing.T) {
	testETHClient := NewTestETHClient()
	assert.NotNil(t, testETHClient)

	t.Run("handler returns without error", func(t *testing.T) {
		expectedGasPrice := big.NewInt(20000000000) // 20 gwei
		handler := func(ctx context.Context) (*big.Int, error) {
			return expectedGasPrice, nil
		}
		testETHClient.SetSuggestGasPriceHandler(handler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		gasPrice, err := testETHClient.SuggestGasPrice(ctx)
		assert.Nil(t, err)
		assert.True(t, gasPrice.Cmp(expectedGasPrice) == 0)
	})

	t.Run("handler returns with error", func(t *testing.T) {
		handler := func(ctx context.Context) (*big.Int, error) {
			return nil, ErrTestCallerExpectsErr
		}
		testETHClient.SetSuggestGasPriceHandler(handler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		gasPrice, err := testETHClient.SuggestGasPrice(ctx)
		assert.NotNil(t, err)
		assert.Nil(t, gasPrice)
	})
}

func TestTestETHClient_SetSuggestGasTipCapHandler(t *testing.T) {
	testETHClient := NewTestETHClient()
	assert.NotNil(t, testETHClient)

	t.Run("handler returns without error", func(t *testing.T) {
		expectedGasTipCap := big.NewInt(1500000000) // 1.5 gwei
		handler := func(ctx context.Context) (*big.Int, error) {
			return expectedGasTipCap, nil
		}
		testETHClient.SetSuggestGasTipCapHandler(handler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		gasTipCap, err := testETHClient.SuggestGasTipCap(ctx)
		assert.Nil(t, err)
		assert.True(t, gasTipCap.Cmp(expectedGasTipCap) == 0)
	})

	t.Run("handler returns with error", func(t *testing.T) {
		handler := func(ctx context.Context) (*big.Int, error) {
			return nil, ErrTestCallerExpectsErr
		}
		testETHClient.SetSuggestGasTipCapHandler(handler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		gasTipCap, err := testETHClient.SuggestGasTipCap(ctx)
		assert.NotNil(t, err)
		assert.Nil(t, gasTipCap)
	})
}
