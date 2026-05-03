package tests

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	rtmetrics "runtime/metrics"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	dataManager_v2 "github.com/PAW122/TsunamiDB/data/dataManager/v2"
	defrag "github.com/PAW122/TsunamiDB/data/defragmentationManager"
	fileSystem_v1 "github.com/PAW122/TsunamiDB/data/fileSystem/v1"
	incindex "github.com/PAW122/TsunamiDB/data/incIndex"
	"github.com/PAW122/TsunamiDB/data/relational"
	TsuClient "github.com/PAW122/TsunamiDB/lib/dbclient"
	export "github.com/PAW122/TsunamiDB/lib/export"
	networkmanager "github.com/PAW122/TsunamiDB/servers/network-manager"
	apimetrics "github.com/PAW122/TsunamiDB/servers/public-api/v1/metrics"
	routes "github.com/PAW122/TsunamiDB/servers/public-api/v1/routes"
)

const (
	defaultResourceDuration  = 10 * time.Minute
	defaultStabilityDuration = time.Hour
	defaultPerformanceTime   = 5 * time.Minute
	defaultPayloadBytes      = 512
)

var specialOriginalWD string

func TestMain(m *testing.M) {
	var err error
	specialOriginalWD, err = os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	_ = os.RemoveAll(filepath.Join(specialOriginalWD, "db"))

	tmpWD, err := os.MkdirTemp("", "tsunamidb-special-tests-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.Chdir(tmpWD); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	resetSpecialStorage()
	code := m.Run()
	dataManager_v2.ShutdownWorkersForTests()
	fileSystem_v1.ShutdownForTests()
	defrag.ResetForTests()
	networkmanager.SetInstanceForTests(nil)
	apimetrics.ResetForTests()
	incindex.ResetForTests()
	relational.ResetForTests()

	_ = os.Chdir(specialOriginalWD)
	_ = os.RemoveAll(tmpWD)
	_ = os.RemoveAll(filepath.Join(specialOriginalWD, "db"))
	os.Exit(code)
}

type operationCounters struct {
	writes              atomic.Int64
	reads               atomic.Int64
	frees               atomic.Int64
	encryptedWrites     atomic.Int64
	encryptedReads      atomic.Int64
	apiActions          atomic.Int64
	goLibActions        atomic.Int64
	dllCandidateActions atomic.Int64
	logicalWriteBytes   atomic.Int64
	logicalReadBytes    atomic.Int64
}

func (c *operationCounters) totalActions() int64 {
	return c.writes.Load() + c.reads.Load() + c.frees.Load() + c.encryptedWrites.Load() + c.encryptedReads.Load()
}

type resourceSnapshot struct {
	at         time.Time
	heapAlloc  uint64
	heapSys    uint64
	totalAlloc uint64
	numGC      uint32
	goroutines int
	cpuSeconds float64
	diskBytes  int64
	actions    int64
}

type resourceSampler struct {
	interval time.Duration
	done     chan struct{}
	mu       sync.Mutex
	samples  []resourceSnapshot
}

func newResourceSampler(interval time.Duration) *resourceSampler {
	if interval <= 0 {
		interval = time.Second
	}
	return &resourceSampler{interval: interval, done: make(chan struct{})}
}

func (s *resourceSampler) start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()
		s.capture(0)
		for {
			select {
			case <-ctx.Done():
				s.capture(0)
				close(s.done)
				return
			case <-ticker.C:
				s.capture(0)
			}
		}
	}()
}

func (s *resourceSampler) stop() {
	<-s.done
}

func (s *resourceSampler) capture(actions int64) {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	snap := resourceSnapshot{
		at:         time.Now(),
		heapAlloc:  mem.HeapAlloc,
		heapSys:    mem.HeapSys,
		totalAlloc: mem.TotalAlloc,
		numGC:      mem.NumGC,
		goroutines: runtime.NumGoroutine(),
		cpuSeconds: runtimeMetricFloat64("/cpu/classes/total:cpu-seconds"),
		diskBytes:  dirSize("db"),
		actions:    actions,
	}

	s.mu.Lock()
	s.samples = append(s.samples, snap)
	s.mu.Unlock()
}

func (s *resourceSampler) snapshots() []resourceSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]resourceSnapshot, len(s.samples))
	copy(out, s.samples)
	return out
}

func requireSpecialTests(t *testing.T) {
	t.Helper()
	if os.Getenv("TSU_SPECIAL_TESTS") != "1" {
		t.Skip("special tests are disabled; set TSU_SPECIAL_TESTS=1 to run long resource/stability/performance tests")
	}
}

func resetSpecialStorage() {
	dataManager_v2.ShutdownWorkersForTests()
	fileSystem_v1.ShutdownForTests()
	defrag.ResetForTests()
	apimetrics.ResetForTests()
	incindex.ResetForTests()
	relational.ResetForTests()
	_ = os.RemoveAll("db")
	dataManager_v2.EnsureDirsForTests()
	networkmanager.SetInstanceForTests(&networkmanager.NetworkManager{ServerIP: "special-test"})
}

func runWorkers(ctx context.Context, workers int, fn func(context.Context, int) error) error {
	if workers <= 0 {
		workers = 1
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	errCh := make(chan error, 1)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		workerID := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := fn(ctx, workerID); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
				select {
				case errCh <- err:
					cancel()
				default:
				}
			}
		}()
	}
	wg.Wait()

	select {
	case err := <-errCh:
		return err
	default:
		return nil
	}
}

func saveReadFreeCycle(table, key string, payload []byte, free bool, counters *operationCounters) error {
	if err := TsuClient.Save(key, table, payload); err != nil {
		return fmt.Errorf("save %s/%s: %w", table, key, err)
	}
	counters.writes.Add(1)
	counters.logicalWriteBytes.Add(int64(len(payload)))

	got, err := TsuClient.Read(key, table)
	if err != nil {
		return fmt.Errorf("read %s/%s: %w", table, key, err)
	}
	if !bytes.Equal(got, payload) {
		return fmt.Errorf("read mismatch for %s/%s: got %d bytes, want %d bytes", table, key, len(got), len(payload))
	}
	counters.reads.Add(1)
	counters.logicalReadBytes.Add(int64(len(got)))

	if free {
		if err := TsuClient.Free(key, table); err != nil {
			return fmt.Errorf("free %s/%s: %w", table, key, err)
		}
		counters.frees.Add(1)
	}
	return nil
}

func exerciseGoClient(table, key string, payload []byte, counters *operationCounters) error {
	counters.goLibActions.Add(1)
	if err := saveReadFreeCycle(table, key, payload, true, counters); err != nil {
		return err
	}

	encKey := "special-secret"
	encryptedKey := key + "_enc"
	if err := TsuClient.SaveEncrypted(encryptedKey, table, encKey, payload); err != nil {
		return err
	}
	counters.encryptedWrites.Add(1)
	counters.logicalWriteBytes.Add(int64(len(payload)))

	got, err := TsuClient.ReadEncrypted(encryptedKey, table, encKey)
	if err != nil {
		return err
	}
	if !bytes.Equal(got, payload) {
		return fmt.Errorf("go encrypted read mismatch for %s/%s", table, encryptedKey)
	}
	counters.encryptedReads.Add(1)
	counters.logicalReadBytes.Add(int64(len(got)))

	_, err = TsuClient.GetKeysByRegex(table, "^"+regexpQuotePrefix(key[:min(8, len(key))]), 10)
	return err
}

func exerciseDLLCandidate(table, key string, payload []byte, counters *operationCounters) error {
	counters.dllCandidateActions.Add(1)
	if err := export.Save(key, table, payload); err != nil {
		return err
	}
	counters.writes.Add(1)
	counters.logicalWriteBytes.Add(int64(len(payload)))

	got, err := export.Read(key, table)
	if err != nil {
		return err
	}
	if !bytes.Equal(got, payload) {
		return fmt.Errorf("export read mismatch for %s/%s", table, key)
	}
	counters.reads.Add(1)
	counters.logicalReadBytes.Add(int64(len(got)))

	encKey := "special-secret"
	encryptedKey := key + "_export_enc"
	if err := export.SaveEncrypted(encryptedKey, table, encKey, payload); err != nil {
		return err
	}
	counters.encryptedWrites.Add(1)
	counters.logicalWriteBytes.Add(int64(len(payload)))

	got, err = export.ReadEncrypted(encryptedKey, table, encKey)
	if err != nil {
		return err
	}
	if !bytes.Equal(got, payload) {
		return fmt.Errorf("export encrypted read mismatch for %s/%s", table, encryptedKey)
	}
	counters.encryptedReads.Add(1)
	counters.logicalReadBytes.Add(int64(len(got)))

	if err := export.Free(key, table); err != nil {
		return err
	}
	counters.frees.Add(1)
	return nil
}

func exerciseHTTPRoutes(table, key string, payload []byte, rng *rand.Rand, counters *operationCounters) error {
	counters.apiActions.Add(1)

	if resp := performRoute(routes.AsyncSave, http.MethodPost, "/save/"+table+"/"+key, bytes.NewReader(payload), nil); resp.Code != http.StatusOK {
		return fmt.Errorf("api save status=%d body=%s", resp.Code, resp.Body.String())
	}
	counters.writes.Add(1)
	counters.logicalWriteBytes.Add(int64(len(payload)))

	if resp := performRoute(routes.AsyncRead, http.MethodGet, "/read/"+table+"/"+key, nil, nil); resp.Code != http.StatusOK || !bytes.Equal(resp.Body.Bytes(), payload) {
		return fmt.Errorf("api read status=%d body_bytes=%d want=%d", resp.Code, resp.Body.Len(), len(payload))
	}
	counters.reads.Add(1)
	counters.logicalReadBytes.Add(int64(len(payload)))

	encKey := "special-secret"
	encryptedKey := key + "_api_enc"
	headers := map[string]string{"encryption_key": encKey}
	if resp := performRoute(routes.SaveEncrypted, http.MethodPost, "/save_encrypted/"+table+"/"+encryptedKey, bytes.NewReader(payload), headers); resp.Code != http.StatusOK {
		return fmt.Errorf("api save encrypted status=%d body=%s", resp.Code, resp.Body.String())
	}
	counters.encryptedWrites.Add(1)
	counters.logicalWriteBytes.Add(int64(len(payload)))

	if resp := performRoute(routes.ReadEncrypted, http.MethodGet, "/read_encrypted/"+table+"/"+encryptedKey, nil, headers); resp.Code != http.StatusOK || !bytes.Equal(resp.Body.Bytes(), payload) {
		return fmt.Errorf("api read encrypted status=%d body_bytes=%d want=%d", resp.Code, resp.Body.Len(), len(payload))
	}
	counters.encryptedReads.Add(1)
	counters.logicalReadBytes.Add(int64(len(payload)))

	if rng.Intn(4) == 0 {
		incKey := key + "_inc"
		incHeaders := map[string]string{"max_entry_size": strconv.Itoa(len(payload) + 64), "entry_key": "entry"}
		resp := performRoute(routes.SaveIncremental, http.MethodPost, "/save_inc/"+table+"/"+incKey, bytes.NewReader(payload), incHeaders)
		if resp.Code != http.StatusOK {
			return fmt.Errorf("api save_inc status=%d body=%s", resp.Code, resp.Body.String())
		}
		readHeaders := map[string]string{"read_type": "by_id", "id": "0"}
		resp = performRoute(routes.ReadIncremental, http.MethodGet, "/read_inc/"+table+"/"+incKey, nil, readHeaders)
		if resp.Code != http.StatusOK {
			return fmt.Errorf("api read_inc status=%d body=%s", resp.Code, resp.Body.String())
		}
		resp = performRoute(routes.DeleteIncremental, http.MethodGet, "/delete_inc/"+table+"/"+incKey, nil, nil)
		if resp.Code != http.StatusOK {
			return fmt.Errorf("api delete_inc status=%d body=%s", resp.Code, resp.Body.String())
		}
	}

	if resp := performRoute(routes.GetKeysByRegex, http.MethodGet, "/key_by_regex/"+table+"?regex=^"+key+"&max=1", nil, nil); resp.Code != http.StatusOK {
		return fmt.Errorf("api regex status=%d body=%s", resp.Code, resp.Body.String())
	}

	if resp := performRoute(func(w http.ResponseWriter, r *http.Request, _ *http.Client) {
		routes.Health(w, r, http.DefaultClient)
	}, http.MethodGet, "/health", nil, nil); resp.Code != http.StatusOK {
		return fmt.Errorf("api health status=%d body=%s", resp.Code, resp.Body.String())
	}

	if resp := performRoute(routes.Free, http.MethodGet, "/free/"+table+"/"+key, nil, nil); resp.Code != http.StatusOK {
		return fmt.Errorf("api free status=%d body=%s", resp.Code, resp.Body.String())
	}
	counters.frees.Add(1)
	return nil
}

func performRoute(handler func(http.ResponseWriter, *http.Request, *http.Client), method, path string, body io.Reader, headers map[string]string) *httptest.ResponseRecorder {
	if body == nil {
		body = http.NoBody
	}
	req := httptest.NewRequest(method, path, body)
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	rec := httptest.NewRecorder()
	handler(rec, req, http.DefaultClient)
	return rec
}

func reportResourceSummary(t *testing.T, name string, duration time.Duration, counters *operationCounters, samples []resourceSnapshot) {
	t.Helper()
	if len(samples) == 0 {
		t.Fatalf("%s resource sampler did not collect samples", name)
	}

	first := samples[0]
	last := samples[len(samples)-1]
	peakHeap := uint64(0)
	peakSys := uint64(0)
	peakDisk := int64(0)
	for _, sample := range samples {
		if sample.heapAlloc > peakHeap {
			peakHeap = sample.heapAlloc
		}
		if sample.heapSys > peakSys {
			peakSys = sample.heapSys
		}
		if sample.diskBytes > peakDisk {
			peakDisk = sample.diskBytes
		}
	}

	cpuDelta := last.cpuSeconds - first.cpuSeconds
	if math.IsNaN(first.cpuSeconds) || math.IsNaN(last.cpuSeconds) {
		cpuDelta = math.NaN()
	}
	t.Logf("%s resource duration target=%s sampled=%s samples=%d", name, duration, last.at.Sub(first.at).Round(time.Millisecond), len(samples))
	t.Logf("actions=%d writes=%d reads=%d frees=%d throughput=%.2f actions/s",
		counters.totalActions(), counters.writes.Load(), counters.reads.Load(), counters.frees.Load(),
		float64(counters.totalActions())/math.Max(last.at.Sub(first.at).Seconds(), 0.001),
	)
	t.Logf("ram heap_alloc start=%s end=%s peak=%s heap_sys_peak=%s total_alloc_delta=%s gc_delta=%d goroutines_end=%d",
		formatBytes(int64(first.heapAlloc)),
		formatBytes(int64(last.heapAlloc)),
		formatBytes(int64(peakHeap)),
		formatBytes(int64(peakSys)),
		formatBytes(int64(last.totalAlloc-first.totalAlloc)),
		last.numGC-first.numGC,
		last.goroutines,
	)
	t.Logf("cpu runtime_total_delta_seconds=%.3f", cpuDelta)
	t.Logf("disk used_start=%s used_end=%s used_peak=%s logical_write_io=%s logical_read_io=%s",
		formatBytes(first.diskBytes),
		formatBytes(last.diskBytes),
		formatBytes(peakDisk),
		formatBytes(counters.logicalWriteBytes.Load()),
		formatBytes(counters.logicalReadBytes.Load()),
	)
}

func durationFromEnv(key string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := time.ParseDuration(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func intFromEnv(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func int64FromEnv(key string, fallback int64) int64 {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return fallback
	}
	return value
}

func deterministicPayload(size int) []byte {
	if size <= 0 {
		size = defaultPayloadBytes
	}
	payload := make([]byte, size)
	for i := range payload {
		payload[i] = byte('a' + i%26)
	}
	return payload
}

func randomPayload(rng *rand.Rand, limit int) []byte {
	if limit <= 0 {
		limit = defaultPayloadBytes
	}
	size := rng.Intn(limit + 1)
	payload := make([]byte, size)
	if _, err := rng.Read(payload); err != nil {
		panic(err)
	}
	return payload
}

func runtimeMetricFloat64(name string) float64 {
	samples := []rtmetrics.Sample{{Name: name}}
	rtmetrics.Read(samples)
	switch samples[0].Value.Kind() {
	case rtmetrics.KindFloat64:
		return samples[0].Value.Float64()
	case rtmetrics.KindUint64:
		return float64(samples[0].Value.Uint64())
	default:
		return math.NaN()
	}
}

func dirSize(root string) int64 {
	var total int64
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err == nil {
			total += info.Size()
		}
		return nil
	})
	return total
}

func mustDirSize(t *testing.T, root string) int64 {
	t.Helper()
	return dirSize(root)
}

func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func regexpQuotePrefix(prefix string) string {
	replacer := strings.NewReplacer(
		`\\`, `\\\\`,
		`.`, `\\.`,
		`+`, `\\+`,
		`*`, `\\*`,
		`?`, `\\?`,
		`(`, `\\(`,
		`)`, `\\)`,
		`[`, `\\[`,
		`]`, `\\]`,
		`{`, `\\{`,
		`}`, `\\}`,
		`^`, `\\^`,
		`$`, `\\$`,
		`|`, `\\|`,
	)
	return replacer.Replace(prefix)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
