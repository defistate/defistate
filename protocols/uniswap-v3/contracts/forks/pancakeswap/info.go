package pancakeswap

import (
	"context"
	"fmt"
	"math/big"

	ethclients "github.com/defistate/defistate/clients/eth-clients"
	uniswapv3abi "github.com/defistate/defistate/protocols/uniswap-v3/abi"
	pancakeswapv3abi "github.com/defistate/defistate/protocols/uniswap-v3/abi/forks/pancakeswap"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
)

var (
	// Extracting method IDs once at package level for performance
	slot0Sig     = pancakeswapv3abi.PancakeswapV3ABI.Methods["slot0"].ID
	liquiditySig = pancakeswapv3abi.PancakeswapV3ABI.Methods["liquidity"].ID
	feeSig       = pancakeswapv3abi.PancakeswapV3ABI.Methods["fee"].ID

	multicall3ABI = uniswapv3abi.Multicall3ABI
)

// Multicall2 structs for tryAggregate compatibility
type Multicall2Call struct {
	Target   common.Address `json:"target"`
	CallData []byte         `json:"callData"`
}

type Multicall2Result struct {
	Success    bool   `json:"success"`
	ReturnData []byte `json:"returnData"`
}

// NewBatchPoolInfoProvider fetches Tick, Liquidity, SqrtPriceX96, and Fee for multiple PancakeSwap V3 pools.
func NewBatchPoolInfoProvider(
	multicallAddress common.Address,
) func(ctx context.Context, poolAddrs []common.Address, getClient func() (ethclients.ETHClient, error), blockNumber *big.Int) (ticks []int64, liquidities []*big.Int, sqrtPriceX96s []*big.Int, fees []uint64, errs []error) {

	return func(ctx context.Context, poolAddrs []common.Address, getClient func() (ethclients.ETHClient, error), blockNumber *big.Int) (ticks []int64, liquidities []*big.Int, sqrtPriceX96s []*big.Int, fees []uint64, errs []error) {
		numPools := len(poolAddrs)
		if numPools == 0 {
			return nil, nil, nil, nil, nil
		}

		client, err := getClient()
		if err != nil {
			errs := make([]error, len(poolAddrs))
			for i := range poolAddrs {
				errs[i] = err
			}
			return nil, nil, nil, nil, errs
		}

		ticks = make([]int64, numPools)
		liquidities = make([]*big.Int, numPools)
		sqrtPriceX96s = make([]*big.Int, numPools)
		fees = make([]uint64, numPools)
		errs = make([]error, numPools)

		// 1. Prepare interleaved triplets: [P1.slot0, P1.liq, P1.fee, P2.slot0...]
		calls := make([]Multicall2Call, numPools*3)
		for i, addr := range poolAddrs {
			baseIdx := i * 3
			calls[baseIdx] = Multicall2Call{Target: addr, CallData: slot0Sig}
			calls[baseIdx+1] = Multicall2Call{Target: addr, CallData: liquiditySig}
			calls[baseIdx+2] = Multicall2Call{Target: addr, CallData: feeSig}
		}

		// 2. Pack and Execute tryAggregate
		callData, err := multicall3ABI.Pack("tryAggregate", false, calls)
		if err != nil {
			markAllErrors(errs, fmt.Errorf("failed to pack multicall: %w", err))
			return
		}

		resp, err := client.CallContract(ctx, ethereum.CallMsg{To: &multicallAddress, Data: callData}, blockNumber)
		if err != nil {
			markAllErrors(errs, fmt.Errorf("multicall RPC failed: %w", err))
			return
		}

		// 3. Unpack Multicall Results
		var results []Multicall2Result
		if err := multicall3ABI.UnpackIntoInterface(&results, "tryAggregate", resp); err != nil {
			markAllErrors(errs, fmt.Errorf("failed to unpack multicall response: %w", err))
			return
		}

		// 4. Decode with PancakeSwap-specific logic
		for i := 0; i < numPools; i++ {
			baseIdx := i * 3
			resS0 := results[baseIdx]
			resLiq := results[baseIdx+1]
			resFee := results[baseIdx+2]

			if !resS0.Success || !resLiq.Success || !resFee.Success {
				errs[i] = fmt.Errorf("pancakeswap pool fetch reverted")
				continue
			}

			// PancakeSwap slot0() manual decode
			// sqrtPriceX96 (uint160) @ 0x00, tick (int24) @ 0x20
			if len(resS0.ReturnData) >= 64 {
				sqrtPriceX96s[i] = new(big.Int).SetBytes(resS0.ReturnData[0:32])
				ticks[i] = new(big.Int).SetBytes(resS0.ReturnData[32:64]).Int64()
			} else {
				errs[i] = fmt.Errorf("invalid slot0 length")
			}

			// Liquidity (uint128)
			if len(resLiq.ReturnData) >= 32 {
				liquidities[i] = new(big.Int).SetBytes(resLiq.ReturnData)
			}

			// Fee (uint24)
			if len(resFee.ReturnData) >= 32 {
				fees[i] = new(big.Int).SetBytes(resFee.ReturnData).Uint64()
			}
		}

		return
	}
}

func markAllErrors(errs []error, err error) {
	for i := range errs {
		errs[i] = err
	}
}
