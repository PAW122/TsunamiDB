# Subscriptions (Go examples)

This feature has two parts with different audiences:
- Server-side (private): HTTP helpers to mint and revoke auth tokens.
- Client-side (public): a WebSocket endpoint that consumes an auth token.

## Server-side (private) HTTP endpoints
- POST `/subscriptions/enable` — returns a short-lived `auth_key` for selected keys
- POST `/subscriptions/disable` — unsubscribes and notifies existing sockets for a key

These should be called by your own backend. Then you pass the `auth_key` to your client (e.g., via your API), which uses it to join over WebSocket.

```go
package main

import (
    "bytes"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
)

type enableReq struct { Keys []string `json:"keys"` }
type enableRes struct { AuthKey string `json:"auth_key"` }

// Mint a one-time token for a set of keys (60s TTL)
func MintSubToken(keys []string) (string, error) {
    body, _ := json.Marshal(enableReq{Keys: keys})
    resp, err := http.Post("http://localhost:5844/subscriptions/enable", "application/json", bytes.NewReader(body))
    if err != nil { return "", err }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        b, _ := io.ReadAll(resp.Body)
        return "", fmt.Errorf("enable failed: %s: %s", resp.Status, string(b))
    }
    var out enableRes
    if err := json.NewDecoder(resp.Body).Decode(&out); err != nil { return "", err }
    return out.AuthKey, nil
}

// Force-unsubscribe a key for all current sockets
func DisableKey(key string) error {
    body, _ := json.Marshal(map[string]string{"key": key})
    resp, err := http.Post("http://localhost:5844/subscriptions/disable", "application/json", bytes.NewReader(body))
    if err != nil { return err }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        b, _ := io.ReadAll(resp.Body)
        return fmt.Errorf("disable failed: %s: %s", resp.Status, string(b))
    }
    return nil
}
```

Tip: If your code runs in-process with TsunamiDB, you can skip HTTP and call `lib/dbclient.EnableSubscription(keys)` and `DisableSubscription(key)` directly.

## Client-side (public) WebSocket
Public endpoint: `ws://localhost:5845/sub`

Clients connect here and must immediately send `{ "auth_key": "..." }` (the token obtained from your backend using the private endpoints above). On success, the server confirms and starts streaming events.

```go
package main

import (
    "encoding/json"
    "fmt"
    "time"

    "github.com/gorilla/websocket"
)

type event struct {
    Event string `json:"event"`
    Key   string `json:"key"`
    Data  string `json:"data"`
}

func connectWS(authKey string) (*websocket.Conn, error) {
    dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second}
    c, _, err := dialer.Dial("ws://localhost:5845/sub", nil)
    if err != nil { return nil, err }
    payload, _ := json.Marshal(map[string]string{"auth_key": authKey})
    _ = c.WriteMessage(websocket.TextMessage, payload)
    return c, nil
}

func main() {
    // Get authKey from YOUR backend API (not from TsunamiDB directly)
    authKey := "<received-from-your-server>"

    c, err := connectWS(authKey)
    if err != nil { panic(err) }
    defer c.Close()

    for {
        _, msg, err := c.ReadMessage()
        if err != nil { fmt.Println("ws closed:", err); return }
        var ev event
        if json.Unmarshal(msg, &ev) == nil {
            fmt.Printf("%s -> %s: %s\n", ev.Event, ev.Key, ev.Data)
        } else {
            fmt.Println(string(msg))
        }
    }
}
```

## Event types
- `{"event":"updated","key":"...","data":"..."}` - after `/save` or `/save_encrypted` (plaintext data)
- `{"event":"deleted","key":"..."}` - after `/free`
- `{"event":"inc_table_update","key":"...","data":{"type":"add|insert|overwrite","new_data":{"id":"...","data":"..."}}}` - after `/save_inc`; `type` reflects whether the write appended, inserted or overwrote an entry and `new_data.id` matches the logical entry id returned by the API
- `{"event":"unsubscribed","key":"..."}` - when a server disables a key via the private endpoint

## Notes
- Auth keys expire after ~60s if unused and are single-use.
- Do not expose `/subscriptions/enable` or `/subscriptions/disable` to the public internet. Use them from the server side only and distribute tokens via your own API.
- If you store secrets, consider not running the subscription server or stripping payloads from updates.




