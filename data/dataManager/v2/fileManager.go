package dataManager_v2

import (
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/PAW122/TsunamiDB/data/defragmentationManager"
)

type fileRequest struct {
	op         string // "read" | "write" | "write_inc" | "write_inc_ow" | "read_inc" | "delete_inc"
	data       []byte
	startPtr   int64
	endPtr     int64
	entrySize  uint64 // używane dla incTables
	inc_id     uint64 // używane dla incTables
	read_type  uint8  // 0 = by id, 1 = last N entries, 2 = first N entries (używane dla incTables)
	count_from string // top | bottom incTables save using custom id
	resp       chan fileResponse
}

type fileResponse struct {
	data     []byte
	startPtr int64
	endPtr   int64
	err      error
}

var (
	fileWorkers       sync.Map // map[string]chan fileRequest
	basePath          = "./db/data"
	baseIncTablesPath = "./db/inc_tables"
	batchInterval     = 5 * time.Millisecond
)

func init() {
	err := os.MkdirAll(basePath, 0755)
	if err != nil {
		panic("Cannot create base directory: " + err.Error())
	}

	err = os.MkdirAll(baseIncTablesPath, 0755)
	if err != nil {
		panic("Cannot create base directory: " + err.Error())
	}
}

func sendToFileWorker(filePath string, req fileRequest) fileResponse {
	// Dla write_inc i read_inc korzystamy z osobnego katalogu inc_tables
	var fullPath string
	if req.op == "write_inc" || req.op == "write_inc_ow" || req.op == "read_inc" || req.op == "delete_inc" {
		fullPath = filepath.Join(baseIncTablesPath, filePath)
	} else {
		fullPath = filepath.Join(basePath, filePath)
	}

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

	select {
	case workerChan <- req:
	case <-time.After(1 * time.Second):
		return fileResponse{err: errors.New("worker is unresponsive (send timeout)")}
	}

	resp, ok := <-req.resp
	if !ok {
		return fileResponse{err: errors.New("worker crashed")}
	}
	return resp
}

func handleDeleteIncFile(file **os.File, fullPath string) error {
	if *file != nil {
		if err := (*file).Close(); err != nil {
			return err
		}
	}

	if err := os.Remove(fullPath); err != nil {
		if !os.IsNotExist(err) {
			reopen, reopenErr := os.OpenFile(fullPath, os.O_RDWR|os.O_CREATE, 0644)
			if reopenErr == nil {
				*file = reopen
			}
			return err
		}
	}

	reopen, err := os.OpenFile(fullPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	*file = reopen
	return nil
}

func fileWorkerLoop(fullPath string, logicalPath string, ch chan fileRequest) {
	defer func() {
		if r := recover(); r != nil {
			close(ch)
		}
	}()

	var (
		pending []fileRequest
		ticker  = time.NewTicker(batchInterval)
	)
	defer ticker.Stop()

	// Otwórz plik raz przed pętlą
	file, err := os.OpenFile(fullPath, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		panic("Cannot open file: " + err.Error())
	}
	defer file.Close()

	for {
		select {
		case req := <-ch:
			if req.op == "delete_inc" {
				if len(pending) > 0 {
					executeBatch(file, logicalPath, pending)
					pending = pending[:0]
				}
				err := handleDeleteIncFile(&file, fullPath)
				req.resp <- fileResponse{err: err}
				continue
			}

			pending = append(pending, req)
		collectLoop:
			for {
				select {
				case req := <-ch:
					if req.op == "delete_inc" {
						if len(pending) > 0 {
							executeBatch(file, logicalPath, pending)
							pending = pending[:0]
						}
						err := handleDeleteIncFile(&file, fullPath)
						req.resp <- fileResponse{err: err}
						continue collectLoop
					}
					pending = append(pending, req)
				default:
					break collectLoop
				}
			}

			if len(pending) > 0 {
				executeBatch(file, logicalPath, pending)
				pending = pending[:0]
			}

		case <-ticker.C:
			if len(pending) > 0 {
				executeBatch(file, logicalPath, pending)
				pending = pending[:0]
			}
		}
	}
}

// todo - potencjalna optymalizacja - tylko 1 przejście for po batchu
func executeBatch(file *os.File, filePath string, batch []fileRequest) {
	// 1) Wydziel write_inc (append-only, stały rekord) ORAZ write (stary tryb)
	var writeIncReqs []*fileRequest
	var overWriteIncReqs []*fileRequest
	var writeReqs []*fileRequest

	for i := range batch {
		switch batch[i].op {
		case "write_inc":
			writeIncReqs = append(writeIncReqs, &batch[i])
		case "write_inc_ow":
			overWriteIncReqs = append(overWriteIncReqs, &batch[i])
		case "write":
			writeReqs = append(writeReqs, &batch[i])
		}
	}

	if len(overWriteIncReqs) > 0 {
		for _, req := range overWriteIncReqs {
			recordSize := int64(req.entrySize) + 3
			if recordSize <= 0 {
				req.resp <- fileResponse{err: errors.New("write_inc_ow: invalid entry size")}
				continue
			}
			if int64(len(req.data)) != recordSize {
				req.resp <- fileResponse{err: errors.New("write_inc_ow: data length mismatch with record size")}
				continue
			}

			// aktualny rozmiar pliku
			fileSize, err := file.Seek(0, io.SeekEnd)
			if err != nil {
				req.resp <- fileResponse{err: err}
				continue
			}
			// wyrównanie pliku do recordSize (sanity)
			if rem := fileSize % recordSize; rem != 0 {
				pad := make([]byte, recordSize-rem)
				if _, err := file.WriteAt(pad, fileSize); err != nil {
					req.resp <- fileResponse{err: err}
					continue
				}
				fileSize += int64(len(pad))
			}

			numRecords := fileSize / recordSize
			prefID := int64(req.inc_id)
			from := strings.ToLower(req.count_from) // "top" | "bottom"

			switch req.read_type {
			case 0: // INSERT (wstaw w prefID, przesuwając ogon)
				// mapowanie prefID -> effID
				var effID int64
				switch from {
				case "top":
					// dozwolone 0..numRecords (0 => jako najnowszy; numRecords => jako najstarszy)
					if prefID < 0 || prefID > numRecords {
						req.resp <- fileResponse{err: errors.New("write_inc_ow: insert id out of range (top)")}
						continue
					}
					// wstaw w miejsce liczone od dołu:
					// 0(top) -> effID = numRecords (append na koniec = najnowszy)
					// 1(top) -> effID = numRecords-1 (tuż pod najnowszym)
					effID = numRecords - prefID
				default: // bottom (domyślnie)
					if prefID < 0 || prefID > numRecords {
						req.resp <- fileResponse{err: errors.New("write_inc_ow: insert id out of range (bottom)")}
						continue
					}
					effID = prefID
				}

				// powiększ plik o jeden rekord
				newSize := fileSize + recordSize
				if err := file.Truncate(newSize); err != nil {
					req.resp <- fileResponse{err: err}
					continue
				}

				// przesuń ogon od końca do effID (reverse-copy, aby nie nadpisać nieprzeczytanych danych)
				failed := false
				buf := make([]byte, recordSize)
				for srcIdx := numRecords - 1; srcIdx >= effID; srcIdx-- {
					srcOff := srcIdx * recordSize
					dstOff := (srcIdx + 1) * recordSize

					if _, err := file.ReadAt(buf, srcOff); err != nil && err != io.EOF {
						req.resp <- fileResponse{err: err}
						failed = true
						break
					}
					if _, err := file.WriteAt(buf, dstOff); err != nil {
						req.resp <- fileResponse{err: err}
						failed = true
						break
					}
				}
				if failed {
					continue // odpowiedź już wysłana
				}

				// zapisz nowy rekord w effID
				offset := effID * recordSize
				if _, err := file.WriteAt(req.data, offset); err != nil {
					req.resp <- fileResponse{err: err}
					continue
				}

				// zwróć globalny id (bottom-based)
				idBuf := make([]byte, 8)
				binary.LittleEndian.PutUint64(idBuf, uint64(effID))
				req.resp <- fileResponse{
					data:     idBuf,
					startPtr: offset,
					endPtr:   offset + recordSize,
					err:      nil,
				}

			case 1: // OVERWRITE istniejącego
				var effID int64
				switch from {
				case "top":
					// dozwolone 0..numRecords-1 (0 => nadpisz najnowszy)
					if prefID < 0 || prefID >= numRecords {
						req.resp <- fileResponse{err: errors.New("write_inc_ow: overwrite id out of range (top)")}
						continue
					}
					// 0(top) -> effID = numRecords-1 (najnowszy)
					// 1(top) -> effID = numRecords-2
					effID = (numRecords - 1) - prefID
				default: // bottom
					if prefID < 0 || prefID >= numRecords {
						req.resp <- fileResponse{err: errors.New("write_inc_ow: overwrite id out of range (bottom)")}
						continue
					}
					effID = prefID
				}

				offset := effID * recordSize
				if _, err := file.WriteAt(req.data, offset); err != nil {
					req.resp <- fileResponse{err: err}
					continue
				}

				idBuf := make([]byte, 8)
				binary.LittleEndian.PutUint64(idBuf, uint64(effID))
				req.resp <- fileResponse{
					data:     idBuf,
					startPtr: offset,
					endPtr:   offset + recordSize,
					err:      nil,
				}

			default:
				req.resp <- fileResponse{err: errors.New("write_inc_ow: invalid read_type (use 0=insert,1=overwrite)")}
			}
		}
	}

	// 2) Obsłuż write_inc – każdy rekord append-only, policz id po rozmiarze pliku
	if len(writeIncReqs) > 0 {
		for _, req := range writeIncReqs {
			// stały rozmiar rekordu
			recordSize := int64(req.entrySize) + 3
			if recordSize <= 0 {
				req.resp <- fileResponse{err: errors.New("invalid entry size for write_inc")}
				continue
			}
			if int64(len(req.data)) != recordSize {
				req.resp <- fileResponse{err: errors.New("write_inc: data length mismatch with record size")}
				continue
			}

			// aktualny rozmiar pliku (EOF)
			fileSize, err := file.Seek(0, io.SeekEnd)
			if err != nil {
				req.resp <- fileResponse{err: err}
				continue
			}

			// jeśli plik nie jest wyrównany do recordSize, dopełnij zerami (ochrona przed uszkodzeniem)
			if rem := fileSize % recordSize; rem != 0 {
				pad := make([]byte, recordSize-rem)
				if _, err := file.WriteAt(pad, fileSize); err != nil {
					req.resp <- fileResponse{err: err}
					continue
				}
				fileSize += int64(len(pad))
			}

			// wylicz id i offset
			id := fileSize / recordSize
			offset := id * recordSize
			end := offset + recordSize

			// write-at (append na wyliczonym offsetcie)
			if _, err := file.WriteAt(req.data, offset); err != nil {
				req.resp <- fileResponse{err: err}
				continue
			}

			// zwróć start/end oraz id jako 8B LE w polu data
			idBuf := make([]byte, 8)
			binary.LittleEndian.PutUint64(idBuf, uint64(id))
			req.resp <- fileResponse{
				data:     idBuf,
				startPtr: offset,
				endPtr:   end,
				err:      nil,
			}
		}
	}

	// 3) Stary tryb write (z defragiem) – bez zmian
	if len(writeReqs) > 0 {
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
			if _, err := file.WriteAt(req.data, req.startPtr); err != nil {
				req.resp <- fileResponse{err: err}
				continue
			}
			defragmentationManager.SaveBlockCheck(req.startPtr, req.endPtr)
			req.resp <- fileResponse{startPtr: req.startPtr, endPtr: req.endPtr, err: nil}
		}
	}

	// --- NOWE: obsługa read_inc ---
	for i := range batch {
		req := &batch[i]
		if req.op != "read_inc" {
			continue
		}

		// rekord inc-table ma stały rozmiar: entrySize + 3
		recordSize := int64(req.entrySize) + 3
		if recordSize <= 0 {
			req.resp <- fileResponse{err: errors.New("read_inc: invalid entry size")}
			continue
		}

		// pobierz rozmiar pliku
		fi, err := file.Stat()
		if err != nil {
			req.resp <- fileResponse{err: err}
			continue
		}
		fileSize := fi.Size()
		if fileSize < 0 {
			fileSize = 0
		}
		// liczba pełnych rekordów (ignorujemy ewentualny ogon)
		numRecords := fileSize / recordSize

		switch req.read_type {
		case 0: // by id
			id := int64(req.inc_id)
			if id < 0 || id >= numRecords {
				req.resp <- fileResponse{err: errors.New("read_inc: id out of range")}
				continue
			}
			offset := id * recordSize
			buf := make([]byte, recordSize)
			_, err := file.ReadAt(buf, offset)
			if err != nil && err != io.EOF {
				req.resp <- fileResponse{err: err}
				continue
			}
			req.resp <- fileResponse{
				data:     buf,
				startPtr: offset,
				endPtr:   offset + recordSize,
				err:      nil,
			}

		case 1: // last N (NEWEST -> OLDEST in output)
			n := int64(req.inc_id)
			if n <= 0 || numRecords == 0 {
				req.resp <- fileResponse{data: []byte{}, err: nil}
				continue
			}
			if n > numRecords {
				n = numRecords
			}
			startIndex := numRecords - n
			startOffset := startIndex * recordSize
			totalBytes := n * recordSize

			// czytamy blok last-N w naturalnej kolejności...
			tmp := make([]byte, totalBytes)
			_, err := file.ReadAt(tmp, startOffset)
			if err != nil && err != io.EOF {
				req.resp <- fileResponse{err: err}
				continue
			}
			// ...i odwracamy kolejność rekordów (żeby zwrócić newest->oldest)
			out := make([]byte, totalBytes)
			for i := int64(0); i < n; i++ {
				srcStart := (n - 1 - i) * recordSize
				dstStart := i * recordSize
				copy(out[dstStart:dstStart+recordSize], tmp[srcStart:srcStart+recordSize])
			}
			req.resp <- fileResponse{
				data:     out,
				startPtr: startOffset,
				endPtr:   startOffset + totalBytes,
				err:      nil,
			}

		case 2: // first N (OLDEST -> NEWER)
			n := int64(req.inc_id)
			if n <= 0 || numRecords == 0 {
				req.resp <- fileResponse{data: []byte{}, err: nil}
				continue
			}
			if n > numRecords {
				n = numRecords
			}
			totalBytes := n * recordSize
			buf := make([]byte, totalBytes)
			_, err := file.ReadAt(buf, 0)
			if err != nil && err != io.EOF {
				req.resp <- fileResponse{err: err}
				continue
			}
			req.resp <- fileResponse{
				data:     buf,
				startPtr: 0,
				endPtr:   totalBytes,
				err:      nil,
			}

		default:
			req.resp <- fileResponse{err: errors.New("read_inc: invalid read_type")}
		}
	}

	// 4) Obsłuż odczyty
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
