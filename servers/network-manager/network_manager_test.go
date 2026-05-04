package networkmanager

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	dataManager_v2 "github.com/PAW122/TsunamiDB/data/dataManager/v2"
	defrag "github.com/PAW122/TsunamiDB/data/defragmentationManager"
	fileSystem_v1 "github.com/PAW122/TsunamiDB/data/fileSystem/v1"
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
		responseChannels: make(map[string]*pendingResponse),
		localTables:      make(map[string]TableInfo),
		ServerIP:         "127.0.0.1:7000",
		NodeID:           "127.0.0.1:7000",
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
}

func setupNetworkManagerDBTest(t *testing.T) {
	t.Helper()
	release := acquireDBTestLock(t)
	t.Cleanup(release)

	dataManager_v2.ShutdownWorkersForTests()
	fileSystem_v1.ShutdownForTests()
	defrag.ResetForTests()
	_ = os.RemoveAll("./db")

	t.Cleanup(func() {
		dataManager_v2.ShutdownWorkersForTests()
		fileSystem_v1.ShutdownForTests()
		defrag.ResetForTests()
		_ = os.RemoveAll("./db")
	})
}

func acquireDBTestLock(t *testing.T) func() {
	t.Helper()

	lockPath := "./db_test.lock"
	deadline := time.Now().Add(30 * time.Second)
	for {
		f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		if err == nil {
			_ = f.Close()
			return func() { _ = os.Remove(lockPath) }
		}
		if !errors.Is(err, os.ErrExist) {
			t.Fatalf("create test lock: %v", err)
		}
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting for db test lock")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func startTestNetworkManagerServer(t *testing.T, nm *NetworkManager) (*httptest.Server, string) {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", nm.handleConnection)
	server := httptest.NewServer(mux)
	host := strings.TrimPrefix(server.URL, "http://")
	nm.ServerIP = host
	nm.NodeID = host
	t.Cleanup(server.Close)
	return server, host
}

func waitForCondition(t *testing.T, timeout time.Duration, cond func() bool, msg string) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for {
		if cond() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal(msg)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

type nmProcess struct {
	name        string
	workdir     string
	networkAddr string
	controlAddr string
	cmd         *exec.Cmd
	stdout      bytes.Buffer
	stderr      bytes.Buffer
}

func reserveTCPPort(t *testing.T) int {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	defer ln.Close()

	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("unexpected listener addr: %T", ln.Addr())
	}
	return addr.Port
}

func startNetworkManagerProcess(t *testing.T, name string, networkPort, controlPort int, knownPeers []string) *nmProcess {
	t.Helper()

	workdir := t.TempDir()
	networkAddr := fmt.Sprintf("127.0.0.1:%d", networkPort)
	controlAddr := fmt.Sprintf("127.0.0.1:%d", controlPort)

	cmd := exec.Command(os.Args[0], "-test.run=^TestNetworkManagerProcessHelper$")
	cmd.Dir = workdir
	cmd.Env = append(os.Environ(),
		"TSU_NM_HELPER=1",
		"TSU_NM_PORT="+strconv.Itoa(networkPort),
		"TSU_NM_CONTROL_PORT="+strconv.Itoa(controlPort),
		"TSU_NM_KNOWN_PEERS="+strings.Join(knownPeers, ","),
		"TSUNAMI_NETWORK_ADVERTISE_ADDR="+networkAddr,
	)

	proc := &nmProcess{
		name:        name,
		workdir:     workdir,
		networkAddr: networkAddr,
		controlAddr: controlAddr,
		cmd:         cmd,
	}
	cmd.Stdout = &proc.stdout
	cmd.Stderr = &proc.stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("start %s process: %v", name, err)
	}

	waitForCondition(t, 5*time.Second, func() bool {
		resp, err := http.Get("http://" + controlAddr + "/health")
		if err != nil {
			return false
		}
		defer resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}, fmt.Sprintf("%s control server not ready\nstdout:\n%s\nstderr:\n%s", name, proc.stdout.String(), proc.stderr.String()))

	t.Cleanup(func() {
		if proc.cmd.Process == nil {
			return
		}
		_ = proc.cmd.Process.Kill()
		_ = proc.cmd.Wait()
	})

	return proc
}

func (p *nmProcess) postTask(t *testing.T, path string, req types.NMmessage) types.NMmessage {
	t.Helper()

	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("%s marshal request: %v", p.name, err)
	}

	resp, err := http.Post("http://"+p.controlAddr+path, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("%s post %s: %v\nstdout:\n%s\nstderr:\n%s", p.name, path, err, p.stdout.String(), p.stderr.String())
	}
	defer resp.Body.Close()

	raw, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		t.Fatalf("%s read %s response: %v", p.name, path, readErr)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("%s %s status %d: %s\nstdout:\n%s\nstderr:\n%s", p.name, path, resp.StatusCode, string(raw), p.stdout.String(), p.stderr.String())
	}

	var res types.NMmessage
	if err := json.Unmarshal(raw, &res); err != nil {
		t.Fatalf("%s decode %s response: %v raw=%q", p.name, path, err, raw)
	}
	return res
}

func (p *nmProcess) sendTask(t *testing.T, req types.NMmessage) types.NMmessage {
	t.Helper()
	return p.postTask(t, "/send-task", req)
}

func (p *nmProcess) localTask(t *testing.T, req types.NMmessage) types.NMmessage {
	t.Helper()
	return p.postTask(t, "/local-task", req)
}

func (p *nmProcess) stats(t *testing.T) Stats {
	t.Helper()

	resp, err := http.Get("http://" + p.controlAddr + "/stats")
	if err != nil {
		t.Fatalf("%s get stats: %v\nstdout:\n%s\nstderr:\n%s", p.name, err, p.stdout.String(), p.stderr.String())
	}
	defer resp.Body.Close()

	raw, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		t.Fatalf("%s read stats: %v", p.name, readErr)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("%s stats status %d: %s", p.name, resp.StatusCode, string(raw))
	}

	var stats Stats
	if err := json.Unmarshal(raw, &stats); err != nil {
		t.Fatalf("%s decode stats: %v raw=%q", p.name, err, raw)
	}
	return stats
}

func TestSnapshotAndInstanceHelpers(t *testing.T) {
	SetInstanceForTests(nil)
	if GetNetworkManager() != nil {
		t.Fatalf("expected nil manager")
	}

	nm := newTestManager()
	nm.port = 8080
	nm.localTables["users"] = TableInfo{Name: "users", Kind: tableKindKV}
	nm.peers["b"] = &Peer{Address: "b", LastActive: time.Now(), tables: map[string]TableInfo{"logs": {Name: "logs", Kind: tableKindKV}}}
	nm.peers["a"] = &Peer{Address: "a", Advertise: "10.0.0.2:7000", NodeID: "node-a", LastActive: time.Now(), tables: map[string]TableInfo{"users": {Name: "users", Kind: tableKindKV}}}
	nm.peers["a"].requestsSent.Store(3)
	nm.peers["a"].requestsReceived.Store(2)
	nm.peers["a"].responsesSent.Store(1)
	nm.peers["a"].responsesReceived.Store(4)
	nm.responseChannels["req"] = &pendingResponse{ch: make(chan types.NMmessage, 1)}
	SetInstanceForTests(nm)
	t.Cleanup(func() { SetInstanceForTests(nil) })

	stats := nm.Snapshot()
	if stats.ServerIP != "127.0.0.1:7000" || stats.Port != 8080 || stats.ConnectedPeers != 2 || stats.PendingResponses != 1 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	if strings.Join(stats.PeerAddresses, ",") != "a,b" {
		t.Fatalf("peers not sorted: %#v", stats.PeerAddresses)
	}
	if len(stats.LocalTables) != 1 || stats.LocalTables[0].Name != "users" {
		t.Fatalf("unexpected local tables: %+v", stats.LocalTables)
	}
	if got := strings.Join(stats.RemoteTables["users"], ","); got != "10.0.0.2:7000" {
		t.Fatalf("unexpected remote owners: %#v", stats.RemoteTables)
	}
	if len(stats.Peers) != 2 {
		t.Fatalf("unexpected peer stats: %+v", stats.Peers)
	}
	if stats.Peers[0].AdvertiseAddr != "10.0.0.2:7000" || stats.Peers[0].RequestsSent != 3 || stats.Peers[0].ResponsesReceived != 4 {
		t.Fatalf("unexpected first peer stats: %+v", stats.Peers[0])
	}
	if zero := (*NetworkManager)(nil).Snapshot(); zero.ConnectedPeers != 0 {
		t.Fatalf("nil snapshot should be empty: %+v", zero)
	}
}

func TestHandleResponseDeliversToRegisteredChannel(t *testing.T) {
	nm := newTestManager()
	ch := make(chan types.NMmessage, 1)
	nm.responseChannels["req-1"] = &pendingResponse{ch: ch}

	res := types.NMmessage{RequestID: "req-1", Task: "read", ReqResBy: "peer", Finished: true}
	nm.HandleResponse(res)
	select {
	case got := <-ch:
		if got.ReqResBy != "peer" {
			t.Fatalf("unexpected response: %+v", got)
		}
	case <-time.After(time.Second):
		t.Fatalf("response not delivered")
	}

	nm.HandleResponse(types.NMmessage{RequestID: "missing"})
}

func TestSendTaskReqWithoutPeers(t *testing.T) {
	nm := newTestManager()
	res := nm.SendTaskReq(types.NMmessage{Task: "read"})
	if res.Finished {
		t.Fatalf("expected unfinished response with no peers")
	}
}

func TestSendTaskReqRoutesByTableAndReceivesPeerResponse(t *testing.T) {
	nm := newTestManager()
	var wrongHits atomic.Int32
	var rightHits atomic.Int32

	newPeerServer := func(counter *atomic.Int32, responder func(types.NMmessage) types.NMmessage) (*httptest.Server, *websocket.Conn) {
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
				frame, err := nm.decodeFrame(raw)
				if err != nil {
					t.Errorf("decode frame: %v", err)
					return
				}
				if frame.Type != frameTypeRequest {
					continue
				}
				counter.Add(1)
				res := responder(frame.Message)
				if err := conn.WriteMessage(websocket.BinaryMessage, mustEncodeFrame(t, nm, protocolFrame{
					Version: 1,
					Type:    frameTypeResponse,
					NodeID:  "peer",
					Message: res,
				})); err != nil {
					return
				}
			}
		}))

		wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
		conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			server.Close()
			t.Fatalf("dial: %v", err)
		}
		return server, conn
	}

	wrongServer, wrongConn := newPeerServer(&wrongHits, func(req types.NMmessage) types.NMmessage {
		return types.NMmessage{RequestID: req.RequestID, Task: req.Task, ReqSendBy: req.ReqSendBy, ReqResBy: "wrong", Finished: false}
	})
	defer wrongServer.Close()
	defer wrongConn.Close()

	rightServer, rightConn := newPeerServer(&rightHits, func(req types.NMmessage) types.NMmessage {
		return types.NMmessage{
			RequestID: req.RequestID,
			Task:      req.Task,
			ReqSendBy: req.ReqSendBy,
			ReqResBy:  "right",
			Content:   []byte("ok"),
			Finished:  true,
		}
	})
	defer rightServer.Close()
	defer rightConn.Close()

	nm.peers["wrong"] = &Peer{Conn: wrongConn, Address: "wrong", LastActive: time.Now(), tables: map[string]TableInfo{"other": {Name: "other", Kind: tableKindKV}}}
	nm.peers["right"] = &Peer{Conn: rightConn, Address: "right", LastActive: time.Now(), tables: map[string]TableInfo{"target": {Name: "target", Kind: tableKindKV}}}
	go nm.listenForMessages("wrong", wrongConn)
	go nm.listenForMessages("right", rightConn)

	res := nm.SendTaskReq(types.NMmessage{Task: "read", Args: []string{"target", "key"}})
	if !res.Finished || string(res.Content) != "ok" || res.ReqResBy != "right" {
		t.Fatalf("unexpected response: %+v", res)
	}
	if got := wrongHits.Load(); got != 0 {
		t.Fatalf("request should not hit unrelated peer, got %d", got)
	}
	if got := rightHits.Load(); got != 1 {
		t.Fatalf("request should hit owning peer once, got %d", got)
	}
}

func TestHandleConnectionSendsHelloAndRegistersPeer(t *testing.T) {
	nm := newTestManager()
	nm.localTables["users"] = TableInfo{Name: "users", Kind: tableKindKV}
	server := httptest.NewServer(http.HandlerFunc(nm.handleConnection))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	_, raw, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read hello: %v", err)
	}
	frame, err := nm.decodeFrame(raw)
	if err != nil {
		t.Fatalf("decode hello: %v", err)
	}
	if frame.Type != frameTypeHello || len(frame.Tables) != 1 || frame.Tables[0].Name != "users" {
		t.Fatalf("unexpected hello frame: %+v", frame)
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

func TestTwoProcessManagersUseSeparateDirsAndPorts(t *testing.T) {
	networkPortB := reserveTCPPort(t)
	controlPortB := reserveTCPPort(t)
	nodeB := startNetworkManagerProcess(t, "node-b", networkPortB, controlPortB, nil)

	networkPortA := reserveTCPPort(t)
	controlPortA := reserveTCPPort(t)
	nodeA := startNetworkManagerProcess(t, "node-a", networkPortA, controlPortA, []string{nodeB.networkAddr})

	if nodeA.networkAddr == nodeB.networkAddr || nodeA.controlAddr == nodeB.controlAddr {
		t.Fatalf("processes should use different ports: A=%s/%s B=%s/%s", nodeA.networkAddr, nodeA.controlAddr, nodeB.networkAddr, nodeB.controlAddr)
	}
	if nodeA.workdir == nodeB.workdir {
		t.Fatalf("processes should use different working directories")
	}

	waitForCondition(t, 5*time.Second, func() bool {
		return nodeA.stats(t).ConnectedPeers == 1 && nodeB.stats(t).ConnectedPeers == 1
	}, "child network managers did not connect")

	saveRes := nodeA.sendTask(t, types.NMmessage{
		Task:    "save",
		Args:    []string{"shared", "key-1"},
		Content: []byte("payload"),
	})
	if !saveRes.Finished || saveRes.ReqResBy != nodeB.networkAddr {
		t.Fatalf("unexpected save response: %+v", saveRes)
	}

	waitForCondition(t, 5*time.Second, func() bool {
		owners := nodeA.stats(t).RemoteTables["shared"]
		return len(owners) == 1 && owners[0] == nodeB.networkAddr
	}, "node A did not learn about shared table on node B")

	localReadA := nodeA.localTask(t, types.NMmessage{
		Task: "read",
		Args: []string{"shared", "key-1"},
	})
	if localReadA.Finished {
		t.Fatalf("node A should not have local data: %+v", localReadA)
	}

	localReadB := nodeB.localTask(t, types.NMmessage{
		Task: "read",
		Args: []string{"shared", "key-1"},
	})
	if !localReadB.Finished || string(localReadB.Content) != "payload" {
		t.Fatalf("node B should own saved data: %+v", localReadB)
	}

	readRes := nodeA.sendTask(t, types.NMmessage{
		Task: "read",
		Args: []string{"shared", "key-1"},
	})
	if !readRes.Finished || readRes.ReqResBy != nodeB.networkAddr || string(readRes.Content) != "payload" {
		t.Fatalf("unexpected read response: %+v", readRes)
	}

	if _, err := os.Stat(filepath.Join(nodeA.workdir, "db")); err != nil {
		t.Fatalf("node A db dir missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(nodeB.workdir, "db")); err != nil {
		t.Fatalf("node B db dir missing: %v", err)
	}

	freeRes := nodeA.sendTask(t, types.NMmessage{
		Task: "free",
		Args: []string{"shared", "key-1"},
	})
	if !freeRes.Finished || freeRes.ReqResBy != nodeB.networkAddr {
		t.Fatalf("unexpected free response: %+v", freeRes)
	}

	missingLocalB := nodeB.localTask(t, types.NMmessage{
		Task: "read",
		Args: []string{"shared", "key-1"},
	})
	if missingLocalB.Finished {
		t.Fatalf("node B local read after free should fail: %+v", missingLocalB)
	}

	missingRemote := nodeA.sendTask(t, types.NMmessage{
		Task: "read",
		Args: []string{"shared", "key-1"},
	})
	if missingRemote.Finished {
		t.Fatalf("remote read after free should fail: %+v", missingRemote)
	}
}

func TestTwoManagersExchangeSaveReadAndFreeRequests(t *testing.T) {
	setupNetworkManagerDBTest(t)

	nodeA := newTestManager()
	nodeB := newTestManager()
	nodeB.localTables["shared"] = TableInfo{Name: "shared", Kind: tableKindKV}

	_, hostA := startTestNetworkManagerServer(t, nodeA)
	_, hostB := startTestNetworkManagerServer(t, nodeB)

	nodeA.connectToPeer(hostB)

	waitForCondition(t, 2*time.Second, func() bool {
		return nodeA.Snapshot().ConnectedPeers == 1 && nodeB.Snapshot().ConnectedPeers == 1
	}, "nodes did not connect")
	waitForCondition(t, 2*time.Second, func() bool {
		owners := nodeA.Snapshot().RemoteTables["shared"]
		return len(owners) == 1 && owners[0] == hostB
	}, "node A did not receive node B catalog")
	waitForCondition(t, 2*time.Second, func() bool {
		for _, peerAddr := range nodeB.listPeers() {
			peer := nodeB.getPeer(peerAddr)
			if peer != nil && peer.NodeID == hostA {
				return true
			}
		}
		return false
	}, "node B did not learn node A identity")

	saveRes := nodeA.SendTaskReq(types.NMmessage{
		Task:    "save",
		Args:    []string{"shared", "key-1"},
		Content: []byte("payload"),
	})
	if !saveRes.Finished || saveRes.ReqResBy != hostB {
		t.Fatalf("unexpected save response: %+v", saveRes)
	}

	readRes := nodeA.SendTaskReq(types.NMmessage{
		Task: "read",
		Args: []string{"shared", "key-1"},
	})
	if !readRes.Finished || readRes.ReqResBy != hostB || string(readRes.Content) != "payload" {
		t.Fatalf("unexpected read response: %+v", readRes)
	}

	freeRes := nodeA.SendTaskReq(types.NMmessage{
		Task: "free",
		Args: []string{"shared", "key-1"},
	})
	if !freeRes.Finished || freeRes.ReqResBy != hostB {
		t.Fatalf("unexpected free response: %+v", freeRes)
	}

	missingRes := nodeA.SendTaskReq(types.NMmessage{
		Task: "read",
		Args: []string{"shared", "key-1"},
	})
	if missingRes.Finished {
		t.Fatalf("expected read after free to fail, got: %+v", missingRes)
	}
}

func TestNetworkManagerProcessHelper(t *testing.T) {
	if os.Getenv("TSU_NM_HELPER") != "1" {
		return
	}

	port, err := strconv.Atoi(strings.TrimSpace(os.Getenv("TSU_NM_PORT")))
	if err != nil {
		t.Fatalf("invalid TSU_NM_PORT: %v", err)
	}
	controlPort, err := strconv.Atoi(strings.TrimSpace(os.Getenv("TSU_NM_CONTROL_PORT")))
	if err != nil {
		t.Fatalf("invalid TSU_NM_CONTROL_PORT: %v", err)
	}

	var knownPeers []string
	if raw := strings.TrimSpace(os.Getenv("TSU_NM_KNOWN_PEERS")); raw != "" {
		knownPeers = strings.Split(raw, ",")
	}

	StartNetworkManager(port, knownPeers)

	networkAddr := fmt.Sprintf("127.0.0.1:%d", port)
	waitForCondition(t, 5*time.Second, func() bool {
		conn, err := net.DialTimeout("tcp", networkAddr, 100*time.Millisecond)
		if err != nil {
			return false
		}
		_ = conn.Close()
		return true
	}, "network manager listener not ready")

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/stats", func(w http.ResponseWriter, r *http.Request) {
		nm := GetNetworkManager()
		if nm == nil {
			http.Error(w, "network manager not initialized", http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(nm.Snapshot())
	})
	mux.HandleFunc("/send-task", func(w http.ResponseWriter, r *http.Request) {
		nm := GetNetworkManager()
		if nm == nil {
			http.Error(w, "network manager not initialized", http.StatusInternalServerError)
			return
		}
		defer r.Body.Close()
		var req types.NMmessage
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(nm.SendTaskReq(req))
	})
	mux.HandleFunc("/local-task", func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var req types.NMmessage
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(executeTask(req))
	})

	if err := http.ListenAndServe(fmt.Sprintf("127.0.0.1:%d", controlPort), mux); err != nil {
		t.Fatalf("control server stopped: %v", err)
	}
}

func TestBroadcastAndSendToPeer(t *testing.T) {
	nm := newTestManager()
	received := make(chan protocolFrame, 2)
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
			frame, err := nm.decodeFrame(raw)
			if err != nil {
				t.Errorf("decode frame: %v", err)
				return
			}
			received <- frame
		}
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	nm.peers["peer"] = &Peer{Conn: conn, Address: "peer", LastActive: time.Now(), tables: map[string]TableInfo{}}

	nm.BroadcastMessage("sender", mustMarshalMessage(t, types.NMmessage{Task: "read", Args: []string{"table", "key"}}))
	if got := <-received; got.Type != frameTypeRequest || got.Message.Task != "read" {
		t.Fatalf("unexpected broadcast frame: %+v", got)
	}

	nm.sendToPeer("peer", mustMarshalMessage(t, types.NMmessage{Task: "free", Args: []string{"table", "key"}}))
	if got := <-received; got.Type != frameTypeRequest || got.Message.Task != "free" {
		t.Fatalf("unexpected direct frame: %+v", got)
	}
	nm.sendToPeer("missing", mustMarshalMessage(t, types.NMmessage{Task: "read"}))
	if peers := nm.listPeers(); len(peers) != 1 || peers[0] != "peer" {
		t.Fatalf("unexpected peers: %#v", peers)
	}
}

func TestListenForMessagesHandlesCatalogResponseAndDisconnect(t *testing.T) {
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

	nm.peers["peer"] = &Peer{Conn: clientConn, Address: "peer", LastActive: time.Now(), tables: map[string]TableInfo{}}
	ch := make(chan types.NMmessage, 1)
	nm.responseChannels["req-1"] = &pendingResponse{ch: ch}
	go nm.listenForMessages("peer", clientConn)

	if err := serverConn.WriteMessage(websocket.BinaryMessage, mustEncodeFrame(t, nm, protocolFrame{
		Version:       1,
		Type:          frameTypeCatalog,
		NodeID:        "peer-node",
		AdvertiseAddr: "10.0.0.3:7000",
		Tables:        []TableInfo{{Name: "remote", Kind: tableKindKV}},
	})); err != nil {
		t.Fatalf("write catalog: %v", err)
	}
	if err := serverConn.WriteMessage(websocket.BinaryMessage, mustEncodeFrame(t, nm, protocolFrame{
		Version: 1,
		Type:    frameTypeResponse,
		NodeID:  "peer-node",
		Message: types.NMmessage{RequestID: "req-1", Task: "read", ReqResBy: "peer-node", Finished: true},
	})); err != nil {
		t.Fatalf("write response: %v", err)
	}

	select {
	case got := <-ch:
		if got.ReqResBy != "peer-node" {
			t.Fatalf("unexpected response: %+v", got)
		}
	case <-time.After(time.Second):
		t.Fatalf("response not handled")
	}

	deadline := time.Now().Add(time.Second)
	for {
		stats := nm.Snapshot()
		if owners, ok := stats.RemoteTables["remote"]; ok && len(owners) == 1 && owners[0] == "10.0.0.3:7000" {
			if len(stats.Peers) != 1 {
				t.Fatalf("expected one peer stats entry, got %+v", stats.Peers)
			}
			peer := stats.Peers[0]
			if peer.CatalogsReceived == 0 || peer.ResponsesReceived == 0 {
				t.Fatalf("expected received counters to be populated, got %+v", peer)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("catalog was not applied: %+v", stats.RemoteTables)
		}
		time.Sleep(10 * time.Millisecond)
	}

	_ = serverConn.Close()
	deadline = time.Now().Add(time.Second)
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

func TestSendTaskReqTracksPerPeerFrameCounters(t *testing.T) {
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

	nm.peers["peer"] = &Peer{Conn: clientConn, Address: "peer", LastActive: time.Now(), tables: map[string]TableInfo{"target": {Name: "target", Kind: tableKindKV}}}
	go nm.listenForMessages("peer", clientConn)

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, raw, err := serverConn.ReadMessage()
		if err != nil {
			return
		}
		frame, err := nm.decodeFrame(raw)
		if err != nil {
			t.Errorf("decode request frame: %v", err)
			return
		}
		if frame.Type != frameTypeRequest {
			t.Errorf("unexpected frame type: %s", frame.Type)
			return
		}
		err = serverConn.WriteMessage(websocket.BinaryMessage, mustEncodeFrame(t, nm, protocolFrame{
			Version: 1,
			Type:    frameTypeResponse,
			NodeID:  "peer-node",
			Message: types.NMmessage{
				RequestID: frame.Message.RequestID,
				Task:      frame.Message.Task,
				ReqSendBy: frame.Message.ReqSendBy,
				ReqResBy:  "peer-node",
				Finished:  true,
			},
		}))
		if err != nil {
			t.Errorf("write response frame: %v", err)
		}
	}()

	res := nm.SendTaskReq(types.NMmessage{Task: "read", Args: []string{"target", "key"}})
	if !res.Finished || res.ReqResBy != "peer-node" {
		t.Fatalf("unexpected response: %+v", res)
	}
	<-done

	waitForCondition(t, time.Second, func() bool {
		stats := nm.Snapshot()
		return len(stats.Peers) == 1 && stats.Peers[0].RequestsSent == 1 && stats.Peers[0].ResponsesReceived == 1
	}, "request/response counters were not tracked")
}

func mustEncodeFrame(t *testing.T, nm *NetworkManager, frame protocolFrame) []byte {
	t.Helper()
	raw, err := nm.encodeFrame(frame)
	if err != nil {
		t.Fatalf("encode frame: %v", err)
	}
	return raw
}

func mustMarshalMessage(t *testing.T, msg types.NMmessage) []byte {
	t.Helper()
	raw, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal message: %v", err)
	}
	return raw
}
