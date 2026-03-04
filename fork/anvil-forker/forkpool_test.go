package fork

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewForkPool(t *testing.T) {
	// Set this to your actual upstream RPC URL
	mainnetURL := os.Getenv("MAINNET_RPC_URL")
	if mainnetURL == "" {
		t.Skip("MAINNET_RPC_URL not set")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 1. Define explicit configurations for each fork
	poolSize := 2
	cfg := make([]ForkConfig, poolSize)
	channels := make([]chan *types.Block, poolSize)

	for i := 0; i < poolSize; i++ {
		channels[i] = make(chan *types.Block, 1)
		cfg[i] = ForkConfig{
			ChainID:          1,
			AnvilPort:        9500 + i, // Explicitly assigned ports
			BlocksBehind:     1,
			GetMainnetRPCURL: func() (string, error) { return mainnetURL, nil },
			NewBlockC:        channels[i],
			Logger:           &testLogger{},
		}
	}

	// 2. Initialize the pool
	pool, err := NewForkPool(ctx, cfg)
	require.NoError(t, err)

	// 3. Trigger initialization across all forks
	mainnetClient, err := ethclient.Dial(mainnetURL)
	require.NoError(t, err)
	defer mainnetClient.Close()

	latest, err := mainnetClient.BlockByNumber(ctx, nil)
	require.NoError(t, err)

	// Feed each individual channel explicitly
	for _, ch := range channels {
		ch <- latest
	}

	// 4. Test Round-Robin distribution
	f1 := pool.Get()
	f2 := pool.Get()
	f3 := pool.Get() // Should wrap back to f1

	assert.NotNil(t, f1)
	assert.NotNil(t, f2)
	assert.Equal(t, f1.anvilPort, f3.anvilPort, "Should wrap around (Round Robin)")
	assert.NotEqual(t, f1.anvilPort, f2.anvilPort, "Instances should have unique ports")

	// 5. Check for readiness
	checkReadiness := func(f *Fork) bool {
		client, err := f.GetAnvilClient()
		if err != nil {
			return false
		}
		num, err := client.BlockNumber(ctx)
		return err == nil && num > 0
	}

	assert.Eventually(t, func() bool {
		return checkReadiness(f1) && checkReadiness(f2)
	}, 10*time.Second, 100*time.Millisecond, "All forks in pool should become ready")

	// 6. Shutdown via context cancellation
	cancel()

	// 7. Verify Stop
	assert.Eventually(t, func() bool {
		_, err1 := f1.GetAnvilClient()
		_, err2 := f2.GetAnvilClient()
		return err1 != nil && err2 != nil
	}, 2*time.Second, 50*time.Millisecond, "All forks should return error after context cancellation")
}
