# Solana Transaction Sender

Simple golang implementation for sending serialized transactions to Solana leaders with accurate leader tracking. 

Transactions are sent via both QUIC & UDP to the leader processing N+1 slot via accurate leader tracking. 

For most use cases outside of TXN spam, users should be able to utilize this service over a hosted RPC solution as long as their tx flow is < SWQOS threshold. 

Features
 * [x] Leader tracking
 * [x] QUIC Support
 * [x] UDP Support

Upcoming
* [ ] Jito detection
* [ ] Whitelist validator set
* [ ] Pre-connect latency

### Environment
To run the transaction sender simply provide it via env or flags

```env
HTTP_PORT=8080
RPC_URL={RPC_ENDPOINT}
```

### Flags
* `http_port` - HTTP port to serve the endpoints on 
* `rpc_url` - RPC Url used to query for leader & slot detail