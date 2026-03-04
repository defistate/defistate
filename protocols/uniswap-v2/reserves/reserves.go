package reserves

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"

	ethclients "github.com/defistate/defistate/clients/eth-clients"
	uniswapv2abi "github.com/defistate/defistate/protocols/uniswap-v2/abi"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

var (
	getReservesSig = uniswapv2abi.UniswapV2ABI.Methods["getReserves"].ID
	multicall3ABI  abi.ABI
)

const (
	defaultRPCTimeout = 10 * time.Second
	// Updated to tryAggregate signature
	multicall3JSON = `[{"inputs":[{"internalType":"bool","name":"requireSuccess","type":"bool"},{"components":[{"internalType":"address","name":"target","type":"address"},{"internalType":"bytes","name":"callData","type":"bytes"}],"internalType":"struct Multicall3.Call[]","name":"calls","type":"tuple[]"}],"name":"tryAggregate","outputs":[{"components":[{"internalType":"bool","name":"success","type":"bool"},{"internalType":"bytes","name":"returnData","type":"bytes"}],"internalType":"struct Multicall3.Result[]","name":"returnData","type":"tuple[]"}],"stateMutability":"payable","type":"function"}]`
)

func init() {
	var err error
	multicall3ABI, err = abi.JSON(strings.NewReader(multicall3JSON))
	if err != nil {
		panic("failed to parse Multicall3 ABI: " + err.Error())
	}
}

// Multicall2Call matches the tryAggregate call struct
type Multicall2Call struct {
	Target   common.Address `json:"target"`
	CallData []byte         `json:"callData"`
}

// Result remains the same as the return structure is identical to aggregate3
type Result struct {
	Success    bool   `json:"success"`
	ReturnData []byte `json:"returnData"`
}

func NewGetReserves(
	maxConcurrentCalls int,
	chunkSize int,
	multicallAddress common.Address,
) func(ctx context.Context, poolAddrs []common.Address, getClient func() (ethclients.ETHClient, error), blockNumber *big.Int) ([]*big.Int, []*big.Int, []error) {

	// Updated signature to include blockNumber *big.Int
	return func(ctx context.Context, poolAddrs []common.Address, getClient func() (ethclients.ETHClient, error), blockNumber *big.Int) ([]*big.Int, []*big.Int, []error) {
		numPools := len(poolAddrs)
		if numPools == 0 {
			return nil, nil, nil
		}

		semaphore := make(chan struct{}, maxConcurrentCalls)
		reserve0s := make([]*big.Int, numPools)
		reserve1s := make([]*big.Int, numPools)
		errs := make([]error, numPools)

		var wg sync.WaitGroup
		for i := 0; i < numPools; i += chunkSize {
			end := i + chunkSize
			if end > numPools {
				end = numPools
			}

			wg.Add(1)
			semaphore <- struct{}{}

			go func(start, end int) {
				defer func() {
					<-semaphore
					wg.Done()
				}()

				if ctx.Err() != nil {
					markChunkError(errs, start, end, ctx.Err())
					return
				}
				// Pass blockNumber to processChunk
				processChunk(ctx, start, end, poolAddrs, multicallAddress, getClient, blockNumber, reserve0s, reserve1s, errs)
			}(i, end)
		}
		wg.Wait()
		return reserve0s, reserve1s, errs
	}
}

func processChunk(ctx context.Context, start, end int, pools []common.Address, mc common.Address, getClient func() (ethclients.ETHClient, error), blockNumber *big.Int, r0s, r1s []*big.Int, errs []error) {
	client, err := getClient()
	if err != nil {
		markChunkError(errs, start, end, err)
		return
	}

	cctx, cancel := context.WithTimeout(ctx, defaultRPCTimeout)
	defer cancel()

	size := end - start
	calls := make([]Multicall2Call, size)
	for i := 0; i < size; i++ {
		calls[i] = Multicall2Call{Target: pools[start+i], CallData: getReservesSig}
	}

	// tryAggregate(requireSuccess bool, calls Call[])
	// Set requireSuccess to false to match the allowFailure behavior
	callData, err := multicall3ABI.Pack("tryAggregate", false, calls)
	if err != nil {
		markChunkError(errs, start, end, err)
		return
	}

	// Pass blockNumber here instead of nil
	resp, err := client.CallContract(cctx, ethereum.CallMsg{To: &mc, Data: callData}, blockNumber)
	if err != nil {
		markChunkError(errs, start, end, err)
		return
	}

	var results []Result
	if err := multicall3ABI.UnpackIntoInterface(&results, "tryAggregate", resp); err != nil {
		markChunkError(errs, start, end, err)
		return
	}

	for i, res := range results {
		idx := start + i
		if !res.Success {
			errs[idx] = fmt.Errorf("revert at %s", pools[idx].Hex())
			continue
		}
		if len(res.ReturnData) < 64 {
			errs[idx] = fmt.Errorf("bad len: %d", len(res.ReturnData))
			continue
		}
		r0s[idx] = new(big.Int).SetBytes(res.ReturnData[0:32])
		r1s[idx] = new(big.Int).SetBytes(res.ReturnData[32:64])
	}
}

func markChunkError(errs []error, start, end int, err error) {
	for i := start; i < end; i++ {
		errs[i] = err
	}
}
