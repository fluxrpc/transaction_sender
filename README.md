# Transaction Sender

Simple golang implementation for sending serialized transactions to SVM (Solana/Fogo) leaders with accurate leader tracking. 

Transactions are sent via both QUIC & UDP to the leader processing N+1 slot via accurate leader tracking. 

For most use cases outside TXN spam, users should be able to utilize this service over a hosted RPC solution as long as their tx flow is < SWQOS threshold. 

## Supported Protocols
* Solana
* Fogo

## Features
 * [x] Leader tracking
 * [x] QUIC Support
 * [x] UDP Support

### Upcoming
* [ ] Jito detection
* [ ] Whitelist validator set
* [ ] Pre-connect latency


## Setup

### Environment
To run the transaction sender simply provide it via env or flags

```env
HTTP_PORT=8080
RPC_URL={RPC_ENDPOINT}
```

### Flags
* `http_port` - HTTP port to serve the endpoints on 
* `rpc_url` - RPC Url used to query for leader & slot detail


## Runtime

### Docker
* Deploy via the supported `Dockerfile`
* Connect to transaction sender via exposed port

### Source

```bash
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags='-s -w -extldflags "-static"' -o txn_worker ./runtime/main.go -o transaction_sender
```