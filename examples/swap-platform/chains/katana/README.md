# Katana Swap Platform Example

A minimal swap application built on top of DeFiState for Katana.

This example shows how to:

-   Subscribe to DeFiState Katana stream
-   Expose a simple backend API for tokens, quotes, and swaps
-   Serve a small frontend that connects a wallet and executes swaps

------------------------------------------------------------------------

## What it provides

The server exposes three endpoints:

**GET /tokens**\
Returns the list of available tokens.

**GET /quote**\
Returns the quoted output amount for a given input.

**GET /swap**\
Returns the transaction plan required to execute the swap.

The frontend uses these endpoints to:

-   Load tokens
-   Request live quotes
-   Connect a wallet
-   Execute swap transactions

------------------------------------------------------------------------

## Requirements

-   Go installed
-   Access to:
    -   A Katana stream endpoint
    -   A Katana RPC endpoint

------------------------------------------------------------------------

## Setup

Clone the repository:

``` bash
git clone https://github.com/defistate/defistate
```

Move into the Katana swap example:

``` bash
cd examples/swap-platform/chains/katana/server
```

Create a `.env` file in the `server` directory:

``` env
KATANA_STREAM_URL=your_katana_stream_url
KATANA_RPC_URL=your_katana_rpc_url
PORT=:8080
```

------------------------------------------------------------------------

## Run

Start the application with:

``` bash
go run .
```

Once the server starts, open:

    http://localhost:8080

------------------------------------------------------------------------

## API

### GET /tokens

Returns the tokens available to the platform.

Example:

    /tokens

------------------------------------------------------------------------

### GET /quote

Query params:

-   `tokenIn`
-   `tokenOut`
-   `amountIn`

Example:

    /quote?tokenIn=0x...&tokenOut=0x...&amountIn=1000000000000000000

Response:

``` json
{
  "amount_out": "123456789"
}
```

------------------------------------------------------------------------

### GET /swap

Query params:

-   `user`
-   `receiver`
-   `tokenIn`
-   `tokenOut`
-   `amountIn`

Example:

    /swap?user=0x...&receiver=0x...&tokenIn=0x...&tokenOut=0x...&amountIn=1000000000000000000

Returns the transaction plan required to perform the swap.

------------------------------------------------------------------------

## How it works

1.  The app connects to the Katana state stream.
2.  DeFiState updates the platform state on every new block.
3.  The backend exposes tokens, quoting, and swap planning over HTTP.
4.  The frontend calls these endpoints and sends the returned
    transactions through the user's wallet.

------------------------------------------------------------------------

## Notes

-   Quotes and amounts are returned as strings to preserve big integer
    precision.
-   The frontend is served directly by the Go server from the
    `../interface` directory.
-   This example demonstrates how simple it is to build a swap interface
    on top of DeFiState.
