package subscriptions

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/PAW122/TsunamiDB/data/valuepatch"
	"github.com/gorilla/websocket"
)

func resetSubscriptionsForTests(tb testing.TB) {
	tb.Helper()
	mu.Lock()
	for c := range connToKeys {
		_ = c.Close()
	}
	activeSubs = make(map[string]map[*websocket.Conn]struct{})
	connToKeys = make(map[*websocket.Conn]map[string]struct{})
	pendingAuthKeys = make(map[string]*Pending)
	connLocks = make(map[*websocket.Conn]*sync.Mutex)
	connDone = make(map[*websocket.Conn]chan struct{})
	mu.Unlock()
	tb.Cleanup(func() {
		mu.Lock()
		for c := range connToKeys {
			_ = c.Close()
		}
		activeSubs = make(map[string]map[*websocket.Conn]struct{})
		connToKeys = make(map[*websocket.Conn]map[string]struct{})
		pendingAuthKeys = make(map[string]*Pending)
		connLocks = make(map[*websocket.Conn]*sync.Mutex)
		connDone = make(map[*websocket.Conn]chan struct{})
		mu.Unlock()
	})
}

func TestEnableSubscriptionHandlers(t *testing.T) {
	resetSubscriptionsForTests(t)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/subscriptions/enable", bytes.NewBufferString(`{"keys":["a","b"]}`))
	HandleEnableSubscription(rr, req, http.DefaultClient)
	if rr.Code != http.StatusOK {
		t.Fatalf("enable status: %d body=%s", rr.Code, rr.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["auth_key"] == "" {
		t.Fatalf("missing auth key")
	}
	if StatsSnapshot().PendingAuthKeys != 1 {
		t.Fatalf("expected one pending key")
	}

	bad := httptest.NewRecorder()
	HandleEnableSubscription(bad, httptest.NewRequest(http.MethodPost, "/subscriptions/enable", strings.NewReader(`{}`)), http.DefaultClient)
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("bad enable status: %d", bad.Code)
	}

	internalKey, err := EnableSubscriptionInternal([]string{"c"})
	if err != nil {
		t.Fatalf("internal enable: %v", err)
	}
	if internalKey == "" {
		t.Fatalf("missing internal auth key")
	}
	if _, err := EnableSubscriptionInternal(nil); err != ErrNoKeys {
		t.Fatalf("expected ErrNoKeys, got %v", err)
	}
}

func TestWebSocketSubscribeNotifyAndDisable(t *testing.T) {
	resetSubscriptionsForTests(t)

	authKey, err := EnableSubscriptionInternal([]string{"item"})
	if err != nil {
		t.Fatalf("enable internal: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(HandleWS))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer conn.Close()

	if err := conn.WriteJSON(map[string]string{"auth_key": authKey}); err != nil {
		t.Fatalf("write auth: %v", err)
	}
	var subscribed struct {
		Event string   `json:"event"`
		Keys  []string `json:"keys"`
	}
	if err := conn.ReadJSON(&subscribed); err != nil {
		t.Fatalf("read subscribed: %v", err)
	}
	if subscribed.Event != "subscribed" || len(subscribed.Keys) != 1 || subscribed.Keys[0] != "item" {
		t.Fatalf("unexpected subscribed response: %+v", subscribed)
	}

	stats := StatsSnapshot()
	if stats.ActiveClients != 1 || stats.KeysWithSubscribers != 1 || stats.ActiveSubscriptions != 1 || stats.PendingAuthKeys != 0 {
		t.Fatalf("unexpected stats after subscribe: %+v", stats)
	}

	NotifySubscribers("item", []byte("value"))
	var update struct {
		Event string `json:"event"`
		Key   string `json:"key"`
		Data  string `json:"data"`
	}
	if err := conn.ReadJSON(&update); err != nil {
		t.Fatalf("read update: %v", err)
	}
	if update.Event != "updated" || update.Key != "item" || update.Data != "value" {
		t.Fatalf("unexpected update: %+v", update)
	}

	NotifyPatchSubscribers("item", []valuepatch.Operation{{Offset: 5, Insert: "!"}})
	var patched struct {
		Event string                 `json:"event"`
		Key   string                 `json:"key"`
		Patch []valuepatch.Operation `json:"patch"`
	}
	if err := conn.ReadJSON(&patched); err != nil {
		t.Fatalf("read patch: %v", err)
	}
	if patched.Event != "patched" || patched.Key != "item" || len(patched.Patch) != 1 || patched.Patch[0].Insert != "!" {
		t.Fatalf("unexpected patch: %+v", patched)
	}

	notified, err := DisableSubscriptionInternal("item")
	if err != nil {
		t.Fatalf("disable internal: %v", err)
	}
	if notified != 1 {
		t.Fatalf("expected one notification, got %d", notified)
	}
	var unsub struct {
		Event string `json:"event"`
		Key   string `json:"key"`
	}
	if err := conn.ReadJSON(&unsub); err != nil {
		t.Fatalf("read unsubscribed: %v", err)
	}
	if unsub.Event != "unsubscribed" || unsub.Key != "item" {
		t.Fatalf("unexpected unsubscribe: %+v", unsub)
	}
	if _, err := DisableSubscriptionInternal(""); err != ErrNoKeyArg {
		t.Fatalf("expected ErrNoKeyArg, got %v", err)
	}
}

func TestInvalidWebSocketAuthAndDeleteNotification(t *testing.T) {
	resetSubscriptionsForTests(t)

	authKey, err := EnableSubscriptionInternal([]string{"gone", "inc"})
	if err != nil {
		t.Fatalf("enable internal: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(HandleWS))
	defer server.Close()
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer conn.Close()

	if err := conn.WriteJSON(map[string]string{"auth_key": "bad"}); err != nil {
		t.Fatalf("write bad auth: %v", err)
	}
	var authErr struct {
		Event   string `json:"event"`
		Message string `json:"message"`
	}
	if err := conn.ReadJSON(&authErr); err != nil {
		t.Fatalf("read auth error: %v", err)
	}
	if authErr.Event != "error" {
		t.Fatalf("unexpected auth error: %+v", authErr)
	}

	if err := conn.WriteJSON(map[string]string{"auth_key": authKey}); err != nil {
		t.Fatalf("write auth: %v", err)
	}
	var subscribed map[string]any
	if err := conn.ReadJSON(&subscribed); err != nil {
		t.Fatalf("read subscribed: %v", err)
	}

	NotifyIncTableSubscribers("inc", "add", 7, []byte("entry"))
	var inc map[string]any
	if err := conn.ReadJSON(&inc); err != nil {
		t.Fatalf("read inc notification: %v", err)
	}
	if inc["event"] != "inc_table_update" || inc["key"] != "inc" {
		t.Fatalf("unexpected inc notification: %+v", inc)
	}

	NotifyDeleteAndRemove("gone")
	var deleted struct {
		Event string `json:"event"`
		Key   string `json:"key"`
	}
	if err := conn.ReadJSON(&deleted); err != nil {
		t.Fatalf("read deleted: %v", err)
	}
	if deleted.Event != "deleted" || deleted.Key != "gone" {
		t.Fatalf("unexpected delete notification: %+v", deleted)
	}
}

func TestDisableSubscriptionHandlerValidation(t *testing.T) {
	resetSubscriptionsForTests(t)

	bad := httptest.NewRecorder()
	HandleDisableSubscription(bad, httptest.NewRequest(http.MethodPost, "/subscriptions/disable", strings.NewReader(`{}`)), http.DefaultClient)
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", bad.Code)
	}

	ok := httptest.NewRecorder()
	HandleDisableSubscription(ok, httptest.NewRequest(http.MethodPost, "/subscriptions/disable", strings.NewReader(`{"key":"missing"}`)), http.DefaultClient)
	if ok.Code != http.StatusOK {
		t.Fatalf("expected 200 for missing key unsubscribe, got %d", ok.Code)
	}
}
