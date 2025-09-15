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

## Notes
- `/sql` currently only supports `create_table` and writes JSON metadata files under `./db/sql_map`. It does not execute queries.
- Regexes are cached server‑side. If you run a large keyspace, prefer anchored/narrow patterns and set `max`.
