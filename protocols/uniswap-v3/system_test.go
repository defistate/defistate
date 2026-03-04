package uniswapv3

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"math/big"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	ethclients "github.com/defistate/defistate/clients/eth-clients"
	"github.com/defistate/defistate/protocols/uniswap-v3/ticks"
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
	failOnIDToAddr bool
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

func (p *mockPersistence) SetFailOnRegister(fail bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.failOnRegister = fail
}

func (p *mockPersistence) SetFailOnIDToAddress(fail bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.failOnIDToAddr = fail
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
	if p.failOnIDToAddr {
		return common.Address{}, errors.New("mock: forced ID to address failure")
	}
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
	if len(t0s) != len(t1s) || len(t0s) != len(poolAddrs) {
		panic("mismatched lengths in mock RegisterPools")
	}
	ids := make([]uint64, len(poolAddrs))
	errs := make([]error, len(poolAddrs))
	for i, addr := range poolAddrs {
		id, err := p.RegisterPool(t0s[i], t1s[i], addr)
		ids[i] = id
		errs[i] = err
	}
	return ids, errs
}

// mockTickIndexer provides a configurable, thread-safe mock for the TickIndexer interface.
type mockTickIndexer struct {
	t            *testing.T
	mu           sync.RWMutex
	pools        map[uint64][]ticks.TickInfo // Internal state to simulate the indexer's storage
	addShouldErr atomic.Bool
	addFunc      func(poolID uint64, address common.Address, spacing uint64) error
	getFunc      func(poolID uint64) ([]ticks.TickInfo, error)
	removeFunc   func(poolID uint64) error
}

func newMockTickIndexer(t *testing.T) *mockTickIndexer {
	m := &mockTickIndexer{
		t:     t,
		pools: make(map[uint64][]ticks.TickInfo),
	}
	// Set up default, in-memory behaviors
	m.OnAdd(func(poolID uint64, address common.Address, spacing uint64) error {
		if m.addShouldErr.Load() {
			return errors.New("mock tick indexer: forced add error")
		}
		m.mu.Lock()
		defer m.mu.Unlock()
		if _, ok := m.pools[poolID]; ok {
			return errors.New("mock tick indexer: pool already exists")
		}
		m.pools[poolID] = []ticks.TickInfo{} // Start with empty ticks
		return nil
	})
	m.OnGet(func(poolID uint64) ([]ticks.TickInfo, error) {
		m.mu.RLock()
		defer m.mu.RUnlock()
		if ts, ok := m.pools[poolID]; ok {
			// Deep copy to prevent race conditions in tests
			ticksCopy := make([]ticks.TickInfo, len(ts))
			for i, tick := range ts {
				ticksCopy[i] = ticks.TickInfo{
					Index:          tick.Index,
					LiquidityGross: new(big.Int).Set(tick.LiquidityGross),
					LiquidityNet:   new(big.Int).Set(tick.LiquidityNet),
				}
			}
			return ticksCopy, nil
		}
		return nil, errors.New("mock tick indexer: pool not found")
	})
	m.OnRemove(func(poolID uint64) error {
		m.mu.Lock()
		defer m.mu.Unlock()
		delete(m.pools, poolID)
		return nil
	})
	return m
}

func (m *mockTickIndexer) OnAdd(f func(poolID uint64, address common.Address, spacing uint64) error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.addFunc = f
}
func (m *mockTickIndexer) OnGet(f func(poolID uint64) ([]ticks.TickInfo, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.getFunc = f
}
func (m *mockTickIndexer) OnRemove(f func(poolID uint64) error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.removeFunc = f
}

func (m *mockTickIndexer) Add(poolID uint64, address common.Address, spacing uint64) error {
	m.mu.RLock()
	f := m.addFunc
	m.mu.RUnlock()
	if f == nil {
		m.t.Fatal("mockTickIndexer.Add was called but not configured")
	}
	return f(poolID, address, spacing)
}

func (m *mockTickIndexer) Get(poolID uint64) ([]ticks.TickInfo, error) {
	m.mu.RLock()
	f := m.getFunc
	m.mu.RUnlock()
	if f == nil {
		m.t.Fatal("mockTickIndexer.Get was called but not configured")
	}
	return f(poolID)
}

func (m *mockTickIndexer) Remove(poolID uint64) error {
	m.mu.RLock()
	f := m.removeFunc
	m.mu.RUnlock()
	if f == nil {
		m.t.Fatal("mockTickIndexer.Remove was called but not configured")
	}
	return f(poolID)
}
func (m *mockTickIndexer) LastUpdatedAtBlock() uint64 { return 0 }
func (m *mockTickIndexer) View() []ticks.TickView     { return nil }

// mockErrorHandler captures errors for inspection in tests.
type mockErrorHandler struct {
	mu   sync.Mutex
	errs []error
}

func (m *mockErrorHandler) Handle(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err != nil {
		m.errs = append(m.errs, err)
	}
}
func (m *mockErrorHandler) Count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.errs)
}
func (m *mockErrorHandler) GetErrors() []error {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Return a copy
	errsCopy := make([]error, len(m.errs))
	copy(errsCopy, m.errs)
	return errsCopy
}

// SystemTestHarness holds all the components and mock configurations for a test run.
type SystemTestHarness struct {
	System                   *UniswapV3System
	Persistence              *mockPersistence
	TickIndexer              *mockTickIndexer
	EthClient                *ethclients.TestETHClient
	ErrorHandler             *mockErrorHandler
	BlockEventer             chan *types.Block
	BlockedList              *mockBlockedList
	mu                       sync.RWMutex
	discoverPoolsFunc        func([]types.Log) ([]common.Address, error)
	swapsInBlockFunc         func(logs []types.Log) ([]common.Address, []int64, []*big.Int, []*big.Int, error)
	mintsAndBurnsInBlockFunc func(logs []types.Log) []common.Address
	poolInitializerFunc      PoolInitializerFunc
	getPoolInfoFunc          GetPoolInfoFunc
	testBloomFunc            func(types.Bloom) bool
	deletedPools             []uint64
}

func (h *SystemTestHarness) OnDiscoverPools(f func([]types.Log) ([]common.Address, error)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.discoverPoolsFunc = f
}
func (h *SystemTestHarness) OnSwapsInBlock(f func(logs []types.Log) ([]common.Address, []int64, []*big.Int, []*big.Int, error)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.swapsInBlockFunc = f
}
func (h *SystemTestHarness) OnMintsAndBurnsInBlock(f func(logs []types.Log) []common.Address) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.mintsAndBurnsInBlockFunc = f
}
func (h *SystemTestHarness) OnPoolInitializer(f PoolInitializerFunc) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.poolInitializerFunc = f
}
func (h *SystemTestHarness) OnGetPoolInfo(f GetPoolInfoFunc) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.getPoolInfoFunc = f
}

func (h *SystemTestHarness) OnTestBloom(f func(types.Bloom) bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.testBloomFunc = f
}
func (h *SystemTestHarness) TrackDeleted(ids []uint64) []error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.deletedPools = append(h.deletedPools, ids...)
	return nil
}
func (h *SystemTestHarness) GetDeletedPools() []uint64 {
	h.mu.RLock()
	defer h.mu.RUnlock()
	cp := make([]uint64, len(h.deletedPools))
	copy(cp, h.deletedPools)
	return cp
}

type mockBlockedList struct {
	mu      sync.Mutex
	blocked map[common.Address]struct{}
}

func (m *mockBlockedList) In(addr common.Address) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.blocked[addr]
	return ok
}
func (m *mockBlockedList) Add(addr common.Address) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.blocked[addr] = struct{}{}
}

func setupTest(t *testing.T, initFreq, pruneFreq, resyncFreq time.Duration) (context.Context, context.CancelFunc, *SystemTestHarness) {
	ctx, cancel := context.WithCancel(context.Background())

	persistence := newMockPersistence()
	tickIndexer := newMockTickIndexer(t)
	ethClient := ethclients.NewTestETHClient()
	errorHandler := &mockErrorHandler{}
	blockedList := &mockBlockedList{blocked: make(map[common.Address]struct{})}

	harness := &SystemTestHarness{
		Persistence:  persistence,
		TickIndexer:  tickIndexer,
		EthClient:    ethClient,
		ErrorHandler: errorHandler,
		BlockEventer: make(chan *types.Block, 10), // Increased buffer to prevent blocking in tests
		BlockedList:  blockedList,
	}

	harness.OnDiscoverPools(func(l []types.Log) ([]common.Address, error) { return nil, nil })
	harness.OnSwapsInBlock(func(l []types.Log) ([]common.Address, []int64, []*big.Int, []*big.Int, error) {
		return nil, nil, nil, nil, nil
	})
	harness.OnMintsAndBurnsInBlock(func(l []types.Log) []common.Address { return nil })
	harness.OnPoolInitializer(func(ctx context.Context, pa []common.Address, gcf GetClientFunc, blockNumber *big.Int) ([]common.Address, []common.Address, []uint64, []uint64, []int64, []*big.Int, []*big.Int, []error) {
		return nil, nil, nil, nil, nil, nil, nil, make([]error, len(pa))
	})

	harness.OnGetPoolInfo(func(ctx context.Context, addrs []common.Address, getClient GetClientFunc, blockNumber *big.Int) (ticks []int64, liquidities []*big.Int, sqrtPricesX96 []*big.Int, fees []uint64, errs []error) {
		count := len(addrs)
		ticks = make([]int64, count)
		liquidities = make([]*big.Int, count)
		sqrtPricesX96 = make([]*big.Int, count)
		fees = make([]uint64, count)
		for i := range addrs {
			liquidities[i] = big.NewInt(0)
			sqrtPricesX96[i] = big.NewInt(0)
		}
		return ticks, liquidities, sqrtPricesX96, fees, make([]error, count)
	})

	harness.OnTestBloom(func(b types.Bloom) bool { return true })
	ethClient.SetFilterLogsHandler(func(ctx context.Context, q ethereum.FilterQuery) ([]types.Log, error) {
		return nil, nil
	})

	cfg := &Config{
		SystemName:      "test-system",
		NewBlockEventer: harness.BlockEventer,
		GetClient:       func() (ethclients.ETHClient, error) { return ethClient, nil },
		PoolInitializer: func(ctx context.Context, addrs []common.Address, c GetClientFunc, blockNumber *big.Int) ([]common.Address, []common.Address, []uint64, []uint64, []int64, []*big.Int, []*big.Int, []error) {
			harness.mu.RLock()
			f := harness.poolInitializerFunc
			harness.mu.RUnlock()
			return f(ctx, addrs, c, blockNumber)
		},
		DiscoverPools: func(l []types.Log) ([]common.Address, error) {
			harness.mu.RLock()
			f := harness.discoverPoolsFunc
			harness.mu.RUnlock()
			return f(l)
		},
		SwapsInBlock: func(l []types.Log) ([]common.Address, []int64, []*big.Int, []*big.Int, error) {
			harness.mu.RLock()
			f := harness.swapsInBlockFunc
			harness.mu.RUnlock()
			if f == nil {
				return nil, nil, nil, nil, nil
			}
			return f(l)
		},
		MintsAndBurnsInBlock: func(l []types.Log) []common.Address {
			harness.mu.RLock()
			f := harness.mintsAndBurnsInBlockFunc
			harness.mu.RUnlock()
			return f(l)
		},

		GetPoolInfo: func(ctx context.Context, addrs []common.Address, c GetClientFunc, blockNumber *big.Int) ([]int64, []*big.Int, []*big.Int, []uint64, []error) {
			harness.mu.RLock()
			f := harness.getPoolInfoFunc
			harness.mu.RUnlock()
			return f(ctx, addrs, c, blockNumber)
		},
		TokenAddressToID: persistence.TokenAddressToID,
		PoolAddressToID:  persistence.PoolAddressToID,
		PoolIDToAddress:  persistence.PoolIDToAddress,
		RegisterPool:     persistence.RegisterPool,
		RegisterPools:    persistence.RegisterPools,
		OnDeletePools:    harness.TrackDeleted,
		ErrorHandler:     errorHandler.Handle,
		TestBloom: func(b types.Bloom) bool {
			harness.mu.RLock()
			f := harness.testBloomFunc
			harness.mu.RUnlock()
			return f(b)
		},
		FilterTopics:        [][]common.Hash{{common.HexToHash("0x1234")}},
		TickIndexer:         tickIndexer,
		InitFrequency:       initFreq,
		MaxInactiveDuration: pruneFreq, // Using pruneFreq for the inactivity threshold in these tests
		PrometheusReg:       prometheus.NewRegistry(),
		Logger:              slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	system, err := NewUniswapV3System(ctx, cfg)
	require.NoError(t, err)
	harness.System = system

	return ctx, cancel, harness
}

// --- Test Helper Functions ---

func newTestBlockWithTime(blockNumber uint64, ts time.Time) *types.Block {
	return types.NewBlock(&types.Header{
		Number: big.NewInt(int64(blockNumber)),
		Time:   uint64(ts.Unix()),
	}, nil, nil, nil)
}

func newTestBlock(blockNumber uint64) *types.Block {
	return newTestBlockWithTime(blockNumber, time.Now())
}

// --- Tests ---

func TestNewUniswapV3System_Validation(t *testing.T) {
	// Build a valid mock configuration purely using the harness dependencies.
	_, cancel, h := setupTest(t, time.Hour, time.Hour, time.Hour)
	defer cancel()

	validCfg := &Config{
		SystemName:      "test-system",
		PrometheusReg:   prometheus.NewRegistry(),
		NewBlockEventer: make(chan *types.Block),
		GetClient:       func() (ethclients.ETHClient, error) { return h.EthClient, nil },
		PoolInitializer: func(context.Context, []common.Address, GetClientFunc, *big.Int) ([]common.Address, []common.Address, []uint64, []uint64, []int64, []*big.Int, []*big.Int, []error) {
			return nil, nil, nil, nil, nil, nil, nil, nil
		},
		DiscoverPools: func([]types.Log) ([]common.Address, error) { return nil, nil },
		SwapsInBlock: func([]types.Log) ([]common.Address, []int64, []*big.Int, []*big.Int, error) {
			return nil, nil, nil, nil, nil
		},
		MintsAndBurnsInBlock: func([]types.Log) []common.Address { return nil },
		GetPoolInfo: func(context.Context, []common.Address, GetClientFunc, *big.Int) ([]int64, []*big.Int, []*big.Int, []uint64, []error) {
			return nil, nil, nil, nil, nil
		},
		TokenAddressToID: h.Persistence.TokenAddressToID,
		PoolAddressToID:  h.Persistence.PoolAddressToID,
		PoolIDToAddress:  h.Persistence.PoolIDToAddress,
		RegisterPool:     h.Persistence.RegisterPool,
		RegisterPools:    h.Persistence.RegisterPools,
		OnDeletePools:    h.TrackDeleted,
		ErrorHandler:     h.ErrorHandler.Handle,
		TestBloom:        func(types.Bloom) bool { return true },
		FilterTopics:     [][]common.Hash{{common.HexToHash("0x1234")}},
		TickIndexer:      h.TickIndexer,
		Logger:           slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	ctx := context.Background()
	testCases := []struct {
		name    string
		mutator func(c *Config)
	}{
		{"nil PrometheusReg", func(c *Config) { c.PrometheusReg = nil }},
		{"nil NewBlockEventer", func(c *Config) { c.NewBlockEventer = nil }},
		{"nil GetClient", func(c *Config) { c.GetClient = nil }},
		{"nil PoolInitializer", func(c *Config) { c.PoolInitializer = nil }},
		{"nil DiscoverPools", func(c *Config) { c.DiscoverPools = nil }},
		{"nil SwapsInBlock", func(c *Config) { c.SwapsInBlock = nil }},
		{"nil MintsAndBurnsInBlock", func(c *Config) { c.MintsAndBurnsInBlock = nil }},
		{"nil TokenAddressToID", func(c *Config) { c.TokenAddressToID = nil }},
		{"nil PoolAddressToID", func(c *Config) { c.PoolAddressToID = nil }},
		{"nil PoolIDToAddress", func(c *Config) { c.PoolIDToAddress = nil }},
		{"nil RegisterPool", func(c *Config) { c.RegisterPool = nil }},
		{"nil ErrorHandler", func(c *Config) { c.ErrorHandler = nil }},
		{"nil TestBloom", func(c *Config) { c.TestBloom = nil }},
		{"nil TickIndexer", func(c *Config) { c.TickIndexer = nil }},
	}

	for _, tc := range testCases {
		t.Run("should fail with "+tc.name, func(t *testing.T) {
			// Shallow copy the valid config to avoid mutating other tests
			cfgCopy := *validCfg
			tc.mutator(&cfgCopy)
			_, err := NewUniswapV3System(ctx, &cfgCopy)
			assert.Error(t, err)
		})
	}
}

func TestUniswapV3System_Lifecycle(t *testing.T) {
	t.Run("should initialize new pools", func(t *testing.T) {
		// --- Setup ---
		_, cancel, h := setupTest(t, 10*time.Millisecond, time.Hour, time.Hour)
		defer cancel()

		// --- Test Data Definitions ---
		token0Addr := common.HexToAddress("0x1")
		token1Addr := common.HexToAddress("0x2")
		poolAddr := common.HexToAddress("0x1000")

		expectedPoolID := uint64(1001) // mockPersistence starts pool counter at 1000
		expectedToken0ID := uint64(101)
		expectedToken1ID := uint64(102)
		expectedTick := int64(85000)
		expectedLiquidity := big.NewInt(1e18)
		expectedSqrtPrice := new(big.Int).Lsh(big.NewInt(1), 96)
		expectedFee := uint64(3000)
		expectedSpacing := uint64(60)

		expectedTickInfo := []ticks.TickInfo{
			{Index: 123, LiquidityGross: big.NewInt(1000), LiquidityNet: big.NewInt(-1000)},
		}

		// --- Mock Configuration ---
		h.OnDiscoverPools(func(l []types.Log) ([]common.Address, error) {
			return []common.Address{poolAddr}, nil
		})
		h.OnPoolInitializer(func(ctx context.Context, addrs []common.Address, c GetClientFunc, blockNumber *big.Int) (
			[]common.Address, []common.Address, []uint64, []uint64, []int64, []*big.Int, []*big.Int, []error) {
			return []common.Address{token0Addr},
				[]common.Address{token1Addr},
				[]uint64{expectedFee},
				[]uint64{expectedSpacing},
				[]int64{expectedTick},
				[]*big.Int{expectedLiquidity},
				[]*big.Int{expectedSqrtPrice},
				[]error{nil}
		})

		h.TickIndexer.OnGet(func(poolID uint64) ([]ticks.TickInfo, error) {
			if poolID == expectedPoolID {
				return expectedTickInfo, nil
			}
			return nil, errors.New("unexpected pool ID")
		})

		// --- Action ---
		h.BlockEventer <- newTestBlock(1)

		// --- Assertions ---
		require.Eventually(t, func() bool {
			view := h.System.View()
			if len(view) != 1 {
				return false
			}
			return view[0].ID == expectedPoolID
		}, 1*time.Second, 10*time.Millisecond, "system view should eventually contain the new pool")

		view := h.System.View()
		require.Len(t, view, 1)
		poolView := view[0]

		assert.Equal(t, expectedPoolID, poolView.ID)
		assert.Equal(t, expectedToken0ID, poolView.Token0)
		assert.Equal(t, expectedToken1ID, poolView.Token1)
		assert.Equal(t, expectedTick, poolView.Tick)
		assert.Equal(t, expectedFee, poolView.Fee)
		assert.Equal(t, expectedSpacing, poolView.TickSpacing)
		assert.Zero(t, expectedLiquidity.Cmp(poolView.Liquidity))
		assert.Zero(t, expectedSqrtPrice.Cmp(poolView.SqrtPriceX96))
		require.Len(t, poolView.Ticks, 1)
		assert.Equal(t, expectedTickInfo[0].Index, poolView.Ticks[0].Index)
		assert.Zero(t, h.ErrorHandler.Count())
	})
}

func TestUniswapV3System_ErrorHandling(t *testing.T) {
	t.Run("should handle partial initializer failure", func(t *testing.T) {
		// --- Setup ---
		_, cancel, h := setupTest(t, 10*time.Millisecond, time.Hour, time.Hour)
		defer cancel()

		goodPool := common.HexToAddress("0x1000")
		badPool := common.HexToAddress("0x2000")

		h.OnDiscoverPools(func(l []types.Log) ([]common.Address, error) {
			return []common.Address{goodPool, badPool}, nil
		})
		h.OnPoolInitializer(func(ctx context.Context, addrs []common.Address, c GetClientFunc, blockNumber *big.Int) (
			[]common.Address, []common.Address, []uint64, []uint64, []int64, []*big.Int, []*big.Int, []error) {

			count := len(addrs)
			t0s, t1s := make([]common.Address, count), make([]common.Address, count)
			fees, spacings := make([]uint64, count), make([]uint64, count)
			ticks, liqs, prices := make([]int64, count), make([]*big.Int, count), make([]*big.Int, count)
			errs := make([]error, count)

			for i, addr := range addrs {
				if addr == goodPool {
					t0s[i], t1s[i] = common.HexToAddress("0x1"), common.HexToAddress("0x2")
					liqs[i], prices[i] = big.NewInt(1), big.NewInt(1)
					errs[i] = nil
				} else if addr == badPool {
					errs[i] = errors.New("forced initializer failure")
				}
			}
			return t0s, t1s, fees, spacings, ticks, liqs, prices, errs
		})

		// --- Action ---
		h.BlockEventer <- newTestBlock(1)

		// --- Assertions ---
		require.Eventually(t, func() bool {
			return h.ErrorHandler.Count() == 1
		}, time.Second, 10*time.Millisecond, "error handler should be called once for the failed pool")

		require.Eventually(t, func() bool {
			view := h.System.View()
			return len(view) == 1
		}, time.Second, 10*time.Millisecond, "only the good pool should be in the view")

		errs := h.ErrorHandler.GetErrors()
		require.Len(t, errs, 1)
		var initErr *InitializationError
		require.ErrorAs(t, errs[0], &initErr, "error should be of type InitializationError")
		assert.Equal(t, badPool, initErr.PoolAddress)
		assert.ErrorContains(t, initErr.Err, "forced initializer failure")
	})

	t.Run("should remove pool if tick indexer fails to add it", func(t *testing.T) {
		// --- Setup ---
		_, cancel, h := setupTest(t, 10*time.Millisecond, time.Hour, time.Hour)
		defer cancel()

		poolAddr := common.HexToAddress("0x1000")
		h.TickIndexer.addShouldErr.Store(true) // Configure tick indexer to fail on Add()

		h.OnDiscoverPools(func(l []types.Log) ([]common.Address, error) { return []common.Address{poolAddr}, nil })
		h.OnPoolInitializer(func(ctx context.Context, addrs []common.Address, c GetClientFunc, blockNumber *big.Int) (
			[]common.Address, []common.Address, []uint64, []uint64, []int64, []*big.Int, []*big.Int, []error) {
			return []common.Address{common.HexToAddress("0x1")}, []common.Address{common.HexToAddress("0x2")}, []uint64{3000}, []uint64{60}, []int64{0}, []*big.Int{big.NewInt(0)}, []*big.Int{big.NewInt(0)}, []error{nil}
		})

		// --- Action ---
		h.BlockEventer <- newTestBlock(1)

		// --- Assertions ---
		require.Eventually(t, func() bool {
			return h.ErrorHandler.Count() > 0
		}, time.Second, 10*time.Millisecond, "error handler should have been called")

		assert.Empty(t, h.System.View(), "view should be empty after rollback")

		errs := h.ErrorHandler.GetErrors()
		require.Len(t, errs, 1)
		var tickErr *TickIndexingError
		require.ErrorAs(t, errs[0], &tickErr, "error should be of type TickIndexingError")
		assert.Equal(t, poolAddr, tickErr.PoolAddress)
		assert.Equal(t, "Add", tickErr.Operation)
		assert.ErrorContains(t, tickErr.Err, "forced add error")
	})

	t.Run("should logger RegistrationError on persistence failure", func(t *testing.T) {
		// --- Setup ---
		_, cancel, h := setupTest(t, 10*time.Millisecond, time.Hour, time.Hour)
		defer cancel()

		poolAddr := common.HexToAddress("0x1000")
		token0Addr := common.HexToAddress("0x1")
		token1Addr := common.HexToAddress("0x2")

		h.Persistence.SetFailOnRegister(true)
		h.OnDiscoverPools(func(l []types.Log) ([]common.Address, error) { return []common.Address{poolAddr}, nil })
		h.OnPoolInitializer(func(ctx context.Context, addrs []common.Address, c GetClientFunc, blockNumber *big.Int) (
			[]common.Address, []common.Address, []uint64, []uint64, []int64, []*big.Int, []*big.Int, []error) {
			return []common.Address{token0Addr}, []common.Address{token1Addr}, []uint64{3000}, []uint64{60}, []int64{0}, []*big.Int{big.NewInt(0)}, []*big.Int{big.NewInt(0)}, []error{nil}
		})

		// --- Action ---
		h.BlockEventer <- newTestBlock(1)

		// --- Assertions ---
		require.Eventually(t, func() bool { return h.ErrorHandler.Count() > 0 }, time.Second, 10*time.Millisecond)

		errs := h.ErrorHandler.GetErrors()
		require.Len(t, errs, 1)
		var regErr *RegistrationError
		require.ErrorAs(t, errs[0], &regErr, "error should be of type RegistrationError")
		assert.Equal(t, poolAddr, regErr.PoolAddress)
		assert.Equal(t, token0Addr, regErr.Token0Address)
		assert.Equal(t, token1Addr, regErr.Token1Address)
		assert.ErrorContains(t, regErr.Err, "forced registration failure")
		assert.Empty(t, h.System.View(), "view should be empty after registration failure")
	})
}

func TestUniswapV3System_Pruning(t *testing.T) {
	// Globally override the pruner ticker so it triggers rapidly during the test
	DefaultPruneInactivePoolsTickerDuration = 20 * time.Millisecond

	t.Run("should naturally prune inactive pools via maxInactiveDuration", func(t *testing.T) {
		// --- Setup ---
		// We set maxInactiveDuration to 1 Hour.
		_, cancel, h := setupTest(t, 10*time.Millisecond, 1*time.Hour, time.Hour)
		defer cancel()

		poolAddr := common.HexToAddress("0x1000")
		baseTime := time.Now()

		// Provide the mock logs so OnDiscoverPools and OnSwapsInBlock can read the BlockNumber
		h.EthClient.SetFilterLogsHandler(func(ctx context.Context, q ethereum.FilterQuery) ([]types.Log, error) {
			return []types.Log{{BlockNumber: q.FromBlock.Uint64()}}, nil
		})

		// 1. Initialize a single pool.
		h.OnDiscoverPools(func(l []types.Log) ([]common.Address, error) {
			if len(l) > 0 && l[0].BlockNumber == 1 {
				return []common.Address{poolAddr}, nil
			}
			return nil, nil
		})
		h.OnPoolInitializer(func(ctx context.Context, addrs []common.Address, c GetClientFunc, blockNumber *big.Int) (
			[]common.Address, []common.Address, []uint64, []uint64, []int64, []*big.Int, []*big.Int, []error) {
			return []common.Address{common.HexToAddress("0x1")}, []common.Address{common.HexToAddress("0x2")}, []uint64{3000}, []uint64{60}, []int64{0}, []*big.Int{big.NewInt(0)}, []*big.Int{big.NewInt(0)}, []error{nil}
		})

		// Emit Block 1 to initialize the pool
		h.BlockEventer <- newTestBlockWithTime(1, baseTime)

		require.Eventually(t, func() bool {
			return len(h.System.View()) == 1
		}, time.Second, 10*time.Millisecond, "pool should be fully initialized before aging")

		poolID, _ := h.Persistence.PoolAddressToID(poolAddr)

		// Ensure it's in the TickIndexer
		_, err := h.TickIndexer.Get(poolID)
		require.NoError(t, err, "pool must exist in TickIndexer before we attempt pruning")

		// 2. Emit Block 2 with a timestamp 2 hours in the past.
		// We trigger a mock swap event so the system registers activity and sets lastUpdatedAt.
		twoHoursAgo := baseTime.Add(-2 * time.Hour)
		h.OnSwapsInBlock(func(l []types.Log) ([]common.Address, []int64, []*big.Int, []*big.Int, error) {
			if len(l) > 0 && l[0].BlockNumber == 2 {
				return []common.Address{poolAddr}, []int64{100}, []*big.Int{big.NewInt(1)}, []*big.Int{big.NewInt(1)}, nil
			}
			return nil, nil, nil, nil, nil
		})

		// This block makes the system flag the pool as last active 2 hours ago.
		h.BlockEventer <- newTestBlockWithTime(2, twoHoursAgo)

		// 3. Wait for the background pruner to notice the stale pool and delete it.
		// Since T-2h exceeds the 1 Hour limit, the view should naturally drop to 0.
		require.Eventually(t, func() bool {
			return len(h.System.View()) == 0
		}, 2*time.Second, 20*time.Millisecond, "view should become empty after pruner removes the inactive pool")

		// 4. Verification: Check that it was deleted from the TickIndexer
		_, err = h.TickIndexer.Get(poolID)
		assert.Error(t, err, "pool should have been removed from the TickIndexer during pruning")

		// Verification: Check the deletion callback
		deleted := h.GetDeletedPools()
		require.Contains(t, deleted, poolID, "onDeletePools should be triggered")
	})

	t.Run("should capture error if tick indexer removal fails during pruning", func(t *testing.T) {
		// --- Setup ---
		_, cancel, h := setupTest(t, 10*time.Millisecond, 1*time.Hour, time.Hour)
		defer cancel()

		poolAddr := common.HexToAddress("0x2000")
		baseTime := time.Now()

		// Provide the mock logs here as well
		h.EthClient.SetFilterLogsHandler(func(ctx context.Context, q ethereum.FilterQuery) ([]types.Log, error) {
			return []types.Log{{BlockNumber: q.FromBlock.Uint64()}}, nil
		})

		h.OnDiscoverPools(func(l []types.Log) ([]common.Address, error) {
			if len(l) > 0 && l[0].BlockNumber == 1 {
				return []common.Address{poolAddr}, nil
			}
			return nil, nil
		})
		h.OnPoolInitializer(func(ctx context.Context, addrs []common.Address, c GetClientFunc, blockNumber *big.Int) (
			[]common.Address, []common.Address, []uint64, []uint64, []int64, []*big.Int, []*big.Int, []error) {
			return []common.Address{common.HexToAddress("0x1")}, []common.Address{common.HexToAddress("0x2")}, []uint64{3000}, []uint64{60}, []int64{0}, []*big.Int{big.NewInt(0)}, []*big.Int{big.NewInt(0)}, []error{nil}
		})

		// Init pool
		h.BlockEventer <- newTestBlockWithTime(1, baseTime)
		require.Eventually(t, func() bool {
			return len(h.System.View()) == 1
		}, time.Second, 10*time.Millisecond)

		poolID, _ := h.Persistence.PoolAddressToID(poolAddr)

		// Force the tick indexer to fail when the pruner attempts removal
		expectedErr := errors.New("forced tick indexer removal failure")
		h.TickIndexer.OnRemove(func(pID uint64) error {
			if pID == poolID {
				return expectedErr
			}
			return nil
		})

		// Trigger update with a stale timestamp
		staleTime := baseTime.Add(-2 * time.Hour)
		h.OnSwapsInBlock(func(l []types.Log) ([]common.Address, []int64, []*big.Int, []*big.Int, error) {
			if len(l) > 0 && l[0].BlockNumber == 2 {
				return []common.Address{poolAddr}, []int64{100}, []*big.Int{big.NewInt(1)}, []*big.Int{big.NewInt(1)}, nil
			}
			return nil, nil, nil, nil, nil
		})

		h.BlockEventer <- newTestBlockWithTime(2, staleTime)

		// The pruner will remove it from the internal registry, updating the view to length 0.
		require.Eventually(t, func() bool {
			return len(h.System.View()) == 0
		}, 2*time.Second, 20*time.Millisecond, "view should become empty despite partial failure")

		// The tick indexer failure during pruning should trigger the error handler
		require.Eventually(t, func() bool {
			return h.ErrorHandler.Count() > 0
		}, 1*time.Second, 10*time.Millisecond)

		errs := h.ErrorHandler.GetErrors()
		require.Len(t, errs, 1)
		assert.ErrorContains(t, errs[0], "failed to remove from tick indexer")
		assert.ErrorContains(t, errs[0], expectedErr.Error())
	})
}
