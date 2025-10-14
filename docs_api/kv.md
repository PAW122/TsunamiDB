# Basic Key-Value (Go examples)

Endpoints:
- POST `/save/<table>/<key>` — write bytes
- GET `/read/<table>/<key>` — read bytes
- GET `/free/<table>/<key>` — delete

Base URL used below: `http://localhost:5844`.

## Save (POST /save)
```go
package main

import (
    "bytes"
    "fmt"
    "io"
    "net/http"
)

func Save(table, key string, data []byte) error {
    url := fmt.Sprintf("http://localhost:5844/save/%s/%s", table, key)
    resp, err := http.Post(url, "application/octet-stream", bytes.NewReader(data))
    if err != nil { return err }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        b, _ := io.ReadAll(resp.Body)
        return fmt.Errorf("save failed: %s: %s", resp.Status, string(b))
    }
    return nil
}
```


## Saving Nested JSON (POST /save with headers)
When you want TsunamiDB to split large JSON payloads into nested chunks, send the body as JSON and add the header `Mode: save_nested_json`.

```go
package main

import (
    "bytes"
    "fmt"
    "io"
    "net/http"
)

func SaveNested(table, key string, body []byte) error {
    url := fmt.Sprintf("http://localhost:5844/save/%s/%s", table, key)
    req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
    if err != nil { return err }
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("Mode", "save_nested_json")
    resp, err := http.DefaultClient.Do(req)
    if err != nil { return err }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        b, _ := io.ReadAll(resp.Body)
        return fmt.Errorf("save failed: %s: %s", resp.Status, string(b))
    }
    return nil
}
```

### Payload Syntax
- Any field whose value starts with `@` is treated as a nested JSON value.
- The substring after `@` should contain valid JSON (stringified) or raw text.
- TsunamiDB stores the nested content in `<table>/nested_values` and replaces the field with an internal pointer (`@ptr:<id>`).

Example payload:
```json
{
  "profile": {
    "name": "edgar",
    "bio": "@{\"links\":[\"https://example.com\",\"https://github.com\"]}"
  },
  "logs": "@[{\"ts\":1730000000,\"event\":\"LOGIN\"}]"
}
```

### What is stored
The top-level record keeps any non-`@` values inline. Each nested value becomes its own record in `nested_values` with a stable pointer id referenced from the parent JSON.

## Reading Nested JSON (GET /read)
By default TsunamiDB leaves nested pointers unresolved and returns `*` placeholders. Use optional headers to pull nested data on demand:

- No headers: you receive the base document with `*` in place of every nested pointer.
- `Read-Nested: field.path other.path`: expand only the listed nested paths, leaving the rest as `*`.
- `Read-Only-Nested: field.path other.path`: return a JSON object composed solely of the requested nested values.

Paths use dot notation relative to the response JSON. You can also pass a JSON array in the header value if you prefer.

```go
package main

import (
    "fmt"
    "io"
    "net/http"
    "strings"
)

func ReadNested(table, key string, readHeader, onlyHeader []string) ([]byte, error) {
    url := fmt.Sprintf("http://localhost:5844/read/%s/%s", table, key)
    req, err := http.NewRequest(http.MethodGet, url, nil)
    if err != nil { return nil, err }
    if len(readHeader) > 0 {
        req.Header.Set("Read-Nested", strings.Join(readHeader, " "))
    }
    if len(onlyHeader) > 0 {
        req.Header.Set("Read-Only-Nested", strings.Join(onlyHeader, " "))
    }
    resp, err := http.DefaultClient.Do(req)
    if err != nil { return nil, err }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        b, _ := io.ReadAll(resp.Body)
        return nil, fmt.Errorf("read failed: %s: %s", resp.Status, string(b))
    }
    return io.ReadAll(resp.Body)
}
```

### Interpreting pointers
- `"*"` indicates a field that points to nested data which was not fetched.
- Values like `"@ptr:abc123"` are raw pointers (e.g. when using `Read-Only-Nested`). You can pass the same paths again to resolve them later.

## Deleting Nested Records (GET /free)
`/free/<table>/<key>` removes the primary record and automatically deletes any nested chunks linked from that record.


## Read (GET /read)
```go
func Read(table, key string) ([]byte, error) {
    url := fmt.Sprintf("http://localhost:5844/read/%s/%s", table, key)
    resp, err := http.Get(url)
    if err != nil { return nil, err }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        b, _ := io.ReadAll(resp.Body)
        return nil, fmt.Errorf("read failed: %s: %s", resp.Status, string(b))
    }
    return io.ReadAll(resp.Body)
}
```

## Free (GET /free)
```go
func Free(table, key string) error {
    url := fmt.Sprintf("http://localhost:5844/free/%s/%s", table, key)
    resp, err := http.Get(url)
    if err != nil { return err }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        b, _ := io.ReadAll(resp.Body)
        return fmt.Errorf("free failed: %s: %s", resp.Status, string(b))
    }
    return nil
}
```

## Quick demo
```go
func main() {
    if err := Save("users.tbl", "users:jane", []byte("hello")); err != nil {
        panic(err)
    }
    b, err := Read("users.tbl", "users:jane")
    if err != nil { panic(err) }
    fmt.Println(string(b))
    _ = Free("users.tbl", "users:jane")
}
```
