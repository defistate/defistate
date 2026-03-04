package pancakeswap

import (
	"github.com/defistate/defistate/protocols/uniswap-v3/abi/forks/pancakeswap"
	"github.com/ethereum/go-ethereum/core/types"
)

func TestPancakeswapV3PoolMint(bloom *types.Bloom) bool {
	return bloom.Test(pancakeswap.PancakeswapV3ABI.Events["Mint"].ID.Bytes())
}

func TestPancakeswapV3PoolBurn(bloom *types.Bloom) bool {
	return bloom.Test(pancakeswap.PancakeswapV3ABI.Events["Burn"].ID.Bytes())
}
