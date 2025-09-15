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
