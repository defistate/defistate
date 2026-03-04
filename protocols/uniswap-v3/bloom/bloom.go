package bloom

import (
	"github.com/defistate/defistate/protocols/uniswap-v3/abi"
	"github.com/ethereum/go-ethereum/core/types"
)

func TestUniswapV3Swap(bloom types.Bloom) bool {
	return bloom.Test(abi.UniswapV3ABI.Events["Swap"].ID.Bytes())
}
