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

// ---------------------------
// Modele i przechowywanie stanu
// ---------------------------

type Pending struct {
	Keys      []string
	ExpiresAt time.Time
}

var (
	ErrNoKeys   = errors.New("enable subscription: empty keys")
	ErrNoKeyArg = errors.New("disable subscription: empty key")
)

var (
	// key -> set(conn)
	activeSubs = make(map[string]map[*websocket.Conn]struct{})
	// conn -> set(key)
	connToKeys = make(map[*websocket.Conn]map[string]struct{})
	// auth_key -> pending keys (TTL)
	pendingAuthKeys = make(map[string]*Pending)

	// per-connection write lock (serializacja zapisów do jednego conn)
	connLocks = make(map[*websocket.Conn]*sync.Mutex)
	// kanał stop dla ping goroutine
	connDone = make(map[*websocket.Conn]chan struct{})

	mu sync.Mutex

	upgrader = websocket.Upgrader{
		// W PROD rozważ restrykcję origin
		CheckOrigin: func(r *http.Request) bool { return true },
	}
)

// ---------------------------
// Narzędzia do zapisu z per-conn lockiem
// ---------------------------

func getConnLock(c *websocket.Conn) *sync.Mutex {
	mu.Lock()
	defer mu.Unlock()
	return connLocks[c]
}

func writeJSON(c *websocket.Conn, v any) error {
	lock := getConnLock(c)
	if lock != nil {
		lock.Lock()
		defer lock.Unlock()
	}
	_ = c.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return c.WriteJSON(v)
}

func writeMessage(c *websocket.Conn, msgType int, payload []byte) error {
	lock := getConnLock(c)
	if lock != nil {
		lock.Lock()
		defer lock.Unlock()
	}
	_ = c.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return c.WriteMessage(msgType, payload)
}

func writePing(c *websocket.Conn) error {
	lock := getConnLock(c)
	if lock != nil {
		lock.Lock()
		defer lock.Unlock()
	}
	deadline := time.Now().Add(10 * time.Second)
	// Ping najlepiej przez WriteControl (krótkie ramki, deadline)
	return c.WriteControl(websocket.PingMessage, nil, deadline)
}

// ---------------------------
// Sprzątanie połączenia
// ---------------------------

func cleanupConn(conn *websocket.Conn) {
	// Zatrzymaj ping goroutine (jeśli jest)
	mu.Lock()
	if done, ok := connDone[conn]; ok {
		select {
		case <-done:
			// już zamknięty
		default:
			close(done)
		}
		delete(connDone, conn)
	}

	// Usuń conn z odwrotnej mapy i z activeSubs
	if keys, ok := connToKeys[conn]; ok {
		for k := range keys {
			if set, ok := activeSubs[k]; ok {
				delete(set, conn)
				if len(set) == 0 {
					delete(activeSubs, k)
				}
			}
		}
		delete(connToKeys, conn)
	}

	// Usuń per-conn lock
	delete(connLocks, conn)
	mu.Unlock()

	// Zamknij socket (może już być zamknięty)
	_ = conn.Close()
}

// ---------------------------
// HTTP Handlery
// ---------------------------

func HandleEnableSubscription(w http.ResponseWriter, r *http.Request, _ *http.Client) {
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

	// TTL czyszczenie
	go func(k string) {
		time.Sleep(60 * time.Second)
		mu.Lock()
		if p, ok := pendingAuthKeys[k]; ok && time.Now().After(p.ExpiresAt) {
			delete(pendingAuthKeys, k)
		}
		mu.Unlock()
	}(authKey)

	_ = json.NewEncoder(w).Encode(map[string]string{"auth_key": authKey})
}

func EnableSubscriptionInternal(keys []string) (string, error) {
	if len(keys) == 0 {
		return "", ErrNoKeys
	}

	authKey := uuid.NewString()

	mu.Lock()
	pendingAuthKeys[authKey] = &Pending{
		Keys:      append([]string(nil), keys...),
		ExpiresAt: time.Now().Add(60 * time.Second),
	}
	mu.Unlock()

	// TTL czyszczenie
	go func(k string) {
		timer := time.NewTimer(60 * time.Second)
		defer timer.Stop()
		<-timer.C
		mu.Lock()
		if p, ok := pendingAuthKeys[k]; ok && time.Now().After(p.ExpiresAt) {
			delete(pendingAuthKeys, k)
		}
		mu.Unlock()
	}(authKey)

	return authKey, nil
}

func HandleDisableSubscription(w http.ResponseWriter, r *http.Request, _ *http.Client) {
	var req struct {
		Key string `json:"key"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Key == "" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("missing key"))
		return
	}

	// Snapshot połączeń, sprzątamy mapy pod lockiem
	mu.Lock()
	set := activeSubs[req.Key]
	conns := make([]*websocket.Conn, 0, len(set))
	for c := range set {
		conns = append(conns, c)
		// usuń odwrotne mapowanie
		if m := connToKeys[c]; m != nil {
			delete(m, req.Key)
			if len(m) == 0 {
				delete(connToKeys, c)
			}
		}
	}
	delete(activeSubs, req.Key)
	mu.Unlock()

	// Wysyłka poza lockiem
	for _, c := range conns {
		if err := writeJSON(c, map[string]string{
			"event": "unsubscribed",
			"key":   req.Key,
		}); err != nil {
			log.Println("unsub notify write failed -> cleanup:", err)
			cleanupConn(c)
		}
	}

	w.WriteHeader(http.StatusOK)
}

func DisableSubscriptionInternal(key string) (int, error) {
	if key == "" {
		return 0, ErrNoKeyArg
	}

	// Snapshot połączeń + sprzątanie map pod lockiem
	mu.Lock()
	set := activeSubs[key]
	conns := make([]*websocket.Conn, 0, len(set))
	for c := range set {
		conns = append(conns, c)
		// usuń odwrotne mapowanie
		if m := connToKeys[c]; m != nil {
			delete(m, key)
			if len(m) == 0 {
				delete(connToKeys, c)
			}
		}
	}
	delete(activeSubs, key)
	mu.Unlock()

	// Wysyłka poza lockiem
	notified := 0
	for _, c := range conns {
		if err := writeJSON(c, map[string]string{
			"event": "unsubscribed",
			"key":   key,
		}); err != nil {
			log.Println("unsub notify write failed -> cleanup:", err)
			cleanupConn(c)
			continue
		}
		notified++
	}

	return notified, nil
}

// WebSocket endpoint: klient po połączeniu wysyła {"auth_key":"..."} aby dołączyć suby.
func HandleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Upgrade error:", err)
		return
	}
	// Od teraz każde wyjście -> sprzątamy
	defer cleanupConn(conn)

	// Zarejestruj per-conn lock + kanał done
	mu.Lock()
	if _, exists := connLocks[conn]; !exists {
		connLocks[conn] = &sync.Mutex{}
	}
	if _, exists := connDone[conn]; !exists {
		connDone[conn] = make(chan struct{})
	}
	localDone := connDone[conn]
	mu.Unlock()

	// Limity i keepalive
	conn.SetReadLimit(1 << 20) // np. 1MB
	_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error {
		// każda ramka pong przedłuża deadline
		return conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	})

	// Ping goroutine
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := writePing(conn); err != nil {
					// Zamykamy połączenie -> reader dostanie błąd i posprząta
					_ = conn.Close()
					return
				}
			case <-localDone:
				return
			}
		}
	}()

	// Reader (jeden goroutine na conn)
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			log.Println("Disconnected/read error:", err)
			break
		}

		// Oczekujemy JSON: {"auth_key":"..."}
		var req struct {
			AuthKey string `json:"auth_key"`
		}
		if err := json.Unmarshal(msg, &req); err != nil || req.AuthKey == "" {
			log.Println("Invalid message:", string(msg))
			continue
		}

		// Zastosuj pending auth key
		mu.Lock()
		pend, ok := pendingAuthKeys[req.AuthKey]
		if ok {
			// initialized odwrotna mapa dla conn
			if _, ok := connToKeys[conn]; !ok {
				connToKeys[conn] = make(map[string]struct{})
			}
			// Dla każdego key: dodaj do setów (idempotentnie)
			for _, key := range pend.Keys {
				if _, ok := activeSubs[key]; !ok {
					activeSubs[key] = make(map[*websocket.Conn]struct{})
				}
				// jeśli już zasubskrybowane przez ten conn, nic nie robi
				if _, already := connToKeys[conn][key]; !already {
					activeSubs[key][conn] = struct{}{}
					connToKeys[conn][key] = struct{}{}
				}
			}
			// Jednorazowo konsumuj auth_key
			delete(pendingAuthKeys, req.AuthKey)
			mu.Unlock()

			// Możesz opcjonalnie odesłać potwierdzenie
			_ = writeJSON(conn, map[string]any{
				"event": "subscribed",
				"keys":  pend.Keys,
			})
			// log.Println("Subscribed on:", pend.Keys)
		} else {
			mu.Unlock()
			_ = writeJSON(conn, map[string]string{
				"event":   "error",
				"message": "invalid_or_expired_auth_key",
			})
			// log.Println("Invalid or expired auth_key:", req.AuthKey)
		}
	}
}

// ---------------------------
// Serwer WS
// ---------------------------

func StartWSServer(port string) error {
	http.HandleFunc("/sub", HandleWS)
	log.Println("WebSocket listening on port", port)
	return http.ListenAndServe(":"+port, nil)
}

// ---------------------------
// Powiadomienia do subskrybentów
// ---------------------------

func NotifySubscribers(key string, data []byte) {
	// Snapshot połączeń
	mu.Lock()
	set := activeSubs[key]
	conns := make([]*websocket.Conn, 0, len(set))
	for c := range set {
		conns = append(conns, c)
	}
	mu.Unlock()

	if len(conns) == 0 {
		return
	}

	payload, _ := json.Marshal(map[string]string{
		"event": "updated",
		"key":   key,
		"data":  string(data),
	})

	for _, c := range conns {
		if err := writeMessage(c, websocket.TextMessage, payload); err != nil {
			log.Println("notify write failed -> cleanup:", err)
			cleanupConn(c)
		}
	}
}

func NotifyDeleteAndRemove(key string) {
	// Snapshot i sprzątanie map
	mu.Lock()
	set := activeSubs[key]
	conns := make([]*websocket.Conn, 0, len(set))
	for c := range set {
		conns = append(conns, c)
		// usuń odwrotne mapowanie
		if m := connToKeys[c]; m != nil {
			delete(m, key)
			if len(m) == 0 {
				delete(connToKeys, c)
			}
		}
	}
	delete(activeSubs, key)
	mu.Unlock()

	// Wysyłka poza lockiem
	for _, c := range conns {
		if err := writeJSON(c, map[string]string{
			"event": "deleted",
			"key":   key,
		}); err != nil {
			log.Println("delete notify write failed -> cleanup:", err)
			cleanupConn(c)
		}
	}
}
