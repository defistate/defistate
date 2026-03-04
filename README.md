
# DeFiState

**DeFiState** is an open infrastructure project for producing **block-synchronized streams of DeFi protocol state on EVM chains**.

It aggregates data from multiple protocol indexers into a unified engine and exposes the resulting state through a **JSON-RPC stream** for simplified consumption by trading systems, analytics platforms, and other DeFi infrastructure.

Instead of building and maintaining multiple protocol indexers, consumers can subscribe to a single **block-synchronized DeFi state feed**.

---

# Overview

Modern DeFi infrastructure requires continuously tracking protocol state across multiple smart contracts and protocols.

DeFiState provides a framework for collecting, synchronizing, and streaming this data in a structured and easily consumable format.

At its core is the **DeFiState Engine**, which:

1. Initializes with a set of protocol providers
2. Polls each protocol for state changes on every block
3. Aggregates the data into a unified **State object**
4. Publishes the state to subscribers via a **JSON-RPC stream**

This enables downstream systems to consume a **consistent view of DeFi protocol state for each block**.

---

# Architecture

```
EVM Chain
   │
   │ new block
   ▼
Protocol Indexers
(Uniswap V2, V3, ERC20, etc.)
   │
   ▼
DeFiState Engine
   │
   │ builds block State
   ▼
JSON-RPC Stream
   │
   ▼
Clients / Consumers
```

The engine acts as the **coordination layer** between protocol indexers and external consumers.

---

# Core Components

## Engine

The **DeFiState Engine** orchestrates the system.

Responsibilities:

- block synchronization
- protocol polling
- state aggregation
- subscriber management

On every block:

1. protocols are polled for updates
2. a new **State** object is constructed
3. the state is broadcast to stream subscribers

---

## Protocol Indexers

Protocols provide domain-specific state information to the engine.

Currently supported protocols:

- **Uniswap V2**
- **Uniswap V3**
- **ERC20 Tokens**

Each protocol implementation reconstructs and updates protocol state for the engine.

---

## ERC20 Analysis

When new tokens are discovered, DeFiState analyzes them using a **Foundry Anvil fork**.

This allows the system to determine:

- token metadata
- transfer gas cost
- fee-on-transfer behavior

This analysis ensures that token behavior is correctly modeled when interacting with liquidity pools.

---

## Token–Pool Graph

DeFiState constructs a **graph representing relationships between tokens and pools**.

The graph enables:

- routing analysis
- path discovery
- arbitrage detection
- liquidity exploration

This structure is also streamed to consumers as part of the state.

---

# Ports

When the system starts, the following ports are available:

| Port | Description |
|-----|-------------|
| 8080 | DeFiState JSON-RPC stream |
| 2112 | Prometheus metrics |
| 6060 | pprof profiling |

---

# Configuration

DeFiState is configured using a YAML configuration file provided to the container at startup.

Example configuration:

```yaml
chain_id: 1 # EVM chain Id
chain: ethereum # chain name

# Configuration for the RPC client pool used by the systems
client_manager:
  rpc_urls:
    - ws://your-evm-rpc
  max_concurrent_requests: 32

# Core engine behavior settings
engine:
  max_wait_until_sync: 1s # Maximum time the engine waits for protocol synchronization per block

# Configuration for the Anvil fork used for simulations
fork:
  rpc_url: ws://your-evm-rpc
  token_overrides:
    "0xTokenAddress":
      gas: 65000
      fee_on_transfer_percentage: 2
```

---

# Running DeFiState

Start the full system using Docker:

```
docker compose up
```

The container will start the DeFiState system using the `config.yaml` file located in the repository root.
---

# Consuming the Stream

DeFiState provides client libraries for connecting to the websocket JSON-RPC stream and receiving block-synchronized protocol state.

To establish a connection, use the `DialJSONRPCStream` function from the appropriate client package located in the `clients/chain` directory.

```go
func DialJSONRPCStream(
    ctx context.Context,
    url string,
    logger *slog.Logger,
    prometheusRegistry prometheus.Registerer,
    opts ...Option,
) (*Client, error)
```

This function establishes the connection and starts the internal processing loop.  
The returned `Client` remains active until the provided context is cancelled.

---

## Accessing State Updates

The client exposes a channel that streams **block-synchronized State objects**.

```go
func (p *Client) State() <-chan *State {
    return p.stateCh
}
```

Consumers receive updates by reading from this channel.

---

## Example

```go
client, err := chain.DialJSONRPCStream(
    ctx,
    "ws://localhost:8080",
    logger,
    prometheusRegistry,
)
if err != nil {
    panic(err)
}

for state := range client.State() {
    // process block-synchronized DeFi state
}
```

Each `State` object contains the aggregated protocol data produced by the DeFiState engine for that block.

This allows downstream systems to build trading logic, analytics pipelines, or research tooling directly on top of the synchronized DeFi state stream.

---

# Supported Protocols

Currently supported:

- Uniswap V2 & forks
- Uniswap V3 & forks (PancakeSwap, Aerodrome, etc.)
- ERC20 tokens

Additional protocols can be added through the protocol interface.

---

# Use Cases

DeFiState can power:

- trading systems
- arbitrage engines
- routing algorithms
- liquidity analytics
- DeFi research infrastructure
- real-time DeFi data platforms

---

# Roadmap

The DeFiState project will continue evolving with a focus on expanding protocol coverage, improving indexers, and strengthening observability.

### Protocol Coverage

Expand support for additional DeFi protocols across EVM chains.

### Improved Indexers

Continue improving the performance and reliability of protocol indexers, focusing on faster synchronization and reduced RPC overhead and persisting indexed state.

### Monitoring & Observability

Enhance monitoring capabilities using the exposed Prometheus metrics to provide deeper visibility into performance and system health.

---

# License

This project is licensed under the **Apache 2.0 License**.
