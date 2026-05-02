package tests

import (
	"context"
	"fmt"
	"math"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PAW122/TsunamiDB/data/relational"
)

const (
	defaultRelationalPerformanceDuration = 30 * time.Second
	defaultRelationalPerformanceWorkers  = 1
	defaultRelationalPerformanceRows     = 1_000
)

type relationalPerformanceCounters struct {
	inserts         atomic.Int64
	reads           atomic.Int64
	equalitySelects atomic.Int64
	likeSelects     atomic.Int64
	rowsReturned    atomic.Int64
}

func (c *relationalPerformanceCounters) totalActions() int64 {
	return c.inserts.Load() + c.reads.Load() + c.equalitySelects.Load() + c.likeSelects.Load()
}

func TestSpecialRelationalPerformance(t *testing.T) {
	requireSpecialTests(t)
	resetSpecialStorage()

	duration := durationFromEnv("TSU_REL_PERF_DURATION", defaultRelationalPerformanceDuration)
	workers := intFromEnv("TSU_REL_PERF_WORKERS", defaultRelationalPerformanceWorkers)
	seedRows := intFromEnv("TSU_REL_PERF_ROWS", defaultRelationalPerformanceRows)

	setupStart := time.Now()
	fmt.Fprintf(os.Stderr, "relational perf setup: workers=%d seed_rows_per_worker=%d duration=%s\n", workers, seedRows, duration)
	if err := runRelationalSetupWorkers(workers, func(workerID int) error {
		return prepareRelationalPerformanceTables(workerID, seedRows)
	}); err != nil {
		t.Fatal(err)
	}
	setupDuration := time.Since(setupStart)
	fmt.Fprintf(os.Stderr, "relational perf setup: done in %s\n", setupDuration.Round(time.Millisecond))

	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()

	counters := &relationalPerformanceCounters{}
	progressCtx, stopProgress := context.WithCancel(ctx)
	progressDone := startRelationalPerformanceProgress(progressCtx, counters)
	start := time.Now()
	err := runWorkers(ctx, workers, func(ctx context.Context, workerID int) error {
		table := relationalPerformanceReadTable(workerID)
		insertTable := relationalPerformanceInsertTable(workerID)
		seq := 0
		for ctx.Err() == nil {
			rowID := uint64((seq * 7919) % seedRows)
			if _, err := relational.ReadRow(table, rowID); err != nil {
				return fmt.Errorf("read %s/%d: %w", table, rowID, err)
			}
			counters.reads.Add(1)

			equalityRows, err := relational.SelectRows(table, relational.Equal("segment", uint64(seq%128)))
			if err != nil {
				return fmt.Errorf("equality select %s: %w", table, err)
			}
			counters.equalitySelects.Add(1)
			counters.rowsReturned.Add(int64(len(equalityRows)))

			likeRows, err := relational.SelectRows(table, relational.Like("note", relationalPerformanceNeedlePattern(seq)))
			if err != nil {
				return fmt.Errorf("like select %s: %w", table, err)
			}
			counters.likeSelects.Add(1)
			counters.rowsReturned.Add(int64(len(likeRows)))

			if _, err := relational.InsertRow(insertTable, relationalPerformanceValues(seq)); err != nil {
				return fmt.Errorf("insert %s/%d: %w", insertTable, seq, err)
			}
			counters.inserts.Add(1)
			seq++
		}
		return nil
	})
	stopProgress()
	<-progressDone
	elapsed := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}

	seconds := math.Max(elapsed.Seconds(), 0.001)
	t.Logf("relational performance setup=%s measured_duration=%s total=%s workers=%d seed_rows_per_worker=%d host_cpus=%d",
		setupDuration.Round(time.Millisecond),
		elapsed.Round(time.Millisecond),
		(setupDuration + elapsed).Round(time.Millisecond),
		workers,
		seedRows,
		runtime.NumCPU(),
	)
	t.Logf("total actions=%d throughput=%.2f actions/s", counters.totalActions(), float64(counters.totalActions())/seconds)
	t.Logf("inserts=%d %.2f/s reads=%d %.2f/s equality_selects=%d %.2f/s like_selects=%d %.2f/s",
		counters.inserts.Load(), float64(counters.inserts.Load())/seconds,
		counters.reads.Load(), float64(counters.reads.Load())/seconds,
		counters.equalitySelects.Load(), float64(counters.equalitySelects.Load())/seconds,
		counters.likeSelects.Load(), float64(counters.likeSelects.Load())/seconds,
	)
	t.Logf("rows returned=%d %.2f rows/s disk used=%s",
		counters.rowsReturned.Load(),
		float64(counters.rowsReturned.Load())/seconds,
		formatBytes(mustDirSize(t, "db")),
	)
}

func TestSpecialRelationalSaturation(t *testing.T) {
	requireSpecialTests(t)
	resetSpecialStorage()

	duration := durationFromEnv("TSU_REL_SAT_DURATION", defaultRelationalPerformanceDuration)
	workers := intFromEnv("TSU_REL_SAT_WORKERS", runtime.NumCPU())
	seedRows := intFromEnv("TSU_REL_SAT_ROWS", defaultRelationalPerformanceRows)
	mode := relationalSaturationMode()

	setupStart := time.Now()
	fmt.Fprintf(os.Stderr, "relational saturation setup: mode=%s workers=%d seed_rows_per_worker=%d duration=%s\n", mode, workers, seedRows, duration)
	if err := runRelationalSetupWorkers(workers, func(workerID int) error {
		switch mode {
		case "insert":
			return createRelationalInsertTable(relationalPerformanceInsertTable(workerID))
		case "read":
			return prepareRelationalReadTable(workerID, seedRows, false, false)
		case "select-eq":
			return prepareRelationalReadTable(workerID, seedRows, true, false)
		case "select-like":
			return prepareRelationalReadTable(workerID, seedRows, false, true)
		case "mixed":
			return prepareRelationalPerformanceTables(workerID, seedRows)
		default:
			return fmt.Errorf("unsupported TSU_REL_SAT_MODE=%q; use read, insert, select-eq, select-like, or mixed", mode)
		}
	}); err != nil {
		t.Fatal(err)
	}
	setupDuration := time.Since(setupStart)
	fmt.Fprintf(os.Stderr, "relational saturation setup: done in %s\n", setupDuration.Round(time.Millisecond))

	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()

	counters := &relationalPerformanceCounters{}
	progressCtx, stopProgress := context.WithCancel(ctx)
	progressDone := startRelationalPerformanceProgress(progressCtx, counters)
	start := time.Now()
	err := runWorkers(ctx, workers, func(ctx context.Context, workerID int) error {
		return runRelationalSaturationWorker(ctx, workerID, seedRows, mode, counters)
	})
	stopProgress()
	<-progressDone
	elapsed := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}

	reportRelationalPerformance(t, "relational saturation", setupDuration, elapsed, workers, seedRows, counters)
}

func runRelationalSetupWorkers(workers int, fn func(workerID int) error) error {
	if workers <= 0 {
		workers = 1
	}

	errCh := make(chan error, 1)
	var wg sync.WaitGroup
	for workerID := 0; workerID < workers; workerID++ {
		workerID := workerID
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := fn(workerID); err != nil {
				select {
				case errCh <- err:
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

func prepareRelationalPerformanceTables(workerID int, rows int) error {
	if err := prepareRelationalReadTable(workerID, rows, true, true); err != nil {
		return err
	}
	return createRelationalInsertTable(relationalPerformanceInsertTable(workerID))
}

func prepareRelationalReadTable(workerID int, rows int, equalityIndex bool, trigramIndex bool) error {
	readTable := relationalPerformanceReadTable(workerID)
	fmt.Fprintf(os.Stderr, "relational perf setup: worker=%d create %s\n", workerID, readTable)
	if _, err := relational.CreateTable(relational.Schema{
		Name: readTable,
		Columns: []relational.Column{
			{Name: "id", Type: relational.ColumnTypeUint64},
			{Name: "segment", Type: relational.ColumnTypeUint64},
			{Name: "active", Type: relational.ColumnTypeBool},
			{Name: "score", Type: relational.ColumnTypeFloat64},
			{Name: "name", Type: relational.ColumnTypeString, Size: 32},
			{Name: "note", Type: relational.ColumnTypeString, Size: 64},
		},
	}); err != nil {
		return fmt.Errorf("CreateTable %s: %w", readTable, err)
	}
	for i := 0; i < rows; i++ {
		if _, err := relational.InsertRow(readTable, relationalPerformanceValues(i)); err != nil {
			return fmt.Errorf("seed %s row %d: %w", readTable, i, err)
		}
		if rows >= 10 && (i+1)%(rows/10) == 0 {
			fmt.Fprintf(os.Stderr, "relational perf setup: worker=%d seeded %d/%d rows\n", workerID, i+1, rows)
		}
	}
	if equalityIndex {
		fmt.Fprintf(os.Stderr, "relational perf setup: worker=%d build segment index\n", workerID)
		if err := relational.CreateIndex(readTable, "segment"); err != nil {
			return fmt.Errorf("CreateIndex %s.segment: %w", readTable, err)
		}
	}
	if trigramIndex {
		fmt.Fprintf(os.Stderr, "relational perf setup: worker=%d build note trigram index\n", workerID)
		if err := relational.CreateTrigramIndex(readTable, "note"); err != nil {
			return fmt.Errorf("CreateTrigramIndex %s.note: %w", readTable, err)
		}
	}
	return nil
}

func createRelationalInsertTable(table string) error {
	fmt.Fprintf(os.Stderr, "relational perf setup: create %s\n", table)
	if _, err := relational.CreateTable(relational.Schema{
		Name: table,
		Columns: []relational.Column{
			{Name: "id", Type: relational.ColumnTypeUint64},
			{Name: "segment", Type: relational.ColumnTypeUint64},
			{Name: "active", Type: relational.ColumnTypeBool},
			{Name: "score", Type: relational.ColumnTypeFloat64},
			{Name: "name", Type: relational.ColumnTypeString, Size: 32},
			{Name: "note", Type: relational.ColumnTypeString, Size: 64},
		},
	}); err != nil {
		return fmt.Errorf("CreateTable %s: %w", table, err)
	}
	return nil
}

func runRelationalSaturationWorker(ctx context.Context, workerID int, seedRows int, mode string, counters *relationalPerformanceCounters) error {
	table := relationalPerformanceReadTable(workerID)
	insertTable := relationalPerformanceInsertTable(workerID)
	seq := 0
	for ctx.Err() == nil {
		switch mode {
		case "read":
			rowID := uint64((seq * 7919) % seedRows)
			if _, err := relational.ReadRow(table, rowID); err != nil {
				return fmt.Errorf("read %s/%d: %w", table, rowID, err)
			}
			counters.reads.Add(1)
		case "insert":
			if _, err := relational.InsertRow(insertTable, relationalPerformanceValues(seq)); err != nil {
				return fmt.Errorf("insert %s/%d: %w", insertTable, seq, err)
			}
			counters.inserts.Add(1)
		case "select-eq":
			rows, err := relational.SelectRows(table, relational.Equal("segment", uint64(seq%128)))
			if err != nil {
				return fmt.Errorf("equality select %s: %w", table, err)
			}
			counters.equalitySelects.Add(1)
			counters.rowsReturned.Add(int64(len(rows)))
		case "select-like":
			rows, err := relational.SelectRows(table, relational.Like("note", relationalPerformanceNeedlePattern(seq)))
			if err != nil {
				return fmt.Errorf("like select %s: %w", table, err)
			}
			counters.likeSelects.Add(1)
			counters.rowsReturned.Add(int64(len(rows)))
		case "mixed":
			rowID := uint64((seq * 7919) % seedRows)
			if _, err := relational.ReadRow(table, rowID); err != nil {
				return fmt.Errorf("read %s/%d: %w", table, rowID, err)
			}
			counters.reads.Add(1)

			equalityRows, err := relational.SelectRows(table, relational.Equal("segment", uint64(seq%128)))
			if err != nil {
				return fmt.Errorf("equality select %s: %w", table, err)
			}
			counters.equalitySelects.Add(1)
			counters.rowsReturned.Add(int64(len(equalityRows)))

			likeRows, err := relational.SelectRows(table, relational.Like("note", relationalPerformanceNeedlePattern(seq)))
			if err != nil {
				return fmt.Errorf("like select %s: %w", table, err)
			}
			counters.likeSelects.Add(1)
			counters.rowsReturned.Add(int64(len(likeRows)))

			if _, err := relational.InsertRow(insertTable, relationalPerformanceValues(seq)); err != nil {
				return fmt.Errorf("insert %s/%d: %w", insertTable, seq, err)
			}
			counters.inserts.Add(1)
		}
		seq++
	}
	return nil
}

func relationalSaturationMode() string {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("TSU_REL_SAT_MODE")))
	if mode == "" {
		return "read"
	}
	return mode
}

func reportRelationalPerformance(t *testing.T, name string, setupDuration time.Duration, elapsed time.Duration, workers int, seedRows int, counters *relationalPerformanceCounters) {
	t.Helper()

	seconds := math.Max(elapsed.Seconds(), 0.001)
	t.Logf("%s setup=%s measured_duration=%s total=%s workers=%d seed_rows_per_worker=%d host_cpus=%d",
		name,
		setupDuration.Round(time.Millisecond),
		elapsed.Round(time.Millisecond),
		(setupDuration + elapsed).Round(time.Millisecond),
		workers,
		seedRows,
		runtime.NumCPU(),
	)
	t.Logf("total actions=%d throughput=%.2f actions/s", counters.totalActions(), float64(counters.totalActions())/seconds)
	t.Logf("inserts=%d %.2f/s reads=%d %.2f/s equality_selects=%d %.2f/s like_selects=%d %.2f/s",
		counters.inserts.Load(), float64(counters.inserts.Load())/seconds,
		counters.reads.Load(), float64(counters.reads.Load())/seconds,
		counters.equalitySelects.Load(), float64(counters.equalitySelects.Load())/seconds,
		counters.likeSelects.Load(), float64(counters.likeSelects.Load())/seconds,
	)
	t.Logf("rows returned=%d %.2f rows/s disk used=%s",
		counters.rowsReturned.Load(),
		float64(counters.rowsReturned.Load())/seconds,
		formatBytes(mustDirSize(t, "db")),
	)
}

func startRelationalPerformanceProgress(ctx context.Context, counters *relationalPerformanceCounters) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)

		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		start := time.Now()
		for {
			select {
			case <-ctx.Done():
				fmt.Fprintf(os.Stderr, "relational perf progress: final elapsed=%s actions=%d inserts=%d reads=%d eq=%d like=%d\n",
					time.Since(start).Round(time.Millisecond),
					counters.totalActions(),
					counters.inserts.Load(),
					counters.reads.Load(),
					counters.equalitySelects.Load(),
					counters.likeSelects.Load(),
				)
				return
			case <-ticker.C:
				elapsed := time.Since(start)
				seconds := math.Max(elapsed.Seconds(), 0.001)
				fmt.Fprintf(os.Stderr, "relational perf progress: elapsed=%s actions=%d throughput=%.2f/s inserts=%d reads=%d eq=%d like=%d\n",
					elapsed.Round(time.Millisecond),
					counters.totalActions(),
					float64(counters.totalActions())/seconds,
					counters.inserts.Load(),
					counters.reads.Load(),
					counters.equalitySelects.Load(),
					counters.likeSelects.Load(),
				)
			}
		}
	}()
	return done
}

func relationalPerformanceReadTable(workerID int) string {
	return fmt.Sprintf("rel_perf_read_%02d", workerID)
}

func relationalPerformanceInsertTable(workerID int) string {
	return fmt.Sprintf("rel_perf_insert_%02d", workerID)
}

func relationalPerformanceValues(i int) map[string]any {
	return map[string]any{
		"id":      uint64(i),
		"segment": uint64(i % 128),
		"active":  i%3 != 0,
		"score":   float64(i%10_000) / 10,
		"name":    fmt.Sprintf("user_%06d", i),
		"note":    fmt.Sprintf("segment_%03d needle_%03d row_%010d", i%128, i%100, i),
	}
}

func relationalPerformanceNeedlePattern(i int) string {
	return fmt.Sprintf("%%needle_%03d%%", i%100)
}
