package dataManager_v2

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/PAW122/TsunamiDB/data/defragmentationManager"
	debug "github.com/PAW122/TsunamiDB/servers/debug"
)

var basePath = "./db/data"
var fileLocks sync.Map
var bufferPool = sync.Pool{
	New: func() interface{} {
		return make([]byte, 8192) // Bufor 8KB dla operacji zapisu
	},
}

// Zestaw workerów do obsługi zapisów
var saveQueue = make(chan saveRequest, 100)

// Struktura zapytania o zapis

type saveRequest struct {
	data     []byte
	filePath string
	result   chan saveResult
}

type saveResult struct {
	startPtr int64
	endPtr   int64
	err      error
}

func init() {
	for i := 0; i < 8; i++ { // 8 równoległych workerów
		go saveWorker()
	}
}

func saveWorker() {
	for req := range saveQueue {
		startPtr, endPtr, err := saveData(req.data, req.filePath)
		req.result <- saveResult{startPtr, endPtr, err}
	}
}

func ensureDirExists(filePath string) error {
	dir := filepath.Dir(filePath)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return os.MkdirAll(dir, 0755)
	}
	return nil
}

func SaveDataToFileAsync(data []byte, filePath string) (int64, int64, error) {
	defer debug.MeasureTime("save-to-file")()

	resultChan := make(chan saveResult)
	saveQueue <- saveRequest{
		data:     data,
		filePath: filePath,
		result:   resultChan,
	}

	res := <-resultChan
	return res.startPtr, res.endPtr, res.err
}

func saveData(data []byte, filePath string) (int64, int64, error) {
	fullPath := filepath.Join(basePath, filePath)
	if err := ensureDirExists(fullPath); err != nil {
		return 0, 0, fmt.Errorf("błąd tworzenia katalogu: %w", err)
	}

	lock, _ := fileLocks.LoadOrStore(fullPath, &sync.Mutex{})
	fileLock := lock.(*sync.Mutex)

	fileLock.Lock()
	defer fileLock.Unlock()

	file, err := os.OpenFile(fullPath, os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return 0, 0, fmt.Errorf("błąd otwierania pliku: %w", err)
	}
	defer file.Close()

	buffer := bufferPool.Get().([]byte)
	defer bufferPool.Put(buffer)

	freeBlock, err := defragmentationManager.GetBlock(int64(len(data)), filePath)
	var startPtr, endPtr int64

	if err != nil && err.Error() == "no suitable free blocks available for the specified file" {
		// fmt.Println("DEFRAG DEBUG: No suitable free blocks available, appending to the end of the file")
		startPtr, err = file.Seek(0, io.SeekEnd)
		if err != nil {
			return 0, 0, fmt.Errorf("błąd ustawiania wskaźnika pliku: %w", err)
		}
	} else if err == nil {
		// fmt.Println("DEFRAG DEBUG: Using free block", freeBlock)
		startPtr = freeBlock.StartPtr
		_, err := file.Seek(startPtr, io.SeekStart)
		// fmt.Println("DEFRAG DEBUG: Seeked to", resPtr)
		if err != nil {
			return 0, 0, fmt.Errorf("błąd ustawiania wskaźnika pliku: %w", err)
		}
	} else {
		return 0, 0, fmt.Errorf("błąd pobierania wolnego bloku: %w", err)
	}

	_, err = file.Write(data)
	if err != nil {
		return 0, 0, fmt.Errorf("błąd zapisu do pliku: %w", err)
	}

	endPtr = startPtr + int64(len(data))
	defragmentationManager.SaveBlockCheck(startPtr, endPtr)

	return startPtr, endPtr, nil
}
