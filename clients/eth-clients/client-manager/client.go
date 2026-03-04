package clientmanager

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	ethclients "github.com/defistate/defistate/clients/eth-clients"
	"github.com/ethereum/go-ethereum/ethclient"
)

// Default configuration values
var (
	defaultMonitorHealthInterval   = 15 * time.Second
	defaultHealthCheckRPCTimeout   = 5 * time.Second
	defaultWaitHealthyPollInterval = 1 * time.Second
	defaultMaxConcurrentETHCalls   = 10 // Default for max concurrent calls to the ETH node
	defaultDialClientTimeout       = 10 * time.Second
	defaultLogger                  = &DefaultLogger{}
)

// ClientConfig holds configuration parameters for the Client.
type ClientConfig struct {
	MonitorHealthInterval   time.Duration // How often to perform health checks.
	HealthCheckRPCTimeout   time.Duration // Timeout for the RPC call within each health check.
	WaitHealthyPollInterval time.Duration // How often to poll for health in WaitUntilHealthy.
	MaxConcurrentETHCalls   int           // Maximum concurrent calls allowed to the underlying ETHClient.
	DialTimeout             time.Duration // timeout for dial client
	Logger                  Logger
}

func (config *ClientConfig) applyDefaults() {
	// Apply default configurations if zero values are provided
	if config.MonitorHealthInterval <= 0 {
		config.MonitorHealthInterval = defaultMonitorHealthInterval
	}
	if config.HealthCheckRPCTimeout <= 0 {
		config.HealthCheckRPCTimeout = defaultHealthCheckRPCTimeout
	}
	if config.WaitHealthyPollInterval <= 0 {
		config.WaitHealthyPollInterval = defaultWaitHealthyPollInterval
	}
	if config.MaxConcurrentETHCalls <= 0 {
		config.MaxConcurrentETHCalls = defaultMaxConcurrentETHCalls
	}

	if config.DialTimeout <= 0 {
		config.DialTimeout = defaultDialClientTimeout
	}

	if config.Logger == nil {
		config.Logger = defaultLogger
	}
}

type Client struct {
	ETHClient         *ethclients.ETHClientWithMaxConcurrentCalls
	healthy           atomic.Bool
	latestBlockNumber uint64
	latency           float64
	cancelFunc        func()
	mu                sync.RWMutex

	monitorHealthInterval   time.Duration
	healthCheckRPCTimeout   time.Duration
	waitHealthyPollInterval time.Duration

	logger Logger

	closed atomic.Bool
}

func NewClientFromDial(
	ctx context.Context, // passed-in context ctx controls the client's lifecycle
	dial string,
	config *ClientConfig,
) (*Client, error) {
	config.applyDefaults()
	// a 10 second dial timeout should be flexible enough
	dialCtx, cancel := context.WithTimeout(ctx, config.DialTimeout)
	defer cancel()

	ethClient, err := ethclient.DialContext(dialCtx, dial)
	if err != nil {
		return nil, err
	}

	// ethclient.Client implements the ETHClient interface
	return NewClientFromETHClient(ctx, ethClient, config)
}

// NewClientFromETHClient creates a new Client with the given underlying ETHClient
// and applies the provided configuration.
func NewClientFromETHClient(
	parentContext context.Context, // Parent context for the client's lifecycle
	ethNodeClient ethclients.ETHClient, // The underlying Ethereum client (e.g., *ethclient.Client)
	config *ClientConfig,
) (*Client, error) {

	// Create a new context that can be cancelled by Client.Close()
	ctx, cancel := context.WithCancel(parentContext)
	config.applyDefaults()
	ethClientWithMaxConcurrentCalls, err := ethclients.NewETHClientWithMaxConcurrentCalls(
		ethNodeClient,
		config.MaxConcurrentETHCalls,
	)

	if err != nil {
		cancel()
		return nil, err
	}

	client := &Client{
		ETHClient:               ethClientWithMaxConcurrentCalls,
		cancelFunc:              cancel,
		monitorHealthInterval:   config.MonitorHealthInterval,
		healthCheckRPCTimeout:   config.HealthCheckRPCTimeout,
		waitHealthyPollInterval: config.WaitHealthyPollInterval,
		logger:                  config.Logger,
	}

	// perform initial health check
	client.performHealthCheck(ctx)
	go client.monitorHealth(ctx) // Pass the cancellable context
	return client, nil
}

func (c *Client) setLatency(l float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.latency = l
}

// MonitorHealth periodically checks the client's health by fetching the latest block.
// It records the duration of each health check and increments success/failure counters.
func (c *Client) monitorHealth(ctx context.Context) {
	ticker := time.NewTicker(c.monitorHealthInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.performHealthCheck(ctx)
		case <-ctx.Done():
			c.logger.Info("[Client]: MonitorHealth shutting down")
			return
		}
	}
}

func (c *Client) performHealthCheck(parentCtx context.Context) {
	start := time.Now() // Start timer for health check duration.
	hcCtx, cancel := context.WithTimeout(parentCtx, c.healthCheckRPCTimeout)
	blockNumber, err := c.ETHClient.BlockNumber(hcCtx)
	cancel()

	duration := time.Since(start).Seconds()
	clientHealthCheckDuration.Observe(duration)
	c.setLatency(duration)

	if err != nil {
		clientHealthCheckCounter.WithLabelValues("failure").Inc()
		// Check if the error is due to the health check context timing out vs parent context cancellation
		if errors.Is(err, context.DeadlineExceeded) && hcCtx.Err() == context.DeadlineExceeded {
			c.logger.Warn(fmt.Sprintf("Health check RPC timed out after %v: %s", c.healthCheckRPCTimeout, err.Error()))
		} else if errors.Is(err, context.Canceled) && parentCtx.Err() == context.Canceled {
			// This means the client is shutting down, no need to logger as a typical failure
			c.logger.Info(fmt.Sprintf("Health check preempted by client shutdown: %v", err))
		} else {
			c.logger.Warn(fmt.Sprintf("Health check failed: %s", err.Error()))
		}
		c.healthy.Store(false)
		// Do NOT update latestBlockNumber on failure; it retains the last known good value.
	} else {
		clientHealthCheckCounter.WithLabelValues("success").Inc()
		c.setLatestBlockNumber(blockNumber)
		c.healthy.Store(true)
		c.logger.Debug(fmt.Sprintf("Health check succeeded, block number: %d, latency: %.4fs", blockNumber, duration))
	}
}

// setLatestBlockNumber safely updates the client's latest block.
func (c *Client) setLatestBlockNumber(blockNumber uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.latestBlockNumber = blockNumber
}

// LatestBlockNumber safely returns the client's latest block number
func (c *Client) LatestBlockNumber() uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.latestBlockNumber
}

func (c *Client) Latency() float64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.latency
}

// Healthy returns the client's health status.
func (c *Client) Healthy() bool {
	return c.healthy.Load()
}

// WaitUntilHealthy waits until the client is healthy or a timeout occurs.
func (c *Client) WaitUntilHealthy(ctx context.Context, timeout time.Duration) error {

	if c.Healthy() {
		return nil
	}

	c.logger.Debug(fmt.Sprintf("Waiting up to %v for client to become healthy...", timeout))

	// Create a timer for the overall timeout of this wait operation.
	timeoutTimer := time.NewTimer(timeout)
	defer timeoutTimer.Stop()

	// Ticker for polling the health status.
	pollTicker := time.NewTicker(c.waitHealthyPollInterval)
	defer pollTicker.Stop()

	for {
		select {
		case <-pollTicker.C:
			if c.Healthy() {
				c.logger.Debug("client is now healthy.")
				return nil
			}
			// Continue polling
		case <-timeoutTimer.C:
			err := errors.New("timeout waiting for client to become healthy")
			c.logger.Error(fmt.Sprintf("%s", err.Error()))
			return err
		case <-ctx.Done(): // If the calling context is cancelled
			c.logger.Error(fmt.Sprintf("waitUntilHealthy cancelled by caller: %v", ctx.Err()))
			return ctx.Err()
		}
	}
}

// Close gracefully stops health monitoring and closes the client connection.
func (c *Client) Close() {
	if c.closed.CompareAndSwap(false, true) {
		c.cancelFunc()
		c.ETHClient.Close()
	}
}
