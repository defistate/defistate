package fork

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"os/exec"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	ethclients "github.com/defistate/defistate/clients/eth-clients"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

// --- Constants & Defaults ---

const (
	DefaultETHBalance           = "0x204fce5e3e25026110000000" // 1e27 wei
	DefaultAnvilRPCTimeout      = 10 * time.Second
	DefaultAnvilRetries         = 50
	DefaultAnvilTick            = 100 * time.Millisecond
	DefaultSimTimeout           = 20 * time.Second
	DefaultReceiptRetryDuration = 10 * time.Second
)

// --- Interfaces ---

type Logger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
}

// --- Configuration & Types ---

type FeeGasOverride struct {
	Gas                     uint
	FeeOnTransferPercentage uint
}

type ForkConfig struct {
	ChainID          uint64
	AnvilPort        int
	BlocksBehind     uint64
	GetMainnetRPCURL func() (string, error) // Functional provider for dynamic URL rotation
	NewBlockC        chan *types.Block
	Logger           Logger
	TokenOverrides   map[common.Address]FeeGasOverride
}

type SimulateTokenTransferRequest struct {
	Token    common.Address
	Holder   common.Address
	Receiver common.Address
}

type SimulateTokenTransferResponse struct {
	Gas                     uint
	FeeOnTransferPercentage uint
	Token                   common.Address
	Error                   string
}

// --- Fork Component ---

type Fork struct {
	chainId          uint64
	anvilPort        int
	blocksBehind     uint64
	getMainnetRPCURL func() (string, error)
	newBlockC        chan *types.Block
	logger           Logger
	tokenOverrides   map[common.Address]FeeGasOverride

	anvilRunning     atomic.Bool
	anvilClient      ethclients.ETHClient
	cmd              *exec.Cmd
	jsonRPCRequestId atomic.Uint64
	mu               sync.RWMutex
}

func NewFork(ctx context.Context, cfg ForkConfig) *Fork {
	f := &Fork{
		chainId:          cfg.ChainID,
		anvilPort:        cfg.AnvilPort,
		blocksBehind:     cfg.BlocksBehind,
		getMainnetRPCURL: cfg.GetMainnetRPCURL,
		newBlockC:        cfg.NewBlockC,
		logger:           cfg.Logger,
		tokenOverrides:   cfg.TokenOverrides,
	}
	go f.loop(ctx)
	return f
}

func (f *Fork) loop(ctx context.Context) {
	for {
		select {
		case b := <-f.newBlockC:
			f.logger.Info("received new block", "block_number", b.NumberU64())
			targetBlock := b.Number().Uint64() - f.blocksBehind
			if err := f.resetState(ctx, targetBlock); err != nil {
				f.logger.Error("failed to reset fork state", "error", err, "targetBlock", targetBlock)
				continue
			}
		case <-ctx.Done():
			f.logger.Info("context cancelled, shutting down fork", "port", f.anvilPort)
			if err := f.stopAnvil(); err != nil {
				f.logger.Error("shutdown errors occurred", "error", err)
			}
			return
		}
	}
}

func (f *Fork) resetState(ctx context.Context, blockNumber uint64) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	rpcURL, err := f.getMainnetRPCURL()
	if err != nil {
		return err
	}
	if !f.anvilRunning.Load() {
		return f.startAnvilProcess(ctx, blockNumber, rpcURL)
	}

	payload := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      f.jsonRPCRequestId.Add(1),
		"method":  "anvil_reset",
		"params": []interface{}{
			map[string]interface{}{
				"forking": map[string]interface{}{
					"jsonRpcUrl":  rpcURL,
					"blockNumber": blockNumber,
				},
			},
		},
	}
	return f.doRPCCall(payload, nil)
}

func (f *Fork) startAnvilProcess(ctx context.Context, blockNumber uint64, rpcURL string) error {
	f.cmd = exec.Command(
		"anvil",
		"--fork-url", rpcURL,
		"--fork-block-number", fmt.Sprintf("%d", blockNumber),
		"--port", fmt.Sprintf("%d", f.anvilPort),
		"--auto-impersonate",
		"--no-storage-caching",
	)

	if err := f.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start anvil: %w", err)
	}

	if err := f.waitForAnvilReady(); err != nil {
		return err
	}

	dialCtx, cancel := context.WithTimeout(ctx, DefaultAnvilRPCTimeout)
	defer cancel()

	client, err := ethclient.DialContext(dialCtx, fmt.Sprintf("http://localhost:%d", f.anvilPort))
	if err != nil {
		return fmt.Errorf("failed to dial anvil: %w", err)
	}

	f.anvilClient = client
	f.anvilRunning.Store(true)
	f.logger.Info("anvil process started", "port", f.anvilPort, "forkUrl", rpcURL)
	return nil
}

func (f *Fork) stopAnvil() error {
	f.anvilRunning.Store(false)
	f.mu.Lock()
	defer f.mu.Unlock()

	var errs []error

	if f.anvilClient != nil {
		f.anvilClient.Close()
		f.anvilClient = nil
	}

	if f.cmd != nil && f.cmd.Process != nil {
		if err := f.cmd.Process.Signal(syscall.SIGTERM); err != nil {
			f.logger.Warn("failed to send SIGTERM, attempting SIGKILL", "error", err)
			if kerr := f.cmd.Process.Kill(); kerr != nil {
				errs = append(errs, fmt.Errorf("failed to kill process: %w", kerr))
			}
		}

		if _, err := f.cmd.Process.Wait(); err != nil {
			errs = append(errs, fmt.Errorf("failed to wait for process exit: %w", err))
		} else {
			f.logger.Info("anvil process terminated successfully")
		}
		f.cmd = nil
	}

	return errors.Join(errs...)
}

// --- Simulation Logic ---

func (f *Fork) SimulateTokenTransfer(req *SimulateTokenTransferRequest) *SimulateTokenTransferResponse {
	// 1. Check for overrides (skip simulation if token is known)
	if override, ok := f.tokenOverrides[req.Token]; ok {
		return &SimulateTokenTransferResponse{
			Token:                   req.Token,
			Gas:                     override.Gas,
			FeeOnTransferPercentage: override.FeeOnTransferPercentage,
		}
	}

	if !f.anvilRunning.Load() {
		return &SimulateTokenTransferResponse{Error: "anvil not running"}
	}
	if req.Receiver == (common.Address{}) {
		return &SimulateTokenTransferResponse{Error: "transfer receiver must be set"}
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), DefaultSimTimeout)
	defer cancel()

	receiver2, err := randomAddress()
	if err != nil {
		return &SimulateTokenTransferResponse{Error: "failed to generate random address: " + err.Error()}
	}

	addrs := []common.Address{req.Holder, req.Receiver, receiver2}
	if err := f.setBalances(addrs, DefaultETHBalance); err != nil {
		return &SimulateTokenTransferResponse{Error: "setup failed: " + err.Error()}
	}

	s1, r1, g1, err := f.transferToken(ctx, req.Token, addrs[0], addrs[1])
	if err != nil {
		return &SimulateTokenTransferResponse{Error: "hop 1: " + err.Error()}
	}

	s2, r2, g2, err := f.transferToken(ctx, req.Token, addrs[1], addrs[2])
	if err != nil {
		return &SimulateTokenTransferResponse{Error: "hop 2: " + err.Error()}
	}

	s3, r3, g3, err := f.transferToken(ctx, req.Token, addrs[2], addrs[0])
	if err != nil {
		return &SimulateTokenTransferResponse{Error: "hop 3: " + err.Error()}
	}

	return &SimulateTokenTransferResponse{
		Token:                   req.Token,
		Gas:                     max(uint(g1), max(uint(g2), uint(g3))),
		FeeOnTransferPercentage: max(calcFee(s1, r1), max(calcFee(s2, r2), calcFee(s3, r3))),
	}
}

// --- Internal RPC Helpers ---

func (f *Fork) GetAnvilClient() (ethclients.ETHClient, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	if f.anvilClient == nil {
		return nil, errors.New("anvil client not set")
	}

	return f.anvilClient, nil
}

func (f *Fork) doRPCCall(payload interface{}, result interface{}) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	resp, err := http.Post(fmt.Sprintf("http://localhost:%d", f.anvilPort), "application/json", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("http post: %w", err)
	}
	defer resp.Body.Close()

	if result != nil {
		if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
			return fmt.Errorf("decode result: %w", err)
		}
	}
	return nil
}

func (f *Fork) sendTransaction(from, to common.Address, data []byte) (common.Hash, error) {
	var res struct {
		Result string `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}

	payload := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      f.jsonRPCRequestId.Add(1),
		"method":  "eth_sendUnsignedTransaction",
		"params": []interface{}{map[string]interface{}{
			"from": from.Hex(), "to": to.Hex(), "data": "0x" + hex.EncodeToString(data),
		}},
	}

	if err := f.doRPCCall(payload, &res); err != nil {
		return common.Hash{}, err
	}
	if res.Error != nil {
		return common.Hash{}, fmt.Errorf("anvil rpc error: %s", res.Error.Message)
	}

	return common.HexToHash(res.Result), nil
}

func (f *Fork) setBalances(addrs []common.Address, hexBal string) error {
	for _, addr := range addrs {
		payload := map[string]interface{}{
			"jsonrpc": "2.0", "id": f.jsonRPCRequestId.Add(1),
			"method": "anvil_setBalance", "params": []interface{}{addr.Hex(), hexBal},
		}
		if err := f.doRPCCall(payload, nil); err != nil {
			return fmt.Errorf("setBalance for %s: %w", addr.Hex(), err)
		}
	}
	return nil
}

func (f *Fork) waitForAnvilReady() error {
	ticker := time.NewTicker(DefaultAnvilTick)
	defer ticker.Stop()
	var lastErr error

	for i := 0; i < DefaultAnvilRetries; i++ {
		payload := `{"jsonrpc":"2.0","id":0,"method":"eth_blockNumber","params":[]}`
		resp, err := http.Post(fmt.Sprintf("http://localhost:%d", f.anvilPort), "application/json", bytes.NewReader([]byte(payload)))
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			return nil
		}
		if err != nil {
			lastErr = err
		}
		<-ticker.C
	}
	return fmt.Errorf("anvil rpc timeout: %w", lastErr)
}

// --- Blockchain Helpers ---

func (f *Fork) transferToken(
	ctx context.Context,
	token common.Address,
	from common.Address,
	to common.Address,
) (amountSent, amountReceived *big.Int, gas uint64, err error) {
	fromBal, err := getERC20Balance(ctx, f.anvilClient, token, from)
	if err != nil {
		return nil, nil, 0, err
	}
	if fromBal.Sign() == 0 {
		return nil, nil, 0, fmt.Errorf("%s has zero balance of token", from.String())
	}

	toBal, err := getERC20Balance(ctx, f.anvilClient, token, to)
	if err != nil {
		return nil, nil, 0, err
	}

	// Internal helper for specific ERC20 transfer execution
	methodID := []byte{0xa9, 0x05, 0x9c, 0xbb}
	data := append(methodID, append(common.LeftPadBytes(to.Bytes(), 32), common.LeftPadBytes(fromBal.Bytes(), 32)...)...)

	txHash, err := f.sendTransaction(from, token, data)
	if err != nil {
		return nil, nil, 0, err
	}

	receipt, err := waitForReceipt(f.anvilClient, txHash)
	if err != nil {
		return nil, nil, 0, err
	}

	toBalAfter, err := getERC20Balance(ctx, f.anvilClient, token, to)
	if err != nil {
		return nil, nil, 0, err
	}

	amountReceived = new(big.Int).Sub(toBalAfter, toBal)
	return fromBal, amountReceived, receipt.GasUsed, nil
}

// --- Helper Functions ---

func waitForReceipt(client ethclients.ETHClient, hash common.Hash) (*types.Receipt, error) {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultReceiptRetryDuration)
	defer cancel()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("receipt timeout for tx %s", hash.Hex())
		case <-ticker.C:
			receipt, err := client.TransactionReceipt(ctx, hash)
			if err != nil {
				if errors.Is(err, ethereum.NotFound) {
					continue
				}
				return nil, err
			}
			return receipt, nil
		}
	}
}

func calcFee(sent, received *big.Int) uint {
	if sent == nil || received == nil || sent.Sign() == 0 || sent.Cmp(received) <= 0 {
		return 0
	}
	diff, fee := new(big.Int).Sub(sent, received), new(big.Int)
	fee.Mul(diff, big.NewInt(100)).Div(fee, sent)
	return uint(fee.Uint64())
}

func randomAddress() (common.Address, error) {
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		return common.Address{}, err
	}
	return common.BytesToAddress(b), nil
}
