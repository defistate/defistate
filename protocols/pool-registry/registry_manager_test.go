package poolregistry

import (
	"errors"
	"sync"
	"testing"

	"github.com/defistate/defistate/engine"
	token "github.com/defistate/defistate/protocols/erc20-token-system"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mock Implementations ---

// mockTokenSystem remains largely the same as tokens are still addressed by common.Address
type mockTokenSystem struct {
	mu     sync.RWMutex
	tokens map[common.Address]token.TokenView
}

func newMockTokenSystem() *mockTokenSystem {
	return &mockTokenSystem{
		tokens: make(map[common.Address]token.TokenView),
	}
}

func (m *mockTokenSystem) GetTokenByAddress(addr common.Address) (token.TokenView, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if t, ok := m.tokens[addr]; ok {
		return t, nil
	}
	return token.TokenView{}, errors.New("token not found")
}

func (m *mockTokenSystem) AddToken(addr common.Address, name, symbol string, decimals uint8) (uint64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if t, ok := m.tokens[addr]; ok {
		return t.ID, nil
	}
	newID := uint64(len(m.tokens) + 1)
	m.tokens[addr] = token.TokenView{ID: newID, Address: addr, Name: name, Symbol: symbol, Decimals: decimals}
	return newID, nil
}

// mockPoolSystem updated to use PoolKey and engine.ProtocolID
type mockPoolSystem struct {
	mu                 sync.RWMutex
	pools              map[PoolKey]uint64
	nextID             uint64
	deleted            map[uint64]bool
	blocklist          map[PoolKey]struct{}
	idAddressToPoolKey map[uint64]PoolKey
	idToProto          map[uint64]engine.ProtocolID
}

func newMockPoolSystem() *mockPoolSystem {
	return &mockPoolSystem{
		pools:              make(map[PoolKey]uint64),
		nextID:             1,
		deleted:            make(map[uint64]bool),
		blocklist:          make(map[PoolKey]struct{}),
		idAddressToPoolKey: make(map[uint64]PoolKey),
		idToProto:          make(map[uint64]engine.ProtocolID),
	}
}

func (m *mockPoolSystem) AddPool(key PoolKey, pID engine.ProtocolID) (uint64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.blocklist[key]; ok {
		return 0, ErrPoolBlocked
	}
	if id, ok := m.pools[key]; ok {
		return id, nil
	}
	id := m.nextID
	m.pools[key] = id
	m.idAddressToPoolKey[id] = key
	m.idToProto[id] = pID
	m.nextID++
	return id, nil
}

func (m *mockPoolSystem) AddPools(keys []PoolKey, pIDs []engine.ProtocolID) ([]uint64, []error) {
	if len(keys) != len(pIDs) {
		panic("mismatched lengths")
	}
	ids := make([]uint64, len(keys))
	errs := make([]error, len(keys))
	var hasError bool
	for i, key := range keys {
		id, err := m.AddPool(key, pIDs[i])
		if err != nil {
			errs[i] = err
			hasError = true
		} else {
			ids[i] = id
		}
	}
	if hasError {
		return ids, errs
	}
	return ids, nil
}

func (m *mockPoolSystem) DeletePool(poolID uint64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key, ok := m.idAddressToPoolKey[poolID]
	if !ok {
		return errors.New("pool not found")
	}
	delete(m.pools, key)
	delete(m.idAddressToPoolKey, poolID)
	delete(m.idToProto, poolID)
	m.deleted[poolID] = true
	return nil
}

func (m *mockPoolSystem) DeletePools(poolIDs []uint64) []error {
	errs := make([]error, len(poolIDs))
	var hasError bool
	for i, id := range poolIDs {
		err := m.DeletePool(id)
		if err != nil {
			errs[i] = err
			hasError = true
		}
	}
	if hasError {
		return errs
	}
	return nil
}

// Other methods to satisfy the interface
func (m *mockPoolSystem) AddToBlockList(key PoolKey)      { m.blocklist[key] = struct{}{} }
func (m *mockPoolSystem) RemoveFromBlockList(key PoolKey) { delete(m.blocklist, key) }
func (m *mockPoolSystem) IsOnBlockList(key PoolKey) bool {
	_, ok := m.blocklist[key]
	return ok
}
func (m *mockPoolSystem) GetID(key PoolKey) (uint64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	id, ok := m.pools[key]
	if !ok {
		return 0, errors.New("not found")
	}
	return id, nil
}
func (m *mockPoolSystem) GetKey(id uint64) (PoolKey, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	key, ok := m.idAddressToPoolKey[id]
	if !ok {
		return PoolKey{}, errors.New("not found")
	}
	return key, nil
}
func (m *mockPoolSystem) GetProtocolID(id uint64) (engine.ProtocolID, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	pID, ok := m.idToProto[id]
	if !ok {
		return "", errors.New("not found")
	}
	return pID, nil
}
func (m *mockPoolSystem) View() PoolRegistryView { return PoolRegistryView{} }

// mockTokenPoolSystem remains largely the same
type mockTokenPoolSystem struct {
	mu         sync.RWMutex
	tokenPools map[uint64]map[uint64]struct{}
}

func newMockTokenPoolSystem() *mockTokenPoolSystem {
	return &mockTokenPoolSystem{
		tokenPools: make(map[uint64]map[uint64]struct{}),
	}
}
func (m *mockTokenPoolSystem) AddPool(tokenIDs []uint64, poolID uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, tokenID := range tokenIDs {
		if _, ok := m.tokenPools[tokenID]; !ok {
			m.tokenPools[tokenID] = make(map[uint64]struct{})
		}
		m.tokenPools[tokenID][poolID] = struct{}{}
	}
}
func (m *mockTokenPoolSystem) AddPools(poolIDs []uint64, tokenIDSets [][]uint64) {
	if len(poolIDs) != len(tokenIDSets) {
		panic("mismatched lengths")
	}
	for i, poolID := range poolIDs {
		m.AddPool(tokenIDSets[i], poolID)
	}
}
func (m *mockTokenPoolSystem) RemovePool(poolID uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for tokenID, poolSet := range m.tokenPools {
		delete(poolSet, poolID)
		if len(poolSet) == 0 {
			delete(m.tokenPools, tokenID)
		}
	}
}
func (m *mockTokenPoolSystem) RemovePools(poolIDs []uint64) {
	for _, id := range poolIDs {
		m.RemovePool(id)
	}
}
func (m *mockTokenPoolSystem) PoolsForToken(tokenID uint64) []uint64 { return nil }
func (m *mockTokenPoolSystem) View() *TokenPoolsRegistryView         { return nil }

// --- Test Suite ---

func TestRegistryManager(t *testing.T) {
	addrWETH := common.HexToAddress("0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2")
	addrUSDC := common.HexToAddress("0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48")
	addrDAI := common.HexToAddress("0x6B175474E89094C44Da98b954EedeAC495271d0F")

	keyPool1 := AddressToPoolKey(common.HexToAddress("0x1001"))
	keyPool2 := AddressToPoolKey(common.HexToAddress("0x1002"))
	keyPool3 := AddressToPoolKey(common.HexToAddress("0x1003"))

	protoUni := engine.ProtocolID("uniswap")
	protoCurve := engine.ProtocolID("curve")
	protoSushi := engine.ProtocolID("sushi")

	t.Run("RegisterPool_Success", func(t *testing.T) {
		mockTS, mockPS, mockTPS, manager := setupMocksAndManager()
		mockTS.AddToken(addrWETH, "WETH", "WETH", 18)
		mockTS.AddToken(addrUSDC, "USDC", "USDC", 6)

		poolID, tokenIDs, err := manager.RegisterPool(keyPool1, []common.Address{addrWETH, addrUSDC}, protoUni)
		require.NoError(t, err)
		assert.Equal(t, uint64(1), poolID)
		require.Len(t, tokenIDs, 2)
		assert.Len(t, mockPS.pools, 1)

		mockTPS.mu.RLock()
		defer mockTPS.mu.RUnlock()
		assert.Contains(t, mockTPS.tokenPools[1], poolID)
		assert.Contains(t, mockTPS.tokenPools[2], poolID)
	})

	t.Run("RegisterPools_Success", func(t *testing.T) {
		mockTS, mockPS, mockTPS, manager := setupMocksAndManager()
		idWETH, _ := mockTS.AddToken(addrWETH, "WETH", "WETH", 18)
		idUSDC, _ := mockTS.AddToken(addrUSDC, "USDC", "USDC", 6)
		idDAI, _ := mockTS.AddToken(addrDAI, "DAI", "DAI", 18)

		poolIDs, tokenIDSets, errs := manager.RegisterPools(
			[]PoolKey{keyPool1, keyPool2},
			[][]common.Address{{addrWETH, addrUSDC}, {addrWETH, addrDAI}},
			[]engine.ProtocolID{protoUni, protoCurve},
		)
		require.Nil(t, errs)
		require.Equal(t, []uint64{1, 2}, poolIDs)
		require.Equal(t, [][]uint64{{idWETH, idUSDC}, {idWETH, idDAI}}, tokenIDSets)

		assert.Len(t, mockPS.pools, 2)
		poolID1, _ := mockPS.GetID(keyPool1)
		poolID2, _ := mockPS.GetID(keyPool2)

		mockTPS.mu.RLock()
		defer mockTPS.mu.RUnlock()
		assert.Contains(t, mockTPS.tokenPools[idWETH], poolID1)
		assert.Contains(t, mockTPS.tokenPools[idUSDC], poolID1)
		assert.Contains(t, mockTPS.tokenPools[idWETH], poolID2)
		assert.Contains(t, mockTPS.tokenPools[idDAI], poolID2)
	})

	t.Run("RegisterPools_PartialFailure", func(t *testing.T) {
		mockTS, mockPS, mockTPS, manager := setupMocksAndManager()
		idWETH, _ := mockTS.AddToken(addrWETH, "WETH", "WETH", 18)
		idUSDC, _ := mockTS.AddToken(addrUSDC, "USDC", "USDC", 6)
		// DAI is not added to the token system

		// Pool 2 is on the blocklist
		mockPS.AddToBlockList(keyPool2)

		poolIDs, tokenIDSets, errs := manager.RegisterPools(
			[]PoolKey{keyPool1, keyPool2, keyPool3},
			[][]common.Address{{addrWETH, addrUSDC}, {addrWETH, addrUSDC}, {addrWETH, addrDAI}},
			[]engine.ProtocolID{protoUni, protoUni, protoSushi},
		)

		// Assertions
		require.NotNil(t, errs)
		require.Len(t, errs, 3)
		assert.Nil(t, errs[0], "Pool 1 should be registered successfully")
		assert.NotNil(t, errs[1], "Pool 2 should fail because it's on the blocklist")
		assert.NotNil(t, errs[2], "Pool 3 should fail because DAI token is missing")

		require.Len(t, poolIDs, 3)
		assert.NotEqual(t, uint64(0), poolIDs[0], "Pool 1 should have a valid ID")
		assert.Equal(t, uint64(0), poolIDs[1], "Pool 2 should have a zero ID")
		assert.Equal(t, uint64(0), poolIDs[2], "Pool 3 should have a zero ID")

		require.Len(t, tokenIDSets, 3)
		assert.Equal(t, []uint64{idWETH, idUSDC}, tokenIDSets[0], "Token IDs for pool 1 should be returned")
		assert.Nil(t, tokenIDSets[1], "Token IDs for failed pool 2 should be nil")
		assert.Nil(t, tokenIDSets[2], "Token IDs for failed pool 3 should be nil")

		// Check final state
		assert.Len(t, mockPS.pools, 1, "Only one pool should have been successfully created")
		poolID1, _ := mockPS.GetID(keyPool1)
		assert.Equal(t, uint64(1), poolID1)

		mockTPS.mu.RLock()
		defer mockTPS.mu.RUnlock()
		assert.Len(t, mockTPS.tokenPools, 2, "Only relationships for the successful pool should exist")
		assert.Contains(t, mockTPS.tokenPools[idWETH], poolID1)
		assert.Contains(t, mockTPS.tokenPools[idUSDC], poolID1)
	})

	t.Run("DeletePools_Success", func(t *testing.T) {
		_, mockPS, mockTPS, manager := setupMocksAndManager()
		poolID1, _ := mockPS.AddPool(keyPool1, protoUni)
		poolID2, _ := mockPS.AddPool(keyPool2, protoUni)
		mockTPS.AddPool([]uint64{1, 2}, poolID1)
		mockTPS.AddPool([]uint64{1, 3}, poolID2)

		errs := manager.DeletePools([]uint64{poolID1, poolID2})
		require.Nil(t, errs)
		assert.True(t, mockPS.deleted[poolID1])
		assert.True(t, mockPS.deleted[poolID2])

		mockTPS.mu.RLock()
		defer mockTPS.mu.RUnlock()
		assert.NotContains(t, mockTPS.tokenPools[1], poolID1)
		assert.NotContains(t, mockTPS.tokenPools[1], poolID2)
	})

	t.Run("BeforeRegisterPoolHook", func(t *testing.T) {
		mockTS, _, _, manager := setupMocksAndManager()
		mockTS.AddToken(addrWETH, "WETH", "WETH", 18)
		mockTS.AddToken(addrUSDC, "USDC", "USDC", 6)
		// Note: addrDAI is NOT added, so pools using it will fail validation.

		// Thread-safe capture for the hook
		var visitedKeys []PoolKey
		var mu sync.Mutex

		// Register the hook
		manager.SetBeforeRegisterPoolFunc(func(pk PoolKey) {
			mu.Lock()
			defer mu.Unlock()
			visitedKeys = append(visitedKeys, pk)
		})

		// Sub-test 1: Single RegisterPool
		t.Run("SinglePool", func(t *testing.T) {
			_, _, err := manager.RegisterPool(keyPool1, []common.Address{addrWETH, addrUSDC}, protoUni)
			require.NoError(t, err)

			mu.Lock()
			defer mu.Unlock()
			assert.Len(t, visitedKeys, 1)
			assert.Equal(t, keyPool1, visitedKeys[0])
		})

		// Reset visited keys
		mu.Lock()
		visitedKeys = nil
		mu.Unlock()

		// Sub-test 2: Batch RegisterPools with Failures
		// - keyPool2: Valid (WETH/USDC)
		// - keyPool3: Invalid (WETH/DAI - DAI missing)
		// Hook should fire for BOTH because it runs before validation.
		t.Run("BatchWithFailures", func(t *testing.T) {
			poolIDs, _, errs := manager.RegisterPools(
				[]PoolKey{keyPool2, keyPool3},
				[][]common.Address{{addrWETH, addrUSDC}, {addrWETH, addrDAI}},
				[]engine.ProtocolID{protoUni, protoSushi},
			)

			// Verify the logic result first (sanity check)
			require.NotNil(t, errs)
			assert.Nil(t, errs[0])    // Pool 2 succeeded
			assert.NotNil(t, errs[1]) // Pool 3 failed
			assert.NotZero(t, poolIDs[0])
			assert.Zero(t, poolIDs[1])

			// Verify the Hook behavior
			mu.Lock()
			defer mu.Unlock()
			assert.Len(t, visitedKeys, 2, "Hook should have fired for both pools regardless of success")
			assert.Contains(t, visitedKeys, keyPool2)
			assert.Contains(t, visitedKeys, keyPool3)
		})
	})
}

// setupMocksAndManager is a helper to reduce boilerplate in tests.
func setupMocksAndManager() (*mockTokenSystem, *mockPoolSystem, *mockTokenPoolSystem, *RegistryManager) {
	mockTS := newMockTokenSystem()
	mockPS := newMockPoolSystem()
	mockTPS := newMockTokenPoolSystem()
	manager := NewRegistryManager(mockTS, mockPS, mockTPS)
	return mockTS, mockPS, mockTPS, manager
}
