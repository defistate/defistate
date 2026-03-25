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

	"github.com/defistate/defistate/clients/chains/celo"
	ethclients "github.com/defistate/defistate/clients/eth-clients"
	"github.com/defistate/defistate/examples/celo-flow/server/app/router"
	token "github.com/defistate/defistate/protocols/erc20-token-system"
	uniswapv2 "github.com/defistate/defistate/protocols/uniswap-v2"
	uniswapv2math "github.com/defistate/defistate/protocols/uniswap-v2/math"
	uniswapv3 "github.com/defistate/defistate/protocols/uniswap-v3"
	uniswapv3math "github.com/defistate/defistate/protocols/uniswap-v3/math"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

const (
	DefaultFindBestSwapPathRuns = 4
)

type Platform struct {
	rpc         ethclients.ETHClient
	swapAddress common.Address
	state       atomic.Pointer[celo.State]
	router      atomic.Pointer[router.Router]

	prices           map[common.Address]float64
	quoteToken       common.Address
	quoteTokenAmount *big.Int

	maxQuoteValuePerFragment *big.Int
	maxFragments             int

	mu sync.RWMutex
}

func NewPlatform(
	ctx context.Context,
	quoteTokenAmount *big.Int,
	quoteToken common.Address,
	swapAddress common.Address,
	rpc ethclients.ETHClient,
	maxQuoteValuePerFragment *big.Int,
	maxFragments int,
) *Platform {
	p := &Platform{
		rpc:                      rpc,
		swapAddress:              swapAddress,
		quoteToken:               quoteToken,
		quoteTokenAmount:         quoteTokenAmount,
		maxQuoteValuePerFragment: maxQuoteValuePerFragment,
		maxFragments:             maxFragments,
		prices:                   make(map[common.Address]float64),
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

func (p *Platform) SetState(state *celo.State) error {
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

	if len(p.prices) == 0 {
		return nil, common.Address{}, errors.New("prices unavailable")
	}
	return p.prices, p.quoteToken, nil
}
func (p *Platform) WaitForPrices(
	ctx context.Context,
	timeout time.Duration,
	requiredTokens ...common.Address,
) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for prices: %w", ctx.Err())

		case <-ticker.C:
			prices, _, err := p.Prices()
			if err != nil {
				continue
			}

			// must have at least one price
			if len(prices) == 0 {
				continue
			}

			ok := true
			for _, token := range requiredTokens {
				if _, exists := prices[token]; !exists {
					ok = false
					break
				}
			}

			if ok {
				return nil
			}
		}
	}
}
func (p *Platform) getFragments(
	amount *big.Int,
	tokenAddress common.Address,
) (int, error) {
	if amount == nil || amount.Sign() <= 0 {
		return 0, fmt.Errorf("invalid amount")
	}
	if p.maxFragments <= 0 {
		return 0, fmt.Errorf("invalid maxFragments: %d", p.maxFragments)
	}
	if p.maxQuoteValuePerFragment == nil || p.maxQuoteValuePerFragment.Sign() <= 0 {
		return 0, errors.New("invalid maxQuoteValuePerFragment")
	}

	state := p.state.Load()
	if state == nil {
		return 0, errors.New("state unavailable")
	}

	tokenIn, ok := state.Tokens.GetByAddress(tokenAddress)
	if !ok {
		return 0, fmt.Errorf("token %s not found", tokenAddress.Hex())
	}

	quoteToken, ok := state.Tokens.GetByAddress(p.quoteToken)
	if !ok {
		return 0, fmt.Errorf("quote token %s not found", p.quoteToken.Hex())
	}

	var quoteValueRaw *big.Int

	// If token is already the quote token, value is just the raw amount.
	if tokenAddress == p.quoteToken {
		quoteValueRaw = new(big.Int).Set(amount)
	} else {
		prices, _, err := p.Prices()
		if err != nil {
			return 0, err
		}

		price, ok := prices[tokenAddress]
		if !ok || price <= 0 {
			return 0, fmt.Errorf("price unavailable for token %s", tokenAddress.Hex())
		}

		// quoteValueRaw =
		//   amountRaw * price * 10^quoteDecimals / 10^tokenDecimals
		amountF := new(big.Float).SetInt(amount)
		priceF := big.NewFloat(price)

		tokenScale := new(big.Float).SetInt(
			new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(tokenIn.Decimals)), nil),
		)
		quoteScale := new(big.Float).SetInt(
			new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(quoteToken.Decimals)), nil),
		)

		quoteValueF := new(big.Float).Mul(amountF, priceF)
		quoteValueF.Mul(quoteValueF, quoteScale)
		quoteValueF.Quo(quoteValueF, tokenScale)

		quoteValueRaw, _ = quoteValueF.Int(nil)
		if quoteValueRaw == nil {
			return 0, errors.New("unable to compute quote value")
		}
	}

	// fragments = ceil(quoteValueRaw / maxQuoteValuePerFragment)
	fragments := new(big.Int).Add(
		quoteValueRaw,
		new(big.Int).Sub(new(big.Int).Set(p.maxQuoteValuePerFragment), big.NewInt(1)),
	)
	fragments.Div(fragments, p.maxQuoteValuePerFragment)
	if !fragments.IsInt64() {
		return p.maxFragments, nil
	}

	n := int(fragments.Int64())
	if n < 1 {
		n = 1
	}
	if n > p.maxFragments {
		n = p.maxFragments
	}
	return n, nil
}

func (p *Platform) QuoteAlgorithm(
	tokenIn common.Address,
	tokenOut common.Address,
	amountIn *big.Int,
	maxFragments int,
) (paths [][]router.TokenPoolPath, amountsIn []*big.Int, amountsOut []*big.Int, err error) {
	if maxFragments <= 0 {
		return nil, nil, nil, fmt.Errorf("invalid maxFragments: %d", maxFragments)
	}

	state := p.state.Load()
	rt := p.router.Load()

	if state == nil || rt == nil {
		return nil, nil, nil, errors.New("state unavailable")
	}

	tIn, ok := state.Tokens.GetByAddress(tokenIn)
	if !ok {
		return nil, nil, nil, fmt.Errorf("tokenIn %s not found", tokenIn.Hex())
	}

	tOut, ok := state.Tokens.GetByAddress(tokenOut)
	if !ok {
		return nil, nil, nil, fmt.Errorf("tokenOut %s not found", tokenOut.Hex())
	}

	amountInPerFragment := new(big.Int).Div(amountIn, big.NewInt(int64(maxFragments)))
	amountInRemainder := new(big.Int).Mod(amountIn, big.NewInt(int64(maxFragments)))

	// else, use all fragments
	paths = [][]router.TokenPoolPath{}
	amountsIn = []*big.Int{}
	amountsOut = []*big.Int{}
	overrides := &router.PoolOverrides{}

	fragmentAmounts := make([]*big.Int, 0, maxFragments)
	// base fragments
	for i := 0; i < maxFragments; i++ {
		if amountInPerFragment.Sign() > 0 {
			fragmentAmounts = append(fragmentAmounts, new(big.Int).Set(amountInPerFragment))
		}
	}

	// handle remainder
	if amountInRemainder.Sign() > 0 {
		// threshold = 25% of a normal fragment
		threshold := new(big.Int).Div(amountInPerFragment, big.NewInt(4))

		if amountInRemainder.Cmp(threshold) >= 0 {
			// significant → keep as its own fragment
			fragmentAmounts = append(fragmentAmounts, new(big.Int).Set(amountInRemainder))
		} else if len(fragmentAmounts) > 0 {
			// too small → merge into last fragment
			fragmentAmounts[len(fragmentAmounts)-1].Add(
				fragmentAmounts[len(fragmentAmounts)-1],
				amountInRemainder,
			)
		}
	}

	for _, fragment := range fragmentAmounts {
		pathFragment, amountOutFragment, err := rt.FindBestSwapPathGreedy(
			DefaultFindBestSwapPathRuns,
			fragment,
			tIn.ID,
			tOut.ID,
			overrides,
		)
		if err != nil {
			return nil, nil, nil, err
		}

		overrides, err = getOverrides(
			fragment,
			pathFragment,
			overrides,
			state,
		)
		if err != nil {
			return nil, nil, nil, err
		}

		duplicatePath := false
		for i, prevPath := range paths {
			if router.EqualTokenPoolPaths(prevPath, pathFragment) {
				duplicatePath = true
				amountsIn[i].Add(amountsIn[i], fragment)
				amountsOut[i].Add(amountsOut[i], amountOutFragment)
				break
			}
		}

		if !duplicatePath {
			paths = append(paths, pathFragment)
			amountsIn = append(amountsIn, new(big.Int).Set(fragment))
			amountsOut = append(amountsOut, amountOutFragment)
		}
	}
	return paths, amountsIn, amountsOut, nil
}

func (p *Platform) Quote(
	tokenIn common.Address,
	tokenOut common.Address,
	amountIn *big.Int,
) (*big.Int, error) {
	fragments, err := p.getFragments(amountIn, tokenIn)
	if err != nil {
		return nil, err
	}
	_, _, amountsOut, err := p.QuoteAlgorithm(
		tokenIn,
		tokenOut,
		amountIn,
		fragments,
	)

	if err != nil {
		return nil, err
	}

	amountOut := new(big.Int)
	for _, b := range amountsOut {
		amountOut.Add(amountOut, b)
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
	slippageBps uint64,
) (txs []Transaction, err error) {
	if amountIn == nil || amountIn.Sign() <= 0 {
		return nil, errors.New("invalid amountIn")
	}

	if slippageBps > 10_000 {
		return nil, errors.New("invalid slippage bps")
	}

	fragments, err := p.getFragments(amountIn, tokenIn)
	if err != nil {
		return nil, err
	}

	paths, amountsIn, amountsOut, err := p.QuoteAlgorithm(
		tokenIn,
		tokenOut,
		amountIn,
		fragments,
	)
	if err != nil {
		return nil, err
	}

	amountOut := new(big.Int)
	for _, b := range amountsOut {
		amountOut.Add(amountOut, b)
	}

	if amountOut.Sign() <= 0 {
		return nil, errors.New("invalid quoted amountOut")
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

	swapData := make([][]byte, len(paths))
	gas := new(big.Int)

	for i, path := range paths {
		var g *big.Int
		swapData[i], g, err = p.generateSwapData(path, receiver)
		if err != nil {
			return nil, err
		}
		gas.Add(gas, g)
	}

	// minOut = amountOut * (10000 - slippageBps) / 10000
	minOut := new(big.Int).Set(amountOut)
	if slippageBps > 0 {
		minOut.Mul(minOut, big.NewInt(int64(10_000-slippageBps)))
		minOut.Div(minOut, big.NewInt(10_000))
	}

	data, err := getTransferFromAndSwapBatchWithMinOut(
		amountIn,
		amountsIn,
		minOut,
		tokenIn,
		tokenOut,
		receiver,
		swapData,
	)
	if err != nil {
		return nil, err
	}

	swapTx := Transaction{
		To:    p.swapAddress,
		Data:  hexutil.Bytes(data),
		Gas:   (*hexutil.Big)(gas),
		Value: (*hexutil.Big)(new(big.Int)),
	}
	txs = append(txs, swapTx)

	return txs, nil
}
func (p *Platform) generateSwapData(
	path []router.TokenPoolPath,
	receiver common.Address,
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
		case celo.UniswapV2ProtocolID:
			pool, ok := state.UniswapV2.GetByID(step.PoolID)
			if !ok {
				return nil, nil, errors.New("pool not found")
			}
			swapData, gas, err = generateUniswapV2SwapDataAndGas(i, tIn, tOut, prevPoolWillTransfer, nextPoolRequiresTransfer, receiver, nextPoolAddress, pool)

		case celo.UniswapV3ProtocolID:
			pool, ok := state.UniswapV3.GetByID(step.PoolID)
			if !ok {
				return nil, nil, errors.New("pool not found")
			}
			swapData, gas, err = generateUniswapV3SwapDataAndGas(i, tIn, tOut, prevPoolWillTransfer, nextPoolRequiresTransfer, receiver, nextPoolAddress, pool)
		case celo.AerodromeV3ProtocolID:
			pool, ok := state.AerodromeV3.GetByID(step.PoolID)
			if !ok {
				return nil, nil, errors.New("pool not found")
			}
			swapData, gas, err = generateUniswapV3SwapDataAndGas(i, tIn, tOut, prevPoolWillTransfer, nextPoolRequiresTransfer, receiver, nextPoolAddress, pool)

		default:
			return nil, nil, errors.New("cannot generate swap data for unsupported protocol")
		}

		allSwapData = append(allSwapData, swapData)
		totalGas.Add(totalGas, gas)

	}

	if len(allSwapData) == 0 {
		return nil, nil, errors.New("could not generate swap data for path")
	}

	gas = totalGas
	return mergeSwapData(allSwapData), gas, nil
}

func getOverrides(amountIn *big.Int, paths []router.TokenPoolPath, prevOverrides *router.PoolOverrides, state *celo.State) (*router.PoolOverrides, error) {
	// ensure we do not mutate inputs
	amountIn = new(big.Int).Set(amountIn)
	overrides := copyOverrides(prevOverrides)
	protocols := state.Pools.GetProtocols()
	for _, path := range paths {
		poolID := path.PoolID
		// @todo we need Index.HasPool(uint64) functions
		p, ok := state.Pools.GetByID(poolID)
		if !ok {
			return nil, fmt.Errorf("pool with id %d nof found in pools", poolID)
		}

		protocolID, ok := protocols[p.Protocol]
		if !ok {
			return nil, fmt.Errorf("protocol %d  not found in Pool protocols", p.Protocol)
		}

		switch protocolID {

		case celo.UniswapV2ProtocolID:
			var pool uniswapv2.Pool
			if overrides.UniswapV2 != nil {
				pool, ok = overrides.UniswapV2[poolID]
				if !ok {
					pool, ok = state.UniswapV2.GetByID(poolID)
					if !ok {
						return nil, fmt.Errorf("pool %d not found in UniswapV3", poolID)
					}
				}
			} else {
				pool, ok = state.UniswapV2.GetByID(poolID)
				if !ok {
					return nil, fmt.Errorf("pool %d not found in UniswapV3", poolID)
				}
			}

			amountOut, u, err := uniswapv2math.SimulateSwap(
				amountIn,
				path.TokenInID,
				path.TokenOutID,
				uniswapv2.PoolView{
					ID:       pool.IDs.Pool,
					Token0:   pool.IDs.Token0,
					Token1:   pool.IDs.Token1,
					Reserve0: pool.Reserve0,
					Reserve1: pool.Reserve1,
					FeeBps:   pool.FeeBps,
				},
			)
			if err != nil {
				return nil, err
			}

			//update amountIn
			amountIn.Set(amountOut)
			// set updated pool
			updatedPool := pool
			updatedPool.Reserve0 = u.Reserve0
			updatedPool.Reserve1 = u.Reserve1
			if overrides.UniswapV2 == nil {
				overrides.UniswapV2 = make(map[uint64]uniswapv2.Pool)
			}
			overrides.UniswapV2[pool.IDs.Pool] = updatedPool
		case celo.UniswapV3ProtocolID:
			var pool uniswapv3.Pool
			if overrides.UniswapV3 != nil {
				pool, ok = overrides.UniswapV3[poolID]
				if !ok {
					pool, ok = state.UniswapV3.GetByID(poolID)
					if !ok {
						return nil, fmt.Errorf("pool %d not found in UniswapV3", poolID)
					}
				}
			} else {
				pool, ok = state.UniswapV3.GetByID(poolID)
				if !ok {
					return nil, fmt.Errorf("pool %d not found in UniswapV3", poolID)
				}
			}

			amountOut, u, err := uniswapv3math.SimulateExactInSwap(
				amountIn,
				nil,
				path.TokenInID,
				uniswapv3.PoolView{
					PoolViewMinimal: uniswapv3.PoolViewMinimal{
						ID:           pool.IDs.Pool,
						Token0:       pool.IDs.Token0,
						Token1:       pool.IDs.Token1,
						Fee:          pool.Fee,
						TickSpacing:  pool.TickSpacing,
						Tick:         pool.Tick,
						Liquidity:    pool.Liquidity,
						SqrtPriceX96: pool.SqrtPriceX96,
					},
					Ticks: pool.Ticks,
				},
			)
			if err != nil {
				return nil, err
			}

			//update amountIn
			amountIn.Set(amountOut)
			// set updated pool
			updatedPool := pool
			updatedPool.Tick = u.Tick
			updatedPool.Liquidity = u.Liquidity
			updatedPool.SqrtPriceX96 = u.SqrtPriceX96
			updatedPool.Ticks = u.Ticks
			if overrides.UniswapV3 == nil {
				overrides.UniswapV3 = make(map[uint64]uniswapv3.Pool)
			}
			overrides.UniswapV3[pool.IDs.Pool] = updatedPool
		case celo.AerodromeV3ProtocolID:
			var pool uniswapv3.Pool
			if overrides.AerodromeV3 != nil {
				pool, ok = overrides.AerodromeV3[poolID]
				if !ok {
					pool, ok = state.AerodromeV3.GetByID(poolID)
					if !ok {
						return nil, fmt.Errorf("pool %d not found in AerodromeV3", poolID)
					}
				}
			} else {
				pool, ok = state.AerodromeV3.GetByID(poolID)
				if !ok {
					return nil, fmt.Errorf("pool %d not found in AerodromeV3", poolID)
				}
			}

			amountOut, u, err := uniswapv3math.SimulateExactInSwap(
				amountIn,
				nil,
				path.TokenInID,
				uniswapv3.PoolView{
					PoolViewMinimal: uniswapv3.PoolViewMinimal{
						ID:           pool.IDs.Pool,
						Token0:       pool.IDs.Token0,
						Token1:       pool.IDs.Token1,
						Fee:          pool.Fee,
						TickSpacing:  pool.TickSpacing,
						Tick:         pool.Tick,
						Liquidity:    pool.Liquidity,
						SqrtPriceX96: pool.SqrtPriceX96,
					},
					Ticks: pool.Ticks,
				},
			)
			if err != nil {
				return nil, err
			}

			//update amountIn
			amountIn.Set(amountOut)
			// set updated pool
			updatedPool := pool
			updatedPool.Tick = u.Tick
			updatedPool.Liquidity = u.Liquidity
			updatedPool.SqrtPriceX96 = u.SqrtPriceX96
			updatedPool.Ticks = u.Ticks
			if overrides.AerodromeV3 == nil {
				overrides.AerodromeV3 = make(map[uint64]uniswapv3.Pool)
			}
			overrides.AerodromeV3[pool.IDs.Pool] = updatedPool
		}

	}

	return overrides, nil

}

func copyOverrides(old *router.PoolOverrides) *router.PoolOverrides {
	if old == nil {
		return &router.PoolOverrides{}
	}
	nw := &router.PoolOverrides{
		UniswapV2:   make(map[uint64]uniswapv2.Pool),
		UniswapV3:   make(map[uint64]uniswapv3.Pool),
		AerodromeV3: make(map[uint64]uniswapv3.Pool),
	}

	if old.UniswapV3 != nil {
		for id, pool := range old.UniswapV3 {
			nw.UniswapV3[id] = uniswapv3.Pool{
				Protocol:     pool.Protocol,
				IDs:          pool.IDs,
				Address:      pool.Address,
				Token0:       pool.Token0,
				Token1:       pool.Token1,
				Fee:          pool.Fee,
				TickSpacing:  pool.TickSpacing,
				Liquidity:    new(big.Int).Set(pool.Liquidity),
				SqrtPriceX96: new(big.Int).Set(pool.SqrtPriceX96),
				Tick:         pool.Tick,
				Ticks:        pool.Ticks, // we use a shallow copy here
			}
		}
	}
	if old.AerodromeV3 != nil {
		for id, pool := range old.AerodromeV3 {
			nw.AerodromeV3[id] = uniswapv3.Pool{
				Protocol:     pool.Protocol,
				IDs:          pool.IDs,
				Address:      pool.Address,
				Token0:       pool.Token0,
				Token1:       pool.Token1,
				Fee:          pool.Fee,
				TickSpacing:  pool.TickSpacing,
				Liquidity:    new(big.Int).Set(pool.Liquidity),
				SqrtPriceX96: new(big.Int).Set(pool.SqrtPriceX96),
				Tick:         pool.Tick,
				Ticks:        pool.Ticks, // we use a shallow copy here
			}
		}
	}
	if old.UniswapV2 != nil {
		for id, pool := range old.UniswapV2 {
			nw.UniswapV2[id] = uniswapv2.Pool{
				Protocol: pool.Protocol,
				IDs:      pool.IDs,
				Address:  pool.Address,
				Token0:   pool.Token0,
				Token1:   pool.Token1,
				Reserve0: new(big.Int).Set(pool.Reserve0),
				Reserve1: new(big.Int).Set(pool.Reserve1),
				FeeBps:   pool.FeeBps,
			}
		}
	}

	return nw

}
