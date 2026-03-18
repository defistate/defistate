package app

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/big"
	"sync"
	"sync/atomic"
	"time"

	"github.com/defistate/defistate/clients/chains/katana"
	ethclients "github.com/defistate/defistate/clients/eth-clients"
	"github.com/defistate/defistate/examples/swap-platform/chains/katana/server/app/router"
	token "github.com/defistate/defistate/protocols/erc20-token-system"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

type Platform struct {
	rpc         ethclients.ETHClient
	swapAddress common.Address
	state       atomic.Pointer[katana.State]
	router      atomic.Pointer[router.Router]

	prices           map[common.Address]float64
	quoteToken       common.Address
	quoteTokenAmount *big.Int
	mu               sync.RWMutex
}

func NewPlatform(
	ctx context.Context,
	quoteTokenAmount *big.Int,
	quoteToken common.Address,
	swapAddress common.Address,
	rpc ethclients.ETHClient,
) *Platform {
	p := &Platform{
		rpc:              rpc,
		swapAddress:      swapAddress,
		quoteToken:       quoteToken,
		quoteTokenAmount: quoteTokenAmount,
		prices:           make(map[common.Address]float64),
	}

	go p.loop(ctx, time.NewTicker(10*time.Second))

	return p
}

func (p *Platform) loop(ctx context.Context, ticker *time.Ticker) {
	for {
		select {
		case <-ticker.C:
			p.updatePrices()
		case <-ctx.Done():
			return
		}
	}
}

func (p *Platform) updatePrices() error {
	state := p.state.Load()
	rt := p.router.Load()

	if state == nil || rt == nil {
		return errors.New("state unavailable")
	}

	quoteToken, ok := state.Tokens.GetByAddress(p.quoteToken)
	if !ok {
		return fmt.Errorf("quote token %s not found in state", p.quoteToken.Hex())
	}

	rates, err := rt.GetExchangeRates(
		p.quoteTokenAmount,
		quoteToken.ID,
		4,
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to get exchange rates: %w", err)
	}

	oneQuoteToken := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(quoteToken.Decimals)), nil)
	prices := map[common.Address]float64{}
	for tokenId, rate := range rates {
		token, ok := state.Tokens.GetByID(tokenId)
		if !ok {
			//maybe warn
			continue
		}

		// normalize rates by multiplying ( 10**base token decimal / baseAmountIn)
		normalizedRate := new(big.Int).Set(rate)
		normalizedRate.Mul(normalizedRate, oneQuoteToken)
		normalizedRate.Div(normalizedRate, p.quoteTokenAmount)
		normalizedRateF64, _ := normalizedRate.Float64()
		prices[token.Address] = 1 / (normalizedRateF64 / (math.Pow(10, float64(token.Decimals))))
	}

	p.mu.Lock()
	p.prices = prices
	p.mu.Unlock()

	return nil
}

func (p *Platform) SetState(state *katana.State) error {
	router, err := router.NewRouter(state)
	if err != nil {
		return err
	}

	p.state.Store(state)
	p.router.Store(router)
	return nil
}

func (p *Platform) Tokens() ([]token.TokenView, error) {
	state := p.state.Load()
	if state == nil {
		return nil, errors.New("state unavailable")
	}

	allTokens := state.Tokens.All()
	routable := make([]token.TokenView, 0, len(allTokens))
	graphedTokens := make(map[uint64]struct{})
	for _, t := range state.Graph.Tokens {
		graphedTokens[t] = struct{}{}
	}

	for _, t := range allTokens {
		if _, ok := graphedTokens[t.ID]; ok {
			routable = append(routable, t)
		}
	}

	// tokens exist that are not routable
	// return only routable tokens
	return routable, nil
}

func (p *Platform) Prices() (prices map[common.Address]float64, quoteToken common.Address, err error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.prices == nil {
		return nil, common.Address{}, errors.New("prices unavailable")
	}
	return p.prices, p.quoteToken, nil
}

func (p *Platform) Quote(
	tokenIn common.Address,
	tokenOut common.Address,
	amountIn *big.Int,
) (amountOut *big.Int, err error) {
	state := p.state.Load()
	rt := p.router.Load()

	if state == nil || rt == nil {
		return nil, errors.New("state unavailable")
	}

	// 1. Resolve tokenIn address to ID
	tIn, ok := state.Tokens.GetByAddress(tokenIn)
	if !ok {
		return nil, fmt.Errorf("tokenIn %s not found", tokenIn.Hex())
	}

	// 2. Resolve tokenOut address to ID
	tOut, ok := state.Tokens.GetByAddress(tokenOut)
	if !ok {
		return nil, fmt.Errorf("tokenOut %s not found", tokenOut.Hex())
	}

	// 3. Find the best path (using 4 runs/hops as a standard DeFi depth)
	_, amountOut, err = rt.FindBestSwapPath(tIn.ID, tOut.ID, amountIn, 4)
	if err != nil {
		return nil, err
	}

	if amountOut == nil {
		return new(big.Int), nil
	}
	return amountOut, nil
}

type Transaction struct {
	To    common.Address `json:"to"`
	Data  hexutil.Bytes  `json:"data"`
	Value *hexutil.Big   `json:"value"`
	Gas   *hexutil.Big   `json:"gas"`
}

func (p *Platform) Swap(
	user common.Address,
	receiver common.Address,
	tokenIn common.Address,
	tokenOut common.Address,
	amountIn *big.Int,
) (txs []Transaction, err error) {
	state := p.state.Load()
	rt := p.router.Load()

	if state == nil || rt == nil {
		return nil, errors.New("state unavailable")
	}

	// 1. Resolve tokenIn address to ID
	tIn, ok := state.Tokens.GetByAddress(tokenIn)
	if !ok {
		return nil, fmt.Errorf("tokenIn %s not found", tokenIn.Hex())
	}

	// 2. Resolve tokenOut address to ID
	tOut, ok := state.Tokens.GetByAddress(tokenOut)
	if !ok {
		return nil, fmt.Errorf("tokenOut %s not found", tokenOut.Hex())
	}

	// 3. Find the best path (using 4 runs/hops as a standard DeFi depth)
	path, amountOut, err := rt.FindBestSwapPath(tIn.ID, tOut.ID, amountIn, 4)
	if err != nil {
		return nil, err
	}

	if path == nil || amountOut == nil || amountOut.Sign() == 0 {
		return nil, errors.New("no route found with positive output")
	}

	allowance, err := getERC20TokenAllowance(
		user,
		p.swapAddress,
		tokenIn,
		p.rpc,
	)

	if err != nil {
		return nil, err
	}

	if allowance.Cmp(amountIn) == -1 {
		// encode approve call and append to txs
		data, err := getERC20ApproveData(p.swapAddress, amountIn)
		if err != nil {
			return nil, errors.New("unable to encode approve data")
		}

		approveTx := Transaction{
			To:    tokenIn,
			Data:  hexutil.Bytes(data),
			Gas:   (*hexutil.Big)(big.NewInt(100000)),
			Value: (*hexutil.Big)(new(big.Int)),
		}
		txs = append(txs, approveTx)
	}

	swapData, gas, err := p.generateSwapData(
		amountIn,
		tokenIn,
		tokenOut,
		receiver,
		path,
	)

	if err != nil {
		return nil, err
	}

	swapTx := Transaction{
		To:    p.swapAddress,
		Data:  hexutil.Bytes(swapData),
		Gas:   (*hexutil.Big)(gas),
		Value: (*hexutil.Big)(new(big.Int)),
	}
	txs = append(txs, swapTx)

	return txs, nil
}

func (p *Platform) generateSwapData(
	amountIn *big.Int,
	tokenIn common.Address,
	tokenOut common.Address,
	receiver common.Address,
	path []router.TokenPoolPath,
) (swapData []byte, gas *big.Int, err error) {
	state := p.state.Load()

	if state == nil {
		return nil, nil, errors.New("state unavailable")
	}

	numSwaps := len(path)
	allSwapData := make([][]byte, 0, numSwaps)
	totalGas := big.NewInt(0)
	protocols := state.Pools.GetProtocols()
	for i := range numSwaps {
		var (
			prevPoolWillTransfer     bool
			nextPoolRequiresTransfer bool
			nextPoolAddress          common.Address
			err                      error
			swapData                 []byte
			gas                      *big.Int
		)

		step := path[i]
		tIn, _ := state.Tokens.GetByID(step.TokenInID)
		tOut, _ := state.Tokens.GetByID(step.TokenOutID)
		poolInfo, _ := state.Pools.GetByID(step.PoolID)

		if i > 0 {
			prevStep := path[i-1]
			prevPoolInfo, _ := state.Pools.GetByID(prevStep.PoolID)
			schema, ok := state.ProtocolResolver.ResolveSchemaFromPoolID(prevPoolInfo.ID)
			if !ok {
				return nil, nil, fmt.Errorf("schema for pool %d not found", prevPoolInfo.ID)
			}
			prevPoolWillTransfer = willTransferTokenIn(schema)
		}

		if i < numSwaps-1 {
			lookaheadStep := path[i+1]
			nextPoolInfo, ok := state.Pools.GetByID(lookaheadStep.PoolID)
			if !ok {
				return nil, nil, fmt.Errorf("next pool id not found")

			}
			schema, ok := state.ProtocolResolver.ResolveSchemaFromPoolID(nextPoolInfo.ID)
			if !ok {
				return nil, nil, fmt.Errorf("pool %d not found", nextPoolInfo.ID)
			}
			nextPoolRequiresTransfer = requiresTransferTokenOut(schema)
			nextPoolAddress, err = nextPoolInfo.Key.ToAddress()
			if err != nil {
				return nil, nil, err
			}
		}

		protocolID := protocols[poolInfo.Protocol]

		switch protocolID {
		case katana.SushiV3ProtocolID:
			pool, ok := state.SushiV3.GetByID(step.PoolID)
			if !ok {
				return nil, nil, errors.New("pool not found")
			}
			swapData, gas, err = generateUniswapV3SwapDataAndGas(i, tIn, tOut, prevPoolWillTransfer, nextPoolRequiresTransfer, receiver, nextPoolAddress, pool)

		default:
			// swap only supports SushiV3 for now
			return nil, nil, errors.New("cannot generate swap data for unsupported protocol")
		}

		allSwapData = append(allSwapData, swapData)
		totalGas.Add(totalGas, gas)

	}

	if len(allSwapData) == 0 {
		return nil, nil, errors.New("could not generate swap data for path")
	}

	swapData, err = getTransferFromAndSwapData(
		amountIn,
		tokenIn,
		tokenOut,
		receiver,
		mergeSwapData(allSwapData),
	)
	if err != nil {
		return nil, nil, err
	}

	gas = totalGas
	return swapData, gas, nil
}
