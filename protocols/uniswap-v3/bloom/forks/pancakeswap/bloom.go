package pancakeswap

import (
	abi "github.com/defistate/defistate/protocols/uniswap-v3/abi/forks/pancakeswap"
	"github.com/ethereum/go-ethereum/core/types"
)

func TestPancakeswapV3Swap(bloom types.Bloom) bool {
	return bloom.Test(abi.PancakeswapV3ABI.Events["Swap"].ID.Bytes())
}
