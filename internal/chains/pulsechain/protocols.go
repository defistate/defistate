package ethereum

import (
	"context"
	"time"

	"github.com/defistate/defistate/engine"
	"github.com/defistate/defistate/internal/chains"
	uniswapv2protocol "github.com/defistate/defistate/internal/protocols/uniswapv2"
	uniswapv3protocol "github.com/defistate/defistate/internal/protocols/uniswapv3"

	uniswapv2poolinitializer "github.com/defistate/defistate/protocols/uniswap-v2/initializer"

	token "github.com/defistate/defistate/protocols/erc20-token-system"
	poolregistry "github.com/defistate/defistate/protocols/pool-registry"
	"github.com/ethereum/go-ethereum/common"
)

// --------------------------------------------------------------------------------
// --- Default Configuration Constants ---
// --------------------------------------------------------------------------------

const (
	// DefaultPruneFrequency controls how often we clean up stale data from memory.
	DefaultPruneFrequency = 10 * time.Minute

	// DefaultInitFrequency controls how often we check for new pools/pairs on-chain.
	DefaultInitFrequency = 10 * time.Second

	// DefaultResyncFrequency controls how often we force a full state reconciliation.
	DefaultResyncFrequency = 5 * time.Minute

	DefaultTickIndexerUpdateFrequency = 10 * time.Second

	// Log settings for retry logic (0 means disabled/default).
	DefaultLogMaxRetries = 0
	DefaultLogRetryDelay = 0
)

// MakePulsechainProtocols initializes all protocols for the Ethereum Mainnet.
// Its job is to initialize protocols and blockSynchronizedProtocols and assign them a protocolID.
func MakePulsechainProtocols(
	ctx context.Context,
	deps chains.Dependencies,
) (
	protocols map[engine.ProtocolID]engine.Protocol,
	blockSynchronizedProtocols map[engine.ProtocolID]engine.BlockSynchronizedProtocol,
	err error,
) {

	err = deps.Validate()
	if err != nil {
		return nil, nil, err
	}

	protocols = make(map[engine.ProtocolID]engine.Protocol)
	blockSynchronizedProtocols = make(map[engine.ProtocolID]engine.BlockSynchronizedProtocol)

	// ---------------------------------------------------------
	// 1. Static Protocols (Block Unaware)
	// ---------------------------------------------------------
	tokenSystemProtocolID := engine.ProtocolID("token-system")
	poolSystemProtocolID := engine.ProtocolID("pool-system")
	tokenPoolSystemProtocolID := engine.ProtocolID("token-pool-graph-system")

	protocols[tokenSystemProtocolID] = token.NewTokenProtocol(deps.TokenSystem)
	protocols[poolSystemProtocolID] = poolregistry.NewPoolProtocol(deps.PoolSystem)
	protocols[tokenPoolSystemProtocolID] = poolregistry.NewTokenPoolProtocol(deps.TokenPoolSystem)

	// create helper functions
	tokenAddressToID := func(addr common.Address) (uint64, error) {
		v, err := deps.TokenSystem.GetTokenByAddress(addr)
		if err != nil {
			return 0, err
		}
		return v.ID, nil
	}

	poolAddressToID := func(addr common.Address) (uint64, error) {
		return deps.PoolSystem.GetIDFromAddress(addr)
	}

	poolIDToAddress := func(id uint64) (common.Address, error) {
		key, err := deps.PoolSystem.GetKey(id)
		if err != nil {
			return common.Address{}, err
		}
		return key.ToAddress()
	}

	// onDeletePools is a helper function that wraps the Registry's DeletePools method.
	onDeletePools := func(poolIDs []uint64) []error {
		return deps.Registry.DeletePools(poolIDs)
	}

	// ---------------------------------------------------------
	// 2. Uniswap V2 (Mainnet)
	// ---------------------------------------------------------
	uniswapV2ProtocolID := engine.ProtocolID("uniswap-v2-pulsechain-mainnet")
	uniswapV2ProtocolPoolRegistrar := func(tokens []common.Address, pool common.Address) (poolID uint64, err error) {
		poolID, _, err = deps.Registry.RegisterPool(
			poolregistry.AddressToPoolKey(pool),
			tokens,
			uniswapV2ProtocolID,
		)
		return poolID, err
	}
	uniswapV2Protocol, uniswapV2ProtocolPlugins, err := uniswapv2protocol.MakeProtocol(
		ctx,
		&uniswapv2protocol.UniswapV2SystemSetupConfig{
			SystemCfg: uniswapv2protocol.UniswapV2SystemConfig{
				SystemName:      string(uniswapV2ProtocolID),
				PruneFrequency:  DefaultPruneFrequency,
				InitFrequency:   DefaultInitFrequency,
				ResyncFrequency: DefaultResyncFrequency,
				LogMaxRetries:   DefaultLogMaxRetries,
				LogRetryDelay:   DefaultLogRetryDelay,
			},
			KnownFactories: []uniswapv2poolinitializer.KnownFactory{
				{
					Address:      common.HexToAddress("0x1715a3e4a142d8b698131108995174f37aeba10d"),
					FeeBps:       30,
					ProtocolName: string(uniswapV2ProtocolID),
				},
				{
					Address:      common.HexToAddress("0x29eA7545DEf87022BAdc76323F373EA1e707C523"),
					FeeBps:       30,
					ProtocolName: string(uniswapV2ProtocolID),
				},
				{
					Address:      common.HexToAddress("0x5b9F077A77db37F3Be0A5b5d31BAeff4bc5C0bD7"),
					FeeBps:       30,
					ProtocolName: string(uniswapV2ProtocolID),
				},
				{
					Address:      common.HexToAddress("0x5C69bEe701ef814a2B6a3EDD4B1652CB9cc5aA6f"),
					FeeBps:       30,
					ProtocolName: string(uniswapV2ProtocolID),
				},
				{
					Address:      common.HexToAddress("0xC0AEe478e3658e2610c5F7A4A2E1777cE9e4f2Ac"),
					FeeBps:       30,
					ProtocolName: string(uniswapV2ProtocolID),
				},
				{
					Address:      common.HexToAddress("0x556F4C3aAa6c6b76e1BBa0409D99D4a483b29997"),
					FeeBps:       30,
					ProtocolName: string(uniswapV2ProtocolID),
				},
				{
					Address:      common.HexToAddress("0x1097053Fd2ea711dad45caCcc45EfF7548fCB362"),
					FeeBps:       30,
					ProtocolName: string(uniswapV2ProtocolID),
				},
				{
					Address:      common.HexToAddress("0x3a0Fa7884dD93f3cd234bBE2A0958Ef04b05E13b"),
					FeeBps:       30,
					ProtocolName: string(uniswapV2ProtocolID),
				},
				{
					Address:      common.HexToAddress("0xD56B9f53A1CAf0a6b66B209a54DAE5C5D40dE622"),
					FeeBps:       30,
					ProtocolName: string(uniswapV2ProtocolID),
				},
				{
					Address:      common.HexToAddress("0x3c75cb212e3A70c407C05bE35Ae4f12E2195E6E6"),
					FeeBps:       30,
					ProtocolName: string(uniswapV2ProtocolID),
				},
				{
					Address:      common.HexToAddress("0xD56B9f53A1CAf0a6b66B209a54DAE5C5D40dE622"),
					FeeBps:       30,
					ProtocolName: string(uniswapV2ProtocolID),
				},
			},
			BlockSubscriberGenerator: deps.BlockSubscriberGenerator,
			GetClient:                deps.GetClient,
			PrometheusRegistry:       deps.PrometheusRegistry,
			TokenAddressToID:         tokenAddressToID,
			PoolAddressToID:          poolAddressToID,
			PoolIDToAddress:          poolIDToAddress,
			Registrar:                uniswapV2ProtocolPoolRegistrar,
			OnDeletePools:            onDeletePools,
			Logger:                   deps.RootLogger.With("component", string(uniswapV2ProtocolID)),
			ErrorHandler:             deps.ErrorHandler,
		},
	)
	if err != nil {
		return nil, nil, err
	}

	// ---------------------------------------------------------
	// 3. Uniswap V3 (Mainnet)
	// ---------------------------------------------------------
	uniswapV3ProtocolID := engine.ProtocolID("uniswap-v3-pulsechain-mainnet")
	uniswapV3ProtocolPoolRegistrar := func(tokens []common.Address, pool common.Address) (poolID uint64, err error) {
		poolID, _, err = deps.Registry.RegisterPool(
			poolregistry.AddressToPoolKey(pool),
			tokens,
			uniswapV3ProtocolID,
		)
		return poolID, err
	}
	uniswapV3Protocol, uniswapV3ProtocolPlugins, err := uniswapv3protocol.MakeUniswapV3Protocol(
		ctx,
		&uniswapv3protocol.UniswapV3SystemSetupConfig{
			ForkID: 0, // forkID 0 represents Uniswap V3
			SystemConfig: uniswapv3protocol.UniswapV3SystemConfig{
				SystemName:      string(uniswapV3ProtocolID),
				PruneFrequency:  DefaultPruneFrequency,
				InitFrequency:   DefaultInitFrequency,
				ResyncFrequency: DefaultResyncFrequency,
				LogMaxRetries:   DefaultLogMaxRetries,
				LogRetryDelay:   DefaultLogRetryDelay,
				KnownFactories: []common.Address{
					common.HexToAddress("0x1F98431c8aD98523631AE4a59f267346ea31F984")},
			},
			TickIndexerConfig: uniswapv3protocol.UniswapV3TickIndexerConfig{
				SystemName:      "uniswap-v3-tick-indexer-pulsechain-mainnet",
				InitFrequency:   DefaultInitFrequency,
				ResyncFrequency: DefaultResyncFrequency,
				UpdateFrequency: DefaultTickIndexerUpdateFrequency,
				LogMaxRetries:   DefaultLogMaxRetries,
				LogRetryDelay:   DefaultLogRetryDelay,
			},
			BlockSubscriberGenerator: deps.BlockSubscriberGenerator,
			GetClient:                deps.GetClient,
			PrometheusRegistry:       deps.PrometheusRegistry,
			TokenAddressToID:         tokenAddressToID,
			PoolAddressToID:          poolAddressToID,
			PoolIDToAddress:          poolIDToAddress,
			Registrar:                uniswapV3ProtocolPoolRegistrar,
			OnDeletePools:            onDeletePools,
			Logger:                   deps.RootLogger.With("component", string(uniswapV3ProtocolID)),
			ErrorHandler:             deps.ErrorHandler,
		},
	)
	if err != nil {
		return nil, nil, err
	}

	// ---------------------------------------------------------
	// 4. PancakeSwap V3 (Mainnet)
	// ---------------------------------------------------------
	pancakeSwapV3ProtocolID := engine.ProtocolID("pancakeswap-v3-pulsechain-mainnet")
	pancakeSwapV3ProtocolPoolRegistrar := func(tokens []common.Address, pool common.Address) (poolID uint64, err error) {
		poolID, _, err = deps.Registry.RegisterPool(
			poolregistry.AddressToPoolKey(pool),
			tokens,
			pancakeSwapV3ProtocolID,
		)
		return poolID, err
	}
	pancakeSwapV3Protocol, pancakeSwapV3ProtocolPlugins, err := uniswapv3protocol.MakeUniswapV3Protocol(
		ctx,
		&uniswapv3protocol.UniswapV3SystemSetupConfig{
			ForkID: 1, // forkID 1 represents PancakeSwap V3
			SystemConfig: uniswapv3protocol.UniswapV3SystemConfig{
				SystemName:      string(pancakeSwapV3ProtocolID),
				PruneFrequency:  DefaultPruneFrequency,
				InitFrequency:   DefaultInitFrequency,
				ResyncFrequency: DefaultResyncFrequency,
				LogMaxRetries:   DefaultLogMaxRetries,
				LogRetryDelay:   DefaultLogRetryDelay,
				// PancakeSwap V3 Factory Address
				KnownFactories: []common.Address{
					common.HexToAddress("0xe50dbdc88e87a2c92984d794bcf3d1d76f619c68"),
					common.HexToAddress("0xcfd33c867c9f031aadff7939cb8086ee5ae88c41"),
					common.HexToAddress("0x271Fd3BDBD6e56c16c8b32b9a72D635191c9ECcf"),
					common.HexToAddress("0x968bac75BAA5FC0C5c3fe0dedf638179CBA0cE04"),
				},
			},
			TickIndexerConfig: uniswapv3protocol.UniswapV3TickIndexerConfig{
				SystemName:      "pancakeswap-v3-tick-indexer-pulsechain-mainnet",
				InitFrequency:   DefaultInitFrequency,
				ResyncFrequency: DefaultResyncFrequency,
				UpdateFrequency: DefaultTickIndexerUpdateFrequency,
				LogMaxRetries:   DefaultLogMaxRetries,
				LogRetryDelay:   DefaultLogRetryDelay,
			},
			BlockSubscriberGenerator: deps.BlockSubscriberGenerator,
			GetClient:                deps.GetClient,
			PrometheusRegistry:       deps.PrometheusRegistry,
			TokenAddressToID:         tokenAddressToID,
			PoolAddressToID:          poolAddressToID,
			PoolIDToAddress:          poolIDToAddress,
			Registrar:                pancakeSwapV3ProtocolPoolRegistrar,
			OnDeletePools:            onDeletePools,
			Logger:                   deps.RootLogger.With("component", string(pancakeSwapV3ProtocolID)),
			ErrorHandler:             deps.ErrorHandler,
		},
	)
	if err != nil {
		return nil, nil, err
	}

	// register plugins
	err = deps.RegisterProtocolPlugins(uniswapV2ProtocolID, uniswapV2ProtocolPlugins)
	if err != nil {
		return nil, nil, err
	}
	err = deps.RegisterProtocolPlugins(uniswapV3ProtocolID, uniswapV3ProtocolPlugins)
	if err != nil {
		return nil, nil, err
	}
	err = deps.RegisterProtocolPlugins(pancakeSwapV3ProtocolID, pancakeSwapV3ProtocolPlugins)
	if err != nil {
		return nil, nil, err
	}
	// register block synchronized protocols
	blockSynchronizedProtocols[uniswapV2ProtocolID] = uniswapV2Protocol
	blockSynchronizedProtocols[uniswapV3ProtocolID] = uniswapV3Protocol
	blockSynchronizedProtocols[pancakeSwapV3ProtocolID] = pancakeSwapV3Protocol

	return protocols, blockSynchronizedProtocols, nil
}
