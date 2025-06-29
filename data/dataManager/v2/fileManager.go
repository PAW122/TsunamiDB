package dataManager_v2

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/PAW122/TsunamiDB/data/defragmentationManager"
)

type fileRequest struct {
	op       string // "read" or "write"
	data     []byte
	startPtr int64
	endPtr   int64
	resp     chan fileResponse
}

type fileResponse struct {
	data     []byte
	startPtr int64
	endPtr   int64
	err      error
}

var (
	fileWorkers   sync.Map // map[string]chan fileRequest
	basePath      = "./db/data"
	batchInterval = 5 * time.Millisecond
)

func init() {
	err := os.MkdirAll(basePath, 0755)
	if err != nil {
		panic("Cannot create base directory: " + err.Error())
	}
}

func sendToFileWorker(filePath string, req fileRequest) fileResponse {
	fullPath := filepath.Join(basePath, filePath)

	chAny, loaded := fileWorkers.Load(fullPath)
	if !loaded {
		ch := make(chan fileRequest, 10000)
		actual, _ := fileWorkers.LoadOrStore(fullPath, ch)
		if actual == ch {
			go fileWorkerLoop(fullPath, filePath, ch)
		}
		chAny = actual
	}

	workerChan := chAny.(chan fileRequest)
	workerChan <- req
	return <-req.resp
}

func fileWorkerLoop(fullPath string, logicalPath string, ch chan fileRequest) {
	var (
		pending []fileRequest
		ticker  = time.NewTicker(batchInterval)
		file    *os.File
		err     error
	)
	defer ticker.Stop()

	for {
		select {
		case req := <-ch:
			pending = append(pending, req)
		collectLoop:
			for {
				select {
				case req := <-ch:
					pending = append(pending, req)
				default:
					break collectLoop
				}
			}

			file, err = os.OpenFile(fullPath, os.O_RDWR|os.O_CREATE, 0644)
			if err != nil {
				for _, req := range pending {
					req.resp <- fileResponse{err: err}
				}
				pending = pending[:0]
				continue
			}
			executeBatch(file, logicalPath, pending)
			file.Close()
			pending = pending[:0]

		case <-ticker.C:
			if len(pending) > 0 {
				file, err = os.OpenFile(fullPath, os.O_RDWR|os.O_CREATE, 0644)
				if err != nil {
					for _, req := range pending {
						req.resp <- fileResponse{err: err}
					}
					pending = pending[:0]
					continue
				}
				executeBatch(file, logicalPath, pending)
				file.Close()
				pending = pending[:0]
			}
		}
	}
}

func executeBatch(file *os.File, filePath string, batch []fileRequest) {
	// Rozdziel read i write
	var writeReqs []*fileRequest
	for i := range batch {
		if batch[i].op == "write" {
			writeReqs = append(writeReqs, &batch[i])
		}
	}

	// Przydział wolnego bloku (jeśli istnieje)
	totalSize := int64(0)
	for _, r := range writeReqs {
		totalSize += int64(len(r.data))
	}

	// Przypisz offsety do write'ów
	freeBlock, err := defragmentationManager.GetBlock(totalSize, filePath)
	var curOffset int64
	if err == nil && freeBlock != nil {
		curOffset = freeBlock.StartPtr
		for _, req := range writeReqs {
			req.startPtr = curOffset
			req.endPtr = curOffset + int64(len(req.data))
			curOffset = req.endPtr
		}
	} else {
		for _, req := range writeReqs {
			offset, err := file.Seek(0, io.SeekEnd)
			if err != nil {
				req.resp <- fileResponse{err: err}
				continue
			}
			req.startPtr = offset
			req.endPtr = offset + int64(len(req.data))
		}
	}

	// Wykonaj zapisy
	for _, req := range writeReqs {
		_, err := file.WriteAt(req.data, req.startPtr)
		if err != nil {
			req.resp <- fileResponse{err: err}
			continue
		}
		defragmentationManager.SaveBlockCheck(req.startPtr, req.endPtr)
		req.resp <- fileResponse{startPtr: req.startPtr, endPtr: req.endPtr, err: nil}
	}

	// Obsłuż odczyty
	for _, req := range batch {
		if req.op == "read" {
			if req.startPtr < 0 || req.endPtr <= req.startPtr {
				req.resp <- fileResponse{err: errors.New("invalid read range")}
				continue
			}
			buffer := make([]byte, req.endPtr-req.startPtr)
			_, err := file.ReadAt(buffer, req.startPtr)
			if err != nil && err != io.EOF {
				req.resp <- fileResponse{err: err}
				continue
			}
			req.resp <- fileResponse{data: buffer, startPtr: req.startPtr, endPtr: req.endPtr, err: nil}
		}
	}
}
