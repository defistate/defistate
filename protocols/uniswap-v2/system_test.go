package uniswapv2

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"math/big"
	"sync"
	"testing"
	"time"

	ethclients "github.com/defistate/defistate/clients/eth-clients"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mock Infrastructure ---

// mockPersistence simulates the upstream database or service that stores permanent ID mappings.
type mockPersistence struct {
	mu             sync.Mutex
	tokenCounter   uint64
	poolCounter    uint64
	tokens         map[common.Address]uint64
	pools          map[common.Address]uint64
	idToPool       map[uint64]common.Address
	failOnRegister bool
}

func newMockPersistence() *mockPersistence {
	return &mockPersistence{
		tokenCounter: 100,
		poolCounter:  1000,
		tokens:       make(map[common.Address]uint64),
		idToPool:     make(map[uint64]common.Address),
		pools:        make(map[common.Address]uint64),
	}
}

func (p *mockPersistence) TokenAddressToID(addr common.Address) (uint64, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if id, ok := p.tokens[addr]; ok {
		return id, nil
	}
	p.tokenCounter++
	p.tokens[addr] = p.tokenCounter
	return p.tokenCounter, nil
}

func (p *mockPersistence) PoolAddressToID(addr common.Address) (uint64, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if id, ok := p.pools[addr]; ok {
		return id, nil
	}
	return 0, errors.New("mock: pool not found")
}

func (p *mockPersistence) PoolIDToAddress(id uint64) (common.Address, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if addr, ok := p.idToPool[id]; ok {
		return addr, nil
	}
	return common.Address{}, errors.New("mock: pool ID not found")
}

func (p *mockPersistence) RegisterPool(t0, t1, poolAddr common.Address) (uint64, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.failOnRegister {
		return 0, errors.New("mock: forced registration failure")
	}
	if _, ok := p.pools[poolAddr]; ok {
		return p.pools[poolAddr], nil
	}
	p.poolCounter++
	id := p.poolCounter
	p.pools[poolAddr] = id
	p.idToPool[id] = poolAddr
	return id, nil
}

func (p *mockPersistence) RegisterPools(t0s, t1s, poolAddrs []common.Address) ([]uint64, []error) {
	ids := make([]uint64, len(poolAddrs))
	errs := make([]error, len(poolAddrs))
	for i, addr := range poolAddrs {
		id, err := p.RegisterPool(t0s[i], t1s[i], addr)
		ids[i] = id
		errs[i] = err
	}
	return ids, errs
}

// --- Test Setup Helper ---

type systemTestConfig struct {
	poolInitializer     PoolInitializerFunc
	discoverPools       DiscoverPoolsFunc
	updatedInBlock      UpdatedInBlockFunc
	getReserves         GetReservesFunc
	testBloom           TestBloomFunc
	initFrequency       time.Duration
	maxInactiveDuration time.Duration
}

type testSystem struct {
	System       *UniswapV2System
	Persistence  *mockPersistence
	TestClient   *ethclients.TestETHClient
	BlockEventer chan *types.Block
	cancel       context.CancelFunc

	// Trackers for assertions
	mu             sync.Mutex
	capturedErrors []error
	deletedPools   []uint64
}

func (ts *testSystem) AddError(err error) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.capturedErrors = append(ts.capturedErrors, err)
}

func (ts *testSystem) GetErrors() []error {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	errsCopy := make([]error, len(ts.capturedErrors))
	copy(errsCopy, ts.capturedErrors)
	return errsCopy
}

func (ts *testSystem) TrackDeleted(ids []uint64) []error {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.deletedPools = append(ts.deletedPools, ids...)
	return nil
}

func (ts *testSystem) GetDeletedPools() []uint64 {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	cp := make([]uint64, len(ts.deletedPools))
	copy(cp, ts.deletedPools)
	return cp
}

func testSetupSystem(t *testing.T, cfg *systemTestConfig) *testSystem {
	ctx, cancel := context.WithCancel(context.Background())

	ts := &testSystem{
		Persistence:  newMockPersistence(),
		TestClient:   ethclients.NewTestETHClient(),
		BlockEventer: make(chan *types.Block, 50),
		cancel:       cancel,
	}

	if cfg == nil {
		cfg = &systemTestConfig{}
	}

	// Default implementations
	poolInitializerFunc := cfg.poolInitializer
	if poolInitializerFunc == nil {
		poolInitializerFunc = func(ctx context.Context, poolAddrs []common.Address, getClient func() (ethclients.ETHClient, error)) (token0s, token1s []common.Address, poolTypes []uint8, feeBps []uint16, reserve0s, reserve1s []*big.Int, errs []error) {
			token0s, token1s = make([]common.Address, len(poolAddrs)), make([]common.Address, len(poolAddrs))
			poolTypes, feeBps = make([]uint8, len(poolAddrs)), make([]uint16, len(poolAddrs))
			reserve0s, reserve1s, errs = make([]*big.Int, len(poolAddrs)), make([]*big.Int, len(poolAddrs)), make([]error, len(poolAddrs))
			for i, addr := range poolAddrs {
				var t0, t1 common.Address
				copy(t0[:], addr[:])
				t0[0] = 'a'
				copy(t1[:], addr[:])
				t1[0] = 'b'
				token0s[i], token1s[i], poolTypes[i], feeBps[i], reserve0s[i], reserve1s[i] = t0, t1, 0, 30, big.NewInt(100), big.NewInt(100)
			}
			return
		}
	}
	getReservesFunc := cfg.getReserves
	if getReservesFunc == nil {
		// Updated signature to include blockNumber
		getReservesFunc = func(ctx context.Context, poolAddrs []common.Address, getClient func() (ethclients.ETHClient, error), blockNumber *big.Int) (reserve0, reserve1 []*big.Int, errs []error) {
			reserve0, reserve1, errs = make([]*big.Int, len(poolAddrs)), make([]*big.Int, len(poolAddrs)), make([]error, len(poolAddrs))
			for i := range poolAddrs {
				reserve0[i], reserve1[i] = big.NewInt(100), big.NewInt(100)
			}
			return
		}
	}
	discoverPoolsFunc := cfg.discoverPools
	if discoverPoolsFunc == nil {
		discoverPoolsFunc = func(logs []types.Log) ([]common.Address, error) { return nil, nil }
	}
	updatedInBlockFunc := cfg.updatedInBlock
	if updatedInBlockFunc == nil {
		updatedInBlockFunc = func(logs []types.Log) (pools []common.Address, reserve0, reserve1 []*big.Int, err error) {
			return nil, nil, nil, nil
		}
	}
	testBloomFunc := cfg.testBloom
	if testBloomFunc == nil {
		testBloomFunc = func(b types.Bloom) bool { return true }
	}

	config := &Config{
		SystemName:          "test_system",
		PrometheusReg:       prometheus.NewRegistry(),
		NewBlockEventer:     ts.BlockEventer,
		GetClient:           func() (ethclients.ETHClient, error) { return ts.TestClient, nil },
		PoolInitializer:     poolInitializerFunc,
		DiscoverPools:       discoverPoolsFunc,
		UpdatedInBlock:      updatedInBlockFunc,
		GetReserves:         getReservesFunc,
		TokenAddressToID:    ts.Persistence.TokenAddressToID,
		PoolAddressToID:     ts.Persistence.PoolAddressToID,
		PoolIDToAddress:     ts.Persistence.PoolIDToAddress,
		RegisterPool:        ts.Persistence.RegisterPool,
		RegisterPools:       ts.Persistence.RegisterPools,
		OnDeletePools:       ts.TrackDeleted,
		ErrorHandler:        func(err error) { ts.AddError(err) },
		TestBloom:           testBloomFunc,
		FilterTopics:        [][]common.Hash{{common.HexToHash("0x1234")}},
		InitFrequency:       cfg.initFrequency,
		MaxInactiveDuration: cfg.maxInactiveDuration,
		Logger:              slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	// Initialize the live system
	sys, err := NewUniswapV2System(ctx, config)
	require.NoError(t, err)

	ts.System = sys

	return ts
}

func testNewBlock(number uint64, timestamp uint64) *types.Block {
	return types.NewBlock(&types.Header{
		Number: big.NewInt(int64(number)),
		Time:   timestamp,
	}, nil, nil, nil)
}

// --- Test Suite ---

func TestUniswapV2System(t *testing.T) {
	addr1 := common.HexToAddress("0x1")
	addr2 := common.HexToAddress("0x2")

	// Adjust this globally to ensure the background ticker triggers rapidly during tests
	DefaultPruneInactivePoolsTickerDuration = 50 * time.Millisecond

	t.Run("HappyPath_Initialization", func(t *testing.T) {
		cfg := &systemTestConfig{
			initFrequency: 10 * time.Millisecond,
			discoverPools: func(logs []types.Log) ([]common.Address, error) {
				if len(logs) > 0 && logs[0].BlockNumber == 1 {
					return []common.Address{addr1}, nil
				}
				return nil, nil
			},
			maxInactiveDuration: 5 * time.Minute,
		}
		ts := testSetupSystem(t, cfg)
		defer ts.cancel()

		ts.TestClient.SetFilterLogsHandler(func(ctx context.Context, q ethereum.FilterQuery) ([]types.Log, error) {
			return []types.Log{{BlockNumber: q.FromBlock.Uint64()}}, nil
		})

		ts.BlockEventer <- testNewBlock(1, 1000)
		require.Eventually(t, func() bool { return len(ts.System.View()) == 1 }, time.Second, 5*time.Millisecond, "pool should be initialized")
		assert.Empty(t, ts.GetErrors())
	})

	t.Run("HandleBlock_UpdatesReserves", func(t *testing.T) {
		cfg := &systemTestConfig{
			initFrequency: 10 * time.Millisecond,
			discoverPools: func(logs []types.Log) ([]common.Address, error) {
				if len(logs) > 0 && logs[0].BlockNumber == 1 {
					return []common.Address{addr1}, nil
				}
				return nil, nil
			},
			// Updated signature to include blockNumber
			getReserves: func(ctx context.Context, poolAddrs []common.Address, getClient func() (ethclients.ETHClient, error), blockNumber *big.Int) (reserve0, reserve1 []*big.Int, errs []error) {
				reserve0, reserve1, errs = make([]*big.Int, len(poolAddrs)), make([]*big.Int, len(poolAddrs)), make([]error, len(poolAddrs))
				for i := range poolAddrs {
					// Return 100 for block 1 initialization, 999 for block 2 updates
					if len(poolAddrs) == 1 {
						reserve0[i], reserve1[i] = big.NewInt(999), big.NewInt(999)
					} else {
						reserve0[i], reserve1[i] = big.NewInt(100), big.NewInt(100)
					}
				}
				return
			},
			maxInactiveDuration: 5 * time.Minute,
		}
		ts := testSetupSystem(t, cfg)
		defer ts.cancel()

		ts.TestClient.SetFilterLogsHandler(func(ctx context.Context, q ethereum.FilterQuery) ([]types.Log, error) {
			return []types.Log{{BlockNumber: q.FromBlock.Uint64()}}, nil
		})

		// Block 1: Discover and Init
		ts.BlockEventer <- testNewBlock(1, 1000)
		require.Eventually(t, func() bool { return len(ts.System.View()) == 1 }, time.Second, 5*time.Millisecond)

		// Block 2: Trigger the RPC call in handleNewBlock that pulls the 999 reserves
		ts.BlockEventer <- testNewBlock(2, 1050)

		// Wait for reserves to update in the external view
		require.Eventually(t, func() bool {
			view := ts.System.View()
			return len(view) == 1 && view[0].Reserve0.Cmp(big.NewInt(999)) == 0
		}, time.Second, 5*time.Millisecond, "reserves should be updated via view")
	})

	t.Run("Pruner_RemovesInactivePools_Naturally", func(t *testing.T) {
		// Set a real-world time to manipulate
		baseTime := time.Now()

		cfg := &systemTestConfig{
			initFrequency:       10 * time.Millisecond,
			maxInactiveDuration: 1 * time.Hour, // Pools inactive for > 1 hour get deleted
			discoverPools: func(logs []types.Log) ([]common.Address, error) {
				if len(logs) > 0 && logs[0].BlockNumber == 1 {
					return []common.Address{addr1, addr2}, nil
				}
				return nil, nil
			},
			updatedInBlock: func(logs []types.Log) (pools []common.Address, reserve0, reserve1 []*big.Int, err error) {
				if len(logs) == 0 {
					return nil, nil, nil, nil
				}
				if logs[0].BlockNumber == 2 {
					// Block 2 artificially touches ONLY pool 1, making it stale (T-2h)
					return []common.Address{addr1}, nil, nil, nil
				}
				if logs[0].BlockNumber == 3 {
					// Block 3 artificially touches ONLY pool 2, setting it to NOW
					return []common.Address{addr2}, nil, nil, nil
				}
				return nil, nil, nil, nil
			},
		}
		ts := testSetupSystem(t, cfg)
		defer ts.cancel()

		ts.TestClient.SetFilterLogsHandler(func(ctx context.Context, q ethereum.FilterQuery) ([]types.Log, error) {
			return []types.Log{{BlockNumber: q.FromBlock.Uint64()}}, nil
		})

		// Block 1: Discover both pools. (lastUpdatedAt = 0 for both, pruner ignores them)
		ts.BlockEventer <- testNewBlock(1, uint64(baseTime.Unix()))
		require.Eventually(t, func() bool { return len(ts.System.View()) == 2 }, time.Second, 5*time.Millisecond)

		poolID1, _ := ts.Persistence.PoolAddressToID(addr1)
		poolID2, _ := ts.Persistence.PoolAddressToID(addr2)

		// Block 2: Emit a block with a timestamp from 2 hours ago.
		// handleBlockLogs updates addr1's lastUpdatedAt to T-2h. addr2 stays at 0.
		twoHoursAgo := baseTime.Add(-2 * time.Hour)
		ts.BlockEventer <- testNewBlock(2, uint64(twoHoursAgo.Unix()))

		// Block 3: Emit a block with a timestamp of NOW.
		// handleBlockLogs updates addr2's lastUpdatedAt to NOW.
		ts.BlockEventer <- testNewBlock(3, uint64(baseTime.Unix()))

		// Wait for the background pruner to run. It will only target pool 1.
		require.Eventually(t, func() bool {
			return len(ts.System.View()) == 1
		}, 2*time.Second, 10*time.Millisecond, "pruner should have removed the inactive pool without internal manipulation")

		// Verify the correct pool was deleted
		view := ts.System.View()
		assert.Equal(t, poolID2, view[0].ID, "active pool (pool 2) should remain")

		// Verify the deletion callback was triggered correctly
		deleted := ts.GetDeletedPools()
		require.Contains(t, deleted, poolID1, "onDeletePools should be called with the stale pool ID")
		assert.NotContains(t, deleted, poolID2, "active pool should not be deleted")
	})
}

func TestNewUniswapV2SystemFromViews(t *testing.T) {
	t.Parallel()

	sourceView := []PoolView{
		{ID: 1001, Token0: 101, Token1: 102, Reserve0: big.NewInt(1000), Reserve1: big.NewInt(2000), Type: 1, FeeBps: 30},
		{ID: 1002, Token0: 101, Token1: 103, Reserve0: big.NewInt(3000), Reserve1: big.NewInt(4000), Type: 2, FeeBps: 5},
	}

	cfg := &Config{
		SystemName:      "test_from_view",
		PrometheusReg:   prometheus.NewRegistry(),
		NewBlockEventer: make(chan *types.Block),
		GetClient:       func() (ethclients.ETHClient, error) { return nil, errors.New("not implemented") },
		PoolInitializer: func(ctx context.Context, poolAddr []common.Address, getClient func() (ethclients.ETHClient, error)) (token0 []common.Address, token1 []common.Address, poolType []uint8, feeBps []uint16, reserve0 []*big.Int, reserve1 []*big.Int, errs []error) {
			return
		},
		DiscoverPools: func(l []types.Log) ([]common.Address, error) { return nil, nil },
		UpdatedInBlock: func(l []types.Log) (pools []common.Address, reserve0 []*big.Int, reserve1 []*big.Int, err error) {
			return
		},
		// Updated signature to include blockNumber
		GetReserves: func(ctx context.Context, poolAddrs []common.Address, getClient func() (ethclients.ETHClient, error), blockNumber *big.Int) (reserve0 []*big.Int, reserve1 []*big.Int, errs []error) {
			return
		},
		TokenAddressToID:    func(common.Address) (uint64, error) { return 0, errors.New("not implemented") },
		PoolAddressToID:     func(common.Address) (uint64, error) { return 0, errors.New("not implemented") },
		PoolIDToAddress:     func(uint64) (common.Address, error) { return common.Address{}, errors.New("not implemented") },
		RegisterPool:        func(token0, token1, poolAddr common.Address) (poolID uint64, err error) { return },
		RegisterPools:       func(token0s, token1s, poolAddrs []common.Address) (poolIDS []uint64, error []error) { return },
		OnDeletePools:       func(poolIDs []uint64) []error { return nil },
		ErrorHandler:        func(err error) { t.Errorf("unexpected error: %v", err) },
		TestBloom:           func(types.Bloom) bool { return false },
		FilterTopics:        [][]common.Hash{{}}, // Must not be empty to pass validation
		MaxInactiveDuration: 24 * time.Hour,
		Logger:              slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	system, err := NewUniswapV2SystemFromViews(ctx, cfg, sourceView)

	require.NoError(t, err)
	require.NotNil(t, system)

	currentView := system.View()
	assert.ElementsMatch(t, sourceView, currentView, "System's initial view should match the snapshot data")
}
