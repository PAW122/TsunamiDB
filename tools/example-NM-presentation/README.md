# Example Network-Manager Presentation

Quick local demo for `tools/network-topology.html`.

## What it does

- starts 2 isolated TsunamiDB nodes in separate working directories,
- connects them through `network-manager`,
- seeds one KV table on each node,
- keeps updating both tables and performs cross-node reads to generate visible P2P traffic.

## Start

```powershell
powershell -ExecutionPolicy Bypass -File .\tools\example-NM-presentation\start-demo.ps1
```

The script uses:

- node A: `http://127.0.0.1:6044`, `ws://127.0.0.1:6045/sub`, network manager `127.0.0.1:6055`
- node B: `http://127.0.0.1:6144`, `ws://127.0.0.1:6145/sub`, network manager `127.0.0.1:6155`

Node A starts first as the listener. Node B then joins node A.

By default it also opens `tools/network-topology.html` with both `/health` endpoints preloaded.

## Stop

```powershell
powershell -ExecutionPolicy Bypass -File .\tools\example-NM-presentation\stop-demo.ps1
```
