package pancakeswap

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	ethclients "github.com/defistate/defistate/clients/eth-clients"
	uniswapv3abi "github.com/defistate/defistate/protocols/uniswap-v3/abi"
	pancakeswapv3abi "github.com/defistate/defistate/protocols/uniswap-v3/abi/forks/pancakeswap"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
)

// Pre-defined errors for the PoolInitializer.
var (
	ErrPackPoolInitMultiFailed   = errors.New("failed to pack multicall input for pool init")
	ErrPoolInitMulticallFailed   = errors.New("pool init multicall rpc call failed")
	ErrUnpackPoolInitMultiFailed = errors.New("failed to unpack pool init multicall results")
	ErrInvalidPoolInitResponse   = errors.New("invalid or incomplete response from pool init multicall")
	ErrUnpackPoolInitReturn      = errors.New("failed to unpack return data for a pool init sub-call")
)

// NewPoolInitializer creates a provider that fetches all core pool information in a single, batched multicall.
// The 'multicallAddress' parameter is the address of the deployed Multicall3 contract on your target chain.
func NewPoolInitializer(multicallAddress common.Address) func(
	ctx context.Context,
	poolAddress common.Address,
	client ethclients.ETHClient,
	blockNumber *big.Int,
) (
	token0 common.Address,
	token1 common.Address,
	fee uint64,
	tickSpacing uint64,
	tick int64,
	liquidity *big.Int,
	sqrtPriceX96 *big.Int,
	err error,
) {
	return func(
		ctx context.Context,
		poolAddress common.Address,
		client ethclients.ETHClient,
		blockNumber *big.Int,
	) (
		token0 common.Address,
		token1 common.Address,
		fee uint64,
		tickSpacing uint64,
		tick int64,
		liquidity *big.Int,
		sqrtPriceX96 *big.Int,
		err error,
	) {
		// --- Step 1: Prepare the calls for the Multicall contract ---
		// We will fetch token0, token1, fee, tickSpacing, slot0, and liquidity.
		callNames := []string{"token0", "token1", "fee", "tickSpacing", "slot0", "liquidity"}
		calls := make([]struct {
			Target   common.Address
			CallData []byte
		}, len(callNames))

		for i, name := range callNames {
			callData, packErr := pancakeswapv3abi.PancakeswapV3ABI.Pack(name)
			if packErr != nil {
				err = fmt.Errorf("failed to pack call data for %s: %w", name, packErr)
				return
			}
			calls[i] = struct {
				Target   common.Address
				CallData []byte
			}{
				Target:   poolAddress,
				CallData: callData,
			}
		}

		// --- Step 2: Execute the batch call ---
		multicallInput, err := uniswapv3abi.Multicall3ABI.Pack("tryAggregate", false, calls)
		if err != nil {
			err = fmt.Errorf("%w: %v", ErrPackPoolInitMultiFailed, err)
			return
		}

		returnData, err := client.CallContract(ctx, ethereum.CallMsg{To: &multicallAddress, Data: multicallInput}, blockNumber)
		if err != nil {
			err = fmt.Errorf("%w for pool %s: %v", ErrPoolInitMulticallFailed, poolAddress.Hex(), err)
			return
		}
		if len(returnData) == 0 {
			err = fmt.Errorf("%w for pool %s: no data returned", ErrPoolInitMulticallFailed, poolAddress.Hex())
			return
		}

		// --- Step 3: Decode the results ---
		type multicallResult struct {
			Success    bool
			ReturnData []byte
		}
		var multicallResults []multicallResult
		err = uniswapv3abi.Multicall3ABI.UnpackIntoInterface(&multicallResults, "tryAggregate", returnData)
		if err != nil {
			err = fmt.Errorf("%w for pool %s: %v", ErrUnpackPoolInitMultiFailed, poolAddress.Hex(), err)
			return
		}

		if len(multicallResults) != len(callNames) {
			err = fmt.Errorf("%w for pool %s: expected %d results, got %d", ErrInvalidPoolInitResponse, poolAddress.Hex(), len(callNames), len(multicallResults))
			return
		}

		// --- Step 4: Unpack each result ---
		// Unpack token0
		if !multicallResults[0].Success {
			err = fmt.Errorf("%w: token0 call failed", ErrInvalidPoolInitResponse)
			return
		}
		err = pancakeswapv3abi.PancakeswapV3ABI.UnpackIntoInterface(&token0, "token0", multicallResults[0].ReturnData)
		if err != nil {
			err = fmt.Errorf("%w (token0): %v", ErrUnpackPoolInitReturn, err)
			return
		}

		// Unpack token1
		if !multicallResults[1].Success {
			err = fmt.Errorf("%w: token1 call failed", ErrInvalidPoolInitResponse)
			return
		}
		err = pancakeswapv3abi.PancakeswapV3ABI.UnpackIntoInterface(&token1, "token1", multicallResults[1].ReturnData)
		if err != nil {
			err = fmt.Errorf("%w (token1): %v", ErrUnpackPoolInitReturn, err)
			return
		}

		// Unpack fee
		if !multicallResults[2].Success {
			err = fmt.Errorf("%w: fee call failed", ErrInvalidPoolInitResponse)
			return
		}
		var feeBig *big.Int
		err = pancakeswapv3abi.PancakeswapV3ABI.UnpackIntoInterface(&feeBig, "fee", multicallResults[2].ReturnData)
		if err != nil {
			err = fmt.Errorf("%w (fee): %v", ErrUnpackPoolInitReturn, err)
			return
		}
		fee = feeBig.Uint64()

		// Unpack tickSpacing
		if !multicallResults[3].Success {
			err = fmt.Errorf("%w: tickSpacing call failed", ErrInvalidPoolInitResponse)
			return
		}
		var tickSpacingBig *big.Int
		err = pancakeswapv3abi.PancakeswapV3ABI.UnpackIntoInterface(&tickSpacingBig, "tickSpacing", multicallResults[3].ReturnData)
		if err != nil {
			err = fmt.Errorf("%w (tickSpacing): %v", ErrUnpackPoolInitReturn, err)
			return
		}
		tickSpacing = tickSpacingBig.Uint64()

		// Unpack slot0
		if !multicallResults[4].Success {
			err = fmt.Errorf("%w: slot0 call failed", ErrInvalidPoolInitResponse)
			return
		}
		// FIX: The destination struct for unpacking must match the full signature of the contract function.
		var slot0Result struct {
			SqrtPriceX96               *big.Int
			Tick                       *big.Int
			ObservationIndex           uint16
			ObservationCardinality     uint16
			ObservationCardinalityNext uint16
			FeeProtocol                uint32
			Unlocked                   bool
		}
		err = pancakeswapv3abi.PancakeswapV3ABI.UnpackIntoInterface(&slot0Result, "slot0", multicallResults[4].ReturnData)
		if err != nil {
			err = fmt.Errorf("%w (slot0): %v", ErrUnpackPoolInitReturn, err)
			return
		}
		sqrtPriceX96 = slot0Result.SqrtPriceX96
		tick = slot0Result.Tick.Int64()

		// Unpack liquidity
		if !multicallResults[5].Success {
			err = fmt.Errorf("%w: liquidity call failed", ErrInvalidPoolInitResponse)
			return
		}
		err = pancakeswapv3abi.PancakeswapV3ABI.UnpackIntoInterface(&liquidity, "liquidity", multicallResults[5].ReturnData)
		if err != nil {
			err = fmt.Errorf("%w (liquidity): %v", ErrUnpackPoolInitReturn, err)
			return
		}

		return
	}
}
