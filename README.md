# transaction_sender

[![Go Reference](https://pkg.go.dev/badge/github.com/fluxrpc/transaction_sender.svg)](https://pkg.go.dev/github.com/fluxrpc/transaction_sender)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

A direct-to-leader transaction sender for Solana-compatible SVM chains. It tracks current and upcoming leaders, preconnects to validator TPU endpoints, and sends serialized transactions over UDP and QUIC.

Development is sponsored and maintained by **[FluxRPC](https://fluxrpc.com)** — Solana & Fogo RPC infrastructure.

- RPC requests, response types, and WebSocket subscriptions are powered by [fluxrpc/solana-go](https://github.com/fluxrpc/solana-go).
- Current and next-epoch leader schedules are loaded from the configured RPC endpoint.
- Upcoming QUIC connections are warmed before a leader rotation.
- Transactions are sent directly to the validator scheduled for the next slot.

The sender performs no RPC preflight or confirmation. Direct TPU delivery is intended for latency-sensitive callers that already simulate, retry, and confirm their transactions. Delivery through an unstaked connection remains subject to validator SWQoS policy.

## Install

```bash
go get github.com/fluxrpc/transaction_sender
go get github.com/fluxrpc/solana-go
```

## Quickstart

```go
import (
	"context"
	"time"

	solana "github.com/fluxrpc/solana-go"
	sender "github.com/fluxrpc/transaction_sender"
)

func send(ctx context.Context, tx *solana.Transaction) error {
	client, err := sender.NewTransactionSender(
		"https://your-rpc-endpoint",
		"wss://your-rpc-endpoint",
	)
	if err != nil {
		return err
	}

	raw, err := tx.MarshalBinary()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return client.Send(ctx, raw)
}
```

## HTTP service

The included runtime accepts a serialized transaction in the body of `POST /`.

| Environment | Flag | Purpose | Default |
|---|---|---|---|
| `RPC_URL` | `-rpc_url` | JSON-RPC endpoint for epoch, cluster, and schedule data | required |
| `WS_URL` | `-ws_url` | WebSocket endpoint for processed slot notifications | required |
| `HTTP_PORT` | `-http_port` | HTTP listen port | `8080` |
| — | `-debug` | Enable debug logging | `false` |

Run from source:

```bash
go run ./runtime -rpc_url https://your-rpc -ws_url wss://your-rpc
curl --fail --data-binary @transaction.bin http://127.0.0.1:8080/
```

Build a static binary:

```bash
CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o transaction-sender ./runtime
```

Or build the included container:

```bash
docker build -t transaction-sender .
docker run --rm -p 8080:8080 \
  -e RPC_URL=https://your-rpc \
  -e WS_URL=wss://your-rpc \
  transaction-sender
```

## Supported chains

- Solana
- Fogo

Chains with different slot timing can use the same schedule-driven sender; transaction size and validator transport support remain chain-specific.

## Development

```bash
go test ./...
go test -race ./...
go vet ./...
```
