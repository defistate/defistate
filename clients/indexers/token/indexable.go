package token

import (
	clients "github.com/defistate/defistate/clients"
	basetoken "github.com/defistate/defistate/protocols/erc20-token-system"
	"github.com/ethereum/go-ethereum/common"
)

// Indexer is a concrete implementation of the clients.TokenIndexer interface.
type Indexer struct{}

// New creates a new Indexer.
func New() *Indexer {
	return &Indexer{}
}

// Index creates an indexed token system from a raw slice of tokens.
func (i *Indexer) Index(tokens []basetoken.TokenView) clients.IndexedTokenSystem {
	return NewIndexableTokenSystem(tokens)
}

// IndexableTokenSystem provides fast, indexed access to token data.
type IndexableTokenSystem struct {
	byID      map[uint64]basetoken.TokenView
	byAddress map[common.Address]basetoken.TokenView
	all       []basetoken.TokenView
}

// NewIndexableTokenSystem creates a new indexed token system from a raw slice.
func NewIndexableTokenSystem(tokens []basetoken.TokenView) *IndexableTokenSystem {
	byID := make(map[uint64]basetoken.TokenView, len(tokens))
	byAddress := make(map[common.Address]basetoken.TokenView, len(tokens))

	for _, t := range tokens {
		byID[t.ID] = t
		byAddress[t.Address] = t
	}

	return &IndexableTokenSystem{
		byID:      byID,
		byAddress: byAddress,
		all:       tokens,
	}
}

// GetByID retrieves a token by its unique ID.
func (its *IndexableTokenSystem) GetByID(id uint64) (basetoken.TokenView, bool) {
	t, ok := its.byID[id]
	return t, ok
}

// GetByAddress retrieves a token by its contract address.
func (its *IndexableTokenSystem) GetByAddress(address common.Address) (basetoken.TokenView, bool) {
	t, ok := its.byAddress[address]
	return t, ok
}

// All returns a defensive copy of the slice of all tokens in the system.
func (its *IndexableTokenSystem) All() []basetoken.TokenView {
	allCopy := make([]basetoken.TokenView, len(its.all))
	copy(allCopy, its.all)
	return allCopy
}
