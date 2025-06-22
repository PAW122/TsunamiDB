package dataManager_v2

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/PAW122/TsunamiDB/data/defragmentationManager"
	debug "github.com/PAW122/TsunamiDB/servers/debug"
)

//	go subServer.NotifySubscribers()
//
// NotifySubscribers
var basePath = "./db/data"
var fileLocks sync.Map

const batchSize = 32
const batchTimeout = 1 * time.Millisecond

var batchQueue = make(chan saveRequest, 100000)

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
	numWorkers := 8 // lub runtime.NumCPU() * 2
	for i := 0; i < numWorkers; i++ {
		go batchSaveWorker()
	}
}

func SaveDataToFileAsync(data []byte, filePath string) (int64, int64, error) {
	defer debug.MeasureTime("save-to-file")()
	resChan := make(chan saveResult, 1)
	batchQueue <- saveRequest{
		data:     data,
		filePath: filePath,
		result:   resChan,
	}
	res := <-resChan
	return res.startPtr, res.endPtr, res.err
}

func batchSaveWorker() {
	var pending []saveRequest
	ticker := time.NewTicker(batchTimeout)
	defer ticker.Stop()

	flush := func() {
		if len(pending) == 0 {
			return
		}

		// Grupujemy zapisy po pliku
		grouped := map[string][]saveRequest{}
		for _, req := range pending {
			grouped[req.filePath] = append(grouped[req.filePath], req)
		}

		for path, group := range grouped {
			writeBatch(path, group)
		}

		pending = pending[:0]
	}

	for {
		select {
		case req := <-batchQueue:
			pending = append(pending, req)
			if len(pending) >= batchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

func writeBatch(filePath string, batch []saveRequest) {
	fullPath := filepath.Join(basePath, filePath)
	if err := ensureDirExists(fullPath); err != nil {
		for _, r := range batch {
			r.result <- saveResult{err: fmt.Errorf("create dir err: %w", err)}
		}
		return
	}

	lock, _ := fileLocks.LoadOrStore(fullPath, &sync.Mutex{})
	fileLock := lock.(*sync.Mutex)

	fileLock.Lock()
	defer fileLock.Unlock()

	file, err := os.OpenFile(fullPath, os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		for _, r := range batch {
			r.result <- saveResult{err: fmt.Errorf("open file err: %w", err)}
		}
		return
	}
	defer file.Close()

	for _, req := range batch {
		freeBlock, err := defragmentationManager.GetBlock(int64(len(req.data)), filePath)
		var startPtr int64

		if err != nil || freeBlock == nil {
			startPtr, err = file.Seek(0, io.SeekEnd)
			if err != nil {
				req.result <- saveResult{err: fmt.Errorf("seek err: %w", err)}
				continue
			}
		} else {
			startPtr = freeBlock.StartPtr
			if _, err := file.Seek(startPtr, io.SeekStart); err != nil {
				req.result <- saveResult{err: fmt.Errorf("seek block err: %w", err)}
				continue
			}
		}

		if _, err := file.Write(req.data); err != nil {
			req.result <- saveResult{err: fmt.Errorf("write err: %w", err)}
			continue
		}

		endPtr := startPtr + int64(len(req.data))
		defragmentationManager.SaveBlockCheck(startPtr, endPtr)
		req.result <- saveResult{startPtr: startPtr, endPtr: endPtr}
	}
}

func ensureDirExists(filePath string) error {
	dir := filepath.Dir(filePath)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return os.MkdirAll(dir, 0755)
	}
	return nil
}
