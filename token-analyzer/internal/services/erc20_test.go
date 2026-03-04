package services

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	token "github.com/defistate/defistate/protocols/erc20-token-system"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockERC20TokenViewer is a test implementation of the ERC20TokenViewer interface.
// It allows us to provide controlled data for our HTTP handlers to serve.
type mockERC20TokenViewer struct {
	viewToReturn []token.TokenView
}

// View returns the pre-configured slice of tokens for the mock.
func (m *mockERC20TokenViewer) View() []token.TokenView {
	return m.viewToReturn
}

func TestERC20APIService(t *testing.T) {
	// --- Arrange ---
	mainnetChainID := uint64(1)
	arbitrumChainID := uint64(42161)

	// Create mock viewers with distinct data for each chain.
	mainnetViewer := &mockERC20TokenViewer{
		viewToReturn: []token.TokenView{
			{ID: 1, Address: common.HexToAddress("0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48"), Name: "USDC"},
		},
	}
	arbitrumViewer := &mockERC20TokenViewer{
		viewToReturn: []token.TokenView{
			{ID: 2, Address: common.HexToAddress("0xFF970A61A04b1cA14834A43f5dE4533eBDDB5CC8"), Name: "USDC.e"},
			{ID: 3, Address: common.HexToAddress("0x82aF49447D8a07e3bd95BD0d56f35241523fBab1"), Name: "WETH"},
		},
	}

	viewers := map[uint64]ERC20TokenViewer{
		mainnetChainID:  mainnetViewer,
		arbitrumChainID: arbitrumViewer,
	}

	// Create the service under test.
	service := NewERC20APIService(viewers)
	require.NotNil(t, service)

	t.Run("Endpoints returns correct sorted routes", func(t *testing.T) {
		// Act
		endpoints := service.Endpoints()

		// Assert that the endpoints are correct and in a deterministic (sorted) order.
		expectedEndpoints := []string{"/1/erc20/", "/42161/erc20/"}
		assert.Equal(t, expectedEndpoints, endpoints)
	})

	t.Run("RegisterRoutes correctly handles requests", func(t *testing.T) {
		// Arrange
		mux := http.NewServeMux()
		service.RegisterRoutes(mux)

		// A helper to decode the JSON response for assertions.
		decodeResponse := func(t *testing.T, body []byte) struct {
			ChainID uint64            `json:"chainId"`
			Tokens  []token.TokenView `json:"tokens"`
		} {
			var respBody struct {
				ChainID uint64            `json:"chainId"`
				Tokens  []token.TokenView `json:"tokens"`
			}
			err := json.Unmarshal(body, &respBody)
			require.NoError(t, err, "Failed to decode JSON response")
			return respBody
		}

		t.Run("mainnet endpoint success", func(t *testing.T) {
			req := httptest.NewRequest("GET", "/1/erc20/", nil)
			rr := httptest.NewRecorder()

			mux.ServeHTTP(rr, req)

			require.Equal(t, http.StatusOK, rr.Code)
			assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))
			respBody := decodeResponse(t, rr.Body.Bytes())
			assert.Equal(t, mainnetChainID, respBody.ChainID)
			assert.Equal(t, mainnetViewer.viewToReturn, respBody.Tokens)
		})

		t.Run("arbitrum endpoint success", func(t *testing.T) {
			req := httptest.NewRequest("GET", "/42161/erc20/", nil)
			rr := httptest.NewRecorder()

			mux.ServeHTTP(rr, req)

			require.Equal(t, http.StatusOK, rr.Code)
			respBody := decodeResponse(t, rr.Body.Bytes())
			assert.Equal(t, arbitrumChainID, respBody.ChainID)
			assert.Equal(t, arbitrumViewer.viewToReturn, respBody.Tokens)
		})

		t.Run("non-GET method is rejected", func(t *testing.T) {
			req := httptest.NewRequest("POST", "/1/erc20/", nil)
			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, req)
			assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
		})

		t.Run("non-existent route returns 404", func(t *testing.T) {
			req := httptest.NewRequest("GET", "/999/erc20/", nil)
			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, req)
			assert.Equal(t, http.StatusNotFound, rr.Code)
		})
	})
}
