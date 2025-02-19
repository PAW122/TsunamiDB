package dataManager_v2

import (
	"errors"
	"io"
	"os"
	"path/filepath"

	debug "TsunamiDB/servers/debug"
)

func ReadDataFromFileAsync(filePath string, dataStartPtr int64, dataEndPtr int64) ([]byte, error) {

	defer debug.MeasureTime("read-from-file")()

	file, err := os.Open(filepath.Join(basePath, filePath))
	if err != nil {
		return nil, err
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		return nil, err
	}
	fileSize := fileInfo.Size()

	if dataStartPtr < 0 || dataEndPtr <= dataStartPtr || dataEndPtr > fileSize {
		return nil, errors.New("invalid read range")
	}

	_, err = file.Seek(dataStartPtr, 0)
	if err != nil {
		return nil, err
	}

	buffer := make([]byte, dataEndPtr-dataStartPtr)
	n, err := io.ReadFull(file, buffer)
	if err != nil && err != io.EOF {
		return nil, err
	}

	return buffer[:n], nil
}
