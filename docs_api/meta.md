# Meta & Utilities (Go examples)

This page shows how to call the `/sql` prototype and key discovery endpoint.

Base URL: `http://localhost:5844`

## POST `/sql` — create table metadata
```go
package main

import (
    "bytes"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
)

type Column struct {
    Name         string `json:"name"`
    Type         string `json:"type"`
    IsPrimaryKey bool   `json:"isPrimaryKey"`
    Default      string `json:"default,omitempty"`
    MaxByteSize  int    `json:"maxByteSize"`
}

type SQLReq struct {
    Query     string   `json:"query"`
    TableName string   `json:"tableName"`
    Columns   []Column `json:"columns"`
}

func CreateTable(name string, cols []Column) error {
    payload, _ := json.Marshal(SQLReq{Query: "create_table", TableName: name, Columns: cols})
    resp, err := http.Post("http://localhost:5844/sql", "application/json", bytes.NewReader(payload))
    if err != nil { return err }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        b, _ := io.ReadAll(resp.Body)
        return fmt.Errorf("sql failed: %s: %s", resp.Status, string(b))
    }
    return nil
}
```

## GET `/key_by_regex?regex=...&max=...`
```go
func KeysByRegex(pattern string, max int) ([]string, error) {
    url := fmt.Sprintf("http://localhost:5844/key_by_regex?regex=%s&max=%d", url.QueryEscape(pattern), max)
    resp, err := http.Get(url)
    if err != nil { return nil, err }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        b, _ := io.ReadAll(resp.Body)
        return nil, fmt.Errorf("regex failed: %s: %s", resp.Status, string(b))
    }
    var out []string
    if err := json.NewDecoder(resp.Body).Decode(&out); err != nil { return nil, err }
    return out, nil
}
```

## GET `/health`
Returns an aggregated JSON report with uptime, rolling averages for HTTP requests and live subscription/network state.

```go
package main

import (
    "encoding/json"
    "fmt"
    "net/http"
)

type Health struct {
    Status    string `json:"status"`
    Timestamp string `json:"timestamp"`
    API       struct {
        UptimeSeconds     float64 `json:"uptime_seconds"`
        TotalRequests     uint64  `json:"total_requests"`
        AverageResponseMS float64 `json:"average_response_ms"`
        LastRequestAt     string  `json:"last_request_at"`
    } `json:"api"`
    Subscriptions struct {
        ActiveClients      int `json:"active_clients"`
        KeysWithSubscribers int `json:"keys_with_subscribers"`
        ActiveSubscriptions int `json:"active_subscriptions"`
        PendingAuthKeys    int `json:"pending_auth_keys"`
    } `json:"subscriptions"`
    Network struct {
        ServerIP         string   `json:"server_ip"`
        Port             int      `json:"port"`
        ConnectedPeers   int      `json:"connected_peers"`
        PeerAddresses    []string `json:"peer_addresses"`
        PendingResponses int      `json:"pending_responses"`
    } `json:"network"`
}

func FetchHealth() (*Health, error) {
    resp, err := http.Get("http://localhost:5844/health")
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("health failed: %s", resp.Status)
    }
    var out Health
    if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
        return nil, err
    }
    return &out, nil
}
```

- `api.average_response_ms` is calculated from every public HTTP request handled since process start (the value stays `0` until the first request is tracked).
- `subscriptions.active_clients` counts distinct WebSocket connections currently attached to `/sub`.
- `network.connected_peers` mirrors the network manager peer map; `pending_responses` shows how many request/response channels are still waiting for data.
- All timestamps use RFC3339 with nanosecond precision and are emitted in UTC.


## Notes
- `/sql` currently only supports `create_table` and writes JSON metadata files under `./db/sql_map`. It does not execute queries.
- Regexes are cached server‑side. If you run a large keyspace, prefer anchored/narrow patterns and set `max`.
