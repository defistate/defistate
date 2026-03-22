package katana

import (
	"context"
	"time"

	"github.com/defistate/defistate/engine"
	"github.com/defistate/defistate/internal/chains"
	uniswapv3protocol "github.com/defistate/defistate/internal/protocols/uniswapv3"

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

// MakeKatanaProtocols initializes all protocols for the Katana Mainnet.
// Its job is to initialize protocols and blockSynchronizedProtocols and assign them a protocolID.
func MakeKatanaProtocols(
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
	// Sushiswap V3 (Mainnet)
	// ---------------------------------------------------------
	sushiswapV3ProtocolID := engine.ProtocolID("sushiswap-v3-katana-mainnet")
	sushiswapV3ProtocolPoolRegistrar := func(tokens []common.Address, pool common.Address) (poolID uint64, err error) {
		poolID, _, err = deps.Registry.RegisterPool(
			poolregistry.AddressToPoolKey(pool),
			tokens,
			sushiswapV3ProtocolID,
		)
		return poolID, err
	}
	sushiswapV3Protocol, sushiswapV3ProtocolPlugins, err := uniswapv3protocol.MakeUniswapV3Protocol(
		ctx,
		&uniswapv3protocol.UniswapV3SystemSetupConfig{
			ForkID: 0, // forkID 0 represents Uniswap V3
			SystemConfig: uniswapv3protocol.UniswapV3SystemConfig{
				SystemName:      string(sushiswapV3ProtocolID),
				PruneFrequency:  DefaultPruneFrequency,
				InitFrequency:   DefaultInitFrequency,
				ResyncFrequency: DefaultResyncFrequency,
				LogMaxRetries:   DefaultLogMaxRetries,
				LogRetryDelay:   DefaultLogRetryDelay,
				KnownFactories:  []common.Address{common.HexToAddress("0x203e8740894c8955cB8950759876d7E7E45E04c1")},
			},
			TickIndexerConfig: uniswapv3protocol.UniswapV3TickIndexerConfig{
				SystemName:      "sushiswap-v3-tick-indexer-katana-mainnet",
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
			Registrar:                sushiswapV3ProtocolPoolRegistrar,
			OnDeletePools:            onDeletePools,
			Logger:                   deps.RootLogger.With("component", string(sushiswapV3ProtocolID)),
			ErrorHandler:             deps.ErrorHandler,
		},
	)
	if err != nil {
		return nil, nil, err
	}

	err = deps.RegisterProtocolPlugins(sushiswapV3ProtocolID, sushiswapV3ProtocolPlugins)
	if err != nil {
		return nil, nil, err
	}

	blockSynchronizedProtocols[sushiswapV3ProtocolID] = sushiswapV3Protocol

	return protocols, blockSynchronizedProtocols, nil
}
