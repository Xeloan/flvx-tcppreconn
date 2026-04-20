# 069 - TCP Pre-connection Relay Integration

Integrate the standalone TCP pre-connection relay (from Xeloan/TCP-preconnection-relay) into FLVX as a per-forward toggle.

## Architecture

- `go-gost/tcp-preconn/tcp_pool.c` — original C source (unchanged)
- CI compiles it for amd64/arm64 alongside the Go agent binary
- Backend stores `tcp_preconn` flag per Forward row
- Backend propagates flag to agent via `metadata.tcpPreconn` in service config
- Agent's preconn manager spawns/stops `tcp_pool` child processes per forward
- Frontend shows a Switch toggle in the forward create/edit form

## Checklist

- [x] Add `go-gost/tcp-preconn/tcp_pool.c`
- [x] Backend: add `TCPPreconn` column to Forward model
- [x] Backend: update `CreateForwardTx` / `UpdateForward`
- [x] Backend: update `forwardCreate` / `forwardUpdate` handlers
- [x] Backend: update `ListForwards` to return `tcpPreconn`
- [x] Backend: pass `tcpPreconn` in service configs to nodes
- [x] Agent: add `preconn_manager.go` for tcp_pool lifecycle
- [x] Agent: integrate preconn manager into service create/update/delete
- [x] Frontend: add `tcpPreconn` to types
- [x] Frontend: add Switch in forward form
- [x] Frontend: show badge in forward table
- [x] CI: compile tcp_pool.c in build workflow
- [x] install.sh: download tcp_pool binary
- [x] Build & test
