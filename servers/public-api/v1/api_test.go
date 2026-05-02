package public_api_v1

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	metrics "github.com/PAW122/TsunamiDB/servers/public-api/v1/metrics"
)

func TestMain(m *testing.M) {
	_ = os.RemoveAll("./db")
	code := m.Run()
	_ = os.RemoveAll("./db")
	os.Exit(code)
}

func TestWithClientInjectsClientAndRecordsMetrics(t *testing.T) {
	metrics.ResetForTests()
	called := false
	handler := withClient(func(w http.ResponseWriter, r *http.Request, c *http.Client) {
		called = true
		if c != HTTPClient {
			t.Fatalf("unexpected client")
		}
		w.WriteHeader(http.StatusAccepted)
	})

	rr := httptest.NewRecorder()
	handler(rr, httptest.NewRequest(http.MethodGet, "/x", nil))
	if !called {
		t.Fatalf("handler was not called")
	}
	if rr.Code != http.StatusAccepted {
		t.Fatalf("unexpected status: %d", rr.Code)
	}
	if snap := metrics.SnapshotStats(); snap.TotalRequests != 1 {
		t.Fatalf("expected one recorded request, got %+v", snap)
	}
}

func TestNewMuxAndServer(t *testing.T) {
	mux := newMux()
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("health through mux status: %d body=%s", rr.Code, rr.Body.String())
	}

	rel := httptest.NewRecorder()
	mux.ServeHTTP(rel, httptest.NewRequest(http.MethodGet, "/rel/schema/missing", nil))
	if rel.Code != http.StatusNotFound {
		t.Fatalf("rel schema through mux status: %d body=%s", rel.Code, rel.Body.String())
	}

	server := newServer(1234, mux)
	if server.Addr != ":1234" {
		t.Fatalf("unexpected addr: %s", server.Addr)
	}
	if server.Handler != mux {
		t.Fatalf("unexpected handler")
	}
	if server.ReadTimeout != 10*time.Second || server.WriteTimeout != 10*time.Second || server.IdleTimeout != 120*time.Second {
		t.Fatalf("unexpected timeouts: %+v", server)
	}
	if server.MaxHeaderBytes != 1<<20 {
		t.Fatalf("unexpected max header bytes: %d", server.MaxHeaderBytes)
	}
}

func TestServePublicAPIReportsListenError(t *testing.T) {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port
	if port == 0 {
		t.Fatalf("unexpected port from %s", listener.Addr())
	}
	second, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err == nil {
		_ = second.Close()
		t.Fatalf("expected reserved port %d to reject a second listener", port)
	}
	if err := servePublicAPI(port); err == nil {
		t.Fatalf("expected listen error")
	}
}
