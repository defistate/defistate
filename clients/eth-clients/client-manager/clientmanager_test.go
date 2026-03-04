package clientmanager

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	// Assuming this path is correct
	ethclients "github.com/defistate/defistate/clients/eth-clients"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/assert"
	// "logger" // Not used directly in tests, but ClientManager uses it.
	// "math/rand" // Not used directly in tests, but ClientManager uses it.
	// "sort" // Not used directly in tests, but ClientManager uses it.
)

func TestNewClientManagerFromClients(t *testing.T) {
	managerConfig := &ClientManagerConfig{}
	ctx := context.Background()
	client, err := NewClientFromETHClient(ctx, ethclients.NewTestETHClient(), &ClientConfig{})
	assert.Nil(t, err)
	clients := []*Client{client}
	mgr, err := NewClientManagerFromClients(clients, managerConfig)
	assert.Nil(t, err)
	assert.NotNil(t, mgr)
	defer mgr.Close()
}

func TestNewClientManager(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	mux := http.NewServeMux()
	// ensures the health check succeeds
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var req struct {
			JSONRPC string            `json:"jsonrpc"`
			Method  string            `json:"method"`
			Params  []json.RawMessage `json:"params,omitempty"` // Params can vary
			ID      interface{}       `json:"id"`               // ID can be string, number, or null
		}

		err = json.Unmarshal(body, &req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		resp := struct {
			JSONRPC string      `json:"jsonrpc"`
			Result  interface{} `json:"result,omitempty"`
			ID      interface{} `json:"id"`
		}{
			Result: "0x1",
		}

		json.NewEncoder(w).Encode(resp)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	mgr, err := NewClientManager(
		ctx,
		[]string{server.URL},
		&ClientManagerConfig{ClientConfig: &ClientConfig{}},
	)
	assert.Nil(t, err)
	assert.NotNil(t, mgr)
	defer mgr.Close()

}

func TestClientManagerGetClient(t *testing.T) {
	t.Run("one unhealthy, one healthy", func(t *testing.T) {
		unhealthy := ethclients.NewTestETHClient()
		unhealthy.SetBlockNumberHandler(func(ctx context.Context) (uint64, error) {
			return 0, errors.New("unhealthy")
		})

		healthy := ethclients.NewTestETHClient()

		mu := sync.Mutex{}
		currentBlockNumberHealthy := uint64(10)
		// set a BlockByNumber handler for healthy
		healthy.SetBlockByNumberHandler(func(ctx context.Context, number *big.Int) (*types.Block, error) {
			mu.Lock()
			defer mu.Unlock()
			// Use the number passed to the handler, not the shared currentBlockNumberHealthy directly for this mock
			headerNum := new(big.Int).Set(number)
			return types.NewBlockWithHeader(&types.Header{Number: headerNum}).WithBody(types.Body{}), nil
		})

		// set a BlockNumber handler for the healthy client's health check
		healthy.SetBlockNumberHandler(func(ctx context.Context) (uint64, error) {
			mu.Lock()
			defer mu.Unlock()
			return currentBlockNumberHealthy, nil
		})

		ctx := context.Background()
		clientCfg := &ClientConfig{
			MonitorHealthInterval: 50 * time.Millisecond, // Interval for health checks
			MaxConcurrentETHCalls: 100,
		}
		client1, err := NewClientFromETHClient(ctx, unhealthy, clientCfg)
		assert.Nil(t, err)
		client2, err := NewClientFromETHClient(ctx, healthy, clientCfg)
		assert.Nil(t, err)

		managerCfg := &ClientManagerConfig{ClientConfig: clientCfg}
		cm, err := NewClientManagerFromClients([]*Client{client1, client2}, managerCfg)
		assert.NoError(t, err)
		defer cm.Close()

		// Allow time for initial health checks and block subscriptions
		time.Sleep(clientCfg.MonitorHealthInterval * 4)

		c, err := cm.GetClient()
		assert.Nil(t, err, "expect no error on GetClient call")
		// check that block number == currentBlockNumberHealthy
		blockNumber, err := c.BlockNumber(ctx)
		assert.Nil(t, err)
		assert.True(t, blockNumber == currentBlockNumberHealthy)

	})

	t.Run("all clients unhealthy", func(t *testing.T) {
		unhealthy1 := ethclients.NewTestETHClient()
		unhealthy1.SetBlockNumberHandler(func(ctx context.Context) (uint64, error) {
			return 0, errors.New("client 1 unhealthy")
		})

		unhealthy2 := ethclients.NewTestETHClient()
		unhealthy2.SetBlockNumberHandler(func(ctx context.Context) (uint64, error) {
			return 0, errors.New("client 2 unhealthy")
		})

		ctx := context.Background()
		clientCfg := &ClientConfig{
			MonitorHealthInterval: 50 * time.Millisecond,
			MaxConcurrentETHCalls: 100,
		}
		client1, err := NewClientFromETHClient(ctx, unhealthy1, clientCfg)
		assert.Nil(t, err)
		client2, err := NewClientFromETHClient(ctx, unhealthy2, clientCfg)
		assert.Nil(t, err)

		managerCfg := &ClientManagerConfig{} // Defaults are fine
		cm, err := NewClientManagerFromClients([]*Client{client1, client2}, managerCfg)
		assert.NoError(t, err)
		defer cm.Close()

		// Allow time for health checks to mark clients as unhealthy
		// Allow time for initial health checks and block subscriptions
		time.Sleep(clientCfg.MonitorHealthInterval * 4)

		_, err = cm.GetClient()
		assert.NotNil(t, err, "expected err when all are unhealthy")

	})
}

func TestClientManagerGetPreferredClient(t *testing.T) {
	t.Run("one fast, one slow, both healthy and up to date", func(t *testing.T) {
		ctx, cancelCtx := context.WithCancel(context.Background())
		defer cancelCtx()

		fastClientMock := ethclients.NewTestETHClient()
		slowClientMock := ethclients.NewTestETHClient()

		setupMockClient := func(mock *ethclients.TestETHClient, latencySim time.Duration, initialBlockNum int64) {
			var mu sync.Mutex
			currentBlockNum := big.NewInt(initialBlockNum)

			mock.SetBlockNumberHandler(func(ctx context.Context) (uint64, error) {
				time.Sleep(latencySim) // Simulate health check latency
				mu.Lock()
				defer mu.Unlock()
				return currentBlockNum.Uint64(), nil
			})

			mock.SetBlockByNumberHandler(func(ctx context.Context, number *big.Int) (*types.Block, error) {
				return types.NewBlockWithHeader(&types.Header{Number: new(big.Int).Set(number)}).WithBody(types.Body{}), nil
			})

			mock.SetSubscribeNewHeadHandler(func(ctx context.Context, ch chan<- *types.Header) (ethereum.Subscription, error) {
				go func() {
					// send only one header
					ch <- &types.Header{Number: currentBlockNum}
				}()

				return ethclients.NewTestSubscription(func() {}, func() <-chan error { return make(chan error) }), nil
			})
		}

		fastClientLatency := 1 * time.Millisecond
		slowClientLatency := 10 * time.Millisecond
		setupMockClient(fastClientMock, fastClientLatency, 100) // Fast client
		setupMockClient(slowClientMock, slowClientLatency, 100) // Slow client

		clientCfg := &ClientConfig{
			MonitorHealthInterval:   30 * time.Millisecond,
			HealthCheckRPCTimeout:   20 * time.Millisecond,
			WaitHealthyPollInterval: 5 * time.Millisecond,
		}
		clientFast, err := NewClientFromETHClient(ctx, fastClientMock, clientCfg)
		assert.Nil(t, err)
		clientSlow, err := NewClientFromETHClient(ctx, slowClientMock, clientCfg)
		assert.Nil(t, err)
		time.Sleep(clientCfg.MonitorHealthInterval)

		managerCfg := &ClientManagerConfig{
			ClientConfig:                        clientCfg,
			PreferredClientMaxLatencyMultiplier: 3,
		}
		cm, err := NewClientManagerFromClients([]*Client{clientFast, clientSlow}, managerCfg)
		assert.NoError(t, err)
		defer cm.Close()

		// Allow time for several health checks and for latencies to be recorded
		// Also for blocks to be processed so cm.LatestBlock() is not nil
		// And client.LatestBlockNumber() to be updated.
		time.Sleep(200 * time.Millisecond)

		for i := 0; i < 10; i++ {
			c, errGet := cm.GetPreferredClient()
			assert.NoError(t, errGet)
			// verify that latency <= slowClientLatency
			start := time.Now()
			_, err := c.BlockNumber(ctx)
			assert.Nil(t, err)
			assert.True(t, time.Since(start) < slowClientLatency)
			time.Sleep(100 * time.Millisecond)
		}

	})
}

func TestClientManagerClose(t *testing.T) {
	ctx := context.Background()
	clientCfg := &ClientConfig{
		MonitorHealthInterval: 50 * time.Millisecond, // Interval for health checks
		MaxConcurrentETHCalls: 100,
	}
	client, err := NewClientFromETHClient(ctx, ethclients.NewTestETHClient(), clientCfg)
	assert.Nil(t, err)
	managerCfg := &ClientManagerConfig{ClientConfig: clientCfg}
	cm, err := NewClientManagerFromClients([]*Client{client}, managerCfg)
	assert.NoError(t, err)
	cm.Close()
	assert.True(t, cm.closed.Load())
	// should not panic and stay closed on second Close call
	cm.Close()
	assert.True(t, cm.closed.Load())
}
