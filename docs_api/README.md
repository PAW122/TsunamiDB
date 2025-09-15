# TsunamiDB HTTP API for Go Developers

This folder contains a public, copy‑pasteable guide for using the TsunamiDB HTTP API from Go. It does not require the Go library — just the standard `net/http` package (plus `gorilla/websocket` for subscriptions).

## Running TsunamiDB
From the repo root (or use the provided binaries):

```bash
# Windows
TsunamiDB.exe 5845

# Linux
./TsunamiDB-linux 5845
```

- Argument `5845` is the P2P port for the network manager.
- The process also starts:
  - HTTP Public API on `http://localhost:5844`
  - WebSocket Subscriptions on `ws://localhost:5845/sub`
- To connect multiple nodes, pass peer addresses as extra args: `TsunamiDB.exe 5845 10.0.0.2:5845 10.0.0.3:5845`.

## Base URL and Conventions
- Base HTTP URL: `http://localhost:5844`
- Most routes follow `/endpoint/<table>/<key>` and accept/return raw bytes unless noted.
- Error statuses: `405` wrong method, `400` bad input, `404` missing key, `500` server/storage error.

## Contents
- [Basic KV](./kv.md)
- [Encrypted](./encryption.md)
- [Incremental Tables](./incremental.md)
- [Subscriptions](./subscriptions.md)
- [Meta & Utilities (SQL, Regex)](./meta.md)

If you prefer the in‑process Go API, see `new_docs/lib/README.md`.
