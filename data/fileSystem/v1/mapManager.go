package fileSystem_v1

import (
	"bufio"
	"errors"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	dbg "github.com/PAW122/TsunamiDB/servers/debug"
)

// ---------------------------------------------------
// Typ trzymany w RAM
// ---------------------------------------------------
type GetElement_output struct {
	Key      string
	FileName string
	StartPtr int
	EndPtr   int
}

type walOp struct {
	op       byte
	key      string
	fileName string
	start    int
	end      int
}

var (
	snapPath = "./db/maps/data_map.snap"
	walPath  = "./db/maps/data_map.wal"
)

const numShards = 256

type shard struct {
	mu sync.RWMutex
	m  map[string]GetElement_output
}

var shardedData [numShards]*shard

func getShard(key string) *shard {
	h := fnv.New32a()
	h.Write([]byte(key))
	return shardedData[uint(h.Sum32())%numShards]
}

var (
	regexCache sync.Map
	walChan    = make(chan walOp, 100_000)
	walFile    *os.File
	walBuf     *bufio.Writer
	walMu      sync.Mutex
)

func init() {
	for i := 0; i < numShards; i++ {
		shardedData[i] = &shard{m: make(map[string]GetElement_output)}
	}
	ensureDir()
	if err := loadIndex(); err != nil {
		dbg.LogExtra("fileSystem [init] loadIndex error:", err)
	}
	setupWalWriter()
	go snapshotWorker()
}

func ensureDir() {
	dir := filepath.Dir(walPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		panic(err)
	}
}

func loadIndex() error {
	if f, err := os.Open(snapPath); err == nil {
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := sc.Text()
			parts := strings.Split(line, "|")
			if len(parts) != 4 {
				continue
			}
			start, _ := strconv.Atoi(parts[2])
			end, _ := strconv.Atoi(parts[3])
			storeKey(parts[0], GetElement_output{parts[0], parts[1], start, end})
		}
		f.Close()
	}

	if f, err := os.Open(walPath); err == nil {
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := sc.Text()
			parts := strings.Split(line, "|")
			if len(parts) < 2 {
				continue
			}
			if parts[0] == "S" && len(parts) == 5 {
				start, _ := strconv.Atoi(parts[3])
				end, _ := strconv.Atoi(parts[4])
				storeKey(parts[1], GetElement_output{parts[1], parts[2], start, end})
			} else if parts[0] == "D" {
				deleteKey(parts[1])
			}
		}
		f.Close()
	}

	return nil
}

var walOpsProcessed int64

func setupWalWriter() {
	var err error
	walFile, err = os.OpenFile(walPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		panic(err)
	}
	walBuf = bufio.NewWriterSize(walFile, 4<<20)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				dbg.LogExtra("walWriter panic:", r)
			}
		}()

		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		pending := 0

		go func() {
			t := time.NewTicker(5 * time.Second)
			defer t.Stop()
			var lastCount int64
			for range t.C {
				count := atomic.LoadInt64(&walOpsProcessed)
				dbg.LogExtra(fmt.Sprintf("[WAL-WRITER] Processed %d ops (+%d)", count, count-lastCount))
				lastCount = count
			}
		}()

		for {
			select {
			case op := <-walChan:
				if err := writeWalLine(op); err != nil {
					dbg.LogExtra("writeWalLine error:", err)
				}
				pending++
				atomic.AddInt64(&walOpsProcessed, 1)
				if pending >= 100 {
					flushWal()
					pending = 0
				}
			case <-ticker.C:
				if pending > 0 {
					flushWal()
					pending = 0
				}
			}
		}
	}()
}

func writeWalLine(op walOp) error {
	defer dbg.MeasureTime("writeWalLine [mapManager]")()
	var line string
	if op.op == 'S' {
		line = fmt.Sprintf("S|%s|%s|%d|%d\n", op.key, op.fileName, op.start, op.end)
	} else {
		line = fmt.Sprintf("D|%s\n", op.key)
	}
	_, err := walBuf.WriteString(line)
	return err
}

func flushWal() {
	defer dbg.MeasureTime("flushWal [mapManager]")()
	walMu.Lock()
	walBuf.Flush()
	walFile.Sync()
	walMu.Unlock()
}

func snapshotWorker() {
	defer dbg.MeasureTime("snapshotWorker [mapManager]")()
	t := time.NewTicker(5 * time.Minute)
	defer t.Stop()

	for range t.C {
		tmp, _ := os.CreateTemp(filepath.Dir(snapPath), "snap_*")
		bw := bufio.NewWriter(tmp)
		for _, s := range shardedData {
			s.mu.RLock()
			for _, val := range s.m {
				fmt.Fprintf(bw, "%s|%s|%d|%d\n",
					val.Key, val.FileName, val.StartPtr, val.EndPtr)
			}
			s.mu.RUnlock()
		}
		bw.Flush()
		tmp.Sync()
		tmp.Close()
		os.Rename(tmp.Name(), snapPath)

		flushWal()

		walMu.Lock()
		walFile.Close()
		os.Rename(walPath, walPath+".old")
		walFile, _ = os.OpenFile(walPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		walBuf.Reset(walFile)
		walMu.Unlock()

		os.Remove(walPath + ".old")
	}
}

func storeKey(k string, v GetElement_output) {
	defer dbg.MeasureTime("storeKey [mapManager]")()
	s := getShard(k)
	s.mu.Lock()
	s.m[k] = v
	s.mu.Unlock()
}

func deleteKey(k string) {
	defer dbg.MeasureTime("deleteKey [mapManager]")()
	s := getShard(k)
	s.mu.Lock()
	delete(s.m, k)
	s.mu.Unlock()
}

func loadKey(k string) (GetElement_output, bool) {
	defer dbg.MeasureTime("loadKey [mapManager]")()
	s := getShard(k)
	s.mu.RLock()
	val, ok := s.m[k]
	s.mu.RUnlock()
	return val, ok
}

func SaveElementByKey(key, file string, start, end int) error {
	defer dbg.MeasureTime("SaveElementByKey [mapManager]")()
	storeKey(key, GetElement_output{key, file, start, end})
	select {
	case walChan <- walOp{'S', key, file, start, end}:
	case <-time.After(5 * time.Second):
		dbg.LogExtra("WAL blocked for 5s on key:", key)
		return errors.New("WAL timeout")
	default:
		flushWal()
		walChan <- walOp{'S', key, file, start, end}
	}
	return nil
}

func RemoveElementByKey(key string) error {
	defer dbg.MeasureTime("RemoveElementByKey [mapManager]")()
	deleteKey(key)
	select {
	case walChan <- walOp{'D', key, "", 0, 0}:
	default:
		flushWal()
		walChan <- walOp{'D', key, "", 0, 0}
	}
	return nil
}

func GetElementByKey(key string) (*GetElement_output, error) {
	defer dbg.MeasureTime("GetElementByKey [mapManager]")()
	if val, ok := loadKey(key); ok {
		return &val, nil
	}
	return nil, errors.New("key not found")
}

func GetKeysByRegex(pattern string, max int) ([]string, error) {
	defer dbg.MeasureTime("GetKeysByRegex [mapManager]")()
	var rx *regexp.Regexp
	if c, ok := regexCache.Load(pattern); ok {
		rx = c.(*regexp.Regexp)
	} else {
		compiled, err := regexp.Compile(pattern)
		if err != nil {
			return nil, err
		}
		regexCache.Store(pattern, compiled)
		rx = compiled
	}

	out := make([]string, 0, max)
	for _, s := range shardedData {
		s.mu.RLock()
		for k := range s.m {
			if rx.MatchString(k) {
				out = append(out, k)
				if max > 0 && len(out) >= max {
					s.mu.RUnlock()
					return out, nil
				}
			}
		}
		s.mu.RUnlock()
	}
	return out, nil
}
