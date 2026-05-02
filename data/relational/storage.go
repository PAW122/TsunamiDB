package relational

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sync"
)

var (
	mkdirAll    = os.MkdirAll
	openFile    = os.OpenFile
	statFile    = os.Stat
	readFile    = os.ReadFile
	writeFile   = os.WriteFile
	removeFile  = os.Remove
	renameFile  = os.Rename
	marshalJSON = json.MarshalIndent
)

var safeNamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)

var tableLocks sync.Map // map[string]*sync.RWMutex

func tableLock(table string) *sync.RWMutex {
	lock, _ := tableLocks.LoadOrStore(table, &sync.RWMutex{})
	return lock.(*sync.RWMutex)
}

type cachedRowsFile struct {
	file *os.File
}

var rowsFiles sync.Map // map[string]*cachedRowsFile

func rowsFileKey(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return abs
}

func openRowsFile(path string, flag int, perm os.FileMode) (*os.File, error) {
	key := rowsFileKey(path)
	if cached, ok := rowsFiles.Load(key); ok {
		return cached.(*cachedRowsFile).file, nil
	}

	if flag&os.O_CREATE != 0 {
		if err := mkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, err
		}
	}
	file, err := openFile(path, flag, perm)
	if err != nil {
		return nil, err
	}

	actual, loaded := rowsFiles.LoadOrStore(key, &cachedRowsFile{file: file})
	if loaded {
		_ = file.Close()
		return actual.(*cachedRowsFile).file, nil
	}
	return file, nil
}

func closeRowsFile(path string) error {
	key := rowsFileKey(path)
	cached, ok := rowsFiles.LoadAndDelete(key)
	if !ok {
		return nil
	}
	return cached.(*cachedRowsFile).file.Close()
}

func resetRowsFilesForTests() {
	rowsFiles.Range(func(key, value any) bool {
		_ = value.(*cachedRowsFile).file.Close()
		rowsFiles.Delete(key)
		return true
	})
}

func ResetForTests() {
	resetRowsFilesForTests()
	resetSchemaCacheForTests()
	tableLocks = sync.Map{}
}

func ensureEmptyFile(path string) error {
	file, err := openFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o644)
	if err == nil {
		return file.Close()
	}
	if errors.Is(err, os.ErrExist) {
		return nil
	}
	return err
}

type paths struct {
	schema string
	rows   string
	free   string
}

func tablePaths(table string) paths {
	return paths{
		schema: filepath.Join(baseDir, table+".schema"),
		rows:   filepath.Join(baseDir, table+".rows"),
		free:   filepath.Join(baseDir, table+".free"),
	}
}

func indexPath(table, column string) string {
	return filepath.Join(baseDir, table+".index."+column)
}

func trigramIndexPath(table, column string) string {
	return filepath.Join(baseDir, table+".index."+column+".trigram")
}
