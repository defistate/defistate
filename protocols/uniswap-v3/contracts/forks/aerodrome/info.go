package aerodrome

import (
	"context"
	"fmt"
	"math/big"

	ethclients "github.com/defistate/defistate/clients/eth-clients"
	uniswapv3abi "github.com/defistate/defistate/protocols/uniswap-v3/abi"
	aerodromeabi "github.com/defistate/defistate/protocols/uniswap-v3/abi/forks/aerodrome"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
)

var (
	// Extracting method IDs specifically from Aerodrome's ABI
	slot0Sig     = aerodromeabi.AerodromeABI.Methods["slot0"].ID
	liquiditySig = aerodromeabi.AerodromeABI.Methods["liquidity"].ID
	feeSig       = aerodromeabi.AerodromeABI.Methods["fee"].ID

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

// NewBatchPoolInfoProvider fetches Tick, Liquidity, SqrtPriceX96, and Fee for Aerodrome pools.
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

		// 1. Prepare triplets: [P1.slot0, P1.liq, P1.fee...]
		calls := make([]Multicall2Call, numPools*3)
		for i, addr := range poolAddrs {
			baseIdx := i * 3
			calls[baseIdx] = Multicall2Call{Target: addr, CallData: slot0Sig}
			calls[baseIdx+1] = Multicall2Call{Target: addr, CallData: liquiditySig}
			calls[baseIdx+2] = Multicall2Call{Target: addr, CallData: feeSig}
		}

		// 2. Execute tryAggregate
		callData, err := multicall3ABI.Pack("tryAggregate", false, calls)
		if err != nil {
			markAllErrors(errs, err)
			return
		}

		resp, err := client.CallContract(ctx, ethereum.CallMsg{To: &multicallAddress, Data: callData}, blockNumber)
		if err != nil {
			markAllErrors(errs, err)
			return
		}

		// 3. Unpack Multicall Results
		var results []Multicall2Result
		if err := multicall3ABI.UnpackIntoInterface(&results, "tryAggregate", resp); err != nil {
			markAllErrors(errs, err)
			return
		}

		// 4. Decode with Aerodrome-specific logic
		for i := 0; i < numPools; i++ {
			baseIdx := i * 3
			resS0 := results[baseIdx]
			resLiq := results[baseIdx+1]
			resFee := results[baseIdx+2]

			if !resS0.Success || !resLiq.Success || !resFee.Success {
				errs[i] = fmt.Errorf("aerodrome pool fetch reverted")
				continue
			}

			// Aerodrome slot0() manual decode
			// Layout: sqrtPriceX96 (uint160), tick (int24), ...
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
