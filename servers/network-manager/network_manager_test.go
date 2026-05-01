package networkmanager

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/PAW122/TsunamiDB/types"
	"github.com/gorilla/websocket"
)

func TestMain(m *testing.M) {
	_ = os.RemoveAll("./db")
	code := m.Run()
	_ = os.RemoveAll("./db")
	os.Exit(code)
}

func newTestManager() *NetworkManager {
	return &NetworkManager{
		peers:            make(map[string]*Peer),
		responseChannels: make(map[string]chan types.NMmessage),
		ServerIP:         "127.0.0.1",
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
}

func TestSnapshotAndInstanceHelpers(t *testing.T) {
	SetInstanceForTests(nil)
	if GetNetworkManager() != nil {
		t.Fatalf("expected nil manager")
	}

	nm := newTestManager()
	nm.port = 8080
	nm.peers["b"] = &Peer{Address: "b", LastActive: time.Now()}
	nm.peers["a"] = &Peer{Address: "a", LastActive: time.Now()}
	nm.responseChannels["req"] = make(chan types.NMmessage, 1)
	SetInstanceForTests(nm)
	t.Cleanup(func() { SetInstanceForTests(nil) })

	if got := GetNetworkManager(); got != nm {
		t.Fatalf("unexpected manager")
	}
	stats := nm.Snapshot()
	if stats.ServerIP != "127.0.0.1" || stats.Port != 8080 || stats.ConnectedPeers != 2 || stats.PendingResponses != 1 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	if strings.Join(stats.PeerAddresses, ",") != "a,b" {
		t.Fatalf("peers not sorted: %#v", stats.PeerAddresses)
	}
	if zero := (*NetworkManager)(nil).Snapshot(); zero.ConnectedPeers != 0 {
		t.Fatalf("nil snapshot should be empty: %+v", zero)
	}
}

func TestHandleResponseDeliversToRegisteredChannel(t *testing.T) {
	nm := newTestManager()
	ch := make(chan types.NMmessage, 1)
	nm.responseChannels["sender_read"] = ch

	res := types.NMmessage{Task: "read", ReqSendBy: "sender", ReqResBy: "peer", Finished: true}
	nm.HandleResponse(res)
	select {
	case got := <-ch:
		if got.ReqResBy != "peer" {
			t.Fatalf("unexpected response: %+v", got)
		}
	case <-time.After(time.Second):
		t.Fatalf("response not delivered")
	}

	nm.HandleResponse(types.NMmessage{Task: "missing", ReqSendBy: "sender"})
}

func TestSendTaskReqWithoutPeers(t *testing.T) {
	nm := newTestManager()
	res := nm.SendTaskReq(types.NMmessage{Task: "read"})
	if res.Finished {
		t.Fatalf("expected unfinished response with no peers")
	}
}

func TestSendTaskReqReceivesPeerResponse(t *testing.T) {
	nm := newTestManager()
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer conn.Close()
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var req types.NMmessage
		if err := json.Unmarshal(raw, &req); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		req.Finished = true
		req.ReqResBy = "peer"
		req.Content = []byte("ok")
		payload, _ := json.Marshal(req)
		_ = conn.WriteMessage(websocket.TextMessage, payload)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	nm.peers["peer"] = &Peer{Conn: conn, Address: "peer", LastActive: time.Now()}
	go nm.listenForMessages("peer", conn)

	res := nm.SendTaskReq(types.NMmessage{Task: "read", Args: []string{"table", "key"}})
	if !res.Finished || string(res.Content) != "ok" || res.ReqResBy != "peer" {
		t.Fatalf("unexpected response: %+v", res)
	}
}

func TestHandleConnectionSendsSetIPAndRegistersPeer(t *testing.T) {
	nm := newTestManager()
	server := httptest.NewServer(http.HandlerFunc(nm.handleConnection))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	var msg types.NMmessage
	if err := conn.ReadJSON(&msg); err != nil {
		t.Fatalf("read set_ip: %v", err)
	}
	if msg.Task != "set_ip" || len(msg.Args) != 1 || msg.Args[0] == "" {
		t.Fatalf("unexpected set_ip message: %+v", msg)
	}

	deadline := time.Now().Add(time.Second)
	for {
		if nm.Snapshot().ConnectedPeers == 1 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("peer was not registered")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestConnectToPeerSuccessAndFailure(t *testing.T) {
	nm := newTestManager()
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer conn.Close()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer server.Close()

	host := strings.TrimPrefix(server.URL, "http://")
	nm.connectToPeer(host)
	if nm.Snapshot().ConnectedPeers != 1 {
		t.Fatalf("expected connected peer")
	}
	nm.connectToPeer("127.0.0.1:1")
}

func TestBroadcastAndSendToPeer(t *testing.T) {
	nm := newTestManager()
	received := make(chan string, 2)
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer conn.Close()
		for {
			_, raw, err := conn.ReadMessage()
			if err != nil {
				return
			}
			received <- string(raw)
		}
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	nm.peers["peer"] = &Peer{Conn: conn, Address: "peer", LastActive: time.Now()}

	nm.BroadcastMessage("sender", []byte("broadcast"))
	if got := <-received; got != "broadcast" {
		t.Fatalf("unexpected broadcast: %q", got)
	}

	nm.sendToPeer("peer", []byte("direct"))
	if got := <-received; got != "direct" {
		t.Fatalf("unexpected direct message: %q", got)
	}
	nm.sendToPeer("missing", []byte("ignored"))
	if peers := nm.listPeers(); len(peers) != 1 || peers[0] != "peer" {
		t.Fatalf("unexpected peers: %#v", peers)
	}
}

func TestListenForMessagesHandlesInvalidFinishedAndRequest(t *testing.T) {
	nm := newTestManager()
	serverConnCh := make(chan *websocket.Conn, 1)
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		serverConnCh <- conn
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	clientConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	serverConn := <-serverConnCh
	defer serverConn.Close()

	nm.peers["peer"] = &Peer{Conn: clientConn, Address: "peer", LastActive: time.Now()}
	ch := make(chan types.NMmessage, 1)
	nm.responseChannels["sender_read"] = ch
	go nm.listenForMessages("peer", clientConn)

	if err := serverConn.WriteMessage(websocket.TextMessage, []byte("{bad")); err != nil {
		t.Fatalf("write invalid: %v", err)
	}
	finished := types.NMmessage{Task: "read", ReqSendBy: "sender", Finished: true}
	if err := serverConn.WriteJSON(finished); err != nil {
		t.Fatalf("write finished: %v", err)
	}
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatalf("finished response not handled")
	}

	if err := serverConn.WriteJSON(types.NMmessage{Task: "unknown"}); err != nil {
		t.Fatalf("write unknown: %v", err)
	}
	_ = serverConn.Close()

	deadline := time.Now().Add(time.Second)
	for {
		if nm.Snapshot().ConnectedPeers == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("peer was not removed")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
