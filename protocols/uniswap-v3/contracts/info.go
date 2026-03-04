package contracts

import (
	"context"
	"fmt"
	"math/big"

	ethclients "github.com/defistate/defistate/clients/eth-clients"
	uniswapv3abi "github.com/defistate/defistate/protocols/uniswap-v3/abi"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
)

var (
	slot0Sig     = uniswapv3abi.UniswapV3ABI.Methods["slot0"].ID
	liquiditySig = uniswapv3abi.UniswapV3ABI.Methods["liquidity"].ID
	feeSig       = uniswapv3abi.UniswapV3ABI.Methods["fee"].ID

	multicall3ABI = uniswapv3abi.Multicall3ABI
)

// Multicall2Call matches the struct used by tryAggregate (v2/v3)
type Multicall2Call struct {
	Target   common.Address
	CallData []byte
}

// Multicall2Result matches the return struct for tryAggregate
type Multicall2Result struct {
	Success    bool
	ReturnData []byte
}

// NewBatchPoolInfoProvider fetches Tick, Liquidity, SqrtPriceX96, and Fee using tryAggregate.
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

		// 1. Prepare triplets for tryAggregate: [P1.slot0, P1.liq, P1.fee, P2.slot0...]
		calls := make([]Multicall2Call, numPools*3)
		for i, addr := range poolAddrs {
			baseIdx := i * 3
			calls[baseIdx] = Multicall2Call{Target: addr, CallData: slot0Sig}
			calls[baseIdx+1] = Multicall2Call{Target: addr, CallData: liquiditySig}
			calls[baseIdx+2] = Multicall2Call{Target: addr, CallData: feeSig}
		}

		// 2. Pack and Execute tryAggregate(requireSuccess bool, calls Call[])
		// We set requireSuccess to false to simulate the allowFailure behavior.
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

		// 3. Unpack Results
		var results []Multicall2Result
		if err := multicall3ABI.UnpackIntoInterface(&results, "tryAggregate", resp); err != nil {
			markAllErrors(errs, fmt.Errorf("failed to unpack multicall response: %w", err))
			return
		}

		// 4. Decode triplets
		for i := 0; i < numPools; i++ {
			baseIdx := i * 3
			resS0 := results[baseIdx]
			resLiq := results[baseIdx+1]
			resFee := results[baseIdx+2]

			if !resS0.Success || !resLiq.Success || !resFee.Success {
				errs[i] = fmt.Errorf("pool data fetch reverted via tryAggregate")
				continue
			}

			// Manual Slot0 decode (sqrtPriceX96, tick)
			if len(resS0.ReturnData) >= 64 {
				sqrtPriceX96s[i] = new(big.Int).SetBytes(resS0.ReturnData[0:32])
				ticks[i] = new(big.Int).SetBytes(resS0.ReturnData[32:64]).Int64()
			} else {
				errs[i] = fmt.Errorf("invalid slot0 length")
			}

			// Manual Liquidity & Fee decode
			if len(resLiq.ReturnData) >= 32 {
				liquidities[i] = new(big.Int).SetBytes(resLiq.ReturnData)
			}
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
