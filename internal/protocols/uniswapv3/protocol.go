package uniswapv3

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/defistate/defistate/engine"
	"github.com/defistate/defistate/internal/chains"

	ethclients "github.com/defistate/defistate/clients/eth-clients"
	uniswapv3 "github.com/defistate/defistate/protocols/uniswap-v3"

	uniswapv3poolinitializer "github.com/defistate/defistate/protocols/uniswap-v3/initializer"
	uniswapv3poolinfo "github.com/defistate/defistate/protocols/uniswap-v3/poolinfo"
	uniswapv3ticks "github.com/defistate/defistate/protocols/uniswap-v3/ticks"

	forkhelpers "github.com/defistate/defistate/protocols/uniswap-v3/forks"
)

var (
	MulticallContractAddress       = common.HexToAddress("0xcA11bde05977b3631167028862bE2a173976CA11")
	DefaultMaxPoolInactiveDuration = 3 * 24 * time.Hour

	// Configuration for UniswapV3 providers
	DefaultPoolInitializerMaxConcurrentCalls  = 5
	DefaultPoolInfoProviderMaxConcurrentCalls = 25
	DefaultPoolInfoProviderChunkSize          = 100
	DefaultTickBitmapProviderBatchSize        = 25
	DefaultTickBitmapProviderMaxConcurrency   = 5
	DefaultTickInfoProviderBatchSize          = 25
	DefaultTickInfoProviderMaxConcurrency     = 5
	DefaultTickDataProviderMaxConcurrentCalls = 5
)

// UniswapV3SystemConfig holds the core settings for the Uniswap V3 system.
type UniswapV3SystemConfig struct {
	SystemName      string           `yaml:"system_name" json:"system_name"`
	PruneFrequency  time.Duration    `yaml:"prune_frequency" json:"prune_frequency"`
	InitFrequency   time.Duration    `yaml:"init_frequency" json:"init_frequency"`
	ResyncFrequency time.Duration    `yaml:"resync_frequency" json:"resync_frequency"`
	LogMaxRetries   int              `yaml:"log_max_retries" json:"log_max_retries"`
	LogRetryDelay   time.Duration    `yaml:"log_retry_delay" json:"log_retry_delay"`
	KnownFactories  []common.Address `yaml:"known_factories" json:"known_factories"`
}

// UniswapV3TickIndexerConfig holds the settings for the Uniswap V3 tick indexer.
type UniswapV3TickIndexerConfig struct {
	SystemName      string        `yaml:"system_name" json:"system_name"`
	InitFrequency   time.Duration `yaml:"init_frequency" json:"init_frequency"`
	ResyncFrequency time.Duration `yaml:"resync_frequency" json:"resync_frequency"`
	UpdateFrequency time.Duration `yaml:"update_frequency" json:"update_frequency"`
	LogMaxRetries   int           `yaml:"log_max_retries" json:"log_max_retries"`
	LogRetryDelay   time.Duration `yaml:"log_retry_delay" json:"log_retry_delay"`
}

type UniswapV3System struct {
	UniswapV3System      *uniswapv3.UniswapV3System
	UniswapV3TickIndexer uniswapv3.TickIndexer
}

// UniswapV3SystemSetupConfig contains all dependencies for SetupUniswapV2System.
type UniswapV3SystemSetupConfig struct {
	// ForkID is used to select the helper functions used to initialize the UniswapV3 Subsystems
	ForkID                   uint8
	SystemConfig             UniswapV3SystemConfig
	TickIndexerConfig        UniswapV3TickIndexerConfig
	BlockSubscriberGenerator func(consumer string) chan *types.Block
	GetClient                func() (ethclients.ETHClient, error)
	PrometheusRegistry       prometheus.Registerer
	TokenAddressToID         func(addr common.Address) (uint64, error)
	PoolAddressToID          func(addr common.Address) (uint64, error)
	PoolIDToAddress          func(uint64) (common.Address, error)
	Registrar                func(tokens []common.Address, pool common.Address) (poolID uint64, err error)
	OnDeletePools            func(poolID []uint64) []error
	Logger                   *slog.Logger
	ErrorHandler             func(error)
}

// validate checks if the cfg contains all the required dependencies.
// This is a critical step to prevent nil pointer panics at runtime.
func (cfg *UniswapV3SystemSetupConfig) validate() error {
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

func SetupUniswapV3System(ctx context.Context, cfg *UniswapV3SystemSetupConfig) (*UniswapV3System, error) {
	err := cfg.validate()
	if err != nil {
		return nil, err
	}

	inBlockedList := func(common.Address) bool {
		return false
	}

	registerPool := func(token0, token1, poolAddr common.Address) (poolID uint64, err error) {
		start := time.Now()
		defer func() {
			cfg.Logger.Info(
				"register pool operation completed",
				"duration", time.Since(start),
				"protocol", "uniswap v3",
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
				"register pools operation completed",
				"duration", time.Since(start),
				"pool_count", len(poolAddrs),
				"protocol", "uniswap v3",
			)
		}()

		for i := range poolAddrs {
			poolID, err := cfg.Registrar([]common.Address{token0s[i], token1s[i]}, poolAddrs[i])
			poolIDs = append(poolIDs, poolID)
			errs = append(errs, err)
		}

		return poolIDs, errs
	}

	// init required vars and funcs based on forkID
	return initializeByForkID(
		ctx,
		inBlockedList,
		registerPool,
		registerPools,
		cfg,
	)

}

func initializeByForkID(
	ctx context.Context,
	inBlockedList func(common.Address) bool,
	registerPool func(token0, token1, poolAddr common.Address) (poolID uint64, err error),
	registerPools func(token0s, token1s, poolAddrs []common.Address) (poolIDS []uint64, error []error),
	cfg *UniswapV3SystemSetupConfig,
) (*UniswapV3System, error) {

	helpers, err := forkhelpers.GetForkData(cfg.ForkID)
	if err != nil {
		return nil, err
	}

	initializerFunc, err := uniswapv3poolinitializer.NewPoolInitializerFunc(
		helpers.System.PoolInitializer(MulticallContractAddress),
		DefaultPoolInitializerMaxConcurrentCalls,
		cfg.SystemConfig.KnownFactories,
	)

	if err != nil {
		return nil, err
	}

	infoProvider, err := uniswapv3poolinfo.NewPoolInfoFunc(
		helpers.System.PoolInfoProvider(MulticallContractAddress),
		DefaultPoolInfoProviderMaxConcurrentCalls,
		DefaultPoolInfoProviderChunkSize,
	)
	if err != nil {
		return nil, err
	}
	tickDataProvider := uniswapv3ticks.NewTickDataProvider(
		cfg.GetClient,
		helpers.TickIndexer.TickBitmapProvider(MulticallContractAddress, DefaultTickBitmapProviderBatchSize, DefaultTickBitmapProviderMaxConcurrency),
		helpers.TickIndexer.TickInfoProvider(MulticallContractAddress, DefaultTickInfoProviderBatchSize, DefaultTickInfoProviderMaxConcurrency),
		DefaultTickDataProviderMaxConcurrentCalls,
	)

	uniswapV3TickIndexer, err := uniswapv3ticks.NewTickIndexer(
		ctx,
		&uniswapv3ticks.Config{
			SystemName:          cfg.TickIndexerConfig.SystemName,
			Registry:            cfg.PrometheusRegistry,
			NewBlockEventer:     cfg.BlockSubscriberGenerator(cfg.TickIndexerConfig.SystemName),
			GetClient:           cfg.GetClient,
			GetTickBitmap:       tickDataProvider.GetTickBitMap,
			GetInitializedTicks: tickDataProvider.GetInitializedTicks,
			GetTicks:            tickDataProvider.GetTicks,
			UpdatedInBlock:      helpers.TickIndexer.UpdatedInBlock,
			ErrorHandler:        cfg.ErrorHandler, // Note: This uses the already-wrapped handler
			TestBloomFunc:       helpers.TickIndexer.TestBloom,
			FilterTopics:        helpers.System.FilterTopics,
			InitFrequency:       cfg.TickIndexerConfig.InitFrequency,
			ResyncFrequency:     cfg.TickIndexerConfig.ResyncFrequency,
			UpdateFrequency:     cfg.TickIndexerConfig.UpdateFrequency,
			LogMaxRetries:       cfg.TickIndexerConfig.LogMaxRetries,
			LogRetryDelay:       cfg.TickIndexerConfig.LogRetryDelay,
			Logger:              cfg.Logger.With("component", "tick-indexer")},
	)

	if err != nil {
		return nil, err
	}

	uniswapV3System, err := uniswapv3.NewUniswapV3System(
		ctx,
		&uniswapv3.Config{
			SystemName:           cfg.SystemConfig.SystemName,
			PrometheusReg:        cfg.PrometheusRegistry,
			NewBlockEventer:      cfg.BlockSubscriberGenerator(cfg.SystemConfig.SystemName),
			GetClient:            cfg.GetClient,
			PoolInitializer:      initializerFunc,
			DiscoverPools:        helpers.System.DiscoverPools,
			SwapsInBlock:         helpers.System.ExtractSwaps,
			MintsAndBurnsInBlock: helpers.System.ExtractMintsAndBurns,
			GetPoolInfo:          infoProvider,
			TokenAddressToID:     cfg.TokenAddressToID,
			PoolAddressToID:      cfg.PoolAddressToID,
			PoolIDToAddress:      cfg.PoolIDToAddress,
			RegisterPool:         registerPool,
			RegisterPools:        registerPools,
			OnDeletePools:        cfg.OnDeletePools,
			ErrorHandler:         cfg.ErrorHandler,
			TestBloom:            helpers.System.TestBloom,
			FilterTopics:         helpers.System.FilterTopics,
			TickIndexer:          uniswapV3TickIndexer,
			InitFrequency:        cfg.SystemConfig.InitFrequency,
			MaxInactiveDuration:  DefaultMaxPoolInactiveDuration,
			Logger:               cfg.Logger,
		},
	)

	if err != nil {
		return nil, err
	}

	return &UniswapV3System{
		UniswapV3System:      uniswapV3System,
		UniswapV3TickIndexer: uniswapV3TickIndexer,
	}, nil
}

func MakeUniswapV3Protocol(
	ctx context.Context,
	cfg *UniswapV3SystemSetupConfig,
) (engine.BlockSynchronizedProtocol, *chains.ProtocolPlugins, error) {
	uniswapV3System, err := SetupUniswapV3System(
		ctx,
		cfg,
	)

	if err != nil {
		return nil, nil, err
	}

	return uniswapv3.NewUniswapV3Protocol(uniswapV3System.UniswapV3System), &chains.ProtocolPlugins{}, nil
}
