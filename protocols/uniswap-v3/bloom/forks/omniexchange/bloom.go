package omniexchange

import (
	abi "github.com/defistate/defistate/protocols/uniswap-v3/abi/forks/omniexchange"
	"github.com/ethereum/go-ethereum/core/types"
)

func TestOmniExchangeSwap(bloom types.Bloom) bool {
	return bloom.Test(abi.OmniExchangeABI.Events["Swap"].ID.Bytes())
}
