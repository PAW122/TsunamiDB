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

const (
	numShards         = 256
	lockWarnThreshold = 100 * time.Microsecond
)

var (
	baseMapsDir    = filepath.Join(".", "db", "maps")
	legacySnapPath = filepath.Join(baseMapsDir, "data_map.snap")
	legacyWalPath  = filepath.Join(baseMapsDir, "data_map.wal")

	indexRegistry  = make(map[string]*tableIndex)
	registryMu     sync.RWMutex
	lastIndexCache atomic.Value // *cachedIndex

	walOpsProcessed    int64
	storeLockSlowCount int64
	defragFreedCount   int64
	defragSkipCount    int64
	debugOnce          sync.Once
)

type shard struct {
	mu sync.RWMutex
	m  map[string]entry
}

type tableIndex struct {
	name     string
	safeName string

	shards [numShards]*shard

	regexCache sync.Map

	walChan chan walOp
	walFile *os.File
	walBuf  *bufio.Writer
	walMu   sync.Mutex

	walSyncMu        sync.Mutex
	walSyncCond      *sync.Cond
	walSyncRequested int64
	walSyncCompleted int64
}

type cachedIndex struct {
	table string
	idx   *tableIndex
}

func init() {
	if err := os.MkdirAll(baseMapsDir, 0755); err != nil {
		panic(err)
	}
	lastIndexCache.Store((*cachedIndex)(nil))
	debugOnce.Do(func() {
		go debugCountersWorker()
	})
}

var tableSanitizer = strings.NewReplacer(
	"/", "_",
	"\\", "_",
	":", "_",
	"*", "_",
	"?", "_",
	"\"", "_",
	"<", "_",
	">", "_",
	"|", "_",
	"..", "__",
)

func sanitizeTableName(name string) string {
	s := strings.TrimSpace(name)
	if s == "" {
		return "default"
	}
	return tableSanitizer.Replace(s)
}

func normalizeTableName(table string) (string, error) {
	if strings.TrimSpace(table) == "" {
		return "", errors.New("table name cannot be empty")
	}
	return table, nil
}

func (ti *tableIndex) tableDir() string {
	return filepath.Join(baseMapsDir, ti.safeName)
}

func (ti *tableIndex) snapPath() string {
	return filepath.Join(ti.tableDir(), "index.snap")
}

func (ti *tableIndex) walPath() string {
	return filepath.Join(ti.tableDir(), "index.wal")
}

func (ti *tableIndex) ensureDir() error {
	return os.MkdirAll(ti.tableDir(), 0755)
}

func newTableIndex(table string) (*tableIndex, error) {
	idx := &tableIndex{
		name:     table,
		safeName: sanitizeTableName(table),
		walChan:  make(chan walOp, 100_000),
	}
	idx.walSyncCond = sync.NewCond(&idx.walSyncMu)
	for i := 0; i < numShards; i++ {
		idx.shards[i] = &shard{m: make(map[string]entry)}
	}

	if err := idx.ensureDir(); err != nil {
		return nil, err
	}
	if err := idx.loadIndex(); err != nil {
		return nil, err
	}
	if err := idx.setupWalWriter(); err != nil {
		return nil, err
	}

	go idx.runWalWriter()
	go idx.walSyncLoop()
	go idx.snapshotWorker()

	return idx, nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func isPointerRangeValid(start, end int) bool {
	return start >= 0 && end > start
}

func (ti *tableIndex) loadIndex() error {
	snapExists := fileExists(ti.snapPath())
	walExists := fileExists(ti.walPath())

	if snapExists {
		if err := ti.loadSnapshot(ti.snapPath(), nil); err != nil {
			return err
		}
	}
	if walExists {
		if err := ti.applyWalFile(ti.walPath(), nil); err != nil {
			return err
		}
	}
	if snapExists || walExists {
		return nil
	}

	return ti.importFromLegacy()
}

func (ti *tableIndex) loadSnapshot(path string, filter func(string, entry) bool) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		parts := strings.Split(line, "|")
		if len(parts) != 4 {
			continue
		}
		start, err1 := strconv.Atoi(parts[2])
		end, err2 := strconv.Atoi(parts[3])
		if err1 != nil || err2 != nil {
			continue
		}
		e := entry{file: parts[1], start: start, end: end}
		if !isPointerRangeValid(e.start, e.end) {
			continue
		}
		if filter != nil && !filter(parts[0], e) {
			continue
		}
		ti.storeKey(parts[0], e)
	}
	return sc.Err()
}

func (ti *tableIndex) applyWalFile(path string, filter func(string, entry) bool) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		parts := strings.Split(line, "|")
		if len(parts) < 2 {
			continue
		}
		switch parts[0] {
		case "S":
			if len(parts) != 5 {
				continue
			}
			start, err1 := strconv.Atoi(parts[3])
			end, err2 := strconv.Atoi(parts[4])
			if err1 != nil || err2 != nil {
				continue
			}
			e := entry{file: parts[2], start: start, end: end}
			if !isPointerRangeValid(e.start, e.end) {
				continue
			}
			if filter != nil && !filter(parts[1], e) {
				continue
			}
			ti.storeKey(parts[1], e)
		case "D":
			ti.deleteKey(parts[1])
		}
	}
	return sc.Err()
}

func (ti *tableIndex) importFromLegacy() error {
	filter := func(_ string, e entry) bool {
		return e.file == ti.name && isPointerRangeValid(e.start, e.end)
	}
	if err := ti.loadSnapshot(legacySnapPath, filter); err != nil {
		return err
	}
	if err := ti.applyWalFile(legacyWalPath, filter); err != nil {
		return err
	}
	return nil
}

func (ti *tableIndex) setupWalWriter() error {
	var err error
	ti.walFile, err = os.OpenFile(ti.walPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	ti.walBuf = bufio.NewWriterSize(ti.walFile, 4<<20)
	return nil
}

func (ti *tableIndex) getShard(key string) *shard {
	h := fnv.New32a()
	h.Write([]byte(key))
	return ti.shards[uint(h.Sum32())%numShards]
}

func (ti *tableIndex) storeKey(k string, v entry) (entry, bool) {
	defer dbg.MeasureTime("storeKey [mapManager]")()
	s := ti.getShard(k)
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

func (ti *tableIndex) deleteKey(k string) (entry, bool) {
	defer dbg.MeasureTime("deleteKey [mapManager]")()
	s := ti.getShard(k)
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

func (ti *tableIndex) loadEntry(k string) (entry, bool) {
	defer dbg.MeasureTime("loadKey [mapManager]")()
	s := ti.getShard(k)
	s.mu.RLock()
	val, ok := s.m[k]
	s.mu.RUnlock()
	return val, ok
}

func (ti *tableIndex) saveElement(key, file string, start, end int) (GetElement_output, bool, error) {
	if !isPointerRangeValid(start, end) {
		return GetElement_output{}, false, fmt.Errorf("invalid pointer range: start=%d end=%d", start, end)
	}
	prevEntry, existed := ti.storeKey(key, entry{file: file, start: start, end: end})
	op := walOp{op: 'S', key: key, fileName: file, start: start, end: end}
	if err := ti.enqueueWal(op); err != nil {
		return GetElement_output{}, existed, err
	}
	if existed {
		return entryToOutput(key, prevEntry), true, nil
	}
	return GetElement_output{}, false, nil
}

func (ti *tableIndex) removeElement(key string) error {
	ti.deleteKey(key)
	return ti.enqueueWal(walOp{op: 'D', key: key})
}

func (ti *tableIndex) getElement(key string) (*GetElement_output, error) {
	if val, ok := ti.loadEntry(key); ok {
		out := entryToOutput(key, val)
		return &out, nil
	}
	return nil, errors.New("key not found")
}

func (ti *tableIndex) keysByRegex(pattern string, max int) ([]string, error) {
	var rx *regexp.Regexp
	if c, ok := ti.regexCache.Load(pattern); ok {
		rx = c.(*regexp.Regexp)
	} else {
		compiled, err := regexp.Compile(pattern)
		if err != nil {
			return nil, err
		}
		ti.regexCache.Store(pattern, compiled)
		rx = compiled
	}

	out := make([]string, 0, max)
	for _, s := range ti.shards {
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

func (ti *tableIndex) enqueueWal(op walOp) error {
	select {
	case ti.walChan <- op:
		return nil
	case <-time.After(5 * time.Second):
		dbg.LogExtra(fmt.Sprintf("WAL blocked for 5s on key: %s (table=%s)", op.key, ti.name))
		return errors.New("WAL timeout")
	default:
		ti.flushWal()
		ti.walChan <- op
		return nil
	}
}

func (ti *tableIndex) runWalWriter() {
	defer func() {
		if r := recover(); r != nil {
			dbg.LogExtra(fmt.Sprintf("walWriter panic (table=%s): %v", ti.name, r))
		}
	}()

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	pending := 0

	for {
		select {
		case op := <-ti.walChan:
			if err := ti.writeWalLine(op); err != nil {
				dbg.LogExtra(fmt.Sprintf("writeWalLine error (table=%s): %v", ti.name, err))
			}
			pending++
			atomic.AddInt64(&walOpsProcessed, 1)
			if pending >= 100 {
				ti.flushWal()
				pending = 0
			}
		case <-ticker.C:
			if pending > 0 {
				ti.flushWal()
				pending = 0
			}
		}
	}
}

func (ti *tableIndex) writeWalLine(op walOp) error {
	defer dbg.MeasureTime("writeWalLine [mapManager]")()
	var line string
	if op.op == 'S' {
		line = fmt.Sprintf("S|%s|%s|%d|%d\n", op.key, op.fileName, op.start, op.end)
	} else {
		line = fmt.Sprintf("D|%s\n", op.key)
	}

	ti.walMu.Lock()
	defer ti.walMu.Unlock()
	if ti.walBuf == nil {
		return errors.New("wal buffer not initialized")
	}
	_, err := ti.walBuf.WriteString(line)
	return err
}

func (ti *tableIndex) flushWal() int64 {
	defer dbg.MeasureTime("flushWal [mapManager]")()
	ti.walMu.Lock()
	if ti.walBuf != nil {
		ti.walBuf.Flush()
	}
	ti.walMu.Unlock()

	seq := atomic.AddInt64(&ti.walSyncRequested, 1)
	ti.walSyncMu.Lock()
	if ti.walSyncCond != nil {
		ti.walSyncCond.Signal()
	}
	ti.walSyncMu.Unlock()
	return seq
}

func (ti *tableIndex) walSyncLoop() {
	var lastSynced int64
	for {
		ti.walSyncMu.Lock()
		for lastSynced == atomic.LoadInt64(&ti.walSyncRequested) {
			if ti.walSyncCond != nil {
				ti.walSyncCond.Wait()
			} else {
				ti.walSyncMu.Unlock()
				time.Sleep(10 * time.Millisecond)
				ti.walSyncMu.Lock()
			}
		}
		ti.walSyncMu.Unlock()

		ti.walMu.Lock()
		if ti.walFile != nil {
			if err := ti.walFile.Sync(); err != nil {
				dbg.LogExtra(fmt.Sprintf("wal sync error (table=%s): %v", ti.name, err))
			}
		}
		ti.walMu.Unlock()

		lastSynced = atomic.LoadInt64(&ti.walSyncRequested)
		atomic.StoreInt64(&ti.walSyncCompleted, lastSynced)

		ti.walSyncMu.Lock()
		if ti.walSyncCond != nil {
			ti.walSyncCond.Broadcast()
		}
		ti.walSyncMu.Unlock()
	}
}

func (ti *tableIndex) waitForWalSync(seq int64) {
	for {
		if atomic.LoadInt64(&ti.walSyncCompleted) >= seq {
			return
		}
		ti.walSyncMu.Lock()
		for atomic.LoadInt64(&ti.walSyncCompleted) < seq {
			if ti.walSyncCond != nil {
				ti.walSyncCond.Wait()
			} else {
				ti.walSyncMu.Unlock()
				time.Sleep(10 * time.Millisecond)
				ti.walSyncMu.Lock()
			}
		}
		ti.walSyncMu.Unlock()
	}
}

func (ti *tableIndex) snapshotWorker() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		ti.writeSnapshot()
	}
}

func (ti *tableIndex) writeSnapshot() {
	defer dbg.MeasureTime("snapshotWorker [mapManager]")()
	tmp, err := os.CreateTemp(ti.tableDir(), "snap_*")
	if err != nil {
		dbg.LogExtra(fmt.Sprintf("snapshot temp error (table=%s): %v", ti.name, err))
		return
	}

	bw := bufio.NewWriter(tmp)
	for _, s := range ti.shards {
		s.mu.RLock()
		for key, val := range s.m {
			fmt.Fprintf(bw, "%s|%s|%d|%d\n", key, val.file, val.start, val.end)
		}
		s.mu.RUnlock()
	}
	if err := bw.Flush(); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		dbg.LogExtra(fmt.Sprintf("snapshot flush error (table=%s): %v", ti.name, err))
		return
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		dbg.LogExtra(fmt.Sprintf("snapshot sync error (table=%s): %v", ti.name, err))
		return
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		dbg.LogExtra(fmt.Sprintf("snapshot close error (table=%s): %v", ti.name, err))
		return
	}

	if err := os.Rename(tmp.Name(), ti.snapPath()); err != nil {
		os.Remove(tmp.Name())
		dbg.LogExtra(fmt.Sprintf("snapshot rename error (table=%s): %v", ti.name, err))
	}

	seq := ti.flushWal()
	ti.waitForWalSync(seq)

	if err := ti.rotateWal(); err != nil {
		dbg.LogExtra(fmt.Sprintf("wal rotate error (table=%s): %v", ti.name, err))
	}
}

func (ti *tableIndex) rotateWal() error {
	ti.walMu.Lock()
	defer ti.walMu.Unlock()

	if ti.walFile != nil {
		if err := ti.walFile.Close(); err != nil {
			return err
		}
	}
	if err := os.Remove(ti.walPath()); err != nil && !os.IsNotExist(err) {
		return err
	}

	file, err := os.OpenFile(ti.walPath(), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	ti.walFile = file
	ti.walBuf = bufio.NewWriterSize(file, 4<<20)
	return nil
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

func entryToOutput(key string, e entry) GetElement_output {
	return GetElement_output{
		Key:      key,
		FileName: e.file,
		StartPtr: e.start,
		EndPtr:   e.end,
	}
}

func loadCachedIndex(table string) *tableIndex {
	if v := lastIndexCache.Load(); v != nil {
		if ci, ok := v.(*cachedIndex); ok && ci != nil {
			if ci.table == table {
				return ci.idx
			}
		}
	}
	return nil
}

func storeCachedIndex(table string, idx *tableIndex) {
	lastIndexCache.Store(&cachedIndex{table: table, idx: idx})
}

func ResetForTests() {
	registryMu.RLock()
	indices := make([]*tableIndex, 0, len(indexRegistry))
	for _, ti := range indexRegistry {
		indices = append(indices, ti)
	}
	registryMu.RUnlock()

	for _, ti := range indices {
		for _, s := range ti.shards {
			s.mu.Lock()
			s.m = make(map[string]entry)
			s.mu.Unlock()
		}
		ti.regexCache = sync.Map{}
	}

	lastIndexCache.Store((*cachedIndex)(nil))
}

func getTableIndex(table string) (*tableIndex, error) {
	normalized, err := normalizeTableName(table)
	if err != nil {
		return nil, err
	}

	if idx := loadCachedIndex(normalized); idx != nil {
		return idx, nil
	}

	registryMu.RLock()
	idx := indexRegistry[normalized]
	registryMu.RUnlock()
	if idx != nil {
		storeCachedIndex(normalized, idx)
		return idx, nil
	}

	registryMu.Lock()
	idx = indexRegistry[normalized]
	if idx == nil {
		idx, err = newTableIndex(normalized)
		if err != nil {
			registryMu.Unlock()
			return nil, err
		}
		indexRegistry[normalized] = idx
	}
	registryMu.Unlock()

	storeCachedIndex(normalized, idx)
	return idx, nil
}

func SaveElementByKey(table, key string, start, end int) (GetElement_output, bool, error) {
	defer dbg.MeasureTime("SaveElementByKey [mapManager]")()
	idx, err := getTableIndex(table)
	if err != nil {
		return GetElement_output{}, false, err
	}
	return idx.saveElement(key, table, start, end)
}

func RemoveElementByKey(table, key string) error {
	defer dbg.MeasureTime("RemoveElementByKey [mapManager]")()
	idx, err := getTableIndex(table)
	if err != nil {
		return err
	}
	return idx.removeElement(key)
}

func GetElementByKey(table, key string) (*GetElement_output, error) {
	defer dbg.MeasureTime("GetElementByKey [mapManager]")()
	idx, err := getTableIndex(table)
	if err != nil {
		return nil, err
	}
	return idx.getElement(key)
}

func GetKeysByRegex(table, pattern string, max int) ([]string, error) {
	defer dbg.MeasureTime("GetKeysByRegex [mapManager]")()
	idx, err := getTableIndex(table)
	if err != nil {
		return nil, err
	}
	return idx.keysByRegex(pattern, max)
}
