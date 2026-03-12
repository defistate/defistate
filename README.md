# DeFiState

**DeFiState** is an open infrastructure project for producing **block-synchronized streams of DeFi protocol state across EVM chains**.

The system aggregates data from protocol indexers and exposes the resulting state through a **WebSocket JSON-RPC stream**, enabling applications to consume real-time DeFi state on every block using a simple subscription interface.

DeFiState is designed for developers who need a continuously updated, structured view of onchain protocol state without building their own end-to-end indexing pipeline.

Developers can self-host the open source system. DeFiState will also offer a managed deployment for teams that prefer not to operate the infrastructure themselves.

---

# Architecture

```
EVM Chain
   │
   │ new block
   ▼
Protocol Indexers
(Uniswap V2, Uniswap V3, ERC20, Pool Registry, etc.)
   │
   ▼
DeFiState Engine
   │
   │ aggregate protocol state
   ▼
WebSocket JSON-RPC Stream
   │
   ▼
Chain Clients
   │
   │ process raw state into indexed chain-specific views
   ▼
client.OnNewBlock(handler)

```

---

# Core Components

## DeFiState Engine

The **DeFiState Engine** orchestrates block-synchronized DeFi state production.

Responsibilities:

- block synchronization
- protocol state aggregation
- construction of a unified per-block state object
- broadcasting state updates to stream consumers

On every block:

1. protocol systems synchronize to the latest block
2. a new `State` object is constructed
3. the state is streamed to connected consumers

---

## Protocol Indexers

DeFiState currently includes indexers for:

- **Uniswap V2**
- **Uniswap V3**
- **ERC20 Tokens**
- **Pool Registry**
- **Token–Pool Graph**


---

## ERC20 Analysis

When new tokens are discovered, DeFiState analyzes them using a **Foundry Anvil fork**.

This allows the system to determine token characteristics such as:

- transfer gas cost
- fee-on-transfer behavior

---

## Token–Pool Graph

DeFiState constructs a graph of token-to-pool relationships.

This graph is included in the state stream and enables downstream applications to efficiently reason about liquidity connectivity across indexed protocols.

---

# Ports

When the system starts, the following ports are exposed:

| Port | Description |
|------|-------------|
| 8080 | DeFiState WebSocket JSON-RPC stream |
| 2112 | Prometheus metrics |
| 6060 | pprof profiling |

---

# Configuration

DeFiState is configured using a YAML file provided to the container at startup.

Example:

```yaml
chain_id: 1 # EVM chain ID
chain: ethereum # chain name

# RPC client pool used by the system
client_manager:
  rpc_urls:
    - ws://your-evm-rpc-1
  max_concurrent_requests: 32

# Core engine behavior settings
engine:
  max_wait_until_sync: 1s # maximum time the engine waits for protocol synchronization per block

# Foundry Anvil fork configuration used for token analysis
fork:
  rpc_url: ws://your-evm-rpc
  token_overrides: # optional
    "0xTokenAddress":
      gas: 65000
      fee_on_transfer_percentage: 2
```

---

# Running DeFiState

Start the full system with Docker:

```bash
docker compose up --build
```

The container starts the DeFiState system using a `config.yaml` file located in the repository root.

---

# Consuming the Stream

DeFiState provides chain-specific client packages for connecting to the WebSocket JSON-RPC stream and consuming synchronized DeFi state.

To establish a connection, use the `DialJSONRPCStream` function from the appropriate package in `clients/chains`.

```go
func DialJSONRPCStream(
    ctx context.Context,
    url string,
    logger *slog.Logger,
    prometheusRegistry prometheus.Registerer,
    opts ...Option,
) (*Client, error)
```

This function:

- establishes the JSON-RPC stream connection
- initializes state decoding and patching
- starts the internal background processing loop
- returns a chain-specific client instance

The returned client remains active until the provided context is cancelled.

---

## Accessing State Updates

Use `OnNewBlock` to register a handler that is executed whenever a new block is fully processed.

```go
func (p *Client) OnNewBlock(handler func(context.Context, *State) error) error
```

The handler receives a chain-specific, indexed `State` snapshot for that block.

Each client also exposes:

```go
func (p *Client) State() *State
```

This returns the latest processed state snapshot stored by the client.

---

## Example

```go
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/defistate/defistate/clients/chains/ethereum"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	registry := prometheus.NewRegistry()

	client, err := ethereum.DialJSONRPCStream(
		ctx,
		"ws://localhost:8080",
		logger,
		registry,
	)
	if err != nil {
		panic(err)
	}

	err = client.OnNewBlock(func(ctx context.Context, state *ethereum.State) error {
		logger.Info("new block processed",
			"number", state.Block.Number,
			"timestamp", state.Timestamp,
		)

		// Access and utilize indexed protocol state
		v2Pools := state.UniswapV2.All()
		v3Pools := state.UniswapV3.All()
		graph := state.Graph

		_ = v2Pools
		_ = v3Pools
		_ = graph



		return nil
	})
	if err != nil {
		panic(err)
	}

	<-ctx.Done()
}
```

---

## Chain Client State

Chain-specific clients transform engine state into higher-level indexed views.

For example, the Ethereum client exposes a state structure shaped like:

```go
type State struct {
    Tokens           clients.IndexedTokenSystem
    Pools            clients.IndexedPoolRegistry
    UniswapV2        clients.IndexedUniswapV2
    UniswapV3        clients.IndexedUniswapV3
    PancakeswapV3    clients.IndexedUniswapV3
    ProtocolResolver *clients.ProtocolResolver
    Graph            *poolregistry.TokenPoolsRegistryView
    Block            engine.BlockSummary
    Timestamp        uint64
}
```

This gives downstream applications direct access to indexed protocol-specific views, token and pool registries, graph structures, and block metadata.

---

# Supported Chains

Current chain support includes:

- Ethereum
- Base
- Arbitrum
- BSC (BNB Smart Chain)
- Celo
- Katana
- PulseChain

---

# Roadmap

DeFiState will continue evolving with a focus on broader protocol support, stronger indexing performance, and better system observability.

## Protocol Coverage

Expand support for additional DeFi protocols across EVM chains.

## Improved Indexers

Continue improving the performance and reliability of protocol indexers, with emphasis on:

- faster synchronization
- reduced RPC overhead
- more efficient state handling
- persisted indexed state

## Monitoring and Observability

Expand Prometheus metrics and profiling support to provide deeper visibility into:

- synchronization health
- indexing latency
- stream performance
- client processing behavior

---

# License

This project is licensed under the **Apache 2.0 License**.
