package fileSystem_v1

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	_ = os.RemoveAll("db")
	code := m.Run()
	ShutdownForTests()
	_ = os.RemoveAll("db")
	os.Exit(code)
}

type fakeDB struct {
	mu      sync.Mutex
	values  map[string]interface{}
	errs    map[string]error
	puts    map[string]interface{}
	putErr  error
	getHits map[string]int
}

func newFakeDB(values map[string]interface{}) *fakeDB {
	return &fakeDB{
		values:  values,
		errs:    make(map[string]error),
		puts:    make(map[string]interface{}),
		getHits: make(map[string]int),
	}
}

func dbSlot(table, key string) string {
	return table + "/" + key
}

func (f *fakeDB) Get(ctx context.Context, table, key string) (interface{}, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	slot := dbSlot(table, key)
	f.getHits[slot]++
	if err := f.errs[slot]; err != nil {
		return nil, err
	}
	v, ok := f.values[slot]
	if !ok {
		return nil, errors.New("not found")
	}
	return v, nil
}

func (f *fakeDB) Put(ctx context.Context, table, key string, value interface{}) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.putErr != nil {
		return f.putErr
	}
	f.puts[dbSlot(table, key)] = value
	return nil
}

func (f *fakeDB) getHitCount(table, key string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.getHits[dbSlot(table, key)]
}

func isolateStorage(t *testing.T) string {
	t.Helper()

	tmp := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}

	oldBaseMapsDir := baseMapsDir
	oldLegacySnapPath := legacySnapPath
	oldLegacyWalPath := legacyWalPath
	oldWalTimeout := walEnqueueTimeout
	oldSnapshotInterval := snapshotInterval
	oldDebugCounterPeriod := debugCounterPeriod
	oldSnapshotWorkerStop := snapshotWorkerStop
	oldDebugCountersStop := debugCountersStop

	baseMapsDir = filepath.Join(".", "db", "maps")
	legacySnapPath = filepath.Join(baseMapsDir, "data_map.snap")
	legacyWalPath = filepath.Join(baseMapsDir, "data_map.wal")
	walEnqueueTimeout = 5 * time.Second
	snapshotInterval = 5 * time.Minute
	debugCounterPeriod = 5 * time.Second
	snapshotWorkerStop = nil
	debugCountersStop = nil

	registryMu.Lock()
	indexRegistry = make(map[string]*tableIndex)
	registryMu.Unlock()
	lastIndexCache.Store((*cachedIndex)(nil))

	if err := os.MkdirAll(baseMapsDir, 0755); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		registryMu.Lock()
		for _, ti := range indexRegistry {
			ti.walMu.Lock()
			if ti.walBuf != nil {
				_ = ti.walBuf.Flush()
			}
			if ti.walFile != nil {
				_ = ti.walFile.Close()
			}
			ti.walBuf = nil
			ti.walFile = nil
			ti.walMu.Unlock()
		}
		indexRegistry = make(map[string]*tableIndex)
		registryMu.Unlock()
		lastIndexCache.Store((*cachedIndex)(nil))

		baseMapsDir = oldBaseMapsDir
		legacySnapPath = oldLegacySnapPath
		legacyWalPath = oldLegacyWalPath
		walEnqueueTimeout = oldWalTimeout
		snapshotInterval = oldSnapshotInterval
		debugCounterPeriod = oldDebugCounterPeriod
		snapshotWorkerStop = oldSnapshotWorkerStop
		debugCountersStop = oldDebugCountersStop
		if err := os.Chdir(oldWD); err != nil {
			t.Fatal(err)
		}
	})

	return tmp
}

func newBareIndex(t *testing.T, name string, withWal bool) *tableIndex {
	t.Helper()

	ti := &tableIndex{
		name:     name,
		safeName: sanitizeTableName(name),
		walChan:  make(chan walOp, 4),
	}
	ti.walSyncCond = sync.NewCond(&ti.walSyncMu)
	for i := 0; i < numShards; i++ {
		ti.shards[i] = &shard{m: make(map[string]entry)}
	}
	if withWal {
		if err := ti.ensureDir(); err != nil {
			t.Fatal(err)
		}
		if err := ti.setupWalWriter(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			ti.walMu.Lock()
			if ti.walBuf != nil {
				_ = ti.walBuf.Flush()
			}
			if ti.walFile != nil {
				_ = ti.walFile.Close()
			}
			ti.walBuf = nil
			ti.walFile = nil
			ti.walMu.Unlock()
		})
	}
	return ti
}

func TestPointerHelpers(t *testing.T) {
	if entry, ok := isPointer([]interface{}{"child", "table"}); !ok || entry.key != "child" || entry.table != "table" {
		t.Fatalf("expected valid pointer, got %#v ok=%v", entry, ok)
	}
	for _, value := range []interface{}{
		"not-pointer",
		[]interface{}{"only-one"},
		[]interface{}{"child", 123},
	} {
		if _, ok := isPointer(value); ok {
			t.Fatalf("expected %#v not to be a pointer", value)
		}
	}
	if got := stripPrefix("$ptr_author"); got != "author" {
		t.Fatalf("stripPrefix returned %q", got)
	}
	if got := stripPrefix("plain"); got != "plain" {
		t.Fatalf("stripPrefix changed non-pointer key to %q", got)
	}
}

func TestReadPtrAllResolvesNestedPointersMissingTargetsAndSharedTargets(t *testing.T) {
	ctx := context.Background()
	db := newFakeDB(map[string]interface{}{
		dbSlot("books", "b1"): map[string]interface{}{
			"title":       "Tsunami",
			"$ptr_author": []interface{}{"a1", "authors"},
			"$ptr_editor": []interface{}{"a1", "authors"},
			"chapters": []interface{}{
				map[string]interface{}{"$ptr_topic": []interface{}{"t1", "topics"}},
				"appendix",
			},
			"$ptr_missing": []interface{}{"missing", "authors"},
		},
		dbSlot("authors", "a1"): map[string]interface{}{
			"name": "Ada",
		},
		dbSlot("topics", "t1"): map[string]interface{}{
			"name": "storage",
		},
	})

	got, err := ReadPtrAll(ctx, db, "books", "b1")
	if err != nil {
		t.Fatal(err)
	}
	if got["title"] != "Tsunami" {
		t.Fatalf("unexpected title: %#v", got["title"])
	}
	if !reflect.DeepEqual(got["author"], map[string]interface{}{"name": "Ada"}) {
		t.Fatalf("author was not resolved: %#v", got["author"])
	}
	if !reflect.DeepEqual(got["editor"], map[string]interface{}{"name": "Ada"}) {
		t.Fatalf("shared pointer was not resolved from cache: %#v", got["editor"])
	}
	if got["missing"] != nil {
		t.Fatalf("missing target should resolve to nil, got %#v", got["missing"])
	}
	if db.getHitCount("authors", "a1") != 1 {
		t.Fatalf("expected one db hit for cached shared target, got %d", db.getHitCount("authors", "a1"))
	}
	chapters := got["chapters"].([]interface{})
	if !reflect.DeepEqual(chapters[0], map[string]interface{}{"topic": map[string]interface{}{"name": "storage"}}) {
		t.Fatalf("nested array pointer was not resolved: %#v", chapters[0])
	}
}

func TestReadPtrAllErrors(t *testing.T) {
	ctx := context.Background()
	t.Run("get error", func(t *testing.T) {
		db := newFakeDB(nil)
		if _, err := ReadPtrAll(ctx, db, "missing", "root"); err == nil {
			t.Fatal("expected root get error")
		}
	})
	t.Run("root is not object", func(t *testing.T) {
		db := newFakeDB(map[string]interface{}{dbSlot("t", "k"): []interface{}{}})
		if _, err := ReadPtrAll(ctx, db, "t", "k"); err == nil {
			t.Fatal("expected root type error")
		}
	})
	t.Run("loop", func(t *testing.T) {
		db := newFakeDB(map[string]interface{}{
			dbSlot("t", "a"): map[string]interface{}{"$ptr_b": []interface{}{"b", "t"}},
			dbSlot("t", "b"): map[string]interface{}{"$ptr_a": []interface{}{"a", "t"}},
		})
		if _, err := ReadPtrAll(ctx, db, "t", "a"); err == nil || !strings.Contains(err.Error(), "pointer loop") {
			t.Fatalf("expected loop error, got %v", err)
		}
	})
	t.Run("nested map loop", func(t *testing.T) {
		db := newFakeDB(map[string]interface{}{
			dbSlot("t", "root"): map[string]interface{}{"nested": map[string]interface{}{"$ptr_a": []interface{}{"a", "t"}}},
			dbSlot("t", "a"):    map[string]interface{}{"$ptr_a": []interface{}{"a", "t"}},
		})
		if _, err := ReadPtrAll(ctx, db, "t", "root"); err == nil || !strings.Contains(err.Error(), "pointer loop") {
			t.Fatalf("expected nested loop error, got %v", err)
		}
	})
	t.Run("array loop", func(t *testing.T) {
		db := newFakeDB(map[string]interface{}{
			dbSlot("t", "root"): map[string]interface{}{"items": []interface{}{map[string]interface{}{"$ptr_a": []interface{}{"a", "t"}}}},
			dbSlot("t", "a"):    map[string]interface{}{"$ptr_a": []interface{}{"a", "t"}},
		})
		if _, err := ReadPtrAll(ctx, db, "t", "root"); err == nil || !strings.Contains(err.Error(), "pointer loop") {
			t.Fatalf("expected array loop error, got %v", err)
		}
	})
}

func TestReadPtrSome(t *testing.T) {
	ctx := context.Background()
	db := newFakeDB(map[string]interface{}{
		dbSlot("books", "b1"): map[string]interface{}{
			"$ptr_author": []interface{}{"a1", "authors"},
			"$ptr_editor": []interface{}{"e1", "authors"},
			"$ptr_ghost":  []interface{}{"ghost", "authors"},
			"title":       "raw",
		},
		dbSlot("authors", "a1"): map[string]interface{}{"name": "Ada"},
	})

	got, err := ReadPtrSome(ctx, db, "books", "b1", []string{"author", "$ptr_ghost"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got["author"], map[string]interface{}{"name": "Ada"}) {
		t.Fatalf("author not resolved by stripped field name: %#v", got["author"])
	}
	if got["ghost"] != nil {
		t.Fatalf("missing selected pointer should be nil, got %#v", got["ghost"])
	}
	if _, ok := got["$ptr_editor"]; !ok {
		t.Fatalf("unselected pointer should stay raw: %#v", got)
	}
	if got["title"] != "raw" {
		t.Fatalf("plain field changed: %#v", got["title"])
	}

	if _, err := ReadPtrSome(ctx, newFakeDB(nil), "missing", "root", nil); err == nil {
		t.Fatal("expected root get error")
	}
	db = newFakeDB(map[string]interface{}{dbSlot("t", "k"): "not-object"})
	if _, err := ReadPtrSome(ctx, db, "t", "k", nil); err == nil {
		t.Fatal("expected root type error")
	}
	db = newFakeDB(map[string]interface{}{
		dbSlot("t", "root"): map[string]interface{}{"$ptr_a": []interface{}{"a", "t"}},
		dbSlot("t", "a"):    map[string]interface{}{"$ptr_a": []interface{}{"a", "t"}},
	})
	if _, err := ReadPtrSome(ctx, db, "t", "root", []string{"a"}); err == nil {
		t.Fatal("expected selected pointer resolve error")
	}
}

func TestCreatePtrObj(t *testing.T) {
	ctx := context.Background()
	db := newFakeDB(map[string]interface{}{
		dbSlot("authors", "a1"): map[string]interface{}{"name": "Ada"},
	})
	if err := CreatePtrObj(ctx, db, "books", "b1", map[string][]string{
		"$ptr_author": {"a1", "authors"},
	}); err != nil {
		t.Fatal(err)
	}
	want := map[string]interface{}{"$ptr_author": []interface{}{"a1", "authors"}}
	if !reflect.DeepEqual(db.puts[dbSlot("books", "b1")], want) {
		t.Fatalf("unexpected put payload: %#v", db.puts[dbSlot("books", "b1")])
	}

	cases := []struct {
		name string
		ptrs map[string][]string
	}{
		{"bad prefix", map[string][]string{"author": {"a1", "authors"}}},
		{"bad target length", map[string][]string{"$ptr_author": {"a1"}}},
		{"missing target", map[string][]string{"$ptr_author": {"missing", "authors"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := CreatePtrObj(ctx, db, "books", "b1", tc.ptrs); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}

	db.putErr = errors.New("put failed")
	if err := CreatePtrObj(ctx, db, "books", "b2", map[string][]string{"$ptr_author": {"a1", "authors"}}); err == nil {
		t.Fatal("expected put error")
	}
}

func TestMapManagerPublicAPI(t *testing.T) {
	isolateStorage(t)

	if _, _, err := SaveElementByKey("", "k", 0, 1, false); err == nil {
		t.Fatal("expected empty table error")
	}
	if _, _, err := SaveElementByKey("table", "bad", 5, 5, false); err == nil {
		t.Fatal("expected invalid range error")
	}

	prev, existed, err := SaveElementByKey("table", "alpha", 1, 5, true)
	if err != nil {
		t.Fatal(err)
	}
	if existed || prev.Key != "" {
		t.Fatalf("new key should not report previous entry: prev=%#v existed=%v", prev, existed)
	}

	got, err := GetElementByKey("table", "alpha")
	if err != nil {
		t.Fatal(err)
	}
	if got.Key != "alpha" || got.FileName != "table" || got.StartPtr != 1 || got.EndPtr != 5 || !got.HasNested {
		t.Fatalf("unexpected element: %#v", got)
	}

	prev, existed, err = SaveElementByKey("table", "alpha", 6, 9, false)
	if err != nil {
		t.Fatal(err)
	}
	if !existed || prev.StartPtr != 1 || prev.EndPtr != 5 || !prev.HasNested {
		t.Fatalf("overwrite did not return previous entry: prev=%#v existed=%v", prev, existed)
	}

	if _, _, err := SaveElementByKey("table", "beta", 10, 12, false); err != nil {
		t.Fatal(err)
	}
	keys, err := GetKeysByRegex("table", "^(alpha|beta)$", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 {
		t.Fatalf("max limit not respected: %#v", keys)
	}
	keys, err = GetKeysByRegex("table", "^(alpha|beta)$", 0)
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(keys)
	if !reflect.DeepEqual(keys, []string{"alpha", "beta"}) {
		t.Fatalf("unexpected regex keys: %#v", keys)
	}
	if _, err := GetKeysByRegex("table", "[", 0); err == nil {
		t.Fatal("expected regex compile error")
	}

	if err := RemoveElementByKey("table", "alpha"); err != nil {
		t.Fatal(err)
	}
	if _, err := GetElementByKey("table", "alpha"); err == nil {
		t.Fatal("expected removed key to be missing")
	}

	if idx := loadCachedIndex("table"); idx == nil {
		t.Fatal("expected table index cache to be populated")
	}
	ResetForTests()
	if _, err := GetElementByKey("table", "beta"); err == nil {
		t.Fatal("ResetForTests should clear entries")
	}
}

func TestLoadSnapshotAndWal(t *testing.T) {
	isolateStorage(t)
	ti := newBareIndex(t, "table", false)

	if err := ti.loadSnapshot("bad\x00path", nil); err == nil {
		t.Fatal("expected invalid snapshot path error")
	}
	if err := ti.loadSnapshot(filepath.Join("missing", "snapshot"), nil); err != nil {
		t.Fatal(err)
	}
	snapPath := filepath.Join(t.TempDir(), "index.snap")
	snap := strings.Join([]string{
		"short",
		"bad|file|x|3",
		"bad-range|file|5|4",
		"filtered|file|1|3|0",
		"ok|file|2|6|true",
		"ok2|file|3|7|1",
	}, "\n")
	if err := os.WriteFile(snapPath, []byte(snap), 0644); err != nil {
		t.Fatal(err)
	}
	if err := ti.loadSnapshot(snapPath, func(key string, e entry) bool {
		return key != "filtered" && e.file == "file"
	}); err != nil {
		t.Fatal(err)
	}
	if _, ok := ti.loadEntry("filtered"); ok {
		t.Fatal("filtered snapshot entry was stored")
	}
	if got, ok := ti.loadEntry("ok"); !ok || !got.hasNested {
		t.Fatalf("snapshot entry not loaded: %#v ok=%v", got, ok)
	}

	if err := ti.applyWalFile("bad\x00path", nil); err == nil {
		t.Fatal("expected invalid WAL path error")
	}
	if err := ti.applyWalFile(filepath.Join("missing", "wal"), nil); err != nil {
		t.Fatal(err)
	}
	walPath := filepath.Join(t.TempDir(), "index.wal")
	wal := strings.Join([]string{
		"",
		"S|short",
		"S|bad|file|x|4|0",
		"S|bad-range|file|4|4|0",
		"S|filtered|file|4|8|0",
		"S|wal1|file|5|9|true",
		"D|wal1",
		"D|missing",
		"S|wal2|file|6|10|1",
		"X|ignored",
	}, "\n")
	if err := os.WriteFile(walPath, []byte(wal), 0644); err != nil {
		t.Fatal(err)
	}
	if err := ti.applyWalFile(walPath, func(key string, e entry) bool {
		return key != "filtered" && e.file == "file"
	}); err != nil {
		t.Fatal(err)
	}
	if _, ok := ti.loadEntry("wal1"); ok {
		t.Fatal("deleted WAL entry still exists")
	}
	if got, ok := ti.loadEntry("wal2"); !ok || !got.hasNested {
		t.Fatalf("WAL entry not loaded: %#v ok=%v", got, ok)
	}
}

func TestLoadIndexAndLegacyImport(t *testing.T) {
	isolateStorage(t)

	ti := newBareIndex(t, "table", false)
	if err := ti.ensureDir(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ti.snapPath(), []byte("from-snap|table|1|3|0\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ti.walPath(), []byte("S|from-wal|table|3|5|1\nD|from-snap\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := ti.loadIndex(); err != nil {
		t.Fatal(err)
	}
	if _, ok := ti.loadEntry("from-snap"); ok {
		t.Fatal("WAL delete was not applied after snapshot")
	}
	if got, ok := ti.loadEntry("from-wal"); !ok || !got.hasNested {
		t.Fatalf("WAL save was not applied: %#v ok=%v", got, ok)
	}

	legacy := newBareIndex(t, "legacy-table", false)
	if err := os.WriteFile(legacySnapPath, []byte("imported|legacy-table|2|6|0\nskip|other|2|6|0\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyWalPath, []byte("S|imported-wal|legacy-table|7|9|1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := legacy.loadIndex(); err != nil {
		t.Fatal(err)
	}
	if _, ok := legacy.loadEntry("skip"); ok {
		t.Fatal("legacy import did not filter by table")
	}
	if _, ok := legacy.loadEntry("imported"); !ok {
		t.Fatal("legacy snapshot entry was not imported")
	}
	if _, ok := legacy.loadEntry("imported-wal"); !ok {
		t.Fatal("legacy WAL entry was not imported")
	}

	badLegacy := newBareIndex(t, "legacy-error", false)
	legacySnapPath = "bad\x00snapshot"
	if err := badLegacy.importFromLegacy(); err == nil {
		t.Fatal("expected legacy snapshot import error")
	}
	legacySnapPath = filepath.Join(baseMapsDir, "missing.snap")
	legacyWalPath = "bad\x00wal"
	if err := badLegacy.importFromLegacy(); err == nil {
		t.Fatal("expected legacy WAL import error")
	}

	badSnapshot := newBareIndex(t, "bad-snapshot", false)
	if err := badSnapshot.ensureDir(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(badSnapshot.snapPath(), []byte(strings.Repeat("x", 70_000)), 0644); err != nil {
		t.Fatal(err)
	}
	if err := badSnapshot.loadIndex(); err == nil {
		t.Fatal("expected loadIndex snapshot scanner error")
	}

	badWal := newBareIndex(t, "bad-wal-load", false)
	if err := badWal.ensureDir(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(badWal.walPath(), []byte(strings.Repeat("x", 70_000)), 0644); err != nil {
		t.Fatal(err)
	}
	if err := badWal.loadIndex(); err == nil {
		t.Fatal("expected loadIndex WAL scanner error")
	}

	legacySnapPath = filepath.Join(baseMapsDir, "data_map.snap")
	legacyWalPath = filepath.Join(baseMapsDir, "data_map.wal")
	setupError := newBareIndex(t, "setup-error", false)
	if err := setupError.ensureDir(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(setupError.walPath(), 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := newTableIndex("setup-error"); err == nil {
		t.Fatal("expected newTableIndex setupWalWriter error")
	}
}

func TestWalAndSnapshotHelpers(t *testing.T) {
	isolateStorage(t)
	ti := newBareIndex(t, "table", true)

	if err := ti.writeWalLine(walOp{op: 'S', key: "a", fileName: "table", start: 1, end: 3}); err != nil {
		t.Fatal(err)
	}
	if err := ti.writeWalLine(walOp{op: 'S', key: "b", fileName: "table", start: 4, end: 6, hasNested: true}); err != nil {
		t.Fatal(err)
	}
	if err := ti.writeWalLine(walOp{op: 'D', key: "a"}); err != nil {
		t.Fatal(err)
	}
	seq := ti.flushWal()
	atomic.StoreInt64(&ti.walSyncCompleted, seq)
	ti.waitForWalSync(seq)

	badWal := newBareIndex(t, "bad-wal", false)
	if err := badWal.writeWalLine(walOp{op: 'D', key: "x"}); err == nil {
		t.Fatal("expected nil WAL buffer error")
	}

	ti.storeKey("snap", entry{file: "table", start: 1, end: 4, hasNested: true})
	go ti.walSyncLoop()
	ti.writeSnapshot()
	if !fileExists(ti.snapPath()) {
		t.Fatal("snapshot was not written")
	}
	rotated := newBareIndex(t, "rotated", true)
	if err := rotated.rotateWal(); err != nil {
		t.Fatal(err)
	}
	if !fileExists(rotated.walPath()) {
		t.Fatal("WAL was not recreated after rotation")
	}

	closed := newBareIndex(t, "closed-rotate", true)
	closed.walMu.Lock()
	_ = closed.walFile.Close()
	closed.walMu.Unlock()
	if err := closed.rotateWal(); err == nil {
		t.Fatal("expected rotateWal close error")
	}

	removeErr := newBareIndex(t, "remove-error", false)
	if err := removeErr.ensureDir(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(removeErr.walPath(), "child"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := removeErr.rotateWal(); err == nil {
		t.Fatal("expected rotateWal remove error for non-empty directory WAL path")
	}

	openErr := newBareIndex(t, "open-error", false)
	if err := os.MkdirAll(openErr.tableDir(), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(openErr.tableDir()); err != nil {
		t.Fatal(err)
	}
	if err := openErr.rotateWal(); err == nil {
		t.Fatal("expected rotateWal open error")
	}
}

func TestRunWalWriterAndEnqueueTimeout(t *testing.T) {
	isolateStorage(t)
	ti := newBareIndex(t, "table", true)
	go ti.runWalWriter()
	go ti.walSyncLoop()

	before := atomic.LoadInt64(&walOpsProcessed)
	for i := 0; i < 101; i++ {
		if err := ti.enqueueWal(walOp{op: 'S', key: "k", fileName: "table", start: i, end: i + 1}); err != nil {
			t.Fatal(err)
		}
	}
	deadline := time.Now().Add(2 * time.Second)
	for atomic.LoadInt64(&walOpsProcessed) < before+101 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if atomic.LoadInt64(&walOpsProcessed) < before+101 {
		t.Fatalf("WAL writer did not process queued ops")
	}
	if err := ti.enqueueWal(walOp{op: 'D', key: "tick"}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(75 * time.Millisecond)

	blocked := newBareIndex(t, "blocked", true)
	for i := 0; i < cap(blocked.walChan); i++ {
		blocked.walChan <- walOp{op: 'S', key: "held", fileName: "blocked", start: i, end: i + 1}
	}
	atomic.StoreInt64(&blocked.walSyncCompleted, 1)
	walEnqueueTimeout = time.Millisecond
	if err := blocked.enqueueWal(walOp{op: 'S', key: "timeout", fileName: "blocked", start: 1, end: 2}); err == nil {
		t.Fatal("expected enqueue timeout when WAL channel stays full")
	}
	atomic.StoreInt64(&blocked.walSyncCompleted, 100)
	if _, _, err := blocked.saveElement("timeout-save", "blocked", 2, 3, false); err == nil {
		t.Fatal("expected saveElement to return enqueue error")
	}

	writerWithBadWal := newBareIndex(t, "bad-writer", false)
	writerWithBadWal.walChan <- walOp{op: 'S', key: "x", fileName: "bad-writer", start: 0, end: 1}
	go writerWithBadWal.runWalWriter()
	time.Sleep(20 * time.Millisecond)

	var nilWriter *tableIndex
	done := make(chan struct{})
	go func() {
		nilWriter.runWalWriter()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("panic recovery path did not return")
	}
}

func TestFileSystemReadElement(t *testing.T) {
	isolateStorage(t)
	if err := os.MkdirAll(filepath.Join("db", "data"), 0755); err != nil {
		t.Fatal(err)
	}
	content := []byte{0x01, 0x02, 'h', 'e', 'l', 'l', 'o'}
	if err := os.WriteFile(filepath.Join("db", "data", "data.bin"), content, 0644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := SaveElementByKey("data.bin", "ok", 0, len(content), false); err != nil {
		t.Fatal(err)
	}
	ok, data, err := ReadElement("ok")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || data != "hello" {
		t.Fatalf("unexpected read: ok=%v data=%q", ok, data)
	}

	ok, data, err = ReadElement("missing")
	if err == nil || ok || data != "" {
		t.Fatalf("expected missing read error, got ok=%v data=%q err=%v", ok, data, err)
	}

	if _, _, err := SaveElementByKey("data.bin", "bad-range", 0, 1, false); err != nil {
		t.Fatal(err)
	}
	ok, data, err = ReadElement("bad-range")
	if err == nil || ok || data != "" {
		t.Fatalf("expected invalid range read error, got ok=%v data=%q err=%v", ok, data, err)
	}
}

func TestMiscHelpers(t *testing.T) {
	isolateStorage(t)

	SaveElement("key", nil)
	value := "encoded"
	SaveElement("key", &value)
	if got, err := GetElementByKey("data.bin", "key"); err != nil || got.EndPtr <= got.StartPtr {
		t.Fatalf("SaveElement did not persist metadata: got=%#v err=%v", got, err)
	}
	RecordDefragFree()
	RecordDefragSkip()
	atomic.StoreInt64(&storeLockSlowCount, 1)
	atomic.StoreInt64(&defragFreedCount, 1)
	atomic.StoreInt64(&defragSkipCount, 1)

	if sanitizeTableName(" \t\n") != "default" {
		t.Fatal("empty sanitized table should become default")
	}
	if got := sanitizeTableName(`a/b\c:*?"<>|..`); strings.ContainsAny(got, `/\:*?"<>|`) || strings.Contains(got, "..") {
		t.Fatalf("table name was not sanitized: %q", got)
	}
	if _, err := normalizeTableName(" "); err == nil {
		t.Fatal("expected empty normalized table error")
	}
	if name, err := normalizeTableName(" table "); err != nil || name != " table " {
		t.Fatalf("normalizeTableName changed valid name: %q %v", name, err)
	}

	ti := newBareIndex(t, "slow", false)
	sh := ti.getShard("locked")
	sh.mu.Lock()
	done := make(chan struct{})
	go func() {
		ti.storeKey("locked", entry{file: "slow", start: 0, end: 1})
		ti.deleteKey("locked")
		close(done)
	}()
	time.Sleep(150 * time.Microsecond)
	sh.mu.Unlock()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("lock wait helper did not complete")
	}

	sh = ti.getShard("delete-locked")
	ti.storeKey("delete-locked", entry{file: "slow", start: 0, end: 1})
	sh.mu.Lock()
	done = make(chan struct{})
	go func() {
		ti.deleteKey("delete-locked")
		close(done)
	}()
	time.Sleep(150 * time.Microsecond)
	sh.mu.Unlock()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("delete lock wait helper did not complete")
	}

	waiter := newBareIndex(t, "waiter", true)
	waiter.walSyncCond = nil
	go func() {
		time.Sleep(20 * time.Millisecond)
		atomic.StoreInt64(&waiter.walSyncCompleted, 1)
	}()
	waiter.waitForWalSync(1)

	syncer := newBareIndex(t, "syncer", true)
	syncer.walSyncCond = nil
	atomic.StoreInt64(&syncer.walSyncRequested, 1)
	go syncer.walSyncLoop()
	time.Sleep(20 * time.Millisecond)

	oldBase := baseMapsDir
	baseMapsDir = "bad\x00base"
	lastIndexCache.Store((*cachedIndex)(nil))
	registryMu.Lock()
	for _, idx := range indexRegistry {
		idx.walMu.Lock()
		if idx.walBuf != nil {
			_ = idx.walBuf.Flush()
		}
		if idx.walFile != nil {
			_ = idx.walFile.Close()
		}
		idx.walBuf = nil
		idx.walFile = nil
		idx.walMu.Unlock()
	}
	indexRegistry = make(map[string]*tableIndex)
	registryMu.Unlock()
	if _, err := getTableIndex("bad-table"); err == nil {
		t.Fatal("expected getTableIndex new index error")
	}
	if err := RemoveElementByKey("bad-table", "k"); err == nil {
		t.Fatal("expected RemoveElementByKey table error")
	}
	if _, err := GetElementByKey("bad-table", "k"); err == nil {
		t.Fatal("expected GetElementByKey table error")
	}
	if _, err := GetKeysByRegex("bad-table", ".*", 0); err == nil {
		t.Fatal("expected GetKeysByRegex table error")
	}
	baseMapsDir = oldBase
}

func TestSaveElementWriteError(t *testing.T) {
	isolateStorage(t)
	if err := os.WriteFile(filepath.Join("db", "data"), []byte("not a directory"), 0644); err != nil {
		t.Fatal(err)
	}
	value := "encoded"
	SaveElement("bad", &value)
}

func TestBackgroundWorkers(t *testing.T) {
	isolateStorage(t)

	snapStop := make(chan struct{})
	snapshotWorkerStop = snapStop
	snapshotInterval = time.Millisecond
	ti := newBareIndex(t, "snapshot-worker", true)
	go ti.walSyncLoop()
	go ti.snapshotWorker()
	deadline := time.Now().Add(time.Second)
	for !fileExists(ti.snapPath()) && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	close(snapStop)
	if !fileExists(ti.snapPath()) {
		t.Fatal("snapshot worker did not write a snapshot")
	}

	debugStop := make(chan struct{})
	debugCountersStop = debugStop
	debugCounterPeriod = time.Millisecond
	atomic.StoreInt64(&storeLockSlowCount, 1)
	atomic.StoreInt64(&defragFreedCount, 1)
	atomic.StoreInt64(&defragSkipCount, 1)
	done := make(chan struct{})
	go func() {
		debugCountersWorker()
		close(done)
	}()
	deadline = time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt64(&storeLockSlowCount) == 0 &&
			atomic.LoadInt64(&defragFreedCount) == 0 &&
			atomic.LoadInt64(&defragSkipCount) == 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	close(debugStop)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("debug counter worker did not stop")
	}
}
