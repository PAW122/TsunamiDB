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

var tableLocks sync.Map // map[string]*sync.Mutex

func tableLock(table string) *sync.Mutex {
	lock, _ := tableLocks.LoadOrStore(table, &sync.Mutex{})
	return lock.(*sync.Mutex)
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
