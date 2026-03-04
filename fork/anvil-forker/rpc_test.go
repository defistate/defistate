package fork

import (
	"context"
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// utility: find open TCP port
func getFreePort(t *testing.T) int {
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("failed to find open port: %v", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func TestForkPoolService_simulateTokenTransfer(t *testing.T) {
	upstream := os.Getenv("MAINNET_RPC_URL")
	if upstream == "" {
		t.Skip("MAINNET_RPC_URL not set")
	}

	// 1. Setup context and explicit reactive dependencies
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	poolSize := 1
	rpcPort := getFreePort(t)

	// Create explicit configurations for each fork in the pool
	forkConfigs := make([]ForkConfig, poolSize)
	channels := make([]chan *types.Block, poolSize)

	for i := 0; i < poolSize; i++ {
		channels[i] = make(chan *types.Block, 1)
		forkConfigs[i] = ForkConfig{
			ChainID:          1,
			AnvilPort:        getFreePort(t), // Each fork gets its own port
			BlocksBehind:     1,
			GetMainnetRPCURL: func() (string, error) { return upstream, nil },
			NewBlockC:        channels[i],
			Logger:           &testLogger{},
		}
	}

	cfg := ForkPoolServiceConfig{
		PoolConfig: forkConfigs,
		RPCPort:    rpcPort,
		Logger:     &testLogger{},
	}

	// 2. Start the Service
	svc, err := NewForkPoolService(ctx, cfg)
	require.NoError(t, err)
	require.NotNil(t, svc)

	// 3. Trigger Fork Initialization
	mainnetClient, err := ethclient.Dial(upstream)
	require.NoError(t, err)
	defer mainnetClient.Close()

	latest, err := mainnetClient.BlockByNumber(ctx, nil)
	require.NoError(t, err)

	// Feed all individual channels to initialize the Anvil processes
	for _, ch := range channels {
		ch <- latest
	}

	// 4. Dial the Service RPC
	client, err := rpc.Dial(fmt.Sprintf("http://localhost:%d", rpcPort))
	require.NoError(t, err)
	defer client.Close()

	// 5. Execute Simulation Request
	recv, err := randomAddress()
	require.NoError(t, err)

	req := SimulateTokenTransferRequest{
		Token:    common.HexToAddress("0x6B175474E89094C44Da98b954EedeAC495271d0F"), // DAI
		Holder:   common.HexToAddress("0x5777d92f208679db4b9778590fa3cab3ac9e2168"), // DAI Whale
		Receiver: recv,
	}

	var resp SimulateTokenTransferResponse

	// Use Eventually to handle asynchronous cold start of Anvil
	assert.Eventually(t, func() bool {
		err = client.Call(&resp, "fork_simulateTokenTransfer", req)
		return err == nil && resp.Error == ""
	}, 15*time.Second, 500*time.Millisecond, "Service should eventually process simulation successfully")

	// 6. Assertions
	assert.Equal(t, req.Token, resp.Token)
	assert.Greater(t, resp.Gas, uint(0))
	assert.Empty(t, resp.Error)

	// 7. Cleanup is handled by cancel()
	cancel()

	// Verify the server is no longer accepting requests
	assert.Eventually(t, func() bool {
		err = client.Call(&resp, "fork_simulateTokenTransfer", req)
		return err != nil
	}, 5*time.Second, 100*time.Millisecond, "Service should reject calls after context cancellation")
}
