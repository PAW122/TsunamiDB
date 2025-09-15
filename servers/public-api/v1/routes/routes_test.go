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
	networkmanager "github.com/PAW122/TsunamiDB/servers/network-manager"
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
	t.Cleanup(func() {
		dataManager_v2.ShutdownWorkersForTests()
		fileSystem_v1.ResetForTests()
		defrag.ResetForTests()
		networkmanager.SetInstanceForTests(nil)
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
