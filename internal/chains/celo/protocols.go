package celo

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
	DefaultLogMaxRetries = 10
	DefaultLogRetryDelay = 50 * time.Millisecond
)

// MakeCeloProtocols initializes all protocols for the Celo Mainnet.
// Its job is to initialize protocols and blockSynchronizedProtocols and assign them a protocolID.
func MakeCeloProtocols(
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
	uniswapV2ProtocolID := engine.ProtocolID("uniswap-v2-celo-mainnet")
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
				{
					Address:      common.HexToAddress("0xc35DADB65012eC5796536bD9864eD8773aBc74C4"),
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
	uniswapV3ProtocolID := engine.ProtocolID("uniswap-v3-celo-mainnet")
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
					common.HexToAddress("0xAfE208a311B21f13EF87E33A90049fC17A7acDEc"),
					common.HexToAddress("0x67FEa58D5a5a4162cED847E13c2c81c73bf8aeC4"),
				},
			},
			TickIndexerConfig: uniswapv3protocol.UniswapV3TickIndexerConfig{
				SystemName:      "uniswap-v3-tick-indexer-celo-mainnet",
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

	aerodromeV3ProtocolID := engine.ProtocolID("aerodrome-v3-celo-mainnet")
	aerodromeV3ProtocolPoolRegistrar := func(tokens []common.Address, pool common.Address) (poolID uint64, err error) {
		poolID, _, err = deps.Registry.RegisterPool(
			poolregistry.AddressToPoolKey(pool),
			tokens,
			aerodromeV3ProtocolID,
		)
		return poolID, err
	}
	aerodromeV3Protocol, aerodromeV3ProtocolPlugins, err := uniswapv3protocol.MakeUniswapV3Protocol(
		ctx,
		&uniswapv3protocol.UniswapV3SystemSetupConfig{
			ForkID: 2, // forkID 2 represents Aerodrome V3
			SystemConfig: uniswapv3protocol.UniswapV3SystemConfig{
				SystemName:      string(aerodromeV3ProtocolID),
				PruneFrequency:  DefaultPruneFrequency,
				InitFrequency:   DefaultInitFrequency,
				ResyncFrequency: DefaultResyncFrequency,
				LogMaxRetries:   DefaultLogMaxRetries,
				LogRetryDelay:   DefaultLogRetryDelay,
				KnownFactories: []common.Address{
					common.HexToAddress("0x04625B046C69577EfC40e6c0Bb83CDBAfab5a55F"),
				},
			},
			TickIndexerConfig: uniswapv3protocol.UniswapV3TickIndexerConfig{
				SystemName:      "aerodrome-v3-tick-indexer-celo-mainnet",
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
			Registrar:                aerodromeV3ProtocolPoolRegistrar,
			OnDeletePools:            onDeletePools,
			Logger:                   deps.RootLogger.With("component", string(aerodromeV3ProtocolID)),
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
	err = deps.RegisterProtocolPlugins(aerodromeV3ProtocolID, aerodromeV3ProtocolPlugins)
	if err != nil {
		return nil, nil, err
	}
	// register block synchronized protocols
	blockSynchronizedProtocols[uniswapV2ProtocolID] = uniswapV2Protocol
	blockSynchronizedProtocols[uniswapV3ProtocolID] = uniswapV3Protocol
	blockSynchronizedProtocols[aerodromeV3ProtocolID] = aerodromeV3Protocol

	return protocols, blockSynchronizedProtocols, nil
}
