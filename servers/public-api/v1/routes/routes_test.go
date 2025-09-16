package routes

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	dataManager_v2 "github.com/PAW122/TsunamiDB/data/dataManager/v2"
	defrag "github.com/PAW122/TsunamiDB/data/defragmentationManager"
	fileSystem_v1 "github.com/PAW122/TsunamiDB/data/fileSystem/v1"
	incindex "github.com/PAW122/TsunamiDB/data/incIndex"
	networkmanager "github.com/PAW122/TsunamiDB/servers/network-manager"
	metrics "github.com/PAW122/TsunamiDB/servers/public-api/v1/metrics"
)

func setupRoutesTest(t *testing.T) {
	t.Helper()
	dataManager_v2.ShutdownWorkersForTests()
	fileSystem_v1.ResetForTests()
	defrag.ResetForTests()
	_ = os.RemoveAll("./db/data")
	_ = os.RemoveAll("./db/inc_tables")
	dataManager_v2.EnsureDirsForTests()
	networkmanager.SetInstanceForTests(&networkmanager.NetworkManager{ServerIP: "127.0.0.1"})
	metrics.ResetForTests()
	incindex.ResetForTests()
	t.Cleanup(func() {
		dataManager_v2.ShutdownWorkersForTests()
		fileSystem_v1.ResetForTests()
		defrag.ResetForTests()
		networkmanager.SetInstanceForTests(nil)
		metrics.ResetForTests()
		incindex.ResetForTests()
		_ = os.RemoveAll("./db/data")
		_ = os.RemoveAll("./db/inc_tables")
	})
}

func perform(handler func(http.ResponseWriter, *http.Request, *http.Client), method, path string, body io.Reader, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, body)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rr := httptest.NewRecorder()
	handler(rr, req, http.DefaultClient)
	return rr
}

func TestSaveAndReadEndpoints(t *testing.T) {
	setupRoutesTest(t)

	saveBody := bytes.NewBufferString("hello-world")
	resp := perform(AsyncSave, http.MethodPost, "/save/table/key", saveBody, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("save status: %d body=%s", resp.Code, resp.Body.String())
	}

	readResp := perform(AsyncRead, http.MethodGet, "/read/table/key", nil, nil)
	if readResp.Code != http.StatusOK {
		t.Fatalf("read status: %d body=%s", readResp.Code, readResp.Body.String())
	}
	if body := readResp.Body.String(); body != "hello-world" {
		t.Fatalf("unexpected read body: %s", body)
	}
}

func TestSaveValidation(t *testing.T) {
	setupRoutesTest(t)
	resp := perform(AsyncSave, http.MethodGet, "/save/table/key", nil, nil)
	if resp.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", resp.Code)
	}

	resp = perform(AsyncSave, http.MethodPost, "/save/table", bytes.NewBufferString(""), nil)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for short path, got %d", resp.Code)
	}
}

func TestFreeRemovesEntry(t *testing.T) {
	setupRoutesTest(t)
	perform(AsyncSave, http.MethodPost, "/save/table/key", bytes.NewBufferString("payload"), nil)
	resp := perform(Free, http.MethodGet, "/free/table/key", nil, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("free status: %d", resp.Code)
	}
	readResp := perform(AsyncRead, http.MethodGet, "/read/table/key", nil, nil)
	if readResp.Code != http.StatusNotFound {
		t.Fatalf("expected 404 after free, got %d", readResp.Code)
	}
}

func TestSaveEncryptedAndRead(t *testing.T) {
	setupRoutesTest(t)
	headers := map[string]string{"encryption_key": "secret"}
	resp := perform(SaveEncrypted, http.MethodPost, "/save_encrypted/table/key", bytes.NewBufferString("top-secret"), headers)
	if resp.Code != http.StatusOK {
		t.Fatalf("save_encrypted status: %d body=%s", resp.Code, resp.Body.String())
	}

	readResp := perform(ReadEncrypted, http.MethodGet, "/read_encrypted/table/key", nil, headers)
	if readResp.Code != http.StatusOK {
		t.Fatalf("read_encrypted status: %d body=%s", readResp.Code, readResp.Body.String())
	}
	if readResp.Body.String() != "top-secret" {
		t.Fatalf("unexpected decrypted body: %s", readResp.Body.String())
	}
}

func TestReadEncryptedValidations(t *testing.T) {
	setupRoutesTest(t)
	resp := perform(ReadEncrypted, http.MethodGet, "/read_encrypted/table/key", nil, nil)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 missing header, got %d", resp.Code)
	}
}

func TestSaveIncAndRead(t *testing.T) {
	setupRoutesTest(t)
	headers := map[string]string{"max_entry_size": "16"}
	resp := perform(SaveIncremental, http.MethodPost, "/save_inc/table/key", bytes.NewBufferString("first"), headers)
	if resp.Code != http.StatusOK {
		t.Fatalf("save_inc status: %d body=%s", resp.Code, resp.Body.String())
	}
	var out map[string]string
	if err := json.Unmarshal(resp.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode id: %v", err)
	}
	if out["id"] != "0" {
		t.Fatalf("expected id 0, got %s", out["id"])
	}

	readHeaders := map[string]string{"read_type": "by_id", "id": "0"}
	readResp := perform(ReadIncremental, http.MethodGet, "/read_inc/table/key", nil, readHeaders)
	if readResp.Code != http.StatusOK {
		t.Fatalf("read_inc status: %d", readResp.Code)
	}
	if !bytes.Contains(readResp.Body.Bytes(), []byte("first")) {
		t.Fatalf("unexpected read_inc body: %s", readResp.Body.String())
	}
}

func TestSaveIncValidation(t *testing.T) {
	setupRoutesTest(t)
	resp := perform(SaveIncremental, http.MethodPost, "/save_inc/table/key", bytes.NewBufferString("bad"), nil)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing header, got %d", resp.Code)
	}
}

func TestDeleteIncEndpoint(t *testing.T) {
	setupRoutesTest(t)
	headers := map[string]string{"max_entry_size": "16"}
	perform(SaveIncremental, http.MethodPost, "/save_inc/table/key", bytes.NewBufferString("first"), headers)
	resp := perform(DeleteIncremental, http.MethodGet, "/delete_inc/table/key", nil, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("delete_inc status: %d body=%s", resp.Code, resp.Body.String())
	}
}

func TestReadMissingKey(t *testing.T) {
	setupRoutesTest(t)
	resp := perform(AsyncRead, http.MethodGet, "/read/table/missing", nil, nil)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.Code)
	}
}

func TestHealthEndpoint(t *testing.T) {
	setupRoutesTest(t)

	resp := perform(Health, http.MethodGet, "/health", nil, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("health status: %d body=%s", resp.Code, resp.Body.String())
	}

	var body struct {
		Status string `json:"status"`
		API    struct {
			UptimeSeconds     float64 `json:"uptime_seconds"`
			TotalRequests     uint64  `json:"total_requests"`
			AverageResponseMS float64 `json:"average_response_ms"`
			LastRequestAt     string  `json:"last_request_at"`
		} `json:"api"`
		Subscriptions struct {
			ActiveClients int `json:"active_clients"`
		} `json:"subscriptions"`
		Network *struct {
			ServerIP string `json:"server_ip"`
		} `json:"network"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode health: %v", err)
	}

	if body.Status != "ok" {
		t.Fatalf("unexpected health status: %s", body.Status)
	}
	if body.API.TotalRequests != 0 {
		t.Fatalf("expected zero recorded requests, got %d", body.API.TotalRequests)
	}
	if body.API.AverageResponseMS < 0 {
		t.Fatalf("average response negative: %f", body.API.AverageResponseMS)
	}
	if body.Subscriptions.ActiveClients != 0 {
		t.Fatalf("expected zero active clients, got %d", body.Subscriptions.ActiveClients)
	}
	if body.Network == nil {
		t.Fatalf("expected network stats in response")
	}

	method := perform(Health, http.MethodPost, "/health", nil, nil)
	if method.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for POST, got %d", method.Code)
	}
}

func TestIncrementalReadByKey(t *testing.T) {
	setupRoutesTest(t)

	basePath := "/save_inc/table/key"
	headers := map[string]string{"max_entry_size": "16", "entry_key": "alpha"}
	if resp := perform(SaveIncremental, http.MethodPost, basePath, bytes.NewBufferString("first"), headers); resp.Code != http.StatusOK {
		t.Fatalf("save alpha status: %d body=%s", resp.Code, resp.Body.String())
	}

	headers2 := map[string]string{"entry_key": "beta"}
	if resp := perform(SaveIncremental, http.MethodPost, basePath, bytes.NewBufferString("second"), headers2); resp.Code != http.StatusOK {
		t.Fatalf("save beta status: %d body=%s", resp.Code, resp.Body.String())
	}

	headers3 := map[string]string{"entry_key": "gamma", "id": "1", "mode": "append", "count_from": "bottom"}
	if resp := perform(SaveIncremental, http.MethodPost, basePath, bytes.NewBufferString("middle"), headers3); resp.Code != http.StatusOK {
		t.Fatalf("insert gamma status: %d body=%s", resp.Code, resp.Body.String())
	}

	readPath := "/read_inc/table/key"
	readHeaders := map[string]string{"read_type": "by_key", "entry_key": "gamma"}
	resp := perform(ReadIncremental, http.MethodGet, readPath, nil, readHeaders)
	if resp.Code != http.StatusOK {
		t.Fatalf("read gamma status: %d body=%s", resp.Code, resp.Body.String())
	}
	var body struct{ Data string }
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode gamma: %v", err)
	}
	if body.Data != "middle" {
		t.Fatalf("gamma unexpected data: %s", body.Data)
	}

	for key, want := range map[string]string{"alpha": "first", "beta": "second"} {
		head := map[string]string{"read_type": "by_key", "entry_key": key}
		resp := perform(ReadIncremental, http.MethodGet, readPath, nil, head)
		if resp.Code != http.StatusOK {
			t.Fatalf("read %s status: %d body=%s", key, resp.Code, resp.Body.String())
		}
		var out struct{ Data string }
		if err := json.Unmarshal(resp.Body.Bytes(), &out); err != nil {
			t.Fatalf("decode %s: %v", key, err)
		}
		if out.Data != want {
			t.Fatalf("%s unexpected data: %s", key, out.Data)
		}
	}

	missingHeaders := map[string]string{"read_type": "by_key", "entry_key": "missing"}
	notFound := perform(ReadIncremental, http.MethodGet, readPath, nil, missingHeaders)
	if notFound.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing key, got %d", notFound.Code)
	}
}
