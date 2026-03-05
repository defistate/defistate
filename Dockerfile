# --- Build Stage: Go Application ---
FROM golang:1.24-alpine AS builder
RUN apk add --no-cache git
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/dse ./cmd/dse

# --- Foundry Stage: use the official image (already has anvil) ---
FROM ghcr.io/foundry-rs/foundry:v1.5.1 AS foundry

# --- Final Stage: glibc base so anvil runs ---
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates \
  && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=builder /app/dse .
COPY --from=foundry /usr/local/bin/anvil /usr/local/bin/anvil

EXPOSE 8080 2112 6060

RUN anvil --version
CMD ["./dse", "-config", "config.yaml"]