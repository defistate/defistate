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

// MakeBaseProtocols initializes all protocols for the Ethereum Mainnet.
// Its job is to initialize protocols and blockSynchronizedProtocols and assign them a protocolID.
func MakeBaseProtocols(
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
	uniswapV2ProtocolID := engine.ProtocolID("uniswap-v2-ethereum-mainnet")
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
					Address:      common.HexToAddress("0x5C69bEe701ef814a2B6a3EDD4B1652CB9cc5aA6f"),
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
	uniswapV3ProtocolID := engine.ProtocolID("uniswap-v3-ethereum-mainnet")
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
				KnownFactories:  []common.Address{common.HexToAddress("0x1F98431c8aD98523631AE4a59f267346ea31F984")},
			},
			TickIndexerConfig: uniswapv3protocol.UniswapV3TickIndexerConfig{
				SystemName:      "uniswap-v3-tick-indexer-ethereum-mainnet",
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
	pancakeSwapV3ProtocolID := engine.ProtocolID("pancakeswap-v3-ethereum-mainnet")
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
				KnownFactories: []common.Address{common.HexToAddress("0x0BFbCF9fa4f9C56B0F40a671Ad40E0805A091865")},
			},
			TickIndexerConfig: uniswapv3protocol.UniswapV3TickIndexerConfig{
				SystemName:      "pancakeswap-v3-tick-indexer-ethereum-mainnet",
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
