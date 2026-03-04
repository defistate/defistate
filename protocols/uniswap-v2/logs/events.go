package logs

import "github.com/defistate/defistate/protocols/uniswap-v2/abi"

var (
	UniswapV2SwapEvent = abi.UniswapV2ABI.Events["Swap"].ID
	UniswapV2SyncEvent = abi.UniswapV2ABI.Events["Sync"].ID
)
