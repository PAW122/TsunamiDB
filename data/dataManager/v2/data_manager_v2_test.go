package dataManager_v2

import (
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	encoding_v1 "github.com/PAW122/TsunamiDB/encoding/v1"
)

func encodedIncEntry(t *testing.T, entrySize uint64, payload string) []byte {
	t.Helper()
	encoded := encoding_v1.EncodeIncEntry(entrySize, []byte(payload))
	if encoded == nil {
		t.Fatalf("encoding inc entry %q returned nil", payload)
	}
	return encoded
}

func decodedIncPayload(t *testing.T, entrySize uint64, raw []byte) string {
	t.Helper()
	decoded, err := encoding_v1.DecodeIncEntry(entrySize, raw)
	if err != nil {
		t.Fatalf("decode inc entry: %v", err)
	}
	return string(decoded.Data)
}

func decodedIncPayloads(t *testing.T, entrySize uint64, raw []byte) []string {
	t.Helper()
	recordSize := int(entrySize) + 3
	if len(raw)%recordSize != 0 {
		t.Fatalf("raw inc payload length %d is not aligned to record size %d", len(raw), recordSize)
	}

	out := make([]string, 0, len(raw)/recordSize)
	for start := 0; start < len(raw); start += recordSize {
		out = append(out, decodedIncPayload(t, entrySize, raw[start:start+recordSize]))
	}
	return out
}

func assertStrings(t *testing.T, got []string, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("payload count mismatch: got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("payload[%d] mismatch: got %q want %q (all=%v)", i, got[i], want[i], got)
		}
	}
}

type fakeFileInfo struct {
	size int64
}

func (f fakeFileInfo) Name() string       { return "fake" }
func (f fakeFileInfo) Size() int64        { return f.size }
func (f fakeFileInfo) Mode() os.FileMode  { return 0o644 }
func (f fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (f fakeFileInfo) IsDir() bool        { return false }
func (f fakeFileInfo) Sys() any           { return nil }

type fakeBatchFile struct {
	size       int64
	seekErr    error
	writeAtErr error
	readAtErr  error
	truncErr   error
	statErr    error
}

func (f *fakeBatchFile) Seek(offset int64, whence int) (int64, error) {
	if f.seekErr != nil {
		return 0, f.seekErr
	}
	if whence == 0 {
		return offset, nil
	}
	return f.size, nil
}

func (f *fakeBatchFile) WriteAt(b []byte, off int64) (int, error) {
	if f.writeAtErr != nil {
		return 0, f.writeAtErr
	}
	if end := off + int64(len(b)); end > f.size {
		f.size = end
	}
	return len(b), nil
}

func (f *fakeBatchFile) ReadAt(b []byte, off int64) (int, error) {
	if f.readAtErr != nil {
		return 0, f.readAtErr
	}
	return len(b), nil
}

func (f *fakeBatchFile) Truncate(size int64) error {
	if f.truncErr != nil {
		return f.truncErr
	}
	f.size = size
	return nil
}

func (f *fakeBatchFile) Stat() (os.FileInfo, error) {
	if f.statErr != nil {
		return nil, f.statErr
	}
	return fakeFileInfo{size: f.size}, nil
}

func executeSingleForTest(file batchFile, req fileRequest) fileResponse {
	req.resp = make(chan fileResponse, 2)
	executeBatch(file, "fake.tbl", []fileRequest{req})
	return <-req.resp
}

func TestMustCreateBaseDirPanicsOnMkdirError(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic for invalid base directory")
		}
	}()
	mustCreateBaseDir(string([]byte{0}))
}

func TestIncTableReadVariantsPutOverwriteAndCount(t *testing.T) {
	setupDataManagerTest(t)

	table := "variants.tbl"
	entrySize := uint64(8)

	for wantID, payload := range []string{"one", "two", "three"} {
		id, err := SaveIncDataToFileAsync(encodedIncEntry(t, entrySize, payload), table, entrySize)
		if err != nil {
			t.Fatalf("append %q: %v", payload, err)
		}
		if id != uint64(wantID) {
			t.Fatalf("append %q id=%d want=%d", payload, id, wantID)
		}
	}

	count, err := GetIncRecordCount(table, entrySize)
	if err != nil {
		t.Fatalf("record count: %v", err)
	}
	if count != 3 {
		t.Fatalf("record count=%d want=3", count)
	}

	firstTwo, err := ReadIncDataFromFileAsync_FirstEntries(table, 2, entrySize)
	if err != nil {
		t.Fatalf("read first two: %v", err)
	}
	assertStrings(t, decodedIncPayloads(t, entrySize, firstTwo), "one", "two")

	lastTwo, err := ReadIncDataFromFileAsync_LastEntries(table, 2, entrySize)
	if err != nil {
		t.Fatalf("read last two: %v", err)
	}
	assertStrings(t, decodedIncPayloads(t, entrySize, lastTwo), "three", "two")

	lastAll, err := ReadIncDataFromFileAsync_LastEntries(table, 99, entrySize)
	if err != nil {
		t.Fatalf("read last all: %v", err)
	}
	assertStrings(t, decodedIncPayloads(t, entrySize, lastAll), "three", "two", "one")

	id, err := SaveIncDataToFileAsync_Put(encodedIncEntry(t, entrySize, "insert"), table, entrySize, 1, "bottom")
	if err != nil {
		t.Fatalf("put bottom: %v", err)
	}
	if id != 1 {
		t.Fatalf("put bottom id=%d want=1", id)
	}

	id, err = SaveIncDataToFileAsync_Put(encodedIncEntry(t, entrySize, "newest"), table, entrySize, 0, "top")
	if err != nil {
		t.Fatalf("put top: %v", err)
	}
	if id != 4 {
		t.Fatalf("put top id=%d want=4", id)
	}

	id, err = SaveIncDataToFileAsync_OverWrite(encodedIncEntry(t, entrySize, "ONE"), table, entrySize, 0, "bottom")
	if err != nil {
		t.Fatalf("overwrite bottom: %v", err)
	}
	if id != 0 {
		t.Fatalf("overwrite bottom id=%d want=0", id)
	}

	id, err = SaveIncDataToFileAsync_OverWrite(encodedIncEntry(t, entrySize, "LATEST"), table, entrySize, 0, "top")
	if err != nil {
		t.Fatalf("overwrite top: %v", err)
	}
	if id != 4 {
		t.Fatalf("overwrite top id=%d want=4", id)
	}

	all, err := ReadIncDataFromFileAsync_FirstEntries(table, 99, entrySize)
	if err != nil {
		t.Fatalf("read all: %v", err)
	}
	assertStrings(t, decodedIncPayloads(t, entrySize, all), "ONE", "insert", "two", "three", "LATEST")

	if _, err := ReadIncDataFromFileAsync_ById(table, 99, entrySize); err == nil || !strings.Contains(err.Error(), "id out of range") {
		t.Fatalf("read out of range error=%v", err)
	}
	if _, err := SaveIncDataToFileAsync_Put(encodedIncEntry(t, entrySize, "bad"), table, entrySize, 99, "bottom"); err == nil {
		t.Fatal("expected put bottom out of range error")
	}
	if _, err := SaveIncDataToFileAsync_Put(encodedIncEntry(t, entrySize, "bad"), table, entrySize, 99, "top"); err == nil {
		t.Fatal("expected put top out of range error")
	}
	if _, err := SaveIncDataToFileAsync_OverWrite(encodedIncEntry(t, entrySize, "bad"), table, entrySize, 99, "bottom"); err == nil {
		t.Fatal("expected overwrite bottom out of range error")
	}
	if _, err := SaveIncDataToFileAsync_OverWrite(encodedIncEntry(t, entrySize, "bad"), table, entrySize, 99, "top"); err == nil {
		t.Fatal("expected overwrite top out of range error")
	}
}

func TestIncTableEmptyReadsMissingCountAndValidationErrors(t *testing.T) {
	setupDataManagerTest(t)

	table := "empty.tbl"
	entrySize := uint64(8)

	last, err := ReadIncDataFromFileAsync_LastEntries(table, 0, entrySize)
	if err != nil {
		t.Fatalf("last zero: %v", err)
	}
	if len(last) != 0 {
		t.Fatalf("last zero len=%d want=0", len(last))
	}

	first, err := ReadIncDataFromFileAsync_FirstEntries(table, 10, entrySize)
	if err != nil {
		t.Fatalf("first empty: %v", err)
	}
	if len(first) != 0 {
		t.Fatalf("first empty len=%d want=0", len(first))
	}

	if _, err := GetIncRecordCount("missing.tbl", entrySize); err == nil {
		t.Fatal("expected missing file count error")
	}
	if err := os.WriteFile(filepath.Join(baseIncTablesPath, "invalid-size.tbl"), []byte("abc"), 0o644); err != nil {
		t.Fatalf("write invalid-size table: %v", err)
	}
	if count, err := GetIncRecordCount("invalid-size.tbl", ^uint64(0)-2); err != nil || count != 0 {
		t.Fatalf("invalid entry size count=%d err=%v, want 0 nil", count, err)
	}

	if _, err := SaveIncDataToFileAsync([]byte("short"), table, entrySize); err == nil {
		t.Fatal("expected write_inc data length error")
	}
	if _, err := SaveIncDataToFileAsync_Put([]byte("short"), table, entrySize, 0, "bottom"); err == nil {
		t.Fatal("expected write_inc_ow put data length error")
	}
	if _, err := SaveIncDataToFileAsync_OverWrite(encodedIncEntry(t, entrySize, "none"), table, entrySize, 0, "bottom"); err == nil {
		t.Fatal("expected overwrite empty table error")
	}
}

func TestReadDataInvalidRangeAndIncompleteRead(t *testing.T) {
	setupDataManagerTest(t)

	if _, err := ReadDataFromFileAsync("bad-range.dat", 5, 5); err == nil || !strings.Contains(err.Error(), "invalid read range") {
		t.Fatalf("invalid range error=%v", err)
	}

	fullPath := filepath.Join(basePath, "short-response.dat")
	ch := make(chan fileRequest, 1)
	fileWorkers.Store(fullPath, ch)
	t.Cleanup(func() {
		fileWorkers.Delete(fullPath)
	})

	go func() {
		req := <-ch
		req.resp <- fileResponse{data: []byte("x")}
	}()

	if _, err := ReadDataFromFileAsync("short-response.dat", 0, 2); err == nil || !strings.Contains(err.Error(), "incomplete read") {
		t.Fatalf("incomplete read error=%v", err)
	}
}

func TestWorkerTimeoutCrashAndExportedTestHelpers(t *testing.T) {
	setupDataManagerTest(t)

	EnsureDirsForTests()
	if _, err := os.Stat(basePath); err != nil {
		t.Fatalf("basePath missing after EnsureDirsForTests: %v", err)
	}
	if _, err := os.Stat(baseIncTablesPath); err != nil {
		t.Fatalf("baseIncTablesPath missing after EnsureDirsForTests: %v", err)
	}

	timeoutPath := filepath.Join(basePath, "timeout.dat")
	blocked := make(chan fileRequest)
	fileWorkers.Store(timeoutPath, blocked)
	start := time.Now()
	timeoutResp := sendToFileWorker("timeout.dat", fileRequest{op: "read", resp: make(chan fileResponse, 1)})
	fileWorkers.Delete(timeoutPath)
	if timeoutResp.err == nil || !strings.Contains(timeoutResp.err.Error(), "send timeout") {
		t.Fatalf("timeout response error=%v", timeoutResp.err)
	}
	if elapsed := time.Since(start); elapsed < time.Second {
		t.Fatalf("send timeout returned too quickly: %s", elapsed)
	}

	crashPath := filepath.Join(basePath, "crash.dat")
	crashing := make(chan fileRequest, 1)
	fileWorkers.Store(crashPath, crashing)
	go func() {
		req := <-crashing
		close(req.resp)
	}()
	crashResp := sendToFileWorker("crash.dat", fileRequest{op: "read", resp: make(chan fileResponse, 1)})
	fileWorkers.Delete(crashPath)
	if crashResp.err == nil || !strings.Contains(crashResp.err.Error(), "worker crashed") {
		t.Fatalf("worker crashed response error=%v", crashResp.err)
	}

	ShutdownWorkersForTests()
}

func TestHandleDeleteIncFileErrorPaths(t *testing.T) {
	setupDataManagerTest(t)

	path := filepath.Join(baseIncTablesPath, "closed.tbl")
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		t.Fatalf("open file: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close file: %v", err)
	}
	if err := handleDeleteIncFile(&file, path); err == nil {
		t.Fatal("expected close error for already closed file")
	}

	dirPath := filepath.Join(baseIncTablesPath, "not-empty-dir")
	if err := os.MkdirAll(filepath.Join(dirPath, "child"), 0o755); err != nil {
		t.Fatalf("mkdir non-empty dir: %v", err)
	}
	var nilFile *os.File
	if err := handleDeleteIncFile(&nilFile, dirPath); err == nil {
		t.Fatal("expected remove error for non-empty directory")
	}

	missingParentPath := filepath.Join(baseIncTablesPath, "missing-parent", "table.tbl")
	if err := handleDeleteIncFile(&nilFile, missingParentPath); err == nil {
		t.Fatal("expected reopen error when parent directory is missing")
	}

	var reopenedTemp string
	removePath = func(string) error {
		return errors.New("remove failed")
	}
	openPathFile = func(string, int, os.FileMode) (*os.File, error) {
		f, err := os.CreateTemp("", "tsunamidb-reopen-*")
		if err == nil {
			reopenedTemp = f.Name()
		}
		return f, err
	}
	t.Cleanup(func() {
		removePath = os.Remove
		openPathFile = os.OpenFile
		if reopenedTemp != "" {
			_ = os.Remove(reopenedTemp)
		}
	})
	if err := handleDeleteIncFile(&nilFile, filepath.Join(baseIncTablesPath, "reopen.tbl")); err == nil {
		t.Fatal("expected remove error after successful reopen")
	}
	if nilFile == nil {
		t.Fatal("expected file pointer to be replaced after successful reopen")
	}
	_ = nilFile.Close()
}

func TestExecuteBatchDirectErrorAndPaddingPaths(t *testing.T) {
	setupDataManagerTest(t)

	filePath := filepath.Join(baseIncTablesPath, "direct.tbl")
	file, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		t.Fatalf("open direct file: %v", err)
	}
	defer file.Close()

	if _, err := file.Write([]byte{0xAA}); err != nil {
		t.Fatalf("seed unaligned byte: %v", err)
	}

	entrySize := uint64(5)
	record := encodedIncEntry(t, entrySize, "abc")
	writeResp := make(chan fileResponse, 1)
	executeBatch(file, "direct.tbl", []fileRequest{{
		op:        "write_inc",
		data:      record,
		entrySize: entrySize,
		resp:      writeResp,
	}})
	resp := <-writeResp
	if resp.err != nil {
		t.Fatalf("write_inc after padding: %v", resp.err)
	}
	if gotID := binary.LittleEndian.Uint64(resp.data); gotID != 1 {
		t.Fatalf("padded write id=%d want=1", gotID)
	}

	invalidSize := ^uint64(0) - 2
	for _, tc := range []struct {
		name string
		req  fileRequest
		want string
	}{
		{
			name: "write inc overwrite invalid entry size",
			req:  fileRequest{op: "write_inc_ow", data: []byte{}, entrySize: invalidSize},
			want: "invalid entry size",
		},
		{
			name: "write inc invalid entry size",
			req:  fileRequest{op: "write_inc", data: []byte{}, entrySize: invalidSize},
			want: "invalid entry size",
		},
		{
			name: "write inc overwrite length mismatch",
			req:  fileRequest{op: "write_inc_ow", data: []byte("short"), entrySize: entrySize, read_type: 0},
			want: "data length mismatch",
		},
		{
			name: "write inc overwrite invalid read type",
			req:  fileRequest{op: "write_inc_ow", data: record, entrySize: entrySize, read_type: 9},
			want: "invalid read_type",
		},
		{
			name: "read inc invalid entry size",
			req:  fileRequest{op: "read_inc", entrySize: invalidSize, read_type: 0},
			want: "invalid entry size",
		},
		{
			name: "read inc invalid read type",
			req:  fileRequest{op: "read_inc", entrySize: entrySize, read_type: 9},
			want: "invalid read_type",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.req.resp = make(chan fileResponse, 1)
			executeBatch(file, "direct.tbl", []fileRequest{tc.req})
			resp := <-tc.req.resp
			if resp.err == nil || !strings.Contains(resp.err.Error(), tc.want) {
				t.Fatalf("error=%v want contains %q", resp.err, tc.want)
			}
		})
	}

	unalignedOWPath := filepath.Join(baseIncTablesPath, "unaligned-ow.tbl")
	unalignedOW, err := os.OpenFile(unalignedOWPath, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		t.Fatalf("open unaligned overwrite file: %v", err)
	}
	defer unalignedOW.Close()
	if _, err := unalignedOW.Write([]byte{0xAA}); err != nil {
		t.Fatalf("seed unaligned overwrite byte: %v", err)
	}
	owResp := make(chan fileResponse, 1)
	executeBatch(unalignedOW, "unaligned-ow.tbl", []fileRequest{{
		op:         "write_inc_ow",
		data:       record,
		entrySize:  entrySize,
		read_type:  0,
		count_from: "bottom",
		resp:       owResp,
	}})
	if resp := <-owResp; resp.err != nil {
		t.Fatalf("write_inc_ow after padding: %v", resp.err)
	}

	closedFile, err := os.OpenFile(filepath.Join(baseIncTablesPath, "closed-direct.tbl"), os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		t.Fatalf("open closed direct file: %v", err)
	}
	if err := closedFile.Close(); err != nil {
		t.Fatalf("close direct file: %v", err)
	}

	for _, tc := range []fileRequest{
		{op: "write_inc", data: record, entrySize: entrySize},
		{op: "write_inc_ow", data: record, entrySize: entrySize, read_type: 0},
		{op: "read_inc", entrySize: entrySize, read_type: 2, inc_id: 1},
		{op: "read", startPtr: 0, endPtr: 1},
	} {
		tc.resp = make(chan fileResponse, 1)
		executeBatch(closedFile, "closed-direct.tbl", []fileRequest{tc})
		resp := <-tc.resp
		if resp.err == nil {
			t.Fatalf("expected closed file error for op %s", tc.op)
		}
	}

	writeReqResp := make(chan fileResponse, 2)
	executeBatch(closedFile, "closed-direct.tbl", []fileRequest{{
		op:   "write",
		data: []byte("x"),
		resp: writeReqResp,
	}})
	if resp := <-writeReqResp; resp.err == nil {
		t.Fatal("expected closed file seek error for write op")
	}
	if resp := <-writeReqResp; resp.err == nil {
		t.Fatal("expected closed file write error for write op")
	}
}

func TestExecuteBatchPropagatesScriptedFileErrors(t *testing.T) {
	setupDataManagerTest(t)

	entrySize := uint64(5)
	recordSize := int64(entrySize) + 3
	record := encodedIncEntry(t, entrySize, "abc")
	writeErr := errors.New("write failed")
	readErr := errors.New("read failed")
	truncErr := errors.New("truncate failed")

	tests := []struct {
		name string
		file *fakeBatchFile
		req  fileRequest
		want string
	}{
		{
			name: "overwrite padding write error",
			file: &fakeBatchFile{size: 1, writeAtErr: writeErr},
			req:  fileRequest{op: "write_inc_ow", data: record, entrySize: entrySize, read_type: 0},
			want: "write failed",
		},
		{
			name: "overwrite truncate error",
			file: &fakeBatchFile{size: 0, truncErr: truncErr},
			req:  fileRequest{op: "write_inc_ow", data: record, entrySize: entrySize, read_type: 0},
			want: "truncate failed",
		},
		{
			name: "overwrite insert move read error",
			file: &fakeBatchFile{size: recordSize, readAtErr: readErr},
			req:  fileRequest{op: "write_inc_ow", data: record, entrySize: entrySize, read_type: 0, inc_id: 0},
			want: "read failed",
		},
		{
			name: "overwrite insert move write error",
			file: &fakeBatchFile{size: recordSize, writeAtErr: writeErr},
			req:  fileRequest{op: "write_inc_ow", data: record, entrySize: entrySize, read_type: 0, inc_id: 0},
			want: "write failed",
		},
		{
			name: "overwrite insert final write error",
			file: &fakeBatchFile{size: 0, writeAtErr: writeErr},
			req:  fileRequest{op: "write_inc_ow", data: record, entrySize: entrySize, read_type: 0},
			want: "write failed",
		},
		{
			name: "overwrite existing write error",
			file: &fakeBatchFile{size: recordSize, writeAtErr: writeErr},
			req:  fileRequest{op: "write_inc_ow", data: record, entrySize: entrySize, read_type: 1},
			want: "write failed",
		},
		{
			name: "write inc padding write error",
			file: &fakeBatchFile{size: 1, writeAtErr: writeErr},
			req:  fileRequest{op: "write_inc", data: record, entrySize: entrySize},
			want: "write failed",
		},
		{
			name: "write inc final write error",
			file: &fakeBatchFile{size: 0, writeAtErr: writeErr},
			req:  fileRequest{op: "write_inc", data: record, entrySize: entrySize},
			want: "write failed",
		},
		{
			name: "read inc by id read error",
			file: &fakeBatchFile{size: recordSize, readAtErr: readErr},
			req:  fileRequest{op: "read_inc", entrySize: entrySize, read_type: 0},
			want: "read failed",
		},
		{
			name: "read inc last read error",
			file: &fakeBatchFile{size: recordSize * 2, readAtErr: readErr},
			req:  fileRequest{op: "read_inc", entrySize: entrySize, read_type: 1, inc_id: 1},
			want: "read failed",
		},
		{
			name: "read inc first read error",
			file: &fakeBatchFile{size: recordSize * 2, readAtErr: readErr},
			req:  fileRequest{op: "read_inc", entrySize: entrySize, read_type: 2, inc_id: 1},
			want: "read failed",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp := executeSingleForTest(tc.file, tc.req)
			if resp.err == nil || !strings.Contains(resp.err.Error(), tc.want) {
				t.Fatalf("error=%v want contains %q", resp.err, tc.want)
			}
		})
	}
}

func TestSaveIncWrapperPropagatesWorkerErrorsAndMissingID(t *testing.T) {
	setupDataManagerTest(t)

	tests := []struct {
		name string
		call func(string) (uint64, error)
	}{
		{
			name: "append",
			call: func(path string) (uint64, error) {
				return SaveIncDataToFileAsync([]byte("ignored"), path, 8)
			},
		},
		{
			name: "put",
			call: func(path string) (uint64, error) {
				return SaveIncDataToFileAsync_Put([]byte("ignored"), path, 8, 0, "bottom")
			},
		},
		{
			name: "overwrite",
			call: func(path string) (uint64, error) {
				return SaveIncDataToFileAsync_OverWrite([]byte("ignored"), path, 8, 0, "bottom")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name+" error", func(t *testing.T) {
			path := tc.name + "-error.tbl"
			ch := make(chan fileRequest, 1)
			fileWorkers.Store(filepath.Join(baseIncTablesPath, path), ch)
			go func() {
				req := <-ch
				req.resp <- fileResponse{err: errors.New("worker failure")}
			}()
			_, err := tc.call(path)
			fileWorkers.Delete(filepath.Join(baseIncTablesPath, path))
			if err == nil || !strings.Contains(err.Error(), "worker failure") {
				t.Fatalf("error=%v", err)
			}
		})

		t.Run(tc.name+" missing id", func(t *testing.T) {
			path := tc.name + "-missing-id.tbl"
			ch := make(chan fileRequest, 1)
			fileWorkers.Store(filepath.Join(baseIncTablesPath, path), ch)
			go func() {
				req := <-ch
				req.resp <- fileResponse{data: []byte{1, 2, 3}}
			}()
			_, err := tc.call(path)
			fileWorkers.Delete(filepath.Join(baseIncTablesPath, path))
			if err == nil || !strings.Contains(err.Error(), "missing id") {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func TestFileWorkerLoopCloseWithoutResponse(t *testing.T) {
	setupDataManagerTest(t)

	fullPath := filepath.Join(basePath, "close-no-response.dat")
	ch := make(chan fileRequest, 1)
	done := make(chan struct{})
	go func() {
		fileWorkerLoop(fullPath, "close-no-response.dat", ch)
		close(done)
	}()

	ch <- fileRequest{op: "close"}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("fileWorkerLoop did not exit on close without response")
	}
}

func TestFileWorkerLoopPanicClosesChannel(t *testing.T) {
	setupDataManagerTest(t)

	ch := make(chan fileRequest, 1)
	go fileWorkerLoop("bad"+string([]byte{0})+string(os.PathSeparator)+"file.dat", "bad-path.dat", ch)

	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected worker channel to be closed after panic")
		}
	case <-time.After(time.Second):
		t.Fatal("worker channel was not closed after open panic")
	}

	dirPath := filepath.Join(basePath, "directory-as-file")
	if err := os.MkdirAll(dirPath, 0o755); err != nil {
		t.Fatalf("mkdir directory-as-file: %v", err)
	}
	ch = make(chan fileRequest, 1)
	go fileWorkerLoop(dirPath, "directory-as-file", ch)

	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected worker channel to be closed after directory open panic")
		}
	case <-time.After(time.Second):
		t.Fatal("worker channel was not closed after directory open panic")
	}
}

func TestFileWorkerLoopDeletesPendingBatchBeforeDelete(t *testing.T) {
	setupDataManagerTest(t)

	fullPath := filepath.Join(baseIncTablesPath, "pending-delete.tbl")
	ch := make(chan fileRequest, 2)
	done := make(chan struct{})
	go func() {
		fileWorkerLoop(fullPath, "pending-delete.tbl", ch)
		close(done)
	}()

	entrySize := uint64(4)
	writeResp := make(chan fileResponse, 1)
	deleteResp := make(chan fileResponse, 1)
	ch <- fileRequest{op: "write_inc", data: encodedIncEntry(t, entrySize, "x"), entrySize: entrySize, resp: writeResp}
	ch <- fileRequest{op: "delete_inc", resp: deleteResp}

	if resp := <-writeResp; resp.err != nil {
		t.Fatalf("pending write before delete: %v", resp.err)
	}
	if resp := <-deleteResp; resp.err != nil {
		t.Fatalf("delete pending: %v", resp.err)
	}

	resp := make(chan fileResponse, 1)
	ch <- fileRequest{op: "close", resp: resp}
	<-resp
	<-done
}

func TestFileWorkerLoopDrainsQueuedNormalRequests(t *testing.T) {
	setupDataManagerTest(t)

	fullPath := filepath.Join(baseIncTablesPath, "drain-normal.tbl")
	ch := make(chan fileRequest, 3)
	done := make(chan struct{})
	go func() {
		fileWorkerLoop(fullPath, "drain-normal.tbl", ch)
		close(done)
	}()

	entrySize := uint64(4)
	firstResp := make(chan fileResponse, 1)
	secondResp := make(chan fileResponse, 1)
	ch <- fileRequest{op: "write_inc", data: encodedIncEntry(t, entrySize, "a"), entrySize: entrySize, resp: firstResp}
	ch <- fileRequest{op: "write_inc", data: encodedIncEntry(t, entrySize, "b"), entrySize: entrySize, resp: secondResp}

	if resp := <-firstResp; resp.err != nil {
		t.Fatalf("first queued write: %v", resp.err)
	}
	if resp := <-secondResp; resp.err != nil {
		t.Fatalf("second queued write: %v", resp.err)
	}

	resp := make(chan fileResponse, 1)
	ch <- fileRequest{op: "close", resp: resp}
	<-resp
	<-done
}

func TestReadIncFirstAndLastPropagateWorkerErrors(t *testing.T) {
	setupDataManagerTest(t)

	for _, tc := range []struct {
		name string
		call func(string) ([]byte, error)
	}{
		{
			name: "last",
			call: func(path string) ([]byte, error) {
				return ReadIncDataFromFileAsync_LastEntries(path, 1, 8)
			},
		},
		{
			name: "first",
			call: func(path string) ([]byte, error) {
				return ReadIncDataFromFileAsync_FirstEntries(path, 1, 8)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := tc.name + "-read-error.tbl"
			fullPath := filepath.Join(baseIncTablesPath, path)
			ch := make(chan fileRequest, 1)
			fileWorkers.Store(fullPath, ch)
			go func() {
				req := <-ch
				req.resp <- fileResponse{err: errors.New("read failure")}
			}()
			_, err := tc.call(path)
			fileWorkers.Delete(fullPath)
			if err == nil || !strings.Contains(err.Error(), "read failure") {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func TestSetupCanResetWorkerMap(t *testing.T) {
	setupDataManagerTest(t)

	fileWorkers.Store(filepath.Join(basePath, "stale.dat"), make(chan fileRequest))
	fileWorkers = sync.Map{}
}
