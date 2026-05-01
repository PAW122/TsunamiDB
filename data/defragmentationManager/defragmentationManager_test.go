package defragmentationManager

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func setupDefragTest(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	t.Cleanup(func() {
		ResetForTests()
		_ = os.Chdir(wd)
	})

	ResetForTests()
	return dir
}

func readBlocks(t *testing.T, table string) map[string]FreeBlock {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("db", "maps", sanitizeName(table), freeFileName))
	if err != nil {
		t.Fatalf("read blocks for %q: %v", table, err)
	}
	var blocks map[string]FreeBlock
	if err := json.Unmarshal(data, &blocks); err != nil {
		t.Fatalf("unmarshal blocks for %q: %v", table, err)
	}
	return blocks
}

func writeBlocks(t *testing.T, table string, blocks map[string]FreeBlock) {
	t.Helper()

	path := filepath.Join("db", "maps", sanitizeName(table), freeFileName)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir blocks dir: %v", err)
	}
	data, err := json.Marshal(blocks)
	if err != nil {
		t.Fatalf("marshal blocks: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write blocks: %v", err)
	}
}

func putFreeList(table string, fl *tableFreeList) {
	freeRegistryMu.Lock()
	freeRegistry[table] = fl
	freeRegistryMu.Unlock()
}

func TestSanitizeNameAndRegistryPaths(t *testing.T) {
	setupDefragTest(t)

	if got := sanitizeName(" \t "); got != "default" {
		t.Fatalf("empty sanitized name=%q want default", got)
	}
	if got := sanitizeName(` ../bad:name?.tbl `); strings.ContainsAny(got, `/\:?"<>|`) || strings.Contains(got, "..") {
		t.Fatalf("name was not sanitized: %q", got)
	}

	if _, err := getTableFreeList(""); err == nil {
		t.Fatal("expected empty table error")
	}

	fl, err := getTableFreeList("table.tbl")
	if err != nil {
		t.Fatalf("get table free list: %v", err)
	}
	cached, err := getTableFreeList("table.tbl")
	if err != nil {
		t.Fatalf("get cached table free list: %v", err)
	}
	if cached != fl {
		t.Fatal("expected cached tableFreeList pointer")
	}

	ResetForTests()
	const concurrentTable = "concurrent.tbl"
	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make(chan error, 32)
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := getTableFreeList(concurrentTable)
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent getTableFreeList: %v", err)
		}
	}
}

func TestMarkAsFreePersistsAndGetBlockChoosesSmallestSuitable(t *testing.T) {
	setupDefragTest(t)

	file := "bucket.dat"
	if err := MarkAsFree("too-small", file, 0, 5); err != nil {
		t.Fatalf("mark too-small: %v", err)
	}
	if err := MarkAsFree("large", file, 20, 50); err != nil {
		t.Fatalf("mark large: %v", err)
	}
	if err := MarkAsFree("best", file, 60, 75); err != nil {
		t.Fatalf("mark best: %v", err)
	}

	if err := MarkAsFree("bad", "", 0, 1); err == nil {
		t.Fatal("expected empty fileName error")
	}

	block, err := GetBlock(10, file)
	if err != nil {
		t.Fatalf("get block: %v", err)
	}
	if block.StartPtr != 60 || block.EndPtr != 75 || block.Size != 15 {
		t.Fatalf("unexpected block: %+v", block)
	}

	blocks := readBlocks(t, file)
	if _, ok := blocks["best"]; ok {
		t.Fatal("selected block was not removed from free list")
	}
	if _, err := GetBlock(100, file); err == nil {
		t.Fatal("expected no suitable block error")
	}
	putFreeList("mismatch.dat", &tableFreeList{
		path:   filepath.Join("db", "maps", "mismatch", freeFileName),
		loaded: true,
		blocks: map[string]FreeBlock{
			"other": {FileName: "other.dat", StartPtr: 0, EndPtr: 20, Size: 20},
		},
	})
	if _, err := GetBlock(1, "mismatch.dat"); err == nil {
		t.Fatal("expected no suitable block error for fileName mismatch")
	}
	if _, err := GetBlock(1, "missing.dat"); err == nil {
		t.Fatal("expected no blocks error for missing file")
	}
	if _, err := GetBlock(1, ""); err == nil {
		t.Fatal("expected empty fileName error")
	}
}

func TestSaveBlockCheckVariants(t *testing.T) {
	setupDefragTest(t)

	file := "fragments.dat"
	cases := []struct {
		name  string
		key   string
		start int64
		end   int64
		want  map[string]FreeBlock
	}{
		{
			name:  "exact delete",
			key:   "exact",
			start: 0,
			end:   10,
			want:  map[string]FreeBlock{},
		},
		{
			name:  "trim start",
			key:   "start",
			start: 0,
			end:   3,
			want: map[string]FreeBlock{
				"start": {FileName: file, StartPtr: 3, EndPtr: 10, Size: 7},
			},
		},
		{
			name:  "trim end",
			key:   "end",
			start: 7,
			end:   10,
			want: map[string]FreeBlock{
				"end": {FileName: file, StartPtr: 0, EndPtr: 7, Size: 7},
			},
		},
		{
			name:  "split middle",
			key:   "middle",
			start: 3,
			end:   7,
			want: map[string]FreeBlock{
				"middle":   {FileName: file, StartPtr: 0, EndPtr: 3, Size: 3},
				"middle_7": {FileName: file, StartPtr: 7, EndPtr: 10, Size: 3},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ResetForTests()
			if err := MarkAsFree(tc.key, file, 0, 10); err != nil {
				t.Fatalf("mark free: %v", err)
			}

			SaveBlockCheck(file, tc.start, tc.end)

			got := readBlocks(t, file)
			if len(got) != len(tc.want) {
				t.Fatalf("block count=%d want=%d: %+v", len(got), len(tc.want), got)
			}
			for key, want := range tc.want {
				if got[key] != want {
					t.Fatalf("block %q=%+v want %+v", key, got[key], want)
				}
			}
		})
	}
}

func TestSaveBlockCheckNoMatchAndErrorPaths(t *testing.T) {
	setupDefragTest(t)

	file := "no-match.dat"
	if err := MarkAsFree("free", file, 10, 20); err != nil {
		t.Fatalf("mark free: %v", err)
	}

	SaveBlockCheck(file, 0, 5)
	blocks := readBlocks(t, file)
	if got := blocks["free"]; got.StartPtr != 10 || got.EndPtr != 20 {
		t.Fatalf("unexpected modification for outside range: %+v", got)
	}

	SaveBlockCheck("", 0, 1)

	loadErrList := &tableFreeList{path: "bad" + string(rune(0)), blocks: map[string]FreeBlock{}}
	putFreeList("load-error.dat", loadErrList)
	SaveBlockCheck("load-error.dat", 0, 1)

	putFreeList("mismatch-check.dat", &tableFreeList{
		path:   filepath.Join("db", "maps", "mismatch-check", freeFileName),
		loaded: true,
		blocks: map[string]FreeBlock{
			"other": {FileName: "other.dat", StartPtr: 0, EndPtr: 10, Size: 10},
		},
	})
	SaveBlockCheck("mismatch-check.dat", 0, 5)

	saveErrPath := filepath.Join("db", "maps", "save-error", freeFileName)
	if err := os.MkdirAll(saveErrPath, 0o755); err != nil {
		t.Fatalf("mkdir save-error path as dir: %v", err)
	}
	saveErrList := &tableFreeList{
		path:   saveErrPath,
		loaded: true,
		blocks: map[string]FreeBlock{
			"free": {FileName: "save-error.dat", StartPtr: 0, EndPtr: 10, Size: 10},
		},
	}
	putFreeList("save-error.dat", saveErrList)
	SaveBlockCheck("save-error.dat", 0, 10)
}

func TestTableFreeListLoadAndSaveErrorPaths(t *testing.T) {
	dir := setupDefragTest(t)

	missing := &tableFreeList{path: filepath.Join("db", "maps", "missing", freeFileName)}
	if err := missing.load(); err != nil {
		t.Fatalf("load missing: %v", err)
	}
	if !missing.loaded {
		t.Fatal("missing file load did not mark list as loaded")
	}
	if err := missing.load(); err != nil {
		t.Fatalf("load already loaded: %v", err)
	}

	nilBlocks := &tableFreeList{path: filepath.Join("db", "maps", "nil", freeFileName)}
	if err := nilBlocks.save(); err != nil {
		t.Fatalf("save nil blocks: %v", err)
	}
	if nilBlocks.blocks == nil {
		t.Fatal("save did not initialize nil blocks")
	}

	validTable := "valid-load.dat"
	writeBlocks(t, validTable, map[string]FreeBlock{
		"a": {FileName: validTable, StartPtr: 1, EndPtr: 4, Size: 3},
	})
	valid := &tableFreeList{path: filepath.Join("db", "maps", sanitizeName(validTable), freeFileName)}
	if err := valid.load(); err != nil {
		t.Fatalf("load valid json: %v", err)
	}
	if got := valid.blocks["a"]; got.Size != 3 {
		t.Fatalf("unexpected loaded block: %+v", got)
	}

	invalidPath := filepath.Join("db", "maps", "invalid-json", freeFileName)
	if err := os.MkdirAll(filepath.Dir(invalidPath), 0o755); err != nil {
		t.Fatalf("mkdir invalid json dir: %v", err)
	}
	if err := os.WriteFile(invalidPath, []byte("{"), 0o644); err != nil {
		t.Fatalf("write invalid json: %v", err)
	}
	if err := (&tableFreeList{path: invalidPath}).load(); err == nil {
		t.Fatal("expected invalid json load error")
	}

	if err := (&tableFreeList{path: "bad" + string(rune(0))}).load(); err == nil {
		t.Fatal("expected open/stat error")
	}
	if err := (&tableFreeList{path: filepath.Join("bad"+string(rune(0)), "file"), blocks: map[string]FreeBlock{}}).save(); err == nil {
		t.Fatal("expected mkdir error")
	}
	if err := (&tableFreeList{path: dir, blocks: map[string]FreeBlock{}}).save(); err == nil {
		t.Fatal("expected create directory error")
	}
}

func TestMarkAsFreeAndGetBlockLoadAndSaveErrors(t *testing.T) {
	setupDefragTest(t)

	putFreeList("mark-load.dat", &tableFreeList{path: "bad" + string(rune(0)), blocks: map[string]FreeBlock{}})
	if err := MarkAsFree("key", "mark-load.dat", 0, 1); err == nil {
		t.Fatal("expected MarkAsFree load error")
	}

	markSavePath := filepath.Join("db", "maps", "mark-save", freeFileName)
	if err := os.MkdirAll(markSavePath, 0o755); err != nil {
		t.Fatalf("mkdir mark save path as dir: %v", err)
	}
	putFreeList("mark-save.dat", &tableFreeList{
		path:   markSavePath,
		loaded: true,
		blocks: map[string]FreeBlock{},
	})
	if err := MarkAsFree("key", "mark-save.dat", 0, 1); err == nil {
		t.Fatal("expected MarkAsFree save error")
	}

	putFreeList("get-load.dat", &tableFreeList{path: "bad" + string(rune(0)), blocks: map[string]FreeBlock{}})
	if _, err := GetBlock(1, "get-load.dat"); err == nil {
		t.Fatal("expected GetBlock load error")
	}

	getSavePath := filepath.Join("db", "maps", "get-save", freeFileName)
	if err := os.MkdirAll(getSavePath, 0o755); err != nil {
		t.Fatalf("mkdir get save path as dir: %v", err)
	}
	putFreeList("get-save.dat", &tableFreeList{
		path:   getSavePath,
		loaded: true,
		blocks: map[string]FreeBlock{
			"free": {FileName: "get-save.dat", StartPtr: 0, EndPtr: 10, Size: 10},
		},
	})
	if _, err := GetBlock(1, "get-save.dat"); err == nil {
		t.Fatal("expected GetBlock save error")
	}
}

func TestAsyncBlockManagerFunctions(t *testing.T) {
	setupDefragTest(t)

	originalDataFilesPath := dataFilesPath
	t.Cleanup(func() {
		dataFilesPath = originalDataFilesPath
		loadedFiles = nil
	})

	dataFilesPath = filepath.Join("db", "data")
	if LoadFiles("ignored") {
		t.Fatal("expected LoadFiles to fail for missing data dir")
	}

	if err := os.MkdirAll(dataFilesPath, 0o755); err != nil {
		t.Fatalf("mkdir data files path: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dataFilesPath, "one.dat"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write data file: %v", err)
	}
	if !LoadFiles("ignored-again") {
		t.Fatal("expected LoadFiles success")
	}
	if len(loadedFiles) != 1 || loadedFiles[0].Name() != "one.dat" {
		t.Fatalf("loadedFiles=%v", loadedFiles)
	}

	LoadTempData()
	CreateNewAsyncBlock()
	saveToTemp()
}
