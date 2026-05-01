package core

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	_ = os.RemoveAll("./db")
	code := m.Run()
	_ = os.RemoveAll("./db")
	os.Exit(code)
}

func TestRunCoreValidation(t *testing.T) {
	deps := testCoreDeps(nil)
	if err := runCore(deps); err == nil {
		t.Fatalf("expected missing port error")
	}

	deps = testCoreDeps([]string{"bad"})
	if err := runCore(deps); err == nil {
		t.Fatalf("expected invalid port error")
	}
}

func TestRunCoreStartsServices(t *testing.T) {
	var loaded string
	var networkPort int
	var peers []string
	var wsPort string
	var apiPort int

	deps := testCoreDeps([]string{"-config", "6000", "peer1", "peer2"})
	deps.loadConfig = func(path string) { loaded = path }
	deps.startNetworkManager = func(port int, knownPeers []string) {
		networkPort = port
		peers = append([]string(nil), knownPeers...)
	}
	wsDone := make(chan struct{}, 1)
	deps.startWSServer = func(port string) error {
		wsPort = port
		wsDone <- struct{}{}
		return nil
	}
	deps.runPublicAPI = func(port int) { apiPort = port }

	if err := runCore(deps); err != nil {
		t.Fatalf("runCore: %v", err)
	}
	<-wsDone

	if loaded != defaultConfigDir {
		t.Fatalf("unexpected config path: %s", loaded)
	}
	if networkPort != 6000 {
		t.Fatalf("unexpected network port: %d", networkPort)
	}
	if len(peers) != 2 || peers[0] != "peer1" || peers[1] != "peer2" {
		t.Fatalf("unexpected peers: %#v", peers)
	}
	if wsPort != "5845" || apiPort != 5844 {
		t.Fatalf("unexpected service ports: ws=%s api=%d", wsPort, apiPort)
	}
}

func testCoreDeps(args []string) coreDeps {
	return coreDeps{
		args:                args,
		loadConfig:          func(string) {},
		startNetworkManager: func(int, []string) {},
		startWSServer:       func(string) error { return nil },
		runPublicAPI:        func(int) {},
	}
}
