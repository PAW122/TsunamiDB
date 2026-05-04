package export

import (
	"bytes"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"hash/fnv"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
	"unsafe"

	dataManager_v2 "github.com/PAW122/TsunamiDB/data/dataManager/v2"
	defrag "github.com/PAW122/TsunamiDB/data/defragmentationManager"
	fileSystem_v1 "github.com/PAW122/TsunamiDB/data/fileSystem/v1"
	incindex "github.com/PAW122/TsunamiDB/data/incIndex"
	encoder_v1 "github.com/PAW122/TsunamiDB/encoding/v1"
	"github.com/PAW122/TsunamiDB/errors"
	networkmanager "github.com/PAW122/TsunamiDB/servers/network-manager"
	"github.com/PAW122/TsunamiDB/types"
	"github.com/gorilla/websocket"
)

func TestMain(m *testing.M) {
	originalWD, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	_ = os.RemoveAll("db")
	tmpWD, err := os.MkdirTemp("", "tsunamidb-lib-export-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.Chdir(tmpWD); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	code := m.Run()
	dataManager_v2.ShutdownWorkersForTests()
	_ = os.Chdir(originalWD)
	_ = os.RemoveAll("db")
	_ = os.RemoveAll(tmpWD)
	os.Exit(code)
}

func testTable(prefix string) string {
	return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
}

func setLocalNetworkManager(t *testing.T) {
	t.Helper()
	networkmanager.SetInstanceForTests(&networkmanager.NetworkManager{ServerIP: "lib-export-test"})
}

func resetStorageForTest(t *testing.T) {
	t.Helper()
	dataManager_v2.ShutdownWorkersForTests()
	dataManager_v2.EnsureDirsForTests()
	fileSystem_v1.ResetForTests()
	incindex.ResetForTests()
	defrag.ResetForTests()
	setLocalNetworkManager(t)
}

func hashKey(data []byte, suffix string) string {
	h := fnv.New64a()
	_, _ = h.Write(data)
	_, _ = h.Write([]byte(suffix))
	return fmt.Sprintf("key_%x", h.Sum64())
}

func FuzzExportRoundTrip(f *testing.F) {
	f.Add([]byte("hello tsunami"), "secret")
	f.Add([]byte{}, "empty-data-key")
	f.Add(bytes.Repeat([]byte{0, 1, 2, 255}, 80), "binary")

	f.Fuzz(func(t *testing.T, data []byte, encryptionKey string) {
		if encryptionKey == "" {
			encryptionKey = "fallback-test-key"
		}
		if len(data) > 4096 {
			data = data[:4096]
		}

		resetStorageForTest(t)
		table := "lib_export_fuzz_roundtrip"
		plainKey := hashKey(data, encryptionKey+"_plain")
		encryptedKey := hashKey(data, encryptionKey+"_encrypted")

		if err := Save(plainKey, table, data); err != nil {
			t.Fatalf("Save() error = %v", err)
		}
		got, err := Read(plainKey, table)
		if err != nil {
			t.Fatalf("Read() error = %v", err)
		}
		if !bytes.Equal(got, data) {
			t.Fatalf("Read() = %q, want %q", got, data)
		}

		updated := append(append([]byte(nil), data...), byte(len(data)))
		if err := Save(plainKey, table, updated); err != nil {
			t.Fatalf("second Save() error = %v", err)
		}
		got, err = Read(plainKey, table)
		if err != nil {
			t.Fatalf("Read() after overwrite error = %v", err)
		}
		if !bytes.Equal(got, updated) {
			t.Fatalf("Read() after overwrite = %q, want %q", got, updated)
		}
		if err := Free(plainKey, table); err != nil {
			t.Fatalf("Free() error = %v", err)
		}
		if _, err := Read(plainKey, table); err == nil {
			t.Fatal("Read() after Free() error = nil, want an error")
		}

		if err := SaveEncrypted(encryptedKey, table, encryptionKey, data); err != nil {
			t.Fatalf("SaveEncrypted() error = %v", err)
		}
		got, err = ReadEncrypted(encryptedKey, table, encryptionKey)
		if err != nil {
			t.Fatalf("ReadEncrypted() error = %v", err)
		}
		if !bytes.Equal(got, data) {
			t.Fatalf("ReadEncrypted() = %q, want %q", got, data)
		}
		if err := SaveEncrypted(encryptedKey, table, encryptionKey, updated); err != nil {
			t.Fatalf("second SaveEncrypted() error = %v", err)
		}
		got, err = ReadEncrypted(encryptedKey, table, encryptionKey)
		if err != nil {
			t.Fatalf("ReadEncrypted() after overwrite error = %v", err)
		}
		if !bytes.Equal(got, updated) {
			t.Fatalf("ReadEncrypted() after overwrite = %q, want %q", got, updated)
		}
		if _, err := ReadEncrypted(encryptedKey, table, encryptionKey+"_wrong"); err == nil {
			t.Fatal("ReadEncrypted() with wrong key error = nil, want an error")
		}
		if err := Free(encryptedKey, table); err != nil {
			t.Fatalf("Free() encrypted error = %v", err)
		}
		if _, err := ReadEncrypted(encryptedKey, table, encryptionKey); err == nil {
			t.Fatal("ReadEncrypted() after Free() error = nil, want an error")
		}
	})
}

func TestSaveRejectsEmptyKeyOrTable(t *testing.T) {
	resetStorageForTest(t)

	if err := Save("", "table", []byte("data")); err == nil {
		t.Fatal("Save() with empty key error = nil, want an error")
	}
	if err := Save("key", "", []byte("data")); err == nil {
		t.Fatal("Save() with empty table error = nil, want an error")
	}
}

func TestSavePropagatesDependencyErrors(t *testing.T) {
	resetStorageForTest(t)
	boom := stderrors.New("boom")

	originalSaveData := saveDataToFileAsync
	originalSaveElement := saveElementByKey
	t.Cleanup(func() {
		saveDataToFileAsync = originalSaveData
		saveElementByKey = originalSaveElement
	})

	saveDataToFileAsync = func([]byte, string) (int64, int64, error) {
		return 0, 0, boom
	}
	if err := Save("key", "table", []byte("data")); !stderrors.Is(err, boom) {
		t.Fatalf("Save() storage error = %v, want %v", err, boom)
	}
	saveDataToFileAsync = originalSaveData

	saveElementByKey = func(string, string, int, int, bool) (fileSystem_v1.GetElement_output, bool, error) {
		return fileSystem_v1.GetElement_output{}, false, boom
	}

	if err := Save("key", testTable("lib_export_save_map_error"), []byte("data")); !stderrors.Is(err, boom) {
		t.Fatalf("Save() map error = %v, want %v", err, boom)
	}
}

func TestSaveExistingSameSpanRecordsDefragSkip(t *testing.T) {
	resetStorageForTest(t)

	table := testTable("lib_export_plain_skip")
	key := "same_span"
	data := []byte("same-span-data")
	encoded, _ := encoder_v1.Encode(data, false)
	if _, _, err := fileSystem_v1.SaveElementByKey(table, key, 0, len(encoded), false); err != nil {
		t.Fatalf("preload metadata error = %v", err)
	}

	if err := Save(key, table, data); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	got, err := Read(key, table)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("Read() = %q, want %q", got, data)
	}
}

func TestSaveEncryptedErrorsAndSameSpanSkip(t *testing.T) {
	resetStorageForTest(t)

	if err := SaveEncrypted("", "table", "secret", []byte("data")); err == nil {
		t.Fatal("SaveEncrypted() with empty key error = nil, want an error")
	}
	if err := SaveEncrypted("key", "", "secret", []byte("data")); err == nil {
		t.Fatal("SaveEncrypted() with empty table error = nil, want an error")
	}
	if err := SaveEncrypted("key", "table", "", []byte("data")); err == nil {
		t.Fatal("SaveEncrypted() with empty encryption key error = nil, want an error")
	}

	boom := stderrors.New("boom")
	originalSaveData := saveDataToFileAsync
	originalSaveElement := saveElementByKey
	t.Cleanup(func() {
		saveDataToFileAsync = originalSaveData
		saveElementByKey = originalSaveElement
	})

	saveDataToFileAsync = func([]byte, string) (int64, int64, error) {
		return 0, 0, boom
	}
	if err := SaveEncrypted("key", "table", "secret", []byte("data")); !stderrors.Is(err, boom) {
		t.Fatalf("SaveEncrypted() storage error = %v, want %v", err, boom)
	}
	saveDataToFileAsync = originalSaveData

	saveElementByKey = func(string, string, int, int, bool) (fileSystem_v1.GetElement_output, bool, error) {
		return fileSystem_v1.GetElement_output{}, false, boom
	}
	if err := SaveEncrypted("key", testTable("lib_export_encrypted_map_error"), "secret", []byte("data")); !stderrors.Is(err, boom) {
		t.Fatalf("SaveEncrypted() map error = %v, want %v", err, boom)
	}
	saveElementByKey = originalSaveElement

	table := testTable("lib_export_encrypted_skip")
	key := "same_span_encrypted"
	data := []byte("encrypted-same-span-data")
	encrypted, err := encoder_v1.Encrypt(data, "secret")
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}
	encoded, _ := encoder_v1.Encode(encrypted, false)
	if _, _, err := fileSystem_v1.SaveElementByKey(table, key, 0, len(encoded), false); err != nil {
		t.Fatalf("preload encrypted metadata error = %v", err)
	}

	if err := SaveEncrypted(key, table, "secret", data); err != nil {
		t.Fatalf("SaveEncrypted() error = %v", err)
	}
	got, err := ReadEncrypted(key, table, "secret")
	if err != nil {
		t.Fatalf("ReadEncrypted() error = %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("ReadEncrypted() = %q, want %q", got, data)
	}
}

func TestReadRequiresNetworkManager(t *testing.T) {
	resetStorageForTest(t)
	networkmanager.SetInstanceForTests(nil)

	if _, err := Read("missing", "table"); err == nil {
		t.Fatal("Read() without network manager error = nil, want an error")
	}
	if _, err := ReadEncrypted("missing", "table", "secret"); err == nil {
		t.Fatal("ReadEncrypted() without network manager error = nil, want an error")
	}
}

func TestReadMissingReturnsNotFound(t *testing.T) {
	resetStorageForTest(t)

	_, err := Read("missing", "table")
	if err != errors.ErrNotFound {
		t.Fatalf("Read() missing error = %v, want %v", err, errors.ErrNotFound)
	}
	if _, err := ReadEncrypted("missing", "table", "secret"); err == nil {
		t.Fatal("ReadEncrypted() missing error = nil, want an error")
	}
}

func TestReadReturnsLocalStorageErrors(t *testing.T) {
	resetStorageForTest(t)

	boom := stderrors.New("boom")
	originalGetElement := getElementByKey
	originalReadAsync := readDataFromFileAsync
	getElementByKey = func(string, string) (*fileSystem_v1.GetElement_output, error) {
		return &fileSystem_v1.GetElement_output{Key: "plain", FileName: "table", StartPtr: 0, EndPtr: 1}, nil
	}
	readDataFromFileAsync = func(string, int64, int64) ([]byte, error) {
		return nil, boom
	}
	if _, err := Read("plain", "table"); !stderrors.Is(err, boom) {
		t.Fatalf("Read() storage error = %v, want %v", err, boom)
	}
	getElementByKey = originalGetElement
	readDataFromFileAsync = originalReadAsync
	t.Cleanup(func() {
		getElementByKey = originalGetElement
		readDataFromFileAsync = originalReadAsync
	})

	table := testTable("lib_export_local_error")
	if _, _, err := fileSystem_v1.SaveElementByKey(table, "encrypted", 1, 2, false); err != nil {
		t.Fatalf("preload metadata error = %v", err)
	}

	if _, err := ReadEncrypted("encrypted", table, "secret"); err == nil {
		t.Fatal("ReadEncrypted() with stale metadata error = nil, want an error")
	}
}

func TestReadFallsBackToNetworkResult(t *testing.T) {
	resetStorageForTest(t)

	payload := []byte("remote-data")
	withPeerNetworkManager(t, func(req types.NMmessage) types.NMmessage {
		return types.NMmessage{
			Task:      req.Task,
			ReqSendBy: req.ReqSendBy,
			ReqResBy:  "peer",
			Content:   payload,
			Finished:  true,
		}
	})

	got, err := Read("remote-key", "remote-table")
	if err != nil {
		t.Fatalf("Read() remote fallback error = %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("Read() remote fallback = %q, want %q", got, payload)
	}
}

func TestReadEncryptedFallsBackToNetworkResult(t *testing.T) {
	resetStorageForTest(t)

	encryptionKey := "remote-secret"
	payload := []byte("remote-encrypted-data")
	encrypted, err := encoder_v1.Encrypt(payload, encryptionKey)
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}
	withPeerNetworkManager(t, func(req types.NMmessage) types.NMmessage {
		return types.NMmessage{
			Task:      req.Task,
			ReqSendBy: req.ReqSendBy,
			ReqResBy:  "peer",
			Content:   encrypted,
			Finished:  true,
		}
	})

	got, err := ReadEncrypted("remote-key", "remote-table", encryptionKey)
	if err != nil {
		t.Fatalf("ReadEncrypted() remote fallback error = %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("ReadEncrypted() remote fallback = %q, want %q", got, payload)
	}
}

func TestReadEncryptedRemoteDecryptError(t *testing.T) {
	resetStorageForTest(t)

	withPeerNetworkManager(t, func(req types.NMmessage) types.NMmessage {
		return types.NMmessage{
			Task:      req.Task,
			ReqSendBy: req.ReqSendBy,
			ReqResBy:  "peer",
			Content:   []byte("not-ciphertext"),
			Finished:  true,
		}
	})

	if _, err := ReadEncrypted("remote-key", "remote-table", "secret"); err == nil {
		t.Fatal("ReadEncrypted() remote decrypt error = nil, want an error")
	}
}

func TestSaveReadIncModes(t *testing.T) {
	resetStorageForTest(t)

	table := testTable("lib_export_inc")
	key := "events"

	first, err := SaveInc(key, table, []byte("first"), SaveIncOptions{MaxEntrySize: 16, EntryKey: "first-key"})
	if err != nil {
		t.Fatalf("SaveInc(first) error = %v", err)
	}
	if first.ID != 0 {
		t.Fatalf("SaveInc(first) id = %d, want 0", first.ID)
	}
	second, err := SaveInc(key, table, []byte("second"), SaveIncOptions{MaxEntrySize: 99, EntryKey: "second-key"})
	if err != nil {
		t.Fatalf("SaveInc(second) error = %v", err)
	}
	if second.ID != 1 {
		t.Fatalf("SaveInc(second) id = %d, want 1", second.ID)
	}
	if second.Warning == "" {
		t.Fatal("SaveInc with mismatched existing max size warning is empty")
	}

	byID, err := ReadInc(key, table, ReadIncOptions{Type: ReadIncByID, ID: 0})
	if err != nil {
		t.Fatalf("ReadInc(by_id) error = %v", err)
	}
	assertIncEntries(t, byID, IncEntry{ID: 0, Data: []byte("first")})

	byKey, err := ReadInc(key, table, ReadIncOptions{Type: ReadIncByKey, EntryKey: "second-key"})
	if err != nil {
		t.Fatalf("ReadInc(by_key) error = %v", err)
	}
	assertIncEntries(t, byKey, IncEntry{ID: 1, Data: []byte("second")})

	insertAt := uint64(1)
	inserted, err := SaveInc(key, table, []byte("middle"), SaveIncOptions{
		ID:        &insertAt,
		CountFrom: IncCountFromBottom,
		EntryKey:  "middle-key",
	})
	if err != nil {
		t.Fatalf("SaveInc(insert) error = %v", err)
	}
	if inserted.ID != 1 {
		t.Fatalf("SaveInc(insert) id = %d, want 1", inserted.ID)
	}

	overwriteID := uint64(0)
	overwritten, err := SaveInc(key, table, []byte("FIRST"), SaveIncOptions{
		ID:        &overwriteID,
		Mode:      SaveIncModeOverwrite,
		CountFrom: IncCountFromBottom,
		EntryKey:  "first-key-updated",
	})
	if err != nil {
		t.Fatalf("SaveInc(overwrite) error = %v", err)
	}
	if overwritten.ID != 0 {
		t.Fatalf("SaveInc(overwrite) id = %d, want 0", overwritten.ID)
	}

	firstEntries, err := ReadInc(key, table, ReadIncOptions{Type: ReadIncFirstEntries, Amount: 3})
	if err != nil {
		t.Fatalf("ReadInc(first_entries) error = %v", err)
	}
	assertIncEntries(t, firstEntries,
		IncEntry{ID: 0, Data: []byte("FIRST")},
		IncEntry{ID: 1, Data: []byte("middle")},
		IncEntry{ID: 2, Data: []byte("second")},
	)

	lastEntries, err := ReadInc(key, table, ReadIncOptions{Type: ReadIncLastEntries, Amount: 2})
	if err != nil {
		t.Fatalf("ReadInc(last_entries) error = %v", err)
	}
	assertIncEntries(t, lastEntries,
		IncEntry{ID: 2, Data: []byte("second")},
		IncEntry{ID: 1, Data: []byte("middle")},
	)

	updatedByKey, err := ReadInc(key, table, ReadIncOptions{Type: ReadIncByKey, EntryKey: "first-key-updated"})
	if err != nil {
		t.Fatalf("ReadInc(updated by_key) error = %v", err)
	}
	assertIncEntries(t, updatedByKey, IncEntry{ID: 0, Data: []byte("FIRST")})

	if _, err := ReadInc(key, table, ReadIncOptions{Type: ReadIncByKey, EntryKey: "first-key"}); err == nil {
		t.Fatal("ReadInc(old entry_key) error = nil, want an error")
	}
}

func TestSaveReadIncValidation(t *testing.T) {
	resetStorageForTest(t)

	if _, err := SaveInc("", "table", []byte("x"), SaveIncOptions{MaxEntrySize: 1}); err == nil {
		t.Fatal("SaveInc with empty key error = nil, want an error")
	}
	if _, err := SaveInc("key", "", []byte("x"), SaveIncOptions{MaxEntrySize: 1}); err == nil {
		t.Fatal("SaveInc with empty table error = nil, want an error")
	}
	if _, err := SaveInc("key", "table", []byte("x"), SaveIncOptions{}); err == nil {
		t.Fatal("SaveInc new table without max size error = nil, want an error")
	}
	if _, err := SaveInc("key", "table", []byte("too-big"), SaveIncOptions{MaxEntrySize: 3}); err == nil {
		t.Fatal("SaveInc body larger than max size error = nil, want an error")
	}
	if _, err := ReadInc("key", "table", ReadIncOptions{Type: "bad"}); err == nil {
		t.Fatal("ReadInc bad type error = nil, want an error")
	}
}

func assertIncEntries(t *testing.T, got []IncEntry, want ...IncEntry) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("inc entries len = %d, want %d; got=%#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i].ID != want[i].ID || !bytes.Equal(got[i].Data, want[i].Data) {
			t.Fatalf("inc entry[%d] = {ID:%d Data:%q}, want {ID:%d Data:%q}", i, got[i].ID, got[i].Data, want[i].ID, want[i].Data)
		}
	}
}

func TestFreeMissingReturnsError(t *testing.T) {
	resetStorageForTest(t)

	if err := Free("missing", "table"); err == nil {
		t.Fatal("Free() missing key error = nil, want an error")
	}
}

func withPeerNetworkManager(t *testing.T, responder func(types.NMmessage) types.NMmessage) {
	t.Helper()

	type wireFrame struct {
		Version int             `json:"version"`
		Type    string          `json:"type"`
		NodeID  string          `json:"node_id,omitempty"`
		Message types.NMmessage `json:"message,omitempty"`
	}

	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			_, raw, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var frame wireFrame
			if err := json.Unmarshal(raw, &frame); err != nil {
				return
			}
			if frame.Type != "request" {
				continue
			}
			res := responder(frame.Message)
			res.RequestID = frame.Message.RequestID
			res.Task = frame.Message.Task
			res.ReqSendBy = frame.Message.ReqSendBy
			if err := conn.WriteJSON(wireFrame{Version: 1, Type: "response", NodeID: "peer", Message: res}); err != nil {
				return
			}
		}
	}))
	t.Cleanup(server.Close)

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial test websocket peer: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	nm := &networkmanager.NetworkManager{ServerIP: "lib-export-test"}
	setUnexportedField(nm, "peers", map[string]*networkmanager.Peer{
		"peer": {Conn: conn, Address: "peer", LastActive: time.Now()},
	})
	networkmanager.SetInstanceForTests(nm)

	go func() {
		for {
			var frame wireFrame
			if err := conn.ReadJSON(&frame); err != nil {
				return
			}
			if frame.Type == "response" {
				nm.HandleResponse(frame.Message)
			}
		}
	}()
}

func setUnexportedField(target any, name string, value any) {
	field := reflect.ValueOf(target).Elem().FieldByName(name)
	reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().Set(reflect.ValueOf(value))
}
