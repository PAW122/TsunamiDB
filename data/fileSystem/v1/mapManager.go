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

type entry struct {
	file  string
	start int
	end   int
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
	m  map[string]entry
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

	walSyncMu        sync.Mutex
	walSyncCond      = sync.NewCond(&walSyncMu)
	walSyncRequested int64
	walSyncCompleted int64
)

func init() {
	for i := 0; i < numShards; i++ {
		shardedData[i] = &shard{m: make(map[string]entry)}
	}
	ensureDir()
	if err := loadIndex(); err != nil {
		dbg.LogExtra("fileSystem [init] loadIndex error:", err)
	}
	setupWalWriter()
	go snapshotWorker()
	go debugCountersWorker()
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
			storeKey(parts[0], entry{file: parts[1], start: start, end: end})
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
				storeKey(parts[1], entry{file: parts[2], start: start, end: end})
			} else if parts[0] == "D" {
				deleteKey(parts[1])
			}
		}
		f.Close()
	}

	return nil
}

var walOpsProcessed int64

const lockWarnThreshold = 100 * time.Microsecond

var (
	storeLockSlowCount int64
	defragFreedCount   int64
	defragSkipCount    int64
)

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

func flushWal() int64 {
	defer dbg.MeasureTime("flushWal [mapManager]")()
	walMu.Lock()
	walBuf.Flush()
	walMu.Unlock()

	seq := atomic.AddInt64(&walSyncRequested, 1)
	walSyncMu.Lock()
	walSyncCond.Signal()
	walSyncMu.Unlock()
	return seq
}

func walSyncLoop() {
	var lastSynced int64
	for {
		walSyncMu.Lock()
		for lastSynced == atomic.LoadInt64(&walSyncRequested) {
			walSyncCond.Wait()
		}
		walSyncMu.Unlock()

		if err := walFile.Sync(); err != nil {
			dbg.LogExtra("wal sync error: " + err.Error())
		}

		lastSynced = atomic.LoadInt64(&walSyncRequested)
		atomic.StoreInt64(&walSyncCompleted, lastSynced)

		walSyncMu.Lock()
		walSyncCond.Broadcast()
		walSyncMu.Unlock()
	}
}

func waitForWalSync(seq int64) {
	for {
		if atomic.LoadInt64(&walSyncCompleted) >= seq {
			return
		}
		walSyncMu.Lock()
		for atomic.LoadInt64(&walSyncCompleted) < seq {
			walSyncCond.Wait()
		}
		walSyncMu.Unlock()
	}
}

func debugCountersWorker() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		if v := atomic.SwapInt64(&storeLockSlowCount, 0); v > 0 {
			dbg.LogExtra(fmt.Sprintf("[mapManager] store lock slow waits: %d", v))
		}
		if v := atomic.SwapInt64(&defragFreedCount, 0); v > 0 {
			dbg.LogExtra(fmt.Sprintf("[mapManager] defrag frees: %d", v))
		}
		if v := atomic.SwapInt64(&defragSkipCount, 0); v > 0 {
			dbg.LogExtra(fmt.Sprintf("[mapManager] defrag skips (same span): %d", v))
		}
	}
}

func RecordDefragFree() {
	atomic.AddInt64(&defragFreedCount, 1)
}

func RecordDefragSkip() {
	atomic.AddInt64(&defragSkipCount, 1)
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
			for key, val := range s.m {
				fmt.Fprintf(bw, "%s|%s|%d|%d\n",
					key, val.file, val.start, val.end)
			}
			s.mu.RUnlock()
		}
		bw.Flush()
		tmp.Sync()
		tmp.Close()
		os.Rename(tmp.Name(), snapPath)

		seq := flushWal()
		waitForWalSync(seq)

		walMu.Lock()
		walFile.Close()
		os.Rename(walPath, walPath+".old")
		walFile, _ = os.OpenFile(walPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		walBuf.Reset(walFile)
		walMu.Unlock()

		os.Remove(walPath + ".old")
	}
}

func storeKey(k string, v entry) (entry, bool) {
	defer dbg.MeasureTime("storeKey [mapManager]")()
	s := getShard(k)
	start := time.Now()
	s.mu.Lock()
	if wait := time.Since(start); wait > lockWarnThreshold {
		atomic.AddInt64(&storeLockSlowCount, 1)
	}
	prev, existed := s.m[k]
	s.m[k] = v
	s.mu.Unlock()
	return prev, existed
}

func deleteKey(k string) (entry, bool) {
	defer dbg.MeasureTime("deleteKey [mapManager]")()
	s := getShard(k)
	start := time.Now()
	s.mu.Lock()
	if wait := time.Since(start); wait > lockWarnThreshold {
		atomic.AddInt64(&storeLockSlowCount, 1)
	}
	prev, existed := s.m[k]
	delete(s.m, k)
	s.mu.Unlock()
	return prev, existed
}

func loadEntry(k string) (entry, bool) {
	defer dbg.MeasureTime("loadKey [mapManager]")()
	s := getShard(k)
	s.mu.RLock()
	val, ok := s.m[k]
	s.mu.RUnlock()
	return val, ok
}

func SaveElementByKey(key, file string, start, end int) (GetElement_output, bool, error) {
	defer dbg.MeasureTime("SaveElementByKey [mapManager]")()
	prevEntry, existed := storeKey(key, entry{file: file, start: start, end: end})
	select {
	case walChan <- walOp{'S', key, file, start, end}:
	case <-time.After(5 * time.Second):
		dbg.LogExtra("WAL blocked for 5s on key:", key)
		return GetElement_output{}, existed, errors.New("WAL timeout")
	default:
		_ = flushWal()
		walChan <- walOp{'S', key, file, start, end}
	}
	if existed {
		return entryToOutput(key, prevEntry), true, nil
	}
	return GetElement_output{}, false, nil
}

func RemoveElementByKey(key string) error {
	defer dbg.MeasureTime("RemoveElementByKey [mapManager]")()
	deleteKey(key)
	select {
	case walChan <- walOp{'D', key, "", 0, 0}:
	default:
		_ = flushWal()
		walChan <- walOp{'D', key, "", 0, 0}
	}
	return nil
}

func GetElementByKey(key string) (*GetElement_output, error) {
	defer dbg.MeasureTime("GetElementByKey [mapManager]")()
	if val, ok := loadEntry(key); ok {
		out := entryToOutput(key, val)
		return &out, nil
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

func entryToOutput(key string, e entry) GetElement_output {
	return GetElement_output{
		Key:      key,
		FileName: e.file,
		StartPtr: e.start,
		EndPtr:   e.end,
	}
}

// ResetForTests clears in-memory metadata caches between test runs.
func ResetForTests() {
	for _, s := range shardedData {
		s.mu.Lock()
		s.m = make(map[string]entry)
		s.mu.Unlock()
	}
	regexCache = sync.Map{}
}
