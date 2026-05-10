package subscriptions

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
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
	Scopes    []string
	ExpiresAt time.Time
}

type SubscriptionTarget struct {
	Table string `json:"table,omitempty"`
	Key   string `json:"key"`
}

var (
	ErrNoKeys   = errors.New("enable subscription: empty keys")
	ErrNoKeyArg = errors.New("disable subscription: empty key")
)

type Stats struct {
	ActiveClients       int `json:"active_clients"`
	KeysWithSubscribers int `json:"keys_with_subscribers"`
	ActiveSubscriptions int `json:"active_subscriptions"`
	PendingAuthKeys     int `json:"pending_auth_keys"`
}

func StatsSnapshot() Stats {
	mu.Lock()
	defer mu.Unlock()

	stats := Stats{
		ActiveClients:       len(connToKeys),
		KeysWithSubscribers: len(activeSubs),
		PendingAuthKeys:     len(pendingAuthKeys),
	}

	totalSubs := 0
	for _, keys := range connToKeys {
		totalSubs += len(keys)
	}
	stats.ActiveSubscriptions = totalSubs

	return stats
}

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
		Keys          []string             `json:"keys"`
		Subscriptions []SubscriptionTarget `json:"subscriptions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	keys, scopes := subscriptionRequestScopes(req.Keys, req.Subscriptions)
	if len(scopes) == 0 {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	authKey := uuid.NewString()

	mu.Lock()
	pendingAuthKeys[authKey] = &Pending{
		Keys:      keys,
		Scopes:    scopes,
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
		Scopes:    append([]string(nil), keys...),
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

func EnableSubscriptionForTargetsInternal(targets []SubscriptionTarget) (string, error) {
	keys, scopes := subscriptionRequestScopes(nil, targets)
	if len(scopes) == 0 {
		return "", ErrNoKeys
	}

	authKey := uuid.NewString()

	mu.Lock()
	pendingAuthKeys[authKey] = &Pending{
		Keys:      keys,
		Scopes:    scopes,
		ExpiresAt: time.Now().Add(60 * time.Second),
	}
	mu.Unlock()

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
		Table string `json:"table"`
		Key   string `json:"key"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Key == "" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("missing key"))
		return
	}
	_, _ = disableSubscriptionScope(subscriptionScope(req.Table, req.Key), req.Table, req.Key)

	w.WriteHeader(http.StatusOK)
}

func DisableSubscriptionInternal(key string) (int, error) {
	if key == "" {
		return 0, ErrNoKeyArg
	}
	return disableSubscriptionScope(key, "", key)
}

func DisableSubscriptionForTargetInternal(table, key string) (int, error) {
	if key == "" {
		return 0, ErrNoKeyArg
	}
	return disableSubscriptionScope(subscriptionScope(table, key), table, key)
}

func disableSubscriptionScope(scope, table, key string) (int, error) {
	mu.Lock()
	set := activeSubs[scope]
	conns := make([]*websocket.Conn, 0, len(set))
	for c := range set {
		conns = append(conns, c)
		if m := connToKeys[c]; m != nil {
			delete(m, scope)
			if len(m) == 0 {
				delete(connToKeys, c)
			}
		}
	}
	delete(activeSubs, scope)
	mu.Unlock()

	notified := 0
	for _, c := range conns {
		if err := writeJSON(c, map[string]string{
			"event": "unsubscribed",
			"key":   key,
			"table": table,
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
			if websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				log.Println("Disconnected/read error:", err)
			}
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
			scopes := pend.Scopes
			if len(scopes) == 0 {
				scopes = pend.Keys
			}
			// Dla każdego scope: dodaj do setów (idempotentnie)
			for _, scope := range scopes {
				if _, ok := activeSubs[scope]; !ok {
					activeSubs[scope] = make(map[*websocket.Conn]struct{})
				}
				// jeśli już zasubskrybowane przez ten conn, nic nie robi
				if _, already := connToKeys[conn][scope]; !already {
					activeSubs[scope][conn] = struct{}{}
					connToKeys[conn][scope] = struct{}{}
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
	notifySubscribersWithPayload(key, map[string]any{
		"event": "updated",
		"key":   key,
		"data":  string(data),
	})
}

func NotifyTableSubscribers(table, key string, data []byte) {
	notifySubscribersWithPayloadForScopes(tableKeyPayload(table, key, map[string]any{
		"event": "updated",
		"data":  string(data),
	}), key, subscriptionScope(table, key))
}

func NotifyPatchSubscribers(key string, patch any) {
	notifySubscribersWithPayload(key, map[string]any{
		"event": "patched",
		"key":   key,
		"patch": patch,
	})
}

func NotifyTablePatchSubscribers(table, key string, patch any) {
	notifySubscribersWithPayloadForScopes(tableKeyPayload(table, key, map[string]any{
		"event": "patched",
		"patch": patch,
	}), key, subscriptionScope(table, key))
}

func NotifyPatchSubscribersWithRevision(key string, patch any, baseRev, rev uint64) {
	notifySubscribersWithPayload(key, map[string]any{
		"event":    "patched",
		"key":      key,
		"base_rev": baseRev,
		"rev":      rev,
		"patch":    patch,
	})
}

func NotifyTablePatchSubscribersWithRevision(table, key string, patch any, baseRev, rev uint64) {
	notifySubscribersWithPayloadForScopes(tableKeyPayload(table, key, map[string]any{
		"event":    "patched",
		"base_rev": baseRev,
		"rev":      rev,
		"patch":    patch,
	}), key, subscriptionScope(table, key))
}

func NotifySubscribersWithRevision(key string, data []byte, rev uint64) {
	notifySubscribersWithPayload(key, map[string]any{
		"event": "updated",
		"key":   key,
		"data":  string(data),
		"rev":   rev,
	})
}

func NotifyTableSubscribersWithRevision(table, key string, data []byte, rev uint64) {
	notifySubscribersWithPayloadForScopes(tableKeyPayload(table, key, map[string]any{
		"event": "updated",
		"data":  string(data),
		"rev":   rev,
	}), key, subscriptionScope(table, key))
}

func NotifyIncTableSubscribers(key string, changeType string, entryID uint64, entryData []byte) {
	notifySubscribersWithPayload(key, map[string]any{
		"event": "inc_table_update",
		"key":   key,
		"data": map[string]any{
			"type": changeType,
			"new_data": map[string]string{
				"id":   strconv.FormatUint(entryID, 10),
				"data": string(entryData),
			},
		},
	})
}

func NotifyTableIncTableSubscribers(table string, key string, changeType string, entryID uint64, entryData []byte) {
	notifySubscribersWithPayloadForScopes(tableKeyPayload(table, key, map[string]any{
		"event": "inc_table_update",
		"data": map[string]any{
			"type": changeType,
			"new_data": map[string]string{
				"id":   strconv.FormatUint(entryID, 10),
				"data": string(entryData),
			},
		},
	}), key, subscriptionScope(table, key))
}

func notifySubscribersWithPayload(key string, payload any) {
	notifySubscribersWithPayloadForScopes(payload, key)
}

func notifySubscribersWithPayloadForScopes(payload any, scopes ...string) {
	conns := snapshotSubscribers(scopes...)
	if len(conns) == 0 {
		return
	}

	for _, c := range conns {
		if err := writeJSON(c, payload); err != nil {
			log.Println("notify write failed -> cleanup:", err)
			cleanupConn(c)
		}
	}
}

func snapshotSubscribers(scopes ...string) []*websocket.Conn {
	mu.Lock()
	defer mu.Unlock()

	seen := make(map[*websocket.Conn]struct{})
	conns := make([]*websocket.Conn, 0)
	for _, scope := range uniqueScopes(scopes...) {
		set := activeSubs[scope]
		for c := range set {
			if _, ok := seen[c]; ok {
				continue
			}
			seen[c] = struct{}{}
			conns = append(conns, c)
		}
	}
	return conns
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

func NotifyTableDeleteAndRemove(table, key string) {
	notifyDeleteAndRemoveScopes(table, key, key, subscriptionScope(table, key))
}

func notifyDeleteAndRemoveScopes(table, key string, scopes ...string) {
	mu.Lock()
	seen := make(map[*websocket.Conn]struct{})
	conns := make([]*websocket.Conn, 0)
	for _, scope := range uniqueScopes(scopes...) {
		set := activeSubs[scope]
		for c := range set {
			if _, ok := seen[c]; !ok {
				seen[c] = struct{}{}
				conns = append(conns, c)
			}
			if m := connToKeys[c]; m != nil {
				delete(m, scope)
				if len(m) == 0 {
					delete(connToKeys, c)
				}
			}
		}
		delete(activeSubs, scope)
	}
	mu.Unlock()

	for _, c := range conns {
		if err := writeJSON(c, tableKeyPayload(table, key, map[string]any{
			"event": "deleted",
		})); err != nil {
			log.Println("delete notify write failed -> cleanup:", err)
			cleanupConn(c)
		}
	}
}

func subscriptionRequestScopes(keys []string, targets []SubscriptionTarget) ([]string, []string) {
	labels := make([]string, 0, len(keys)+len(targets))
	scopes := make([]string, 0, len(keys)+len(targets))
	for _, key := range keys {
		if key == "" {
			continue
		}
		labels = append(labels, key)
		scopes = append(scopes, subscriptionScope("", key))
	}
	for _, target := range targets {
		if target.Key == "" {
			continue
		}
		labels = append(labels, displaySubscriptionScope(target.Table, target.Key))
		scopes = append(scopes, subscriptionScope(target.Table, target.Key))
	}
	return labels, uniqueScopes(scopes...)
}

func subscriptionScope(table, key string) string {
	if table == "" {
		return key
	}
	return table + "\x00" + key
}

func displaySubscriptionScope(table, key string) string {
	if table == "" {
		return key
	}
	return table + "/" + key
}

func uniqueScopes(scopes ...string) []string {
	seen := make(map[string]struct{}, len(scopes))
	out := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		if scope == "" {
			continue
		}
		if _, ok := seen[scope]; ok {
			continue
		}
		seen[scope] = struct{}{}
		out = append(out, scope)
	}
	return out
}

func tableKeyPayload(table, key string, payload map[string]any) map[string]any {
	payload["key"] = key
	if table != "" {
		payload["table"] = table
	}
	return payload
}
