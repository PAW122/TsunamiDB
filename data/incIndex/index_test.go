package incindex

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var (
	defaultMkdirAll    = mkdirAll
	defaultReadFile    = readFile
	defaultWriteFile   = writeFile
	defaultRenameFile  = renameFile
	defaultRemoveFile  = removeFile
	defaultMarshalJSON = marshalJSON
)

func TestMain(m *testing.M) {
	_ = os.RemoveAll("db")
	code := m.Run()
	ResetForTests()
	_ = os.RemoveAll("db")
	os.Exit(code)
}

func resetState(t *testing.T) {
	t.Helper()
	restoreHooks()
	ResetForTests()
	t.Cleanup(func() {
		restoreHooks()
		ResetForTests()
	})
}

func restoreHooks() {
	mkdirAll = defaultMkdirAll
	readFile = defaultReadFile
	writeFile = defaultWriteFile
	renameFile = defaultRenameFile
	removeFile = defaultRemoveFile
	marshalJSON = defaultMarshalJSON
}

func testTable(t *testing.T, suffix string) string {
	t.Helper()
	replacer := strings.NewReplacer("/", "_", "\\", "_", " ", "_")
	name := strings.ToLower(replacer.Replace(t.Name()))
	table := name + "_" + suffix + ".tbl"
	t.Cleanup(func() {
		restoreHooks()
		_ = DropTable(table)
		_ = os.Remove(filepath.Join(baseDir, table+".idx"))
		_ = os.Remove(filepath.Join(baseDir, table+".idx.tmp"))
	})
	return table
}

func mustLookup(t *testing.T, table, key string) (uint64, bool) {
	t.Helper()
	pos, ok, err := Lookup(table, key)
	if err != nil {
		t.Fatalf("Lookup(%q) returned error: %v", key, err)
	}
	return pos, ok
}

func TestSetLookupPersistenceAndDrop(t *testing.T) {
	resetState(t)
	table := testTable(t, "set_lookup")

	if err := Set(table, 2, "gamma"); err != nil {
		t.Fatalf("Set gamma: %v", err)
	}
	if err := Set(table, 0, "alpha"); err != nil {
		t.Fatalf("Set alpha: %v", err)
	}
	if err := Set(table, 2, "gamma"); err != nil {
		t.Fatalf("Set same key at same position: %v", err)
	}

	if pos, ok := mustLookup(t, table, "gamma"); !ok || pos != 2 {
		t.Fatalf("gamma lookup = (%d, %v), want (2, true)", pos, ok)
	}
	if _, ok := mustLookup(t, table, "missing"); ok {
		t.Fatal("missing key unexpectedly found")
	}

	indices.Delete(table)
	if pos, ok := mustLookup(t, table, "alpha"); !ok || pos != 0 {
		t.Fatalf("persisted alpha lookup = (%d, %v), want (0, true)", pos, ok)
	}

	if err := DropTable(table); err != nil {
		t.Fatalf("DropTable existing index: %v", err)
	}
	if err := DropTable(table); err != nil {
		t.Fatalf("DropTable missing index should be ignored: %v", err)
	}
}

func TestInsertShiftsPositionsAndRejectsDuplicates(t *testing.T) {
	resetState(t)
	table := testTable(t, "insert")

	if err := Insert(table, 0, ""); err != nil {
		t.Fatalf("Insert empty key: %v", err)
	}
	if err := Set(table, 0, "alpha"); err != nil {
		t.Fatalf("Set alpha: %v", err)
	}
	if err := Set(table, 1, "charlie"); err != nil {
		t.Fatalf("Set charlie: %v", err)
	}
	if err := Insert(table, 1, "bravo"); err != nil {
		t.Fatalf("Insert bravo: %v", err)
	}

	for key, want := range map[string]uint64{"alpha": 0, "bravo": 1, "charlie": 2} {
		if pos, ok := mustLookup(t, table, key); !ok || pos != want {
			t.Fatalf("%s lookup = (%d, %v), want (%d, true)", key, pos, ok, want)
		}
	}

	if err := Insert(table, 1, "alpha"); !errors.Is(err, ErrDuplicateKey) {
		t.Fatalf("Insert duplicate error = %v, want ErrDuplicateKey", err)
	}
}

func TestSetReplaceAndRejectDuplicate(t *testing.T) {
	resetState(t)
	table := testTable(t, "replace")

	if err := Set(table, 0, ""); err != nil {
		t.Fatalf("Set empty key: %v", err)
	}
	if err := Set(table, 0, "alpha"); err != nil {
		t.Fatalf("Set alpha: %v", err)
	}
	if err := Set(table, 1, "bravo"); err != nil {
		t.Fatalf("Set bravo: %v", err)
	}
	if err := Set(table, 0, "charlie"); err != nil {
		t.Fatalf("replace alpha with charlie: %v", err)
	}
	if _, ok := mustLookup(t, table, "alpha"); ok {
		t.Fatal("old key alpha should not remain after replacement")
	}
	if pos, ok := mustLookup(t, table, "charlie"); !ok || pos != 0 {
		t.Fatalf("charlie lookup = (%d, %v), want (0, true)", pos, ok)
	}
	if err := Set(table, 1, "charlie"); !errors.Is(err, ErrDuplicateKey) {
		t.Fatalf("Set duplicate error = %v, want ErrDuplicateKey", err)
	}
}

func TestRemoveClearsKeysAndToleratesMissing(t *testing.T) {
	resetState(t)
	table := testTable(t, "remove")

	if err := Remove(table, ""); err != nil {
		t.Fatalf("Remove empty key: %v", err)
	}
	if err := Remove(table, "missing"); err != nil {
		t.Fatalf("Remove missing key: %v", err)
	}
	if err := Set(table, 1, "bravo"); err != nil {
		t.Fatalf("Set bravo: %v", err)
	}
	if err := Remove(table, "bravo"); err != nil {
		t.Fatalf("Remove bravo: %v", err)
	}
	if _, ok := mustLookup(t, table, "bravo"); ok {
		t.Fatal("removed key bravo should not be found")
	}

	idx, err := getIndex(table)
	if err != nil {
		t.Fatalf("getIndex: %v", err)
	}
	idx.mu.Lock()
	idx.positions["ghost"] = 99
	idx.mu.Unlock()
	if err := Remove(table, "ghost"); err != nil {
		t.Fatalf("Remove key beyond keys length: %v", err)
	}
}

func TestLoadHandlesEmptyAndCorruptFiles(t *testing.T) {
	resetState(t)
	table := testTable(t, "load")
	path := filepath.Join(baseDir, table+".idx")

	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		t.Fatalf("mkdir baseDir: %v", err)
	}
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatalf("write empty index: %v", err)
	}
	if _, ok := mustLookup(t, table, "alpha"); ok {
		t.Fatal("empty index should not contain alpha")
	}

	indices.Delete(table)
	if err := os.WriteFile(path, []byte("{"), 0o644); err != nil {
		t.Fatalf("write corrupt index: %v", err)
	}
	if _, _, err := Lookup(table, "alpha"); err == nil {
		t.Fatal("Lookup should fail on corrupt JSON")
	}

	indices.Delete(table)
	if err := os.WriteFile(path, []byte(`{"keys":["alpha","alpha"]}`), 0o644); err != nil {
		t.Fatalf("write duplicate index: %v", err)
	}
	if _, _, err := Lookup(table, "alpha"); !errors.Is(err, ErrDuplicateKey) {
		t.Fatalf("Lookup duplicate index error = %v, want ErrDuplicateKey", err)
	}
}

func TestSaveErrorsAreReturned(t *testing.T) {
	resetState(t)
	table := testTable(t, "save_errors")
	sentinel := errors.New("sentinel")

	marshalJSON = func(v any) ([]byte, error) {
		return nil, sentinel
	}
	if err := Set(table, 0, "alpha"); !errors.Is(err, sentinel) {
		t.Fatalf("Set marshal error = %v, want sentinel", err)
	}

	restoreHooks()
	indices.Delete(table)
	writeFile = func(name string, data []byte, perm fs.FileMode) error {
		return sentinel
	}
	if err := Set(table, 0, "alpha"); !errors.Is(err, sentinel) {
		t.Fatalf("Set write error = %v, want sentinel", err)
	}

	restoreHooks()
	indices.Delete(table)
	renameFile = func(oldpath, newpath string) error {
		return sentinel
	}
	if err := Set(table, 0, "alpha"); !errors.Is(err, sentinel) {
		t.Fatalf("Set rename error = %v, want sentinel", err)
	}

	restoreHooks()
	indices.Delete(table)
	if err := Set(table, 0, "alpha"); err != nil {
		t.Fatalf("Set initial alpha: %v", err)
	}
	calls := 0
	renameFile = func(oldpath, newpath string) error {
		calls++
		if calls == 1 {
			return sentinel
		}
		return defaultRenameFile(oldpath, newpath)
	}
	if err := Set(table, 0, "bravo"); err != nil {
		t.Fatalf("Set should recover from first rename failure by replacing target: %v", err)
	}
	if calls != 2 {
		t.Fatalf("renameFile calls = %d, want 2", calls)
	}

	restoreHooks()
	indices.Delete(table)
	if err := Set(table, 0, "alpha"); err != nil {
		t.Fatalf("Set initial alpha before remove error: %v", err)
	}
	removeSentinel := errors.New("remove sentinel")
	renameFile = func(oldpath, newpath string) error {
		return sentinel
	}
	removeFile = func(name string) error {
		return removeSentinel
	}
	if err := Set(table, 0, "bravo"); !errors.Is(err, removeSentinel) {
		t.Fatalf("Set remove-before-retry error = %v, want remove sentinel", err)
	}
}

func TestOperationLoadErrorsAreReturned(t *testing.T) {
	resetState(t)
	sentinel := errors.New("sentinel")

	mkdirAll = func(path string, perm fs.FileMode) error {
		return sentinel
	}
	if err := Insert(testTable(t, "insert_load_error"), 0, "alpha"); !errors.Is(err, sentinel) {
		t.Fatalf("Insert load error = %v, want sentinel", err)
	}

	if err := Set(testTable(t, "set_load_error"), 0, "alpha"); !errors.Is(err, sentinel) {
		t.Fatalf("Set load error = %v, want sentinel", err)
	}

	if _, _, err := Lookup(testTable(t, "lookup_load_error"), "alpha"); !errors.Is(err, sentinel) {
		t.Fatalf("Lookup load error = %v, want sentinel", err)
	}

	if err := Remove(testTable(t, "remove_load_error"), "alpha"); !errors.Is(err, sentinel) {
		t.Fatalf("Remove load error = %v, want sentinel", err)
	}

	restoreHooks()
	readFile = func(name string) ([]byte, error) {
		return nil, sentinel
	}
	if _, _, err := Lookup(testTable(t, "read_error"), "alpha"); !errors.Is(err, sentinel) {
		t.Fatalf("Lookup read error = %v, want sentinel", err)
	}
}

func TestDropTableReturnsUnexpectedRemoveError(t *testing.T) {
	resetState(t)
	table := testTable(t, "drop_error")
	sentinel := errors.New("sentinel")

	removeFile = func(name string) error {
		return sentinel
	}
	if err := DropTable(table); !errors.Is(err, sentinel) {
		t.Fatalf("DropTable error = %v, want sentinel", err)
	}
}

func TestResetForTestsClearsLoadedIndices(t *testing.T) {
	resetState(t)
	table := testTable(t, "reset")

	if err := Set(table, 0, "alpha"); err != nil {
		t.Fatalf("Set alpha: %v", err)
	}
	ResetForTests()

	if _, ok := indices.Load(table); ok {
		t.Fatal("ResetForTests should remove cached table index")
	}
	if _, err := os.Stat(filepath.Join(baseDir, table+".idx")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("index file after ResetForTests stat error = %v, want os.ErrNotExist", err)
	}
}

func TestReindexFromClampsStartToLength(t *testing.T) {
	idx := &tableIndex{
		keys:      []string{"", "alpha"},
		positions: map[string]uint64{"alpha": 42},
	}
	idx.reindexFrom(0)
	if got := idx.positions["alpha"]; got != 1 {
		t.Fatalf("reindexFrom should move alpha to position 1, got %d", got)
	}
	idx.reindexFrom(99)
	if got := idx.positions["alpha"]; got != 1 {
		t.Fatalf("reindexFrom beyond length changed alpha position to %d", got)
	}
}
