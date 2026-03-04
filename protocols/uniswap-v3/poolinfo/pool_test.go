package poolinfo

import (
	"context"
	"fmt"
	"math/big"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	ethclients "github.com/defistate/defistate/clients/eth-clients"
	system "github.com/defistate/defistate/protocols/uniswap-v3"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mock Implementations ---

type mockPoolData struct {
	tick         int64
	liquidity    *big.Int
	sqrtPriceX96 *big.Int
	fee          uint64
}

type mockBatchDataSource struct {
	mu    sync.RWMutex
	pools map[common.Address]mockPoolData
}

func newMockBatchDataSource() *mockBatchDataSource {
	return &mockBatchDataSource{pools: make(map[common.Address]mockPoolData)}
}

func (ds *mockBatchDataSource) mockBatchProvider(
	ctx context.Context,
	poolAddrs []common.Address,
	_ func() (ethclients.ETHClient, error),
	blockNumber *big.Int,
) ([]int64, []*big.Int, []*big.Int, []uint64, []error) {
	num := len(poolAddrs)
	ticks := make([]int64, num)
	liqs := make([]*big.Int, num)
	prices := make([]*big.Int, num)
	fees := make([]uint64, num)
	errs := make([]error, num)

	ds.mu.RLock()
	defer ds.mu.RUnlock()

	for i, addr := range poolAddrs {
		if data, ok := ds.pools[addr]; ok {
			ticks[i] = data.tick
			liqs[i] = data.liquidity
			prices[i] = data.sqrtPriceX96
			fees[i] = data.fee
		} else {
			errs[i] = fmt.Errorf("pool not found: %s", addr.Hex())
		}
	}
	return ticks, liqs, prices, fees, errs
}

// --- Test Suite ---

func TestNewPoolInfoFunc_Comprehensive(t *testing.T) {
	ds := newMockBatchDataSource()
	var mockGetClient system.GetClientFunc = func() (ethclients.ETHClient, error) {
		return ethclients.NewTestETHClient(), nil
	}

	blockNumber := big.NewInt(1)
	// Setup 3 pools with distinct values
	p1, p2, p3 := common.HexToAddress("0x1"), common.HexToAddress("0x2"), common.HexToAddress("0x3")
	ds.pools[p1] = mockPoolData{tick: 100, liquidity: big.NewInt(10001), sqrtPriceX96: big.NewInt(10002), fee: 500}
	ds.pools[p2] = mockPoolData{tick: 200, liquidity: big.NewInt(20001), sqrtPriceX96: big.NewInt(20002), fee: 3000}
	ds.pools[p3] = mockPoolData{tick: 300, liquidity: big.NewInt(30001), sqrtPriceX96: big.NewInt(30002), fee: 100}

	t.Run("Full slice validation with jagged chunks", func(t *testing.T) {
		// Concurrency 2, ChunkSize 2.
		// For 3 pools, this results in Chunk A (P1, P2) and Chunk B (P3).
		getPoolInfo, _ := NewPoolInfoFunc(ds.mockBatchProvider, 2, 2)

		ticks, liqs, prices, fees, errs := getPoolInfo(
			context.Background(),
			[]common.Address{p1, p2, p3},
			mockGetClient,
			blockNumber,
		)

		require.Len(t, ticks, 3)
		require.Len(t, liqs, 3)
		require.Len(t, prices, 3)
		require.Len(t, fees, 3)
		require.Len(t, errs, 3)

		// Pool 1 (Chunk 1, Index 0)
		assert.NoError(t, errs[0])
		assert.Equal(t, int64(100), ticks[0])
		assert.Equal(t, uint64(10001), liqs[0].Uint64())
		assert.Equal(t, uint64(10002), prices[0].Uint64())
		assert.Equal(t, uint64(500), fees[0])

		// Pool 2 (Chunk 1, Index 1)
		assert.NoError(t, errs[1])
		assert.Equal(t, int64(200), ticks[1])
		assert.Equal(t, uint64(20001), liqs[1].Uint64())
		assert.Equal(t, uint64(20002), prices[1].Uint64())
		assert.Equal(t, uint64(3000), fees[1])

		// Pool 3 (Chunk 2, Index 2)
		assert.NoError(t, errs[2])
		assert.Equal(t, int64(300), ticks[2])
		assert.Equal(t, uint64(30001), liqs[2].Uint64())
		assert.Equal(t, uint64(30002), prices[2].Uint64())
		assert.Equal(t, uint64(100), fees[2])
	})

	t.Run("Concurrency Limit Verification", func(t *testing.T) {
		var activeCalls atomic.Int32
		var maxActive atomic.Int32
		limit := 2

		trackingProvider := func(ctx context.Context, addrs []common.Address, c func() (ethclients.ETHClient, error), blockNumber *big.Int) ([]int64, []*big.Int, []*big.Int, []uint64, []error) {
			current := activeCalls.Add(1)
			for {
				m := maxActive.Load()
				if current <= m || maxActive.CompareAndSwap(m, current) {
					break
				}
			}
			time.Sleep(10 * time.Millisecond) // Hold slot
			defer activeCalls.Add(-1)
			return ds.mockBatchProvider(ctx, addrs, c, blockNumber)
		}

		// 10 pools, chunkSize 1 = 10 chunks. Max 2 should run at once.
		getPoolInfo, _ := NewPoolInfoFunc(trackingProvider, limit, 1)
		manyPools := make([]common.Address, 10)
		for i := range manyPools {
			manyPools[i] = p1
		}

		_, _, _, _, _ = getPoolInfo(context.Background(), manyPools, mockGetClient, blockNumber)
		assert.LessOrEqual(t, maxActive.Load(), int32(limit), "Max concurrent chunks exceeded limit")
	})

	t.Run("Error propagation in batch", func(t *testing.T) {
		getPoolInfo, _ := NewPoolInfoFunc(ds.mockBatchProvider, 1, 5)

		// p1 exists, p4 does not
		p4 := common.HexToAddress("0x4")
		_, _, _, _, errs := getPoolInfo(context.Background(), []common.Address{p1, p4}, mockGetClient, blockNumber)

		assert.NoError(t, errs[0])
		assert.Error(t, errs[1])
		assert.Contains(t, errs[1].Error(), "pool not found")
	})
}
