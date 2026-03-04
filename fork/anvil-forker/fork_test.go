package fork

import (
	"context"
	"math/big"
	"os"
	"testing"
	"time"

	ethclients "github.com/defistate/defistate/clients/eth-clients"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// No-op logger for tests
type testLogger struct{}

func (l *testLogger) Debug(msg string, args ...any) {}
func (l *testLogger) Info(msg string, args ...any)  {}
func (l *testLogger) Warn(msg string, args ...any)  {}
func (l *testLogger) Error(msg string, args ...any) {}

func getUpstreamURL(t *testing.T) string {
	upstream := os.Getenv("MAINNET_RPC_URL")
	if upstream == "" {
		t.Skip("MAINNET_RPC_URL not set")
	}
	return upstream
}

func getRPCProvider(url string) func() (string, error) {
	return func() (string, error) { return url, nil }
}

func TestFork_StartAndStop(t *testing.T) {
	upstream := getUpstreamURL(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	newBlockC := make(chan *types.Block, 1)
	fork := NewFork(ctx, ForkConfig{
		ChainID:          1,
		GetMainnetRPCURL: getRPCProvider(upstream),
		AnvilPort:        8547,
		BlocksBehind:     1,
		NewBlockC:        newBlockC,
		Logger:           &testLogger{},
	})

	mainnetClient, _ := ethclient.Dial(upstream)
	latest, _ := mainnetClient.BlockByNumber(ctx, nil)
	newBlockC <- latest

	assert.Eventually(t, func() bool {
		client, err := fork.GetAnvilClient()
		if err != nil {
			return false
		}
		num, err := client.BlockNumber(context.Background())
		return err == nil && num > 0
	}, 10*time.Second, 100*time.Millisecond)

	cancel()
	assert.Eventually(t, func() bool {
		return !fork.anvilRunning.Load()
	}, 5*time.Second, 100*time.Millisecond)
}

func TestFork_TokenOverrides(t *testing.T) {
	upstream := getUpstreamURL(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tokenAddr := common.HexToAddress("0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48") // USDC
	overrides := map[common.Address]FeeGasOverride{
		tokenAddr: {
			Gas:                     21000,
			FeeOnTransferPercentage: 5,
		},
	}

	newBlockC := make(chan *types.Block, 1)
	fork := NewFork(ctx, ForkConfig{
		ChainID:          1,
		GetMainnetRPCURL: getRPCProvider(upstream),
		AnvilPort:        8555,
		BlocksBehind:     1,
		NewBlockC:        newBlockC,
		Logger:           &testLogger{},
		TokenOverrides:   overrides,
	})

	// Trigger start
	mainnetClient, _ := ethclient.Dial(upstream)
	latest, _ := mainnetClient.BlockByNumber(ctx, nil)
	newBlockC <- latest

	// Check readiness
	assert.Eventually(t, func() bool {
		_, err := fork.GetAnvilClient()
		return err == nil
	}, 10*time.Second, 100*time.Millisecond)

	req := &SimulateTokenTransferRequest{
		Token:    tokenAddr,
		Holder:   common.HexToAddress("0x0000000000000000000000000000000000000001"),
		Receiver: common.HexToAddress("0x0000000000000000000000000000000000000002"),
	}

	// This should be instantaneous as it bypasses simulation logic
	start := time.Now()
	resp := fork.SimulateTokenTransfer(req)
	duration := time.Since(start)

	assert.Empty(t, resp.Error)
	assert.Equal(t, uint(21000), resp.Gas)
	assert.Equal(t, uint(5), resp.FeeOnTransferPercentage)
	assert.Less(t, duration, 10*time.Millisecond, "Override should be nearly instantaneous")
}

func TestFork_ResetState(t *testing.T) {
	upstream := getUpstreamURL(t)
	anvilPort := 8554
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	newBlockC := make(chan *types.Block, 1)
	fork := NewFork(ctx, ForkConfig{
		ChainID:          1,
		GetMainnetRPCURL: getRPCProvider(upstream),
		AnvilPort:        anvilPort,
		BlocksBehind:     1,
		NewBlockC:        newBlockC,
		Logger:           &testLogger{},
	})

	mainnetClient, _ := ethclient.Dial(upstream)
	latest, err := mainnetClient.BlockByNumber(ctx, nil)
	require.NoError(t, err)

	oldBlockNum := latest.NumberU64() - 20
	oldBlock, _ := mainnetClient.BlockByNumber(ctx, new(big.Int).SetUint64(oldBlockNum))
	newBlockC <- oldBlock

	// Wait for start
	var client ethclients.ETHClient
	assert.Eventually(t, func() bool {
		var err error
		client, err = fork.GetAnvilClient()
		return err == nil
	}, 10*time.Second, 100*time.Millisecond)

	startBlock, _ := client.BlockNumber(ctx)
	assert.Equal(t, oldBlockNum-1, startBlock)

	// Modify state
	testAddr := common.HexToAddress("0x0000000000000000000000000000000000000BEE")
	require.NoError(t, fork.setBalances([]common.Address{testAddr}, "0xDE0B6B3A7640000"))

	// Trigger reset
	newBlockC <- latest

	// Verify reset
	assert.Eventually(t, func() bool {
		currBlock, _ := client.BlockNumber(ctx)
		bal, _ := client.BalanceAt(ctx, testAddr, nil)
		return currBlock == latest.NumberU64()-1 && bal.Sign() == 0
	}, 10*time.Second, 100*time.Millisecond)
}

func TestFork_SimulateTokenTransfer(t *testing.T) {
	upstream := getUpstreamURL(t)
	anvilPort := 8549
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mainnetClient, _ := ethclient.Dial(upstream)
	latest, _ := mainnetClient.BlockByNumber(ctx, nil)

	newBlockC := make(chan *types.Block, 1)
	fork := NewFork(ctx, ForkConfig{
		ChainID:          1,
		GetMainnetRPCURL: getRPCProvider(upstream),
		AnvilPort:        anvilPort,
		BlocksBehind:     1,
		NewBlockC:        newBlockC,
		Logger:           &testLogger{},
	})

	newBlockC <- latest

	assert.Eventually(t, func() bool {
		_, err := fork.GetAnvilClient()
		return err == nil
	}, 10*time.Second, 100*time.Millisecond)

	recv, _ := randomAddress()
	req := &SimulateTokenTransferRequest{
		Token:    common.HexToAddress("0x6B175474E89094C44Da98b954EedeAC495271d0F"),
		Holder:   common.HexToAddress("0x5777d92f208679db4b9778590fa3cab3ac9e2168"),
		Receiver: recv,
	}

	resp := fork.SimulateTokenTransfer(req)
	assert.Empty(t, resp.Error)
	assert.Greater(t, resp.Gas, uint(0))
}

func TestFork_SetCoinbase(t *testing.T) {
	upstream := getUpstreamURL(t)
	anvilPort := 8553
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	newBlockC := make(chan *types.Block, 1)
	fork := NewFork(ctx, ForkConfig{
		ChainID:          1,
		GetMainnetRPCURL: getRPCProvider(upstream),
		AnvilPort:        anvilPort,
		BlocksBehind:     1,
		NewBlockC:        newBlockC,
		Logger:           &testLogger{},
	})

	mainnetClient, _ := ethclient.Dial(upstream)
	latest, _ := mainnetClient.BlockByNumber(ctx, nil)
	newBlockC <- latest

	assert.Eventually(t, func() bool {
		_, err := fork.GetAnvilClient()
		return err == nil
	}, 10*time.Second, 100*time.Millisecond)

	newMiner, _ := randomAddress()
	require.NoError(t, fork.doRPCCall(map[string]interface{}{
		"jsonrpc": "2.0", "id": 1, "method": "anvil_setCoinbase", "params": []interface{}{newMiner.Hex()},
	}, nil))

	require.NoError(t, fork.doRPCCall(map[string]interface{}{
		"jsonrpc": "2.0", "id": 1, "method": "evm_mine", "params": []interface{}{},
	}, nil))

	client, _ := fork.GetAnvilClient()
	block, _ := client.BlockByNumber(context.Background(), nil)
	assert.Equal(t, newMiner.Hex(), block.Coinbase().Hex())
}
