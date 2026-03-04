package clientmanager

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	ethclients "github.com/defistate/defistate/clients/eth-clients"
	"github.com/stretchr/testify/assert"
)

func TestNewClientFromDial(t *testing.T) {
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
			Params  []json.RawMessage `json:"params,omitempty"`
			ID      interface{}       `json:"id"` // ID can be string, number, or null
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
	config := &ClientConfig{}
	client, err := NewClientFromDial(ctx, server.URL, config)
	assert.Nil(t, err)
	assert.NotNil(t, client)
	defer client.Close()
}

func TestNewClientFromClient(t *testing.T) {
	ctx := context.Background()
	testETHClient := ethclients.NewTestETHClient()
	config := &ClientConfig{}
	client, err := NewClientFromETHClient(ctx, testETHClient, config)
	assert.Nil(t, err)
	assert.NotNil(t, client)
	defer client.Close()

}

func TestClientLatestBlockNumber(t *testing.T) {
	ctx := context.Background()
	testETHClient := ethclients.NewTestETHClient()
	expectedBlockNumber := uint64(100)
	testETHClient.SetBlockNumberHandler(func(ctx context.Context) (uint64, error) {
		return expectedBlockNumber, nil
	})
	config := &ClientConfig{
		MonitorHealthInterval: 1 * time.Millisecond,
	}
	client, err := NewClientFromETHClient(ctx, testETHClient, config)
	assert.Nil(t, err)
	assert.NotNil(t, client)
	defer client.Close()
	assert.Equal(t, client.LatestBlockNumber(), expectedBlockNumber)
}

func TestClientLatency(t *testing.T) {
	ctx := context.Background()
	testETHClient := ethclients.NewTestETHClient()
	expectedLatency := 10 * time.Millisecond
	expectedMinLatency := expectedLatency.Seconds()
	expectedMaxLatency := expectedMinLatency * 2
	testETHClient.SetBlockNumberHandler(func(ctx context.Context) (uint64, error) {
		//simulate latency
		time.Sleep(expectedLatency)
		return 1, nil
	})
	config := &ClientConfig{
		MonitorHealthInterval: 1 * time.Millisecond,
	}
	client, err := NewClientFromETHClient(ctx, testETHClient, config)
	assert.Nil(t, err)
	assert.NotNil(t, client)
	defer client.Close()
	assert.GreaterOrEqual(t, client.Latency(), expectedMinLatency)
	assert.LessOrEqual(t, client.Latency(), expectedMaxLatency)
}

func TestClientHealth(t *testing.T) {
	t.Run("test healthy client", func(t *testing.T) {
		ctx := context.Background()
		testETHClient := ethclients.NewTestETHClient()
		testETHClient.SetBlockNumberHandler(func(ctx context.Context) (uint64, error) {
			return 1, nil
		})
		config := &ClientConfig{
			MonitorHealthInterval: 1 * time.Millisecond,
		}
		client, err := NewClientFromETHClient(ctx, testETHClient, config)
		assert.Nil(t, err)
		assert.NotNil(t, client)
		defer client.Close()
		assert.True(t, client.Healthy())

	})

	t.Run("test unhealthy client", func(t *testing.T) {
		ctx := context.Background()
		testETHClient := ethclients.NewTestETHClient()
		testETHClient.SetBlockNumberHandler(func(ctx context.Context) (uint64, error) {
			return 0, errors.New("unhealthy client")
		})
		config := &ClientConfig{
			MonitorHealthInterval: 1 * time.Millisecond,
		}
		client, err := NewClientFromETHClient(ctx, testETHClient, config)
		assert.Nil(t, err)
		assert.NotNil(t, client)
		defer client.Close()
		assert.False(t, client.Healthy())

	})

	t.Run("test unhealthy client becomes healthy", func(t *testing.T) {
		ctx := context.Background()
		testETHClient := ethclients.NewTestETHClient()
		attempts := atomic.Uint32{}
		testETHClient.SetBlockNumberHandler(func(ctx context.Context) (uint64, error) {
			n := attempts.Add(1)
			if n == 1 {
				return 0, errors.New("unhealthy client")
			}

			return 100, nil
		})
		config := &ClientConfig{
			MonitorHealthInterval: 10 * time.Millisecond,
		}
		client, err := NewClientFromETHClient(ctx, testETHClient, config)
		assert.Nil(t, err)
		assert.NotNil(t, client)
		defer client.Close()
		assert.False(t, client.Healthy())
		time.Sleep(config.MonitorHealthInterval * 2)
		assert.True(t, client.Healthy())
	})
}

func TestClientWaitUntilHealthy(t *testing.T) {
	t.Run("should return immediately for healthy client", func(t *testing.T) {
		ctx := context.Background()
		testETHClient := ethclients.NewTestETHClient()
		testETHClient.SetBlockNumberHandler(func(ctx context.Context) (uint64, error) {
			return 1, nil
		})
		config := &ClientConfig{
			MonitorHealthInterval: 1 * time.Millisecond,
		}
		client, err := NewClientFromETHClient(ctx, testETHClient, config)
		assert.Nil(t, err)
		assert.NotNil(t, client)
		defer client.Close()
		assert.True(t, client.Healthy())
		start := time.Now()
		client.WaitUntilHealthy(ctx, 1*time.Second)
		// Still very fast, but gives a tiny bit more leeway
		assert.True(t, time.Since(start) < 5*time.Millisecond, "WaitUntilHealthy should return almost immediately for a healthy client")
	})
	t.Run("wait until healthy for unhealthy client that becomes healthy on 2nd health check attempt", func(t *testing.T) {
		ctx := context.Background()
		testETHClient := ethclients.NewTestETHClient()
		attempts := atomic.Uint32{}
		testETHClient.SetBlockNumberHandler(func(ctx context.Context) (uint64, error) {
			n := attempts.Add(1)
			if n == 1 {
				return 0, errors.New("unhealthy client")
			}

			return 100, nil
		})
		config := &ClientConfig{
			MonitorHealthInterval:   50 * time.Millisecond,
			WaitHealthyPollInterval: 100 * time.Millisecond,
		}
		client, err := NewClientFromETHClient(ctx, testETHClient, config)
		assert.Nil(t, err)
		assert.NotNil(t, client)
		defer client.Close()
		// client is initially unhealthy
		assert.False(t, client.Healthy())

		waitCtx, cancel := context.WithTimeout(context.Background(), config.WaitHealthyPollInterval*2)
		defer cancel()
		start := time.Now()
		err = client.WaitUntilHealthy(waitCtx, config.WaitHealthyPollInterval*2)
		// we must not receive an error
		assert.Nil(t, err)
		// we need to be sure that we actually waited
		assert.True(t, time.Since(start) > config.WaitHealthyPollInterval)
	})
}

func TestClientClose(t *testing.T) {
	t.Run("close on NewClientFromETHClient", func(t *testing.T) {
		ctx := context.Background()
		testETHClient := ethclients.NewTestETHClient()
		testETHClient.SetBlockNumberHandler(func(ctx context.Context) (uint64, error) {
			return 1, nil
		})
		config := &ClientConfig{
			MonitorHealthInterval: 1 * time.Millisecond,
		}
		client, err := NewClientFromETHClient(ctx, testETHClient, config)
		assert.Nil(t, err)
		assert.NotNil(t, client)
		client.Close()
		assert.True(t, client.closed.Load())
		// client shouldn't panic on double close
		client.Close()
		// client.closed should remain true
		assert.True(t, client.closed.Load())
	})

	t.Run("close on NewClientFromDial", func(t *testing.T) {
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
		config := &ClientConfig{}
		client, err := NewClientFromDial(ctx, server.URL, config)
		assert.Nil(t, err)
		assert.NotNil(t, client)
		client.Close()
		assert.True(t, client.closed.Load())
		// client shouldn't panic on double close
		client.Close()
		// client.closed should remain true
		assert.True(t, client.closed.Load())
	})

}
