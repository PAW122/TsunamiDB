package tasks

import (
	"errors"
	"os"
	"testing"
	"time"

	dataManager_v2 "github.com/PAW122/TsunamiDB/data/dataManager/v2"
	defrag "github.com/PAW122/TsunamiDB/data/defragmentationManager"
	fileSystem_v1 "github.com/PAW122/TsunamiDB/data/fileSystem/v1"
	types "github.com/PAW122/TsunamiDB/types"
)

func TestMain(m *testing.M) {
	_ = os.RemoveAll("./db")
	code := m.Run()
	dataManager_v2.ShutdownWorkersForTests()
	fileSystem_v1.ResetForTests()
	defrag.ResetForTests()
	_ = os.RemoveAll("./db")
	time.Sleep(50 * time.Millisecond)
	_ = os.RemoveAll("./db")
	os.Exit(code)
}

func setupTasksTest(t *testing.T) {
	t.Helper()
	release := acquireDBTestLock(t)
	t.Cleanup(release)
	dataManager_v2.ShutdownWorkersForTests()
	fileSystem_v1.ResetForTests()
	defrag.ResetForTests()
	_ = os.RemoveAll("./db")
	dataManager_v2.EnsureDirsForTests()
	t.Cleanup(func() {
		dataManager_v2.ShutdownWorkersForTests()
		fileSystem_v1.ResetForTests()
		defrag.ResetForTests()
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

func TestSaveReadAndFreeTask(t *testing.T) {
	setupTasksTest(t)

	saveReq := types.NMmessage{Task: "save", Args: []string{"table", "key"}, Content: []byte("payload")}
	saveRes := Save(saveReq)
	if !saveRes.Finished {
		t.Fatalf("save did not finish")
	}
	if len(saveRes.Content) != 0 {
		t.Fatalf("save response should drop content")
	}

	readRes := Read(types.NMmessage{Task: "read", Args: []string{"table", "key"}})
	if !readRes.Finished {
		t.Fatalf("read did not finish")
	}
	if string(readRes.Content) != "payload" {
		t.Fatalf("unexpected content: %q", readRes.Content)
	}

	freeRes := Free(types.NMmessage{Task: "free", Args: []string{"table", "key"}})
	if !freeRes.Finished {
		t.Fatalf("free did not finish")
	}

	missing := Read(types.NMmessage{Task: "read", Args: []string{"table", "key"}})
	if missing.Finished {
		t.Fatalf("expected read after free to fail")
	}
}

func TestTasksValidateArgumentsAndMissingData(t *testing.T) {
	setupTasksTest(t)

	cases := []struct {
		name string
		fn   func(types.NMmessage) types.NMmessage
		req  types.NMmessage
	}{
		{"save no args", Save, types.NMmessage{}},
		{"save empty file", Save, types.NMmessage{Args: []string{"", "key"}}},
		{"read no args", Read, types.NMmessage{}},
		{"read missing", Read, types.NMmessage{Args: []string{"table", "missing"}}},
		{"free no args", Free, types.NMmessage{}},
		{"free missing", Free, types.NMmessage{Args: []string{"table", "missing"}}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if res := tc.fn(tc.req); res.Finished {
				t.Fatalf("expected unfinished response")
			}
		})
	}
}
