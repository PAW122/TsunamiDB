# Encrypted (Go examples)

Endpoints:
- POST `/save_encrypted/<table>/<key>` — write encrypted
- GET `/read_encrypted/<table>/<key>` — read and decrypt

Headers: `encryption_key: <your passphrase>`

## Save Encrypted
```go
package main

import (
    "bytes"
    "fmt"
    "io"
    "net/http"
)

func SaveEncrypted(table, key, encKey string, data []byte) error {
    url := fmt.Sprintf("http://localhost:5844/save_encrypted/%s/%s", table, key)
    req, _ := http.NewRequest("POST", url, bytes.NewReader(data))
    req.Header.Set("Content-Type", "application/octet-stream")
    req.Header.Set("encryption_key", encKey)

    resp, err := http.DefaultClient.Do(req)
    if err != nil { return err }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        b, _ := io.ReadAll(resp.Body)
        return fmt.Errorf("save_encrypted failed: %s: %s", resp.Status, string(b))
    }
    return nil
}
```

## Read Encrypted
```go
func ReadEncrypted(table, key, encKey string) ([]byte, error) {
    url := fmt.Sprintf("http://localhost:5844/read_encrypted/%s/%s", table, key)
    req, _ := http.NewRequest("GET", url, nil)
    req.Header.Set("encryption_key", encKey)

    resp, err := http.DefaultClient.Do(req)
    if err != nil { return nil, err }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        b, _ := io.ReadAll(resp.Body)
        return nil, fmt.Errorf("read_encrypted failed: %s: %s", resp.Status, string(b))
    }
    return io.ReadAll(resp.Body)
}
```

## Demo
```go
func main() {
    encKey := "correct horse battery staple"
    if err := SaveEncrypted("secrets.tbl", "secrets:api", encKey, []byte("hunter2")); err != nil {
        panic(err)
    }
    plain, err := ReadEncrypted("secrets.tbl", "secrets:api", encKey)
    if err != nil { panic(err) }
    fmt.Println(string(plain))
}
```
