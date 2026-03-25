package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"math/big"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/defistate/defistate/clients/chains/celo"
	"github.com/defistate/defistate/examples/celo-flow/server/app"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/joho/godotenv"
	"github.com/prometheus/client_golang/prometheus"
)

func main() {
	// 1. Setup Signal Context
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	// 2. Load .env file
	if err := godotenv.Load(); err != nil {
		// We log a warning instead of a fatal error because in some
		// environments (like Docker/K8s), vars are passed directly without a .env file.
		logger.Warn("no .env file found, falling back to system environment variables")
	}

	// 3. Fetch URL from Environment
	celoStreamURL := os.Getenv("CELO_STREAM_URL")
	if celoStreamURL == "" {
		logger.Error("celo_URL is not set in environment")
		os.Exit(1)
	}

	celoRPCURL := os.Getenv("CELO_RPC_URL")
	if celoRPCURL == "" {
		logger.Error("CELO_RPC_URL is not set in environment")
		os.Exit(1)
	}

	dialCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ethClient, err := ethclient.DialContext(dialCtx, celoRPCURL)
	if err != nil {
		logger.Error("failed to ethClient client", "error", err)
		return
	}

	platform := app.NewPlatform(
		ctx,
		big.NewInt(10_000_000), // 10 USDT (6 decimals)
		common.HexToAddress("0x48065fbbe25f71c9282ddf5e1cd6d6a887483d5e"),
		common.HexToAddress("0x5dA532AAb9d9a3249beaF37E33aA12E11534e529"),
		ethClient,
		big.NewInt(10_000_000_000), // 10k USDT
		1000,
	)

	client, err := celo.DialJSONRPCStream(
		ctx,
		celoStreamURL,
		logger,
		prometheus.DefaultRegisterer,
	)
	if err != nil {
		logger.Error("failed to init client", "error", err)
		return
	}

	client.OnNewBlock(func(ctx context.Context, s *celo.State) error {
		return platform.SetState(s)
	})

	err = platform.WaitForPrices(
		ctx,
		2*time.Minute,
	)
	if err != nil {
		logger.Error("failed to load prices", "error", err)
		return
	}

	// --- HTTP SERVER SETUP ---
	mux := http.NewServeMux()
	// serve our platform frontend
	mux.Handle("/", http.FileServer(http.Dir("../interface")))
	// register handlers
	mux.HandleFunc("GET /tokens", handleGetTokens(platform, logger))
	mux.HandleFunc("GET /quote", handleQuote(platform, logger))
	mux.HandleFunc("GET /swap", handleSwap(platform, logger))
	mux.HandleFunc("GET /prices", handlePrices(platform, logger))
	serverPort := os.Getenv("PORT")

	server := &http.Server{
		Addr:    serverPort,
		Handler: mux,
	}

	go func() {
		logger.Info("starting http server", "addr", server.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("listen error", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	logger.Info("shutting down gracefully...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("forced shutdown", "error", err)
	}

	logger.Info("server exited")
}

// --- HANDLER FUNCTIONS ---

func handleGetTokens(p *app.Platform, l *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tokens, err := p.Tokens()
		if err != nil {
			l.Warn("failed to fetch tokens", "error", err)
			http.Error(w, "State unavailable", http.StatusServiceUnavailable)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tokens)
	}
}

func handleQuote(p *app.Platform, l *slog.Logger) http.HandlerFunc {
	type response struct {
		AmountOut string `json:"amount_out"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		// 1. Extract query parameters with explicit naming
		tokenInStr := r.URL.Query().Get("tokenIn")
		tokenOutStr := r.URL.Query().Get("tokenOut")
		amountInStr := r.URL.Query().Get("amountIn")

		// 2. Parse and Validate
		if !common.IsHexAddress(tokenInStr) || !common.IsHexAddress(tokenOutStr) {
			l.Debug("invalid hex address provided", "tokenIn", tokenInStr, "tokenOut", tokenOutStr)
			http.Error(w, "invalid token address", http.StatusBadRequest)
			return
		}

		amountIn := new(big.Int)
		if _, ok := amountIn.SetString(amountInStr, 10); !ok {
			l.Debug("invalid amount provided", "amountIn", amountInStr)
			http.Error(w, "invalid amount", http.StatusBadRequest)
			return
		}

		tokenIn := common.HexToAddress(tokenInStr)
		tokenOut := common.HexToAddress(tokenOutStr)

		// 3. Execute Quote via Platform
		amountOut, err := p.Quote(tokenIn, tokenOut, amountIn)
		if err != nil {
			l.Warn("quote execution failed",
				"error", err,
				"tokenIn", tokenIn.Hex(),
				"tokenOut", tokenOut.Hex(),
			)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// 4. Respond with JSON string to preserve big.Int precision
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response{
			AmountOut: amountOut.String(),
		})
	}
}

func handleSwap(p *app.Platform, l *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 1. Extract query parameters
		userStr := r.URL.Query().Get("user")
		receiverStr := r.URL.Query().Get("receiver")
		tokenInStr := r.URL.Query().Get("tokenIn")
		tokenOutStr := r.URL.Query().Get("tokenOut")
		amountInStr := r.URL.Query().Get("amountIn")
		slippageStr := r.URL.Query().Get("slippageBps")
		if slippageStr == "" {
			slippageStr = "50" // default .5%
		}

		slippageBps, err := strconv.ParseUint(slippageStr, 10, 64)
		if err != nil {
			http.Error(w, "invalid slippageBps value", http.StatusBadRequest)
			return
		}

		// 2. Validate Hex Addresses
		if !common.IsHexAddress(userStr) || !common.IsHexAddress(receiverStr) ||
			!common.IsHexAddress(tokenInStr) || !common.IsHexAddress(tokenOutStr) {
			http.Error(w, "invalid hex address provided", http.StatusBadRequest)
			return
		}

		// 3. Parse big.Int
		amountIn := new(big.Int)
		if _, ok := amountIn.SetString(amountInStr, 10); !ok {
			http.Error(w, "invalid amountIn value", http.StatusBadRequest)
			return
		}

		// 4. Execute the Swap logic
		txs, err := p.Swap(
			common.HexToAddress(userStr),
			common.HexToAddress(receiverStr),
			common.HexToAddress(tokenInStr),
			common.HexToAddress(tokenOutStr),
			amountIn,
			slippageBps,
		)

		if err != nil {
			l.Error("swap generation failed", "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// 5. Return the transactions as JSON
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(txs); err != nil {
			l.Error("failed to encode txs", "error", err)
		}
	}
}

func handlePrices(p *app.Platform, l *slog.Logger) http.HandlerFunc {
	type priceResponse struct {
		QuoteToken string             `json:"quote_token"`
		Prices     map[string]float64 `json:"prices"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		prices, quoteToken, err := p.Prices()
		if err != nil {
			l.Warn("failed to fetch prices", "error", err)
			http.Error(w, "prices unavailable", http.StatusServiceUnavailable)
			return
		}

		out := make(map[string]float64, len(prices))
		for token, price := range prices {
			out[token.Hex()] = price
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(priceResponse{
			QuoteToken: quoteToken.Hex(),
			Prices:     out,
		}); err != nil {
			l.Error("failed to encode prices response", "error", err)
		}
	}
}
