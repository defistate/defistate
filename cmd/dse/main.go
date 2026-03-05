package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"log/slog"

	"math/big"
	"net/http"
	"net/http/pprof"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/defistate/defistate/cmd/dse/config"
	"github.com/defistate/defistate/differ"
	"github.com/defistate/defistate/engine"
	"github.com/defistate/defistate/internal/chains"

	arbitrumprotocols "github.com/defistate/defistate/internal/chains/arbitrum"
	baseprotocols "github.com/defistate/defistate/internal/chains/base"
	bscprotocols "github.com/defistate/defistate/internal/chains/bsc"
	celoprotocols "github.com/defistate/defistate/internal/chains/celo"
	ethprotocols "github.com/defistate/defistate/internal/chains/ethereum"
	katanaprotocols "github.com/defistate/defistate/internal/chains/katana"
	pulsechainprotocols "github.com/defistate/defistate/internal/chains/pulsechain"

	arbitrumstateops "github.com/defistate/defistate/streams/jsonrpc/stateops/chains/arbitrum"
	basestateops "github.com/defistate/defistate/streams/jsonrpc/stateops/chains/base"
	bscstateops "github.com/defistate/defistate/streams/jsonrpc/stateops/chains/bsc"
	celostateops "github.com/defistate/defistate/streams/jsonrpc/stateops/chains/celo"
	ethstateops "github.com/defistate/defistate/streams/jsonrpc/stateops/chains/ethereum"
	katanastateops "github.com/defistate/defistate/streams/jsonrpc/stateops/chains/katana"
	pulsechainstateops "github.com/defistate/defistate/streams/jsonrpc/stateops/chains/pulsechain"

	clientmanager "github.com/defistate/defistate/clients/eth-clients/client-manager"
	fork "github.com/defistate/defistate/fork/anvil-forker"
	"github.com/defistate/defistate/internal/helpers"
	token "github.com/defistate/defistate/protocols/erc20-token-system"
	poolregistry "github.com/defistate/defistate/protocols/pool-registry"
	"github.com/defistate/defistate/streams/jsonrpc/server"
	"github.com/defistate/defistate/token-analyzer/bloom"
	"github.com/defistate/defistate/token-analyzer/erc20analyzer"
	"github.com/defistate/defistate/token-analyzer/initializers"

	feegasrequester "github.com/defistate/defistate/token-analyzer/fork"
	erc20logvolumeanalyzer "github.com/defistate/defistate/token-analyzer/logs/erc20analyzer"
	logextractor "github.com/defistate/defistate/token-analyzer/logs/extractor"
)

var (
	// DefaultNewBlockEventerBuffer is the buffer size for the main block event channel.
	DefaultNewBlockEventerBuffer uint = 100

	// App metrics and pprofing defaults
	DefaultMetricsPort int = 2112
	DefaultPprofPort   int = 6060

	// Fork defaults
	DefaultForkPoolServiceRPCPort               int    = 10006
	DefaultForkPoolServiceAnvilPort             int    = 10009
	DefaultForkPoolServiceMaxConcurrentRequests int    = 1
	DefaultForkBlocksBehind                     uint64 = 10

	// Engine Defaults
	DefaultEnginePollSyncInterval time.Duration = 10 * time.Millisecond
	DefaultEngineBlockQueueSize   int           = 100

	// ERC20 Analyzer defaults
	DefaultERC20VolumeAnalyzerExpiryCheckFrequency time.Duration = 1 * time.Minute
	DefaultERC20VolumeAnalyzerRecordStaleDuration  time.Duration = 1 * time.Hour
	DefaultERC20AnalyzerFeeAndGasUpdateFrequency   time.Duration = 5 * time.Minute  // fee and gas analysis will be performed every 5 minutes
	DefaultERC20AnalyzerMinTokenUpdateInterval     time.Duration = 30 * time.Minute // tokens will be updated at least every 30 minutes

	// JSON RPC Stream defaults
	DefaultJSONRPCStreamPort              int      = 8080
	DefaultJSONRPCStreamAllowedOrigins    []string = []string{"*"}
	DefaultJSONRPCStreamFullStateInterval uint64   = 100000000
)

// AppErrorHandler provides structured, context-aware error handling for the application's subsystems.
type AppErrorHandler struct {
	logger  *slog.Logger
	chainID uint64
}

// Handle logs an error with the associated chain ID for context.
func (h *AppErrorHandler) Handle(err error) {
	h.logger.Error(
		"a component reported an error",
		"error", err,
		"chain_id", h.chainID,
	)
}

type ChainProtocolsProvider func(context.Context, chains.Dependencies) (
	protocols map[engine.ProtocolID]engine.Protocol,
	blockSynchronizedProtocols map[engine.ProtocolID]engine.BlockSynchronizedProtocol,
	err error,
)

type ChainStateOps interface {
	Diff(old *engine.State, new *engine.State) (*differ.StateDiff, error)
	Patch(oldState *engine.State, diff *differ.StateDiff) (*engine.State, error)
	DecodeStateJSON(schema engine.ProtocolSchema, data json.RawMessage) (any, error)
	DecodeStateDiffJSON(schema engine.ProtocolSchema, data json.RawMessage) (any, error)
}

func main() {

	// create the logger handler
	rootLogHandler := slog.NewJSONHandler(os.Stdout, nil)
	close := func() {
		os.Exit(1)
	}
	rootLogger := slog.New(rootLogHandler)
	prometheusRegistry := prometheus.DefaultRegisterer

	cfg, err := loadConfig()
	if err != nil {
		rootLogger.Error("Failed to load configuration", "error", err)
		close()
	}

	// Create a context that cancels when the OS sends an interrupt (Ctrl+C) or termination signal.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	appErrHandler := &AppErrorHandler{
		logger:  rootLogger,
		chainID: cfg.ChainID.Uint64(),
	}

	// initialize the client manager
	clientManagerLogger := rootLogger.With("component", "eth-client-manager")
	clientLogger := rootLogger.With("component", "eth-client")
	clientManager, err := clientmanager.NewClientManager(
		ctx,
		cfg.ClientManager.RPCURLs,
		&clientmanager.ClientManagerConfig{
			ClientConfig: &clientmanager.ClientConfig{
				MaxConcurrentETHCalls: cfg.ClientManager.MaxConcurrentRequests,
				Logger:                clientLogger,
			},
			Logger: clientManagerLogger,
		})

	if err != nil {
		rootLogger.Error("Failed to initialize client manager", "error", err)
		close()
	}

	blockSubscriberLogger := rootLogger.With("component", "block-subscriber")
	newBlockSubscription, startBlockFanOut := helpers.MakeBlockSubscriberGenerator(
		ctx,
		clientManager,
		blockSubscriberLogger,
		cfg.ChainID.Uint64(),
	)

	// initialize the base systems
	tokenSystem := token.NewTokenSystem()
	poolSystem := poolregistry.NewPoolSystem()
	tokenPoolSystem := poolregistry.NewTokenPoolSystem(1000)
	poolRegistryManager := poolregistry.NewRegistryManager(
		tokenSystem,
		poolSystem,
		tokenPoolSystem,
	)

	forkPoolRPCURL, err := setupForkService(
		ctx,
		cfg,
		newBlockSubscription,
		rootLogger,
	)
	if err != nil {
		rootLogger.Error("Failed to setup fork pool service", "error", err)
		close()
	}

	addPoolToAllowedList, err := setupERC20Analyzer(
		ctx,
		rootLogger,
		newBlockSubscription,
		clientManager,
		tokenSystem,
		forkPoolRPCURL,
		DefaultForkPoolServiceMaxConcurrentRequests,
		appErrHandler,
	)
	if err != nil {
		rootLogger.Error("Failed to setup ERC20 token analyzer", "error", err)
		close()
	}

	// set the registry beforeRegistrerPool hook
	// this allows our erc20 analyzer store allowed pools
	poolRegistryManager.SetBeforeRegisterPoolFunc(func(pk poolregistry.PoolKey) {
		addr, err := pk.ToAddress()
		if err != nil {
			//pool key not an address, simply return
			return
		}

		// else add address to allow list!
		addPoolToAllowedList(addr)
	})

	var (
		chainProtocolsProvider ChainProtocolsProvider
		chainStateOps          ChainStateOps
	)
	switch cfg.ChainID.Uint64() {
	case chains.Mainnet:
		// Ethereum Mainnet
		chainProtocolsProvider = ethprotocols.MakeEthereumProtocols
		chainStateOps, err = ethstateops.NewStateOps(rootLogger, prometheusRegistry)
		if err != nil {
			rootLogger.Error("Failed to initialize Chain State Ops", "chain_id", cfg.ChainID, "error", err)
			close()
		}
	case chains.Katana:
		// Katana Mainnet
		chainProtocolsProvider = katanaprotocols.MakeKatanaProtocols
		chainStateOps, err = katanastateops.NewStateOps(rootLogger, prometheusRegistry)
		if err != nil {
			rootLogger.Error("Failed to initialize Chain State Ops", "chain_id", cfg.ChainID, "error", err)
			close()
		}

	case chains.Pulsechain:
		// Pulsechain Mainnet
		chainProtocolsProvider = pulsechainprotocols.MakePulsechainProtocols
		chainStateOps, err = pulsechainstateops.NewStateOps(rootLogger, prometheusRegistry)
		if err != nil {
			rootLogger.Error("Failed to initialize Chain State Ops", "chain_id", cfg.ChainID, "error", err)
			close()
		}

	case chains.Celo:
		// Celo Mainnet
		chainProtocolsProvider = celoprotocols.MakeCeloProtocols
		chainStateOps, err = celostateops.NewStateOps(rootLogger, prometheusRegistry)
		if err != nil {
			rootLogger.Error("Failed to initialize Chain State Ops", "chain_id", cfg.ChainID, "error", err)
			close()
		}

	case chains.Arbitrum:
		// Arbitrum One
		chainProtocolsProvider = arbitrumprotocols.MakeArbitrumProtocols
		chainStateOps, err = arbitrumstateops.NewStateOps(rootLogger, prometheusRegistry)
		if err != nil {
			rootLogger.Error("Failed to initialize Chain State Ops", "chain_id", cfg.ChainID, "error", err)
			close()
		}
	case chains.Base:
		// Base
		chainProtocolsProvider = baseprotocols.MakeBaseProtocols
		chainStateOps, err = basestateops.NewStateOps(rootLogger, prometheusRegistry)
		if err != nil {
			rootLogger.Error("Failed to initialize Chain State Ops", "chain_id", cfg.ChainID, "error", err)
			close()
		}
	case chains.BSC:
		// BSC
		chainProtocolsProvider = bscprotocols.MakeBSCProtocols
		chainStateOps, err = bscstateops.NewStateOps(rootLogger, prometheusRegistry)
		if err != nil {
			rootLogger.Error("Failed to initialize Chain State Ops", "chain_id", cfg.ChainID, "error", err)
			close()
		}

	default:
		// we don't know this chain, logger error and close.
		rootLogger.Error(fmt.Errorf("Chain Protocols Provider not found for chain with ID %d", cfg.ChainID.Uint64()).Error())
		close()
	}

	deps := chains.Dependencies{
		GetClient:                clientManager.GetClient,
		BlockSubscriberGenerator: newBlockSubscription,
		TokenSystem:              tokenSystem,
		PoolSystem:               poolSystem,
		TokenPoolSystem:          tokenPoolSystem,
		Registry:                 poolRegistryManager,
		ErrorHandler:             appErrHandler.Handle,
		RootLogger:               rootLogger,
		PrometheusRegistry:       prometheusRegistry,
	}

	protocols, blockSynchronizedProtocols, err := chainProtocolsProvider(
		ctx,
		deps,
	)
	if err != nil {
		rootLogger.Error("Failed to setup chain protocols", "error", err)
		close()
	}

	engineLogger := rootLogger.With("component", "defistate")
	engine, err := setupEngine(
		ctx,
		cfg.ChainID,
		cfg.Engine,
		newBlockSubscription("defistate"),
		protocols,
		blockSynchronizedProtocols,
		prometheusRegistry,
		engineLogger,
		appErrHandler.Handle,
	)

	if err != nil {
		rootLogger.Error("Failed to setup defi state engine", "error", err)
		close()
	}

	jsonRPCStateStreamErrCh, err := setupJSONRPCStateStreamer(
		ctx,
		engine,
		chainStateOps,
		rootLogger,
	)

	if err != nil {
		rootLogger.Error("Failed to setup server service", "error", err)
		close()
	}

	metricsServerErrCh := startMetricsServer(
		ctx,
		DefaultMetricsPort,
		DefaultPprofPort,
		rootLogger,
	)

	// we start block fan out after all setups have passed
	err = startBlockFanOut()
	if err != nil {
		rootLogger.Error("Failed to start block fan out", "error", err)
		close()
	}

	rootLogger.Info("Application started successfully")

	select {
	case <-ctx.Done(): // Wait for context cancellation (e.g., from an OS signal) to gracefully shut down.
		rootLogger.Info("Application shutting down")
	case err := <-jsonRPCStateStreamErrCh:
		rootLogger.Error("JSON RPC Stream Errored", "error", err)
	case err := <-metricsServerErrCh:
		rootLogger.Error("Metrics Server Errored", "error", err)
	}
}

// setupERC20Analyzer encapsulates the logic for creating the shared allowlist
// and wiring it between the pool subsystems and the token analyzer.
func setupERC20Analyzer(
	ctx context.Context,
	rootLogger *slog.Logger,
	blockSubscriberGenerator func(string) chan *types.Block,
	clientManager *clientmanager.ClientManager,
	tokenSystem *token.TokenSystem,
	forkPoolServiceURL string,
	forkPoolServiceMaxConcurrentRequests int,
	appErrHandler *AppErrorHandler,
) (addPoolToAllowedList func(common.Address), err error) {

	// Use a sync.Map for a concurrency-safe allowlist.
	var erc20AnalyzerAllowlist sync.Map

	// addPoolToAllowedList defines a callback function that acts as a hook into the
	// pool discovery and registration process.
	//
	// It is passed into the pool subsystems and is invoked just before a newly
	// discovered liquidity pool address is formally registered. Its primary purpose is to
	// act as a communication bridge between otherwise decoupled components.
	//
	// When a pool is found, this function adds its address to a shared,
	// concurrency-safe allowlist. This allowlist is then used by the ERC20 token
	// analyzer's filter function (`isAllowedAddressForAnalyzer`) to identify which
	// addresses are valid liquidity pools, enabling real-time analysis of their
	// transfer activity.
	//
	// We use pools as allowed addresses because they are the only Token holders we can trust
	// Because we trust the engine Subsystems
	addPoolToAllowedList = func(poolAddress common.Address) {
		erc20AnalyzerAllowlist.Store(poolAddress, true)
	}

	// The filter function that the ERC20 analyzer will use.
	isAllowedAddressForAnalyzer := func(addr common.Address) bool {
		// Load is a concurrency-safe read. The 'ok' boolean indicates presence.
		_, ok := erc20AnalyzerAllowlist.Load(addr)
		return ok
	}

	erc20TokenAnalyzerLogger := rootLogger.With("component", "erc20-token-analyzer")

	// getFiltererForLogsExtractor is a helper function that returns a closure for filtering logs.
	// This abstracts the process of obtaining a logger filterer from the client manager.
	getFiltererForLogsExtractor := func(
		clientManager *clientmanager.ClientManager,
	) func() (func(context.Context, ethereum.FilterQuery) ([]types.Log, error), error) {
		return func() (func(ctx context.Context, q ethereum.FilterQuery) ([]types.Log, error), error) {
			client, err := clientManager.GetClient()
			if err != nil {
				return nil, err
			}
			return client.FilterLogs, nil
		}
	}
	blockLogsExtractor, err := logextractor.NewLiveExtractor(
		bloom.ERC20TransferLikelyInBloom,
		getFiltererForLogsExtractor(clientManager),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create logger extractor: %w", err)
	}

	tokenHolderAnalyzer, err := erc20logvolumeanalyzer.NewVolumeAnalyzer(
		ctx,
		erc20logvolumeanalyzer.Config{
			ExpiryCheckFrequency: DefaultERC20VolumeAnalyzerExpiryCheckFrequency,
			RecordStaleDuration:  DefaultERC20VolumeAnalyzerRecordStaleDuration,
			IsAllowedAddress:     isAllowedAddressForAnalyzer,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create tokenHolderAnalyzer: %w", err)
	}

	feeAndGasRequester, err := feegasrequester.NewERC20FeeAndGasRequester(
		forkPoolServiceURL,
		forkPoolServiceMaxConcurrentRequests,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create fee and gas requester: %w", err)
	}

	tokenInitializer, err := initializers.NewERC20Initializer(clientManager.GetClient)
	if err != nil {
		return nil, fmt.Errorf("failed to create token initializer: %w", err)
	}

	errorHandler := func(err error) {
		appErrHandler.Handle(fmt.Errorf("ERC20 Analyzer System: %w", err))
	}
	analyzerCfg := erc20analyzer.Config{
		NewBlockEventer:          blockSubscriberGenerator("erc20-token-analyzer"),
		TokenStore:               tokenSystem,
		TokenInitializer:         tokenInitializer,
		BlockExtractor:           blockLogsExtractor,
		TokenHolderAnalyzer:      tokenHolderAnalyzer,
		FeeAndGasRequester:       feeAndGasRequester,
		FeeAndGasUpdateFrequency: DefaultERC20AnalyzerFeeAndGasUpdateFrequency,
		MinTokenUpdateInterval:   DefaultERC20AnalyzerMinTokenUpdateInterval,
		ErrorHandler:             errorHandler,
		Logger:                   erc20TokenAnalyzerLogger,
	}

	_, err = erc20analyzer.NewAnalyzer(ctx, analyzerCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create ERC20 token analyzer: %w", err)
	}

	return addPoolToAllowedList, nil
}

func setupEngine(
	ctx context.Context,
	chainID *big.Int,
	cfg config.EngineConfig,
	newBlockEventer chan *types.Block,
	protocols map[engine.ProtocolID]engine.Protocol,
	blockSynchronizedProtocols map[engine.ProtocolID]engine.BlockSynchronizedProtocol,
	registry prometheus.Registerer,
	logger engine.Logger,
	errHandler func(error),
) (*engine.Engine, error) {
	return engine.NewEngine(
		ctx,
		&engine.Config{
			ChainID:                    chainID,
			NewBlockEventer:            newBlockEventer,
			Protocols:                  protocols,
			BlockSynchronizedProtocols: blockSynchronizedProtocols,
			PollSyncInterval:           DefaultEnginePollSyncInterval,
			MaxWaitUntilSync:           cfg.MaxWaitUntilSync,
			BlockQueueSize:             int(DefaultEngineBlockQueueSize),
			ErrorHandler:               errHandler,
			Registry:                   registry,
			Logger:                     logger,
		},
	)
}

// startMetricsServer starts dedicated HTTP servers for Prometheus metrics and Pprof.
// It runs in the background and supports graceful shutdown via the provided context.
// It returns a channel that will receive an error if the server fails to start or exits unexpectedly.
func startMetricsServer(
	ctx context.Context,
	metricsPort,
	pprofPort int,
	logger engine.Logger,
) <-chan error {
	// ---- Metrics server ----
	metricsMux := http.NewServeMux()
	metricsMux.Handle("/metrics", promhttp.Handler())

	metricsSrv := &http.Server{
		Addr:    fmt.Sprintf("0.0.0.0:%d", metricsPort),
		Handler: metricsMux,
	}

	// ---- pprof server (loopback only) ----
	pprofMux := http.NewServeMux()
	pprofMux.HandleFunc("/debug/pprof/", pprof.Index)
	pprofMux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	pprofMux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	pprofMux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	pprofMux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	pprofSrv := &http.Server{
		Addr:    fmt.Sprintf("0.0.0.0:%d", pprofPort),
		Handler: pprofMux,
	}

	errChan := make(chan error, 2)

	// metrics goroutine
	go func() {
		go func() {
			<-ctx.Done()
			logger.Info("metrics server shutting down...")
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = metricsSrv.Shutdown(shutdownCtx)
		}()

		logger.Info("metrics server starting", "address", metricsSrv.Addr)
		if err := metricsSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- fmt.Errorf("metrics server error: %w", err)
		}
	}()

	// pprof goroutine
	go func() {
		go func() {
			<-ctx.Done()
			logger.Info("pprof server shutting down...")
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = pprofSrv.Shutdown(shutdownCtx)
		}()

		logger.Info("pprof server starting", "address", pprofSrv.Addr)
		if err := pprofSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- fmt.Errorf("pprof server error: %w", err)
		}
	}()

	return errChan
}

// setupJSONRPCStateStreamer assembles the differ and the JSON RPC streamer, then starts the server.
// It encapsulates the final "wiring" of the user-facing components.
func setupJSONRPCStateStreamer(
	ctx context.Context,
	engine *engine.Engine,
	differ server.StateDiffer,
	logger *slog.Logger,
) (<-chan error, error) {

	streamer, err := server.NewStateStreamer(&server.Config{
		Logger:            logger.With("component", "jsonrpc-defi-state-streamer"),
		Engine:            engine,
		Differ:            differ,
		FullStateInterval: DefaultJSONRPCStreamFullStateInterval,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create the JSONRPC streamer: %w", err)
	}

	return server.StartStreamer(
		ctx,
		streamer,
		DefaultJSONRPCStreamPort,
		DefaultJSONRPCStreamAllowedOrigins,
	)
}

func setupForkService(
	ctx context.Context,
	cfg *config.Config,
	newBlockSubscription func(consumerName string) chan *types.Block,
	logger *slog.Logger,
) (string, error) {
	getRPCURL := func() (string, error) {
		return cfg.Fork.RPCURL, nil
	}

	tokenOverrides := map[common.Address]fork.FeeGasOverride{}

	switch cfg.ChainID.Uint64() {
	case chains.Celo:
		// anvil fails for the Celo native token analysis @todo fix
		// we simply add this override for now
		tokenOverrides[common.HexToAddress("0x471EcE3750Da237f93B8E339c536989b8978a438")] = fork.FeeGasOverride{
			Gas:                     27000,
			FeeOnTransferPercentage: 0,
		}

	}

	if cfg.Fork.TokenOverrides != nil {
		for t, o := range cfg.Fork.TokenOverrides {
			tokenOverrides[common.HexToAddress(t)] = fork.FeeGasOverride{
				FeeOnTransferPercentage: o.FeeOnTransferPercentage,
				Gas:                     o.Gas,
			}
		}
	}

	poolCfg := []fork.ForkConfig{{
		ChainID:          cfg.ChainID.Uint64(),
		AnvilPort:        DefaultForkPoolServiceAnvilPort,
		BlocksBehind:     DefaultForkBlocksBehind,
		GetMainnetRPCURL: getRPCURL,
		NewBlockC:        newBlockSubscription("fork-instance-1"),
		Logger:           logger.With("component", "fork-instance-1"),
		TokenOverrides:   tokenOverrides,
	}}
	_, err := fork.NewForkPoolService(
		ctx,
		fork.ForkPoolServiceConfig{
			PoolConfig: poolCfg,
			RPCPort:    DefaultForkPoolServiceRPCPort,
			Logger:     logger.With("component", "fork-service"),
		},
	)

	if err != nil {
		return "", err
	}

	return fmt.Sprintf("http://localhost:%d", DefaultForkPoolServiceRPCPort), nil
}

func loadConfig() (*config.Config, error) {
	configPath := flag.String("config", "config.yaml", "Path to the configuration file.")
	flag.Parse()
	log.Printf("Loading configuration from: %s", *configPath)
	return config.LoadConfig(*configPath)
}
