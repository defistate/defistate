// Package services provides the HTTP layer for the token analysis application.
// It exposes token data via a JSON API.
package services

import (
	"encoding/json"
	"fmt"
	"log"

	"net/http"
	"sort"

	token "github.com/defistate/defistate/protocols/erc20-token-system"
)

// ERC20TokenViewer defines the contract for any component that can provide a view
// of all tracked tokens for a specific chain.
type ERC20TokenViewer interface {
	View() []token.TokenView
}

// ERC20APIService holds the map of viewers for all supported chains and manages routing.
type ERC20APIService struct {
	viewersByChainID map[uint64]ERC20TokenViewer
}

// NewERC20APIService creates a new instance of the main API service for ERC20 tokens.
// It takes a map where keys are chain IDs and values are the corresponding
// ERC20TokenViewer implementations.
func NewERC20APIService(viewers map[uint64]ERC20TokenViewer) *ERC20APIService {
	return &ERC20APIService{
		viewersByChainID: viewers,
	}
}

// RegisterRoutes iterates through all the provided viewers and dynamically
// creates a dedicated HTTP handler for each chain.
func (s *ERC20APIService) RegisterRoutes(mux *http.ServeMux) {
	for chainID, viewer := range s.viewersByChainID {
		// Create a route and handler specific to this chainID and viewer.
		route := fmt.Sprintf("/%d/erc20/", chainID)
		handler := s.createTokenHandler(chainID, viewer)
		mux.HandleFunc(route, handler)
	}
}

// Endpoints returns a sorted slice of all the route patterns that the service
// will register. This allows the caller (e.g., main.go) to know which routes
// are managed by this service, preventing conflicts.
func (s *ERC20APIService) Endpoints() []string {
	if s.viewersByChainID == nil {
		return nil
	}
	chainIDs := make([]uint64, 0, len(s.viewersByChainID))
	for chainID := range s.viewersByChainID {
		chainIDs = append(chainIDs, chainID)
	}

	// Sort the chain IDs to ensure a deterministic order of endpoints.
	sort.Slice(chainIDs, func(i, j int) bool {
		return chainIDs[i] < chainIDs[j]
	})

	routes := make([]string, len(chainIDs))
	for i, chainID := range chainIDs {
		routes[i] = fmt.Sprintf("/%d/erc20/", chainID)
	}
	return routes
}

// createTokenHandler is a higher-order function that returns an HTTP handler
// closure for a specific chainID and viewer. This is crucial for capturing the
// correct variables for each route.
func (s *ERC20APIService) createTokenHandler(chainID uint64, viewer ERC20TokenViewer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		type response struct {
			ChainID uint64            `json:"chainId"`
			Tokens  []token.TokenView `json:"tokens"`
		}

		tokens := viewer.View()
		payload := response{
			ChainID: chainID,
			Tokens:  tokens,
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(payload); err != nil {
			log.Printf("ERROR: Failed to encode tokens for chain %d: %v", chainID, err)
			http.Error(w, "Failed to serialize token data", http.StatusInternalServerError)
		}
	}
}
