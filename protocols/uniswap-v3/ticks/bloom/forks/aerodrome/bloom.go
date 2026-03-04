package pancakeswap

import (
	"github.com/defistate/defistate/protocols/uniswap-v3/abi/forks/aerodrome"
	"github.com/ethereum/go-ethereum/core/types"
)

func TestAerodromePoolMint(bloom *types.Bloom) bool {
	return bloom.Test(aerodrome.AerodromeABI.Events["Mint"].ID.Bytes())
}

func TestAerodromePoolBurn(bloom *types.Bloom) bool {
	return bloom.Test(aerodrome.AerodromeABI.Events["Burn"].ID.Bytes())
}
