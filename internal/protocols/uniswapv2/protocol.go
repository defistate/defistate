package uniswapv2

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/prometheus/client_golang/prometheus"

	ethclients "github.com/defistate/defistate/clients/eth-clients"
	"github.com/defistate/defistate/engine"
	"github.com/defistate/defistate/internal/chains"
	uniswapv2 "github.com/defistate/defistate/protocols/uniswap-v2"
	uniswapv2abi "github.com/defistate/defistate/protocols/uniswap-v2/abi"
	uniswapv2poolinitializer "github.com/defistate/defistate/protocols/uniswap-v2/initializer"
	uniswapv2poollogs "github.com/defistate/defistate/protocols/uniswap-v2/logs"
	uniswapv2poolreserves "github.com/defistate/defistate/protocols/uniswap-v2/reserves"
)

var (
	MulticallContractAddress              = common.HexToAddress("0xcA11bde05977b3631167028862bE2a173976CA11")
	DefaultGetReservesMaxConcurrentChunks = 1
	DefaultGetReservesChunkSize           = 500
	DefaultMaxPoolInactiveDuration        = 3 * 24 * time.Hour
)

// UniswapV2Factory holds configuration for a recognized Uniswap V2-style protocol.
// This struct acts as a security whitelist, ensuring we only interact with trusted protocols.
type UniswapV2Factory struct {
	// Address is the unique, canonical factory address for the protocol (e.g., Uniswap V2's factory).
	Address common.Address `yaml:"address" json:"address"`
	// ProtocolName is a human-readable identifier (e.g., "Uniswap-V2", "SushiSwap").
	ProtocolName string `yaml:"protocol_name" json:"protocol_name"`
	// FeeBps is the protocol's trading fee in basis points (e.g., 30 for 0.3%).
	FeeBps uint16 `yaml:"fee_bps" json:"fee_bps"`
}

// UniswapV2SystemConfig holds the core settings for the Uniswap V2 system.
type UniswapV2SystemConfig struct {
	SystemName      string        `yaml:"system_name" json:"system_name"`
	PruneFrequency  time.Duration `yaml:"prune_frequency" json:"prune_frequency"`
	InitFrequency   time.Duration `yaml:"init_frequency" json:"init_frequency"`
	ResyncFrequency time.Duration `yaml:"resync_frequency" json:"resync_frequency"`
	LogMaxRetries   int           `yaml:"log_max_retries" json:"log_max_retries"`
	LogRetryDelay   time.Duration `yaml:"log_retry_delay" json:"log_retry_delay"`
}

// UniswapV2SystemSetupConfig contains all dependencies for SetupUniswapV2System.
type UniswapV2SystemSetupConfig struct {
	SystemCfg                UniswapV2SystemConfig
	KnownFactories           []uniswapv2poolinitializer.KnownFactory
	BlockSubscriberGenerator func(consumer string) chan *types.Block
	GetClient                func() (ethclients.ETHClient, error)
	PrometheusRegistry       prometheus.Registerer
	TokenAddressToID         func(addr common.Address) (uint64, error)
	PoolAddressToID          func(addr common.Address) (uint64, error)
	PoolIDToAddress          func(uint64) (common.Address, error)
	Registrar                func(tokens []common.Address, pool common.Address) (poolID uint64, err error)
	OnDeletePools            func(poolIDs []uint64) []error
	Logger                   *slog.Logger
	ErrorHandler             func(error)
}

// validate checks if the cfg contains all the required dependencies.
// This is a critical step to prevent nil pointer panics at runtime.
func (cfg *UniswapV2SystemSetupConfig) validate() error {
	if cfg.BlockSubscriberGenerator == nil {
		return errors.New("BlockSubscriberGenerator cannot be nil")
	}
	if cfg.GetClient == nil {
		return errors.New("GetClient function cannot be nil")
	}
	if cfg.PrometheusRegistry == nil {
		return errors.New("PrometheusRegistry cannot be nil")
	}
	if cfg.TokenAddressToID == nil {
		return errors.New("TokenAddressToID cannot be nil")
	}
	if cfg.PoolAddressToID == nil {
		return errors.New("PoolAddressToID cannot be nil")
	}
	if cfg.PoolIDToAddress == nil {
		return errors.New("PoolIDToAddress cannot be nil")
	}
	if cfg.Registrar == nil {
		return errors.New("Registrar cannot be nil")
	}
	if cfg.OnDeletePools == nil {
		return errors.New("OnDeletePools function cannot be nil")
	}
	if cfg.Logger == nil {
		return errors.New("Logger cannot be nil")
	}
	if cfg.ErrorHandler == nil {
		return errors.New("ErrorHandler function cannot be nil")
	}
	return nil
}

// SetupUniswapV2System initializes the Uniswap V2 system with a clean, consolidated config.
func SetupUniswapV2System(
	ctx context.Context,
	cfg *UniswapV2SystemSetupConfig,

) (*uniswapv2.UniswapV2System, error) {
	err := cfg.validate()
	if err != nil {
		return nil, err
	}

	registerPool := func(token0, token1, poolAddr common.Address) (poolID uint64, err error) {
		start := time.Now()
		defer func() {
			cfg.Logger.Info(
				"RegisterPool operation completed",
				"duration", time.Since(start),
				"protocol", "UniswapV2",
				"pool_address", poolAddr.Hex(),
				"error", err,
			)
		}()
		return cfg.Registrar([]common.Address{token0, token1}, poolAddr)
	}

	registerPools := func(token0s, token1s, poolAddrs []common.Address) (poolIDs []uint64, errs []error) {
		start := time.Now()
		defer func() {
			cfg.Logger.Info(
				"RegisterPools operation completed",
				"duration", time.Since(start),
				"pool_count", len(poolAddrs),
				"protocol", "UniswapV2",
			)
		}()

		for i := range poolAddrs {
			poolID, err := cfg.Registrar([]common.Address{token0s[i], token1s[i]}, poolAddrs[i])
			poolIDs = append(poolIDs, poolID)
			errs = append(errs, err)
		}

		return poolIDs, errs
	}

	initializer, err := uniswapv2poolinitializer.NewPoolInitializer(cfg.KnownFactories, 25)
	if err != nil {
		return nil, err
	}

	config := &uniswapv2.Config{
		SystemName:      cfg.SystemCfg.SystemName,
		PrometheusReg:   cfg.PrometheusRegistry,
		NewBlockEventer: cfg.BlockSubscriberGenerator(cfg.SystemCfg.SystemName),
		GetClient:       cfg.GetClient,
		PoolInitializer: initializer.Initialize,
		DiscoverPools:   uniswapv2poollogs.DiscoverPools,
		UpdatedInBlock:  uniswapv2poollogs.UpdatedInBlock,
		GetReserves: uniswapv2poolreserves.NewGetReserves(
			DefaultGetReservesMaxConcurrentChunks,
			DefaultGetReservesChunkSize,
			MulticallContractAddress,
		),
		TokenAddressToID: cfg.TokenAddressToID,
		PoolAddressToID:  cfg.PoolAddressToID,
		PoolIDToAddress:  cfg.PoolIDToAddress,
		RegisterPool:     registerPool,
		RegisterPools:    registerPools,
		ErrorHandler:     cfg.ErrorHandler,
		TestBloom:        uniswapv2poollogs.SwapEventInBloom, // if there is a swap event, there will be a corresponding sync event
		FilterTopics: [][]common.Hash{
			{
				// we need both swap and sync events
				uniswapv2abi.UniswapV2ABI.Events["Swap"].ID,
				uniswapv2abi.UniswapV2ABI.Events["Sync"].ID,
			},
		},
		OnDeletePools:       cfg.OnDeletePools,
		InitFrequency:       cfg.SystemCfg.InitFrequency,
		MaxInactiveDuration: DefaultMaxPoolInactiveDuration,
		Logger:              cfg.Logger,
	}

	return uniswapv2.NewUniswapV2System(ctx, config)
}

func MakeProtocol(
	ctx context.Context,
	cfg *UniswapV2SystemSetupConfig,
) (engine.BlockSynchronizedProtocol, *chains.ProtocolPlugins, error) {
	uniswapV2System, err := SetupUniswapV2System(
		ctx,
		cfg,
	)

	if err != nil {
		return nil, nil, err
	}

	return uniswapv2.NewUniswapV2Protocol(uniswapV2System), &chains.ProtocolPlugins{}, nil
}
