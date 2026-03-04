package fork

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/ethereum/go-ethereum/rpc"
)

// ForkPoolServiceConfig defines the settings for the JSON-RPC microservice.
type ForkPoolServiceConfig struct {
	PoolConfig ForkPoolConfig
	RPCPort    int
	Logger     Logger
}

// ForkPoolService exposes fork simulation methods via a JSON-RPC server.
type ForkPoolService struct {
	pool   *ForkPool
	logger Logger
}

// NewForkPoolService initializes the pool and mounts the JSON-RPC server.
// The service's lifetime is strictly bound to the provided ctx.
func NewForkPoolService(ctx context.Context, cfg ForkPoolServiceConfig) (*ForkPoolService, error) {
	if cfg.Logger == nil {
		return nil, fmt.Errorf("logger is required for ForkPoolService")
	}

	// Initialize the pool using the root context.
	// When ctx is cancelled, all forks in the pool will stop.
	pool, err := NewForkPool(ctx, cfg.PoolConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize pool: %w", err)
	}

	s := &ForkPoolService{
		pool:   pool,
		logger: cfg.Logger,
	}

	if err := s.startRPCServer(ctx, cfg.RPCPort); err != nil {
		return nil, err
	}

	return s, nil
}

// startRPCServer configures and launches the HTTP JSON-RPC listener.
func (s *ForkPoolService) startRPCServer(ctx context.Context, port int) error {
	server := rpc.NewServer()

	// Register the service under the "fork" namespace.
	if err := server.RegisterName("fork", s); err != nil {
		return fmt.Errorf("failed to register rpc service: %w", err)
	}

	httpServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: server,
	}

	// Start the listener in a background goroutine.
	go func() {
		s.logger.Info("JSON-RPC server listening", "port", port)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("RPC server ListenAndServe error", "error", err)
		}
	}()

	// Graceful shutdown triggered by the root context.
	go func() {
		<-ctx.Done()
		s.logger.Info("root context cancelled, shutting down RPC server...")

		// Create a brief timeout for finishing existing RPC requests.
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			s.logger.Error("RPC server shutdown error", "error", err)
		}
	}()

	return nil
}

// SimulateTokenTransfer is the exported RPC method: fork_simulateTokenTransfer.
func (s *ForkPoolService) SimulateTokenTransfer(
	req SimulateTokenTransferRequest,
) (*SimulateTokenTransferResponse, error) {
	// Delegating to the pool's round-robin selection.
	return s.pool.Get().SimulateTokenTransfer(&req), nil
}
