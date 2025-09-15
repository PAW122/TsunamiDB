package dataManager_v2

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	defrag "github.com/PAW122/TsunamiDB/data/defragmentationManager"
	encoding_v1 "github.com/PAW122/TsunamiDB/encoding/v1"
)

func setupDataManagerTest(t *testing.T) func() {
	t.Helper()
	dir, err := os.MkdirTemp("", "tsunamidb-data-test-")
	if err != nil {
		t.Fatalf("mkdtemp: %v", err)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		shutdownFileWorkersForTests()
		_ = os.Chdir(wd)
		_ = os.RemoveAll(dir)
	})

	basePath = filepath.Join(dir, "data")
	baseIncTablesPath = filepath.Join(dir, "inc")
	if err := os.MkdirAll(basePath, 0o755); err != nil {
		t.Fatalf("mkdir data: %v", err)
	}
	if err := os.MkdirAll(baseIncTablesPath, 0o755); err != nil {
		t.Fatalf("mkdir inc: %v", err)
	}

	fileWorkers = sync.Map{}
	defrag.ResetForTests()
	return func() {}
}

func TestSaveReadFreeReuse(t *testing.T) {
	setupDataManagerTest(t)

	file := "bucket.dat"
	data1 := []byte("hello world")

	start1, end1, err := SaveDataToFileAsync(data1, file)
	if err != nil {
		t.Fatalf("save1: %v", err)
	}
	if got := end1 - start1; got != int64(len(data1)) {
		t.Fatalf("unexpected span: got %d want %d", got, len(data1))
	}

	stat1, err := os.Stat(filepath.Join(basePath, file))
	if err != nil {
		t.Fatalf("stat1: %v", err)
	}
	if stat1.Size() != end1 {
		t.Fatalf("size mismatch after first save: got %d want %d", stat1.Size(), end1)
	}

	read1, err := ReadDataFromFileAsync(file, start1, end1)
	if err != nil {
		t.Fatalf("read1: %v", err)
	}
	if string(read1) != string(data1) {
		t.Fatalf("read1 mismatch: got %q want %q", read1, data1)
	}

	if err := defrag.MarkAsFree("test-key", file, start1, end1); err != nil {
		t.Fatalf("mark free: %v", err)
	}

	data2 := []byte("HELLO WORLD")
	start2, end2, err := SaveDataToFileAsync(data2, file)
	if err != nil {
		t.Fatalf("save2: %v", err)
	}
	if start2 != start1 || end2 != end1 {
		t.Fatalf("expected reuse span [%d,%d), got [%d,%d)", start1, end1, start2, end2)
	}

	stat2, err := os.Stat(filepath.Join(basePath, file))
	if err != nil {
		t.Fatalf("stat2: %v", err)
	}
	if stat2.Size() != stat1.Size() {
		t.Fatalf("file grew after reuse: size1=%d size2=%d", stat1.Size(), stat2.Size())
	}

	read2, err := ReadDataFromFileAsync(file, start2, end2)
	if err != nil {
		t.Fatalf("read2: %v", err)
	}
	if string(read2) != string(data2) {
		t.Fatalf("read2 mismatch: got %q want %q", read2, data2)
	}
}

func TestSaveDataSequentialGrowth(t *testing.T) {
	setupDataManagerTest(t)

	file := "seq.dat"
	first := []byte("abc")
	second := []byte("12345")

	s1, e1, err := SaveDataToFileAsync(first, file)
	if err != nil {
		t.Fatalf("save first: %v", err)
	}

	s2, e2, err := SaveDataToFileAsync(second, file)
	if err != nil {
		t.Fatalf("save second: %v", err)
	}

	if s2 != e1 {
		t.Fatalf("expected second write to append at %d, got %d", e1, s2)
	}

	stat, err := os.Stat(filepath.Join(basePath, file))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	expectedSize := int64(len(first) + len(second))
	if stat.Size() != expectedSize || e2 != expectedSize {
		t.Fatalf("unexpected file size: stat=%d end=%d want=%d", stat.Size(), e2, expectedSize)
	}

	readBack, err := ReadDataFromFileAsync(file, s1, e1)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(readBack) != string(first) {
		t.Fatalf("unexpected data after append: %q", readBack)
	}
}

func TestIncTableAppendAndDelete(t *testing.T) {
	setupDataManagerTest(t)

	table := "inc_table_test.tbl"
	entrySize := uint64(8)

	enc1 := encoding_v1.EncodeIncEntry(entrySize, []byte("foo"))
	id1, err := SaveIncDataToFileAsync(enc1, table, entrySize)
	if err != nil {
		t.Fatalf("save inc 1: %v", err)
	}
	if id1 != 0 {
		t.Fatalf("expected first id 0, got %d", id1)
	}

	enc2 := encoding_v1.EncodeIncEntry(entrySize, []byte("bar"))
	id2, err := SaveIncDataToFileAsync(enc2, table, entrySize)
	if err != nil {
		t.Fatalf("save inc 2: %v", err)
	}
	if id2 != 1 {
		t.Fatalf("expected second id 1, got %d", id2)
	}

	recordSize := int64(entrySize) + 3
	stat, err := os.Stat(filepath.Join(baseIncTablesPath, table))
	if err != nil {
		t.Fatalf("stat inc: %v", err)
	}
	if stat.Size() != recordSize*2 {
		t.Fatalf("unexpected inc size: got %d want %d", stat.Size(), recordSize*2)
	}

	raw, err := ReadIncDataFromFileAsync_ById(table, 1, entrySize)
	if err != nil {
		t.Fatalf("read inc: %v", err)
	}
	decoded, err := encoding_v1.DecodeIncEntry(entrySize, raw)
	if err != nil {
		t.Fatalf("decode inc: %v", err)
	}
	if string(decoded.Data) != "bar" {
		t.Fatalf("unexpected inc payload: %q", decoded.Data)
	}

	if err := DeleteIncTableFile(table); err != nil {
		t.Fatalf("delete inc: %v", err)
	}
	stat, err = os.Stat(filepath.Join(baseIncTablesPath, table))
	if err != nil {
		t.Fatalf("stat inc after delete: %v", err)
	}
	if stat.Size() != 0 {
		t.Fatalf("expected truncated file, size=%d", stat.Size())
	}

	shutdownFileWorkersForTests()

	id3, err := SaveIncDataToFileAsync(enc1, table, entrySize)
	if err != nil {
		t.Fatalf("save inc after delete: %v", err)
	}
	if id3 != 0 {
		t.Fatalf("expected id reset to 0, got %d", id3)
	}

	shutdownFileWorkersForTests()
}
