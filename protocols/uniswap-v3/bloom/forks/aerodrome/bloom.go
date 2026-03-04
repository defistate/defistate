package aerodrome

import (
	abi "github.com/defistate/defistate/protocols/uniswap-v3/abi/forks/aerodrome"
	"github.com/ethereum/go-ethereum/core/types"
)

func TestAerodromeSwap(bloom types.Bloom) bool {
	return bloom.Test(abi.AerodromeABI.Events["Swap"].ID.Bytes())
}
