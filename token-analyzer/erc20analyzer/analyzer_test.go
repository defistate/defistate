package erc20analyzer

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	token "github.com/defistate/defistate/protocols/erc20-token-system"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mock Implementations for Dependencies ---

type mockTokenStore struct {
	mu       sync.RWMutex
	updateCh chan struct {
		ID, Gas uint64
		Fee     float64
	}
	tokensByAddr map[common.Address]token.TokenView
}

func newMockTokenStore() *mockTokenStore {
	return &mockTokenStore{
		updateCh: make(chan struct {
			ID, Gas uint64
			Fee     float64
		}, 10),
		tokensByAddr: make(map[common.Address]token.TokenView),
	}
}
func (m *mockTokenStore) AddToken(addr common.Address, name, symbol string, decimals uint8) (uint64, error) {
	// Not directly tested via Analyzer, but implemented for completeness.
	return 0, nil
}
func (m *mockTokenStore) DeleteToken(idToDelete uint64) error { return nil }
func (m *mockTokenStore) UpdateToken(id uint64, fee float64, gas uint64) error {
	m.updateCh <- struct {
		ID, Gas uint64
		Fee     float64
	}{id, gas, fee}
	return nil
}
func (m *mockTokenStore) View() []token.TokenView { return nil }
func (m *mockTokenStore) GetTokenByID(id uint64) (token.TokenView, error) {
	return token.TokenView{}, nil
}
func (m *mockTokenStore) GetTokenByAddress(addr common.Address) (token.TokenView, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if t, ok := m.tokensByAddr[addr]; ok {
		return t, nil
	}
	return token.TokenView{}, errors.New("token not found")
}

type mockTokenHolderAnalyzer struct {
	mu              sync.RWMutex
	holdersToReturn map[common.Address]common.Address
}

func newMockTokenHolderAnalyzer() *mockTokenHolderAnalyzer {
	return &mockTokenHolderAnalyzer{holdersToReturn: make(map[common.Address]common.Address)}
}
func (m *mockTokenHolderAnalyzer) Update(logs []types.Log) {}
func (m *mockTokenHolderAnalyzer) TokenByMaxKnownHolder() map[common.Address]common.Address {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make(map[common.Address]common.Address)
	for k, v := range m.holdersToReturn {
		result[k] = v
	}
	return result
}

type mockBlockExtractor struct {
	runEnded chan struct{}
}

func newMockBlockExtractor() *mockBlockExtractor {
	return &mockBlockExtractor{runEnded: make(chan struct{})}
}
func (m *mockBlockExtractor) Run(ctx context.Context, _ <-chan *types.Block, logsHandler func(context.Context, []types.Log) error, errHandler func(error)) {
	<-ctx.Done()
	close(m.runEnded)
}

type mockFeeAndGasRequester struct {
	mu           sync.RWMutex
	requestAllCh chan map[common.Address]common.Address
	results      map[common.Address]FeeAndGasResult
	errToReturn  error
}

func newMockFeeAndGasRequester() *mockFeeAndGasRequester {
	return &mockFeeAndGasRequester{
		requestAllCh: make(chan map[common.Address]common.Address, 10),
		results:      make(map[common.Address]FeeAndGasResult),
	}
}
func (m *mockFeeAndGasRequester) RequestAll(_ context.Context, tokensByHolder map[common.Address]common.Address) (map[common.Address]FeeAndGasResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	// Use non-blocking send in case test doesn't read from channel.
	select {
	case m.requestAllCh <- tokensByHolder:
	default:
	}
	return m.results, m.errToReturn
}

type mockTokenInitializer struct {
	mu           sync.RWMutex
	initializeCh chan common.Address
	viewToReturn token.TokenView
	errToReturn  error
	onInitialize func(addr common.Address, store TokenStore)
}

func newMockTokenInitializer() *mockTokenInitializer {
	return &mockTokenInitializer{initializeCh: make(chan common.Address, 10)}
}
func (m *mockTokenInitializer) Initialize(ctx context.Context, tokenAddress common.Address, store TokenStore) (token.TokenView, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	select {
	case m.initializeCh <- tokenAddress:
	case <-ctx.Done():
		return token.TokenView{}, ctx.Err()
	default:
	}
	if m.onInitialize != nil {
		m.onInitialize(tokenAddress, store)
	}
	return m.viewToReturn, m.errToReturn
}

// --- Main Analyzer Test Suite ---

// setupTest creates a full set of mocks and a valid config for testing.
func setupTest(t *testing.T) (*mockTokenStore, *mockTokenHolderAnalyzer, *mockBlockExtractor, *mockFeeAndGasRequester, *mockTokenInitializer, *sync.Mutex, *[]error, Config) {
	tokenStore := newMockTokenStore()
	holderAnalyzer := newMockTokenHolderAnalyzer()
	blockExtractor := newMockBlockExtractor()
	feeAndGasRequester := newMockFeeAndGasRequester()
	tokenInitializer := newMockTokenInitializer()

	var errMtx sync.Mutex
	// Initialize a slice and return a pointer to it.
	capturedErrors := make([]error, 0, 10)
	errorHandler := func(err error) {
		errMtx.Lock()
		capturedErrors = append(capturedErrors, err)
		errMtx.Unlock()
	}

	cfg := Config{
		TokenStore:               tokenStore,
		TokenHolderAnalyzer:      holderAnalyzer,
		BlockExtractor:           blockExtractor,
		FeeAndGasRequester:       feeAndGasRequester,
		TokenInitializer:         tokenInitializer,
		NewBlockEventer:          make(chan *types.Block),
		ErrorHandler:             errorHandler,
		MinTokenUpdateInterval:   100 * time.Millisecond,
		FeeAndGasUpdateFrequency: 50 * time.Millisecond,
	}

	return tokenStore, holderAnalyzer, blockExtractor, feeAndGasRequester, tokenInitializer, &errMtx, &capturedErrors, cfg
}

func TestAnalyzer(t *testing.T) {
	t.Run("updates existing token successfully", func(t *testing.T) {
		// Arrange
		store, holder, _, requester, _, _, _, cfg := setupTest(t)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		tokenAddr := common.HexToAddress("0x1")
		holderAddr := common.HexToAddress("0xA")
		tokenID := uint64(1)

		// token exist in store
		store.mu.Lock()
		store.tokensByAddr[tokenAddr] = token.TokenView{ID: tokenID, Address: tokenAddr}
		store.mu.Unlock()

		// holder for token
		holder.mu.Lock()
		holder.holdersToReturn[tokenAddr] = holderAddr
		holder.mu.Unlock()

		// fee and gas for token
		requester.mu.Lock()
		requester.results[tokenAddr] = FeeAndGasResult{Fee: 2.5, Gas: 60000}
		requester.mu.Unlock()

		// Act
		_, err := NewAnalyzer(ctx, cfg)
		require.NoError(t, err)

		// Assert: Requester is called with the token
		select {
		case req := <-requester.requestAllCh:
			require.Contains(t, req, tokenAddr)
			assert.Equal(t, holderAddr, req[tokenAddr])
		case <-time.After(100 * time.Millisecond):
			t.Fatal("timed out waiting for FeeAndGasRequester to be called")
		}

		// Assert: Store is updated with the correct fee and gas
		select {
		case update := <-store.updateCh:
			assert.Equal(t, tokenID, update.ID)
			assert.Equal(t, float64(2.5), update.Fee)
			assert.Equal(t, uint64(60000), update.Gas)
		case <-time.After(100 * time.Millisecond):
			t.Fatal("timed out waiting for TokenStore to be updated")
		}
	})

	t.Run("initializes and updates new token", func(t *testing.T) {
		// Arrange
		store, holder, _, requester, initializer, _, _, cfg := setupTest(t)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		tokenAddr := common.HexToAddress("0x2")
		holderAddr := common.HexToAddress("0xB")
		tokenID := uint64(2)

		// This token does NOT exist in the store initially.
		holder.mu.Lock()
		holder.holdersToReturn[tokenAddr] = holderAddr
		holder.mu.Unlock()

		requester.mu.Lock()
		requester.results[tokenAddr] = FeeAndGasResult{Fee: 1.0, Gas: 30000}
		requester.mu.Unlock()

		// Configure initializer to add the token to the store when called.
		initializer.mu.Lock()
		initializer.onInitialize = func(addr common.Address, s TokenStore) {
			s.(*mockTokenStore).mu.Lock()
			s.(*mockTokenStore).tokensByAddr[addr] = token.TokenView{ID: tokenID, Address: addr}
			s.(*mockTokenStore).mu.Unlock()
		}
		initializer.viewToReturn = token.TokenView{ID: tokenID, Address: tokenAddr}
		initializer.mu.Unlock()

		// Act
		_, err := NewAnalyzer(ctx, cfg)
		require.NoError(t, err)

		// Assert: Initializer is called first
		select {
		case addr := <-initializer.initializeCh:
			assert.Equal(t, tokenAddr, addr)
		case <-time.After(100 * time.Millisecond):
			t.Fatal("timed out waiting for TokenInitializer to be called")
		}

		// Assert: Store is updated after initialization
		select {
		case update := <-store.updateCh:
			assert.Equal(t, tokenID, update.ID)
			assert.Equal(t, float64(1.0), update.Fee)
			assert.Equal(t, uint64(30000), update.Gas)
		case <-time.After(100 * time.Millisecond):
			t.Fatal("timed out waiting for TokenStore to be updated")
		}
	})

	t.Run("handles initialization failure gracefully", func(t *testing.T) {
		// Arrange
		store, holder, _, requester, initializer, errMtx, capturedErrs, cfg := setupTest(t)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		tokenAddr := common.HexToAddress("0x3")
		holderAddr := common.HexToAddress("0xC")
		initErr := errors.New("failed to initialize")

		holder.mu.Lock()
		holder.holdersToReturn[tokenAddr] = holderAddr
		holder.mu.Unlock()

		requester.mu.Lock()
		requester.results[tokenAddr] = FeeAndGasResult{Fee: 1.0, Gas: 30000}
		requester.mu.Unlock()

		initializer.mu.Lock()
		initializer.errToReturn = initErr
		initializer.mu.Unlock()

		// Act
		_, err := NewAnalyzer(ctx, cfg)
		require.NoError(t, err)

		// Assert: Initializer is called
		select {
		case <-initializer.initializeCh:
		case <-time.After(100 * time.Millisecond):
			t.Fatal("timed out waiting for TokenInitializer to be called")
		}

		// Assert: No update is sent to the store
		select {
		case <-store.updateCh:
			t.Fatal("TokenStore should not be updated on initialization failure")
		case <-time.After(50 * time.Millisecond):
			// Success
		}

		// Assert: The error was captured by dereferencing the pointer to the slice.
		errMtx.Lock()
		defer errMtx.Unlock()
		require.NotEmpty(t, *capturedErrs)
		assert.ErrorIs(t, (*capturedErrs)[0], initErr)
	})

	t.Run("respects update interval cooldown", func(t *testing.T) {
		// Arrange
		_, holder, _, requester, _, _, _, cfg := setupTest(t)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		tokenAddr := common.HexToAddress("0x4")
		holderAddr := common.HexToAddress("0xD")

		holder.mu.Lock()
		holder.holdersToReturn[tokenAddr] = holderAddr
		holder.mu.Unlock()

		// Act: Create analyzer and immediately set the last check time for the token.
		analyzer, err := NewAnalyzer(ctx, cfg)
		require.NoError(t, err)

		analyzer.mu.Lock()
		analyzer.lastFeeAndGasCheck[tokenAddr] = time.Now()
		analyzer.mu.Unlock()

		// Trigger a new update cycle.
		analyzer.performFeeAndGasUpdate(ctx, requester)

		// Assert: Requester channel should NOT receive the token because it's on cooldown.
		select {
		case req := <-requester.requestAllCh:
			assert.Empty(t, req, "Requester should not be called for a token on cooldown")
		case <-time.After(50 * time.Millisecond):
			// Success, this is the expected path as the filter is synchronous.
		}
	})

	t.Run("shuts down gracefully on context cancellation", func(t *testing.T) {
		// Arrange
		_, _, blockExtractor, _, _, _, _, cfg := setupTest(t)
		ctx, cancel := context.WithCancel(context.Background())

		// Act
		_, err := NewAnalyzer(ctx, cfg)
		require.NoError(t, err)
		cancel() // Cancel the context to signal shutdown.

		// Assert
		select {
		case <-blockExtractor.runEnded:
			// Success
		case <-time.After(1 * time.Second):
			t.Fatal("BlockExtractor did not shut down within the timeout")
		}
	})
}

func TestAnalyzer_ConfigValidation(t *testing.T) {
	// A helper to quickly get a valid config for modification.
	getValidConfig := func() Config {
		return Config{
			TokenStore:          newMockTokenStore(),
			TokenHolderAnalyzer: newMockTokenHolderAnalyzer(),
			BlockExtractor:      newMockBlockExtractor(),
			FeeAndGasRequester:  newMockFeeAndGasRequester(),
			TokenInitializer:    newMockTokenInitializer(),
			NewBlockEventer:     make(chan *types.Block),
			ErrorHandler:        func(error) {},
		}
	}

	testCases := []struct {
		name      string
		modifier  func(*Config)
		expectErr string
	}{
		{"nil TokenStore", func(c *Config) { c.TokenStore = nil }, "a TokenStore implementation is a required dependency"},
		{"nil TokenInitializer", func(c *Config) { c.TokenInitializer = nil }, "a TokenInitializer is a required dependency"},
		{"nil BlockExtractor", func(c *Config) { c.BlockExtractor = nil }, "a BlockExtractor implementation is a required dependency"},
		{"nil TokenHolderAnalyzer", func(c *Config) { c.TokenHolderAnalyzer = nil }, "a TokenHolderAnalyzer implementation is a required dependency"},
		{"nil FeeAndGasRequester", func(c *Config) { c.FeeAndGasRequester = nil }, "a FeeAndGasRequester implementation is a required dependency"},
		{"nil NewBlockEventer", func(c *Config) { c.NewBlockEventer = nil }, "NewBlockEventer channel is required"},
		{"nil ErrorHandler", func(c *Config) { c.ErrorHandler = nil }, "an ErrorHandler is a required dependency"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := getValidConfig()
			tc.modifier(&cfg)

			_, err := NewAnalyzer(context.Background(), cfg)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.expectErr)
		})
	}
}
