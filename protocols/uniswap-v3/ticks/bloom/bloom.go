package bloom

import (
	"github.com/defistate/defistate/protocols/uniswap-v3/abi"
	"github.com/ethereum/go-ethereum/core/types"
)

func TestUniswapV3PoolMint(bloom *types.Bloom) bool {
	return bloom.Test(abi.UniswapV3ABI.Events["Mint"].ID.Bytes())
}

func TestUniswapV3PoolBurn(bloom *types.Bloom) bool {
	return bloom.Test(abi.UniswapV3ABI.Events["Burn"].ID.Bytes())
}
