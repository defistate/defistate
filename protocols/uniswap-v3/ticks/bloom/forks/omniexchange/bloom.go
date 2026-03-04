package pancakeswap

import (
	"github.com/defistate/defistate/protocols/uniswap-v3/abi/forks/omniexchange"
	"github.com/ethereum/go-ethereum/core/types"
)

func TestOmniExchangePoolMint(bloom *types.Bloom) bool {
	return bloom.Test(omniexchange.OmniExchangeABI.Events["Mint"].ID.Bytes())
}

func TestOmniExchangePoolBurn(bloom *types.Bloom) bool {
	return bloom.Test(omniexchange.OmniExchangeABI.Events["Burn"].ID.Bytes())
}
