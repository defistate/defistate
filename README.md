
# DeFiState

**DeFiState** is an open infrastructure project for producing **block-synchronized streams of DeFi protocol state on EVM chains**. The system aggregates data from protocol indexers and exposes the resulting state through a **JSON-RPC interface**, enabling applications to consume real-time DeFi state using a simple subscription.

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
   │ aggregate data
   ▼
WebSocket JSON-RPC Stream
   │
   ▼
Clients 
```

---

# Core Components

## Engine

The **DeFiState Engine** orchestrates the system.

Responsibilities:

- block synchronization
- state aggregation

On every block:

1. a new **State** object is constructed using aggregated data
2. the state is broadcast to subscribers

---

## Protocol Indexers

Currently supported protocol indexers:

- **Uniswap V2**
- **Uniswap V3**
- **ERC20 Tokens**

---

## ERC20 Analysis

When new tokens are discovered, DeFiState analyzes them using a **Foundry Anvil fork**.

This allows the system to determine:
- token transfer gas cost
- token transfer fees

---

## Token–Pool Graph

DeFiState constructs a **graph representing relationships between tokens and pools**.

This structure is also streamed to consumers as part of the state.

---

# Ports

When the system starts, the following ports are available:

| Port | Description |
|-----|-------------|
| 8080 | DeFiState WebSocket JSON-RPC stream |
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
    - ws://your-evm-rpc-1
  max_concurrent_requests: 32

# Core engine behavior settings
engine:
  max_wait_until_sync: 1s # Maximum time the engine waits for protocol synchronization per block

# Configuration for the Anvil fork used for simulations
fork:
  rpc_url: ws://your-evm-rpc
  token_overrides: # optional
    "0xTokenAddress":
      gas: 65000
      fee_on_transfer_percentage: 2
```

---

# Running DeFiState

Start the full system using Docker:

```
docker compose build
docker compose up
```

The container will start the DeFiState system using a `config.yml` file located in the repository root.

---

# Consuming the Stream

DeFiState provides client libraries for connecting to the WebSocket JSON-RPC stream and receiving block-synchronized protocol state.

To establish a connection, use the `DialJSONRPCStream` function from the appropriate client package located in the `clients/chains` directory.

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

---


# Roadmap

The DeFiState project will continue evolving with a focus on expanding protocol coverage, improving indexers, and strengthening observability.

### Protocol Coverage

Expand support for additional DeFi protocols across EVM chains.

### Improved Indexers

Continue improving the performance and reliability of protocol indexers, focusing on faster synchronization, reduced RPC overhead and persisting indexed state.

### Monitoring & Observability

Enhance monitoring capabilities using the exposed Prometheus metrics to provide deeper visibility into performance and system health.

---

# License

This project is licensed under the **Apache 2.0 License**.
