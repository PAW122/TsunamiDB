# Incremental Tables (Go examples)

Endpoints:
- POST `/save_inc/<table>/<key>` - create metadata (if missing) and write an entry
- GET `/read_inc/<table>/<key>` - read entries by id/first/last
- GET `/delete_inc/<table>/<key>` - delete the incremental table file and free the KV metadata entry

Headers:
- Save: `max_entry_size` (required for the first write; optional afterwards), optional: `id`, `mode` (`append`|`overwrite`), `count_from` (`top`|`bottom`), `entry_key` (stable identifier for fast lookup)
- Read: `read_type` (`by_id`|`first_entries`|`last_entries`|`by_key`) plus `id`, `amount_to_read`, or `entry_key` depending on the mode

### Header reference

| Request | Header | Required | Values | Notes |
| --- | --- | --- | --- | --- |
| save | `max_entry_size` | first write | integer (bytes) | must match the size declared when the table was created |
| save | `entry_key` | optional | string | stable logical key; must be unique per table, otherwise request fails with 409 |
| save | `id` | optional | integer | together with `mode` controls overwrite/insert; omit to append sequentially |
| save | `mode` | optional | `append` (default) or `overwrite` | with `id` indicates whether to insert/overwrite |
| save | `count_from` | optional | `top` or `bottom` (default) | influences how `id` is resolved (`top` counts from newest) |
| read | `read_type` | yes | `by_id` (default), `last_entries`, `first_entries`, `by_key` | selects which companion headers to provide |
| read | `id` | when `read_type=by_id` | integer | zero-based index counted from oldest entry |
| read | `amount_to_read` | when `read_type` is `last_entries` or `first_entries` | integer | number of rows to fetch |
| read | `entry_key` | when `read_type=by_key` | string | must match value provided during save |

Base URL: `http://localhost:5844`

## Save: auto‑ID append
```go
package main

import (
    "bytes"
    "fmt"
    "io"
    "net/http"
)

type SaveIncResp struct {
    ID string `json:"id"`
}

func SaveIncAppend(table, key string, maxEntry uint64, payload []byte) (string, error) {
    url := fmt.Sprintf("http://localhost:5844/save_inc/%s/%s", table, key)
    req, _ := http.NewRequest("POST", url, bytes.NewReader(payload))
    req.Header.Set("Content-Type", "application/octet-stream")
    req.Header.Set("max_entry_size", fmt.Sprintf("%d", maxEntry))

    resp, err := http.DefaultClient.Do(req)
    if err != nil { return "", err }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        b, _ := io.ReadAll(resp.Body)
        return "", fmt.Errorf("save_inc failed: %s: %s", resp.Status, string(b))
    }
    var out SaveIncResp
    data, _ := io.ReadAll(resp.Body)
    // Response is a tiny JSON like {"id":"42"}
    _ = json.Unmarshal(data, &out)
    return out.ID, nil
}
```

## Save: insert/overwrite at position
Use `id` together with `mode` for deterministic placement. Set `mode=overwrite` to replace an existing record, or `mode=append` with a custom `id` to insert relative to the `count_from` side (`top` counts from the newest entries). You can still supply `entry_key` to update or insert specific logical rows.

```go
func SaveIncAt(table, key string, maxEntry uint64, payload []byte, id uint64, mode, countFrom string) (string, error) {
    url := fmt.Sprintf("http://localhost:5844/save_inc/%s/%s", table, key)
    req, _ := http.NewRequest("POST", url, bytes.NewReader(payload))
    req.Header.Set("Content-Type", "application/octet-stream")
    req.Header.Set("max_entry_size", fmt.Sprintf("%d", maxEntry))
    req.Header.Set("id", fmt.Sprintf("%d", id))
    req.Header.Set("mode", mode)           // "append" or "overwrite"
    req.Header.Set("count_from", countFrom) // "top" or "bottom"

    resp, err := http.DefaultClient.Do(req)
    if err != nil { return "", err }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        b, _ := io.ReadAll(resp.Body)
        return "", fmt.Errorf("save_inc failed: %s: %s", resp.Status, string(b))
    }
    var out SaveIncResp
    _ = json.NewDecoder(resp.Body).Decode(&out)
    return out.ID, nil
}
```

## Read: by id
```go
func ReadIncByID(table, key string, id uint64) (string, error) {
    url := fmt.Sprintf("http://localhost:5844/read_inc/%s/%s", table, key)
    req, _ := http.NewRequest("GET", url, nil)
    req.Header.Set("read_type", "by_id")
    req.Header.Set("id", fmt.Sprintf("%d", id))

    resp, err := http.DefaultClient.Do(req)
    if err != nil { return "", err }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        b, _ := io.ReadAll(resp.Body)
        return "", fmt.Errorf("read_inc by_id failed: %s: %s", resp.Status, string(b))
    }
    var body struct{ Data string `json:"data"` }
    _ = json.NewDecoder(resp.Body).Decode(&body)
    return body.Data, nil
}
```

## Read: by key
Missing or unknown keys respond with HTTP 404.

```go
func ReadIncByKey(table, key string, entryKey string) (string, error) {
    url := fmt.Sprintf("http://localhost:5844/read_inc/%s/%s", table, key)
    req, _ := http.NewRequest("GET", url, nil)
    req.Header.Set("read_type", "by_key")
    req.Header.Set("entry_key", entryKey)

    resp, err := http.DefaultClient.Do(req)
    if err != nil { return "", err }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        b, _ := io.ReadAll(resp.Body)
        return "", fmt.Errorf("read_inc by_key failed: %s: %s", resp.Status, string(b))
    }
    var body struct{ Data string `json:"data"` }
    _ = json.NewDecoder(resp.Body).Decode(&body)
    return body.Data, nil
}
```

entry_key stays bound to a logical row even if you insert new entries at arbitrary positions, so lookups remain O(1) across very large tables.

## Read: newest N (last_entries)
```go
type IncEntry struct {
    ID   uint64 `json:"id"`
    Data string `json:"data"`
}

func ReadIncLast(table, key string, n uint64) ([]IncEntry, error) {
    url := fmt.Sprintf("http://localhost:5844/read_inc/%s/%s", table, key)
    req, _ := http.NewRequest("GET", url, nil)
    req.Header.Set("read_type", "last_entries")
    req.Header.Set("amount_to_read", fmt.Sprintf("%d", n))

    resp, err := http.DefaultClient.Do(req)
    if err != nil { return nil, err }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        b, _ := io.ReadAll(resp.Body)
        return nil, fmt.Errorf("read_inc last_entries failed: %s: %s", resp.Status, string(b))
    }
    var out []IncEntry
    _ = json.NewDecoder(resp.Body).Decode(&out)
    return out, nil
}
```

## Read: oldest N (first_entries)
```go
func ReadIncFirst(table, key string, n uint64) ([]IncEntry, error) {
    url := fmt.Sprintf("http://localhost:5844/read_inc/%s/%s", table, key)
    req, _ := http.NewRequest("GET", url, nil)
    req.Header.Set("read_type", "first_entries")
    req.Header.Set("amount_to_read", fmt.Sprintf("%d", n))

    resp, err := http.DefaultClient.Do(req)
    if err != nil { return nil, err }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        b, _ := io.ReadAll(resp.Body)
        return nil, fmt.Errorf("read_inc first_entries failed: %s: %s", resp.Status, string(b))
    }
    var out []IncEntry
    _ = json.NewDecoder(resp.Body).Decode(&out)
    return out, nil
}
```

## Delete: cleanup table
```go
func DeleteInc(table, key string) error {
    url := fmt.Sprintf("http://localhost:5844/delete_inc/%s/%s", table, key)
    req, _ := http.NewRequest("GET", url, nil)

    resp, err := http.DefaultClient.Do(req)
    if err != nil { return err }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        body, _ := io.ReadAll(resp.Body)
        return fmt.Errorf("delete_inc failed: %s: %s", resp.Status, string(body))
    }
    return nil
}
```

## Subscriptions
If you enable the WebSocket subscription server, every successful `/save_inc` emits an event of the form `{"event":"inc_table_update","key":"<key>","data":{"type":"add|insert|overwrite","new_data":{"id":"<id>","data":"<payload>"}}}`. The `type` tracks whether the write appended a new entry, inserted at a position, or overwrote an existing one, and `new_data.id` matches the value returned by the HTTP endpoint.

## Notes
- `max_entry_size` is fixed per table. After the first write, future writes must use the same size.
- When you send `max_entry_size` for an existing table the server ignores mismatched values and returns the entry id together with a `warning` message in the JSON body.
- `GET /delete_inc/<table>/<key>` removes the backing inc-table file (resetting the worker state) and behaves like `/free` for the KV metadata; subscribers receive the usual `deleted` event for that key.
- Payloads are treated as strings in responses; for arbitrary binary, base64-encode before saving and decode after reading.
- Skipped entries (logical deletes) are filtered out by the readers.
- Entry-key collisions result in HTTP 409; use `read_type=by_key` to detect duplicates before writing, or handle the error response.
- Numeric `id` values are positional; inserts or overwrites can shift later rows, so treat `entry_key` as the stable lookup identifier.
