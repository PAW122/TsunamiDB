package routes

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	dataManager_v2 "github.com/PAW122/TsunamiDB/data/dataManager/v2"
	defrag "github.com/PAW122/TsunamiDB/data/defragmentationManager"
	fileSystem_v1 "github.com/PAW122/TsunamiDB/data/fileSystem/v1"
	incindex "github.com/PAW122/TsunamiDB/data/incIndex"
	networkmanager "github.com/PAW122/TsunamiDB/servers/network-manager"
	metrics "github.com/PAW122/TsunamiDB/servers/public-api/v1/metrics"
	types "github.com/PAW122/TsunamiDB/types"
)

func TestMain(m *testing.M) {
	_ = os.RemoveAll("./db")
	code := m.Run()
	dataManager_v2.ShutdownWorkersForTests()
	fileSystem_v1.ResetForTests()
	defrag.ResetForTests()
	networkmanager.SetInstanceForTests(nil)
	metrics.ResetForTests()
	incindex.ResetForTests()
	_ = os.RemoveAll("./db")
	time.Sleep(50 * time.Millisecond)
	_ = os.RemoveAll("./db")
	os.Exit(code)
}

func setupRoutesTest(t *testing.T) {
	t.Helper()
	release := acquireDBTestLock(t)
	t.Cleanup(release)
	dataManager_v2.ShutdownWorkersForTests()
	fileSystem_v1.ResetForTests()
	defrag.ResetForTests()
	_ = os.RemoveAll("./db")
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

func TestShortPathsReturnBadRequest(t *testing.T) {
	setupRoutesTest(t)

	cases := []struct {
		name    string
		handler func(http.ResponseWriter, *http.Request, *http.Client)
		method  string
		path    string
		body    io.Reader
		headers map[string]string
	}{
		{"read", AsyncRead, http.MethodGet, "/read/table", nil, nil},
		{"free", Free, http.MethodGet, "/free/table", nil, nil},
		{"save encrypted", SaveEncrypted, http.MethodPost, "/save_encrypted/table", bytes.NewBufferString("x"), map[string]string{"encryption_key": "secret"}},
		{"read encrypted", ReadEncrypted, http.MethodGet, "/read_encrypted/table", nil, map[string]string{"encryption_key": "secret"}},
		{"save inc", SaveIncremental, http.MethodPost, "/save_inc/table", bytes.NewBufferString("x"), map[string]string{"max_entry_size": "16"}},
		{"read inc", ReadIncremental, http.MethodGet, "/read_inc/table", nil, map[string]string{"read_type": "by_id", "id": "0"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := perform(tc.handler, tc.method, tc.path, tc.body, tc.headers)
			if resp.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d body=%s", resp.Code, resp.Body.String())
			}
		})
	}
}

func TestParseArgsAndPathHeader(t *testing.T) {
	args := ParseArgs("/save/table/key", "save")
	if len(args) != 4 || args[2] != "table" || args[3] != "key" {
		t.Fatalf("unexpected args: %#v", args)
	}

	for _, raw := range []string{`["a.b","c"]`, "a.b,c", "a.b c"} {
		paths, err := parsePathHeader(raw)
		if err != nil {
			t.Fatalf("parse %q: %v", raw, err)
		}
		if len(paths) != 2 {
			t.Fatalf("expected two paths for %q, got %#v", raw, paths)
		}
	}
	if _, err := parsePathHeader("[bad"); err == nil {
		t.Fatalf("expected invalid json error")
	}
}

func TestNestedHelpers(t *testing.T) {
	normalized, entries, err := processNestedPayload([]byte(`{"plain":1,"nested":"@{\"x\":\"y\"}","arr":["@z"]}`))
	if err != nil {
		t.Fatalf("process nested: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected two nested entries, got %d", len(entries))
	}
	var root map[string]interface{}
	if err := json.Unmarshal(normalized, &root); err != nil {
		t.Fatalf("decode normalized: %v", err)
	}
	if !isPointerPlaceholder(root["nested"].(string)) {
		t.Fatalf("expected pointer placeholder: %#v", root["nested"])
	}

	ids, err := pointerIDsFromJSON(normalized)
	if err != nil {
		t.Fatalf("pointer ids: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("expected two pointer ids, got %#v", ids)
	}
	if extractPointerID("plain") != "" {
		t.Fatalf("plain string should not be pointer")
	}

	clone := cloneWithPlaceholders(root).(map[string]interface{})
	if clone["nested"] != pointerUnresolvedMarker {
		t.Fatalf("expected unresolved marker, got %#v", clone["nested"])
	}
	if id, ok := pointerIDAtPath(root, []string{"nested"}); !ok || id == "" {
		t.Fatalf("expected pointer at path, got %q %v", id, ok)
	}
	out := setValueAtPath(map[string]interface{}{}, []string{"a", "b"}, "c").(map[string]interface{})
	if out["a"].(map[string]interface{})["b"] != "c" {
		t.Fatalf("unexpected setValueAtPath output: %#v", out)
	}
	if _, _, err := processNestedPayload([]byte("{bad")); err == nil {
		t.Fatalf("expected invalid JSON error")
	}
}

func TestIncrementalReadModesAndValidation(t *testing.T) {
	setupRoutesTest(t)

	saveHeaders := map[string]string{"max_entry_size": "16"}
	for _, payload := range []string{"first", "second", "third"} {
		resp := perform(SaveIncremental, http.MethodPost, "/save_inc/table_modes/key_modes", bytes.NewBufferString(payload), saveHeaders)
		if resp.Code != http.StatusOK {
			t.Fatalf("save_inc %q status: %d body=%s", payload, resp.Code, resp.Body.String())
		}
	}

	first := perform(ReadIncremental, http.MethodGet, "/read_inc/table_modes/key_modes", nil, map[string]string{"read_type": "first_entries", "amount_to_read": "2"})
	if first.Code != http.StatusOK {
		t.Fatalf("first_entries status: %d body=%s", first.Code, first.Body.String())
	}
	if !bytes.Contains(first.Body.Bytes(), []byte("first")) || !bytes.Contains(first.Body.Bytes(), []byte("second")) {
		t.Fatalf("unexpected first_entries body: %s", first.Body.String())
	}

	last := perform(ReadIncremental, http.MethodGet, "/read_inc/table_modes/key_modes", nil, map[string]string{"read_type": "last_entries", "amount_to_read": "2"})
	if last.Code != http.StatusOK {
		t.Fatalf("last_entries status: %d body=%s", last.Code, last.Body.String())
	}
	if !bytes.Contains(last.Body.Bytes(), []byte("third")) {
		t.Fatalf("unexpected last_entries body: %s", last.Body.String())
	}

	validations := []map[string]string{
		nil,
		{"read_type": "by_id"},
		{"read_type": "by_id", "id": "bad"},
		{"read_type": "last_entries"},
		{"read_type": "last_entries", "amount_to_read": "bad"},
		{"read_type": "by_key"},
		{"read_type": "unsupported"},
	}
	for _, headers := range validations {
		resp := perform(ReadIncremental, http.MethodGet, "/read_inc/table_modes/key_modes", nil, headers)
		if resp.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for headers %#v, got %d body=%s", headers, resp.Code, resp.Body.String())
		}
	}
}

func TestIncrementalBinaryHelpers(t *testing.T) {
	in := types.IncTableEntryData{EntrySize: 12, TableFileName: "table.tbl"}
	raw, err := StructToBytesBinary(in)
	if err != nil {
		t.Fatalf("serialize: %v", err)
	}
	out, err := BytesToStructBinary(raw)
	if err != nil {
		t.Fatalf("deserialize: %v", err)
	}
	if out != in {
		t.Fatalf("roundtrip mismatch: %+v != %+v", out, in)
	}
	if _, err := BytesToStructBinary([]byte{1, 2}); err == nil {
		t.Fatalf("expected short buffer error")
	}
	corrupted := append([]byte{}, raw[:12]...)
	corrupted[8] = 100
	if _, err := BytesToStructBinary(corrupted); err == nil {
		t.Fatalf("expected corrupted payload error")
	}
}

func TestKeyByRegexEndpoint(t *testing.T) {
	setupRoutesTest(t)
	perform(AsyncSave, http.MethodPost, "/save/table/alpha", bytes.NewBufferString("a"), nil)
	perform(AsyncSave, http.MethodPost, "/save/table/beta", bytes.NewBufferString("b"), nil)

	resp := perform(GetKeysByRegex, http.MethodGet, "/key_by_regex/table?regex=^a&max=1", nil, nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("regex status: %d body=%s", resp.Code, resp.Body.String())
	}
	if !bytes.Contains(resp.Body.Bytes(), []byte("alpha")) {
		t.Fatalf("expected alpha in response: %s", resp.Body.String())
	}

	for _, path := range []string{"/key_by_regex/", "/key_by_regex/table", "/key_by_regex/table?regex=*&max=bad"} {
		resp := perform(GetKeysByRegex, http.MethodGet, path, nil, nil)
		if resp.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for %s, got %d", path, resp.Code)
		}
	}
	method := perform(GetKeysByRegex, http.MethodPost, "/key_by_regex/table?regex=.", nil, nil)
	if method.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", method.Code)
	}
}

func TestSQLEndpointAndScript(t *testing.T) {
	setupRoutesTest(t)

	method := perform(SQL_api, http.MethodGet, "/sql", nil, nil)
	if method.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", method.Code)
	}
	resp := perform(SQL_api, http.MethodPost, "/sql", bytes.NewBufferString(`{"query":"noop"}`), nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("sql status: %d body=%s", resp.Code, resp.Body.String())
	}

	rr := httptest.NewRecorder()
	Script(rr, httptest.NewRequest(http.MethodGet, "/script", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("empty script should keep default 200, got %d", rr.Code)
	}
}
