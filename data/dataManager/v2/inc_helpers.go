package dataManager_v2

import (
	"os"
	"path/filepath"
)

func GetIncRecordCount(filePath string, entrySize uint64) (uint64, error) {
	full := filepath.Join(baseIncTablesPath, filePath)
	fi, err := os.Stat(full)
	if err != nil {
		return 0, err
	}
	recordSize := int64(entrySize) + 3
	if recordSize <= 0 {
		return 0, nil
	}
	return uint64(fi.Size() / recordSize), nil
}
