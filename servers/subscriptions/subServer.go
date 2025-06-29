package subscriptions

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

type Subscription struct {
	Key       string
	AuthKey   string
	ExpiresAt time.Time
	Connected bool
	WSConn    *websocket.Conn
}

//todo: test wydajności , do reszty api i lib i do prod

var (
	activeSubs = map[string][]*Subscription{} // key -> list of subscriptions
	mu         sync.Mutex
)

type Pending struct {
	Keys      []string
	ExpiresAt time.Time
}

var pendingAuthKeys = map[string]*Pending{}
var connToKeys = make(map[*websocket.Conn]map[string]bool)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func HandleEnableSubscription(w http.ResponseWriter, r *http.Request, c *http.Client) {
	var req struct {
		Keys []string `json:"keys"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Keys) == 0 {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	authKey := uuid.NewString()
	mu.Lock()
	pendingAuthKeys[authKey] = &Pending{
		Keys:      req.Keys,
		ExpiresAt: time.Now().Add(60 * time.Second),
	}
	mu.Unlock()

	// kasowanie po TTL
	go func(k string) {
		time.Sleep(60 * time.Second)
		mu.Lock()
		if p, ok := pendingAuthKeys[k]; ok && time.Now().After(p.ExpiresAt) {
			delete(pendingAuthKeys, k)
		}
		mu.Unlock()
	}(authKey)

	json.NewEncoder(w).Encode(map[string]string{"auth_key": authKey})
}

func HandleDisableSubscription(w http.ResponseWriter, r *http.Request, c *http.Client) {
	var req struct {
		Key string `json:"key"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	mu.Lock()
	subs, ok := activeSubs[req.Key]
	if !ok {
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
		return
	}

	// Wyślij powiadomienie i usuń WSConn z mapy connToKeys
	for _, sub := range subs {
		if sub.WSConn != nil {
			_ = sub.WSConn.WriteJSON(map[string]string{
				"event": "unsubscribed",
				"key":   req.Key,
			})
			// usuń z odwrotnej mapy
			if m, exists := connToKeys[sub.WSConn]; exists {
				delete(m, req.Key)
				if len(m) == 0 {
					delete(connToKeys, sub.WSConn)
				}
			}
		}
	}
	// Usuń z głównej mapy
	delete(activeSubs, req.Key)
	mu.Unlock()

	w.WriteHeader(http.StatusOK)
}

func HandleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Upgrade error:", err)
		return
	}
	defer conn.Close()

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			log.Println("Disconnected:", err)
			break
		}

		var req struct {
			AuthKey string `json:"auth_key"`
		}
		if err := json.Unmarshal(msg, &req); err != nil {
			log.Println("Invalid message:", string(msg))
			continue
		}

		mu.Lock()
		pend, ok := pendingAuthKeys[req.AuthKey]
		if ok {
			if _, ok := connToKeys[conn]; !ok { // ← init
				connToKeys[conn] = make(map[string]bool)
			}
			for _, key := range pend.Keys {
				sub := &Subscription{
					Key:       key,
					AuthKey:   req.AuthKey,
					WSConn:    conn,
					Connected: true,
				}
				activeSubs[key] = append(activeSubs[key], sub)
				connToKeys[conn][key] = true // ← bez paniki
			}
			delete(pendingAuthKeys, req.AuthKey)
			log.Println("Subscribed on:", pend.Keys)
		} else {
			log.Println("Invalid or expired auth_key:", req.AuthKey)
		}
		mu.Unlock()

	}
}

// Start WS server (use this in main)
func StartWSServer(port string) error {
	http.HandleFunc("/sub", HandleWS)
	log.Println("WebSocket listening on port", port)
	return http.ListenAndServe(":"+port, nil)
}

func NotifySubscribers(key string, data []byte) {
	if len(activeSubs) == 0 {
		return
	}

	mu.Lock()
	subs := activeSubs[key]
	mu.Unlock()

	if len(subs) == 0 {
		return
	}

	payload, _ := json.Marshal(map[string]string{
		"event": "updated",
		"key":   key,
		"data":  string(data),
	})

	for _, sub := range subs {
		if connToKeys[sub.WSConn][key] {
			_ = sub.WSConn.WriteMessage(websocket.TextMessage, payload)
		}
	}

}

func NotifyDeleteAndRemove(key string) {
	if len(activeSubs) == 0 {
		return
	}

	mu.Lock()
	subs, ok := activeSubs[key]
	if !ok {
		mu.Unlock()
		return
	}
	delete(activeSubs, key)
	mu.Unlock()
	for _, sub := range subs {
		if m, ok := connToKeys[sub.WSConn]; ok && m[key] {
			continue
		}
		if sub.WSConn != nil {
			err := sub.WSConn.WriteJSON(map[string]string{
				"event": "deleted",
				"key":   key,
			})
			if err != nil {
				log.Println("Błąd wysyłania WS:", err)
			}
		}
	}
}

func EnableSubscription(keys []string) (string, error) {
	if len(keys) == 0 {
		return "", errors.New("no keys provided")
	}

	authKey := uuid.NewString()

	mu.Lock()
	pendingAuthKeys[authKey] = &Pending{
		Keys:      keys,
		ExpiresAt: time.Now().Add(60 * time.Second),
	}
	mu.Unlock()

	// kasowanie po TTL
	go func(k string) {
		time.Sleep(60 * time.Second)
		mu.Lock()
		if p, ok := pendingAuthKeys[k]; ok && time.Now().After(p.ExpiresAt) {
			delete(pendingAuthKeys, k)
		}
		mu.Unlock()
	}(authKey)

	return authKey, nil
}

func DisableSubscription(key string) error {
	if key == "" {
		return errors.New("key is empty")
	}

	mu.Lock()
	defer mu.Unlock()

	subs, ok := activeSubs[key]
	if !ok {
		return nil // nic nie było aktywne
	}

	for _, sub := range subs {
		if sub.WSConn != nil {
			_ = sub.WSConn.WriteJSON(map[string]string{
				"event": "unsubscribed",
				"key":   key,
			})
			// usuń z odwrotnej mapy
			if m, exists := connToKeys[sub.WSConn]; exists {
				delete(m, key)
				if len(m) == 0 {
					delete(connToKeys, sub.WSConn)
				}
			}
		}
	}
	delete(activeSubs, key)

	return nil
}
