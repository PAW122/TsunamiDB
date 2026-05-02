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
	defaultRelationalReportDuration      = 10 * time.Second
	relationalRelatedUserCount           = 64
)

type relationalPerformanceCounters struct {
	inserts         atomic.Int64
	reads           atomic.Int64
	equalitySelects atomic.Int64
	likeSelects     atomic.Int64
	relatedSelects  atomic.Int64
	rowsReturned    atomic.Int64
}

type relationalReportResult struct {
	mode           string
	setup          time.Duration
	measured       time.Duration
	workers        int
	rows           int
	actions        int64
	inserts        int64
	reads          int64
	eqSelects      int64
	likeSelects    int64
	relatedSelects int64
	rowsReturned   int64
	diskBytes      int64
	actionsPerSec  float64
	rowsPerSec     float64
}

func (c *relationalPerformanceCounters) totalActions() int64 {
	return c.inserts.Load() + c.reads.Load() + c.equalitySelects.Load() + c.likeSelects.Load() + c.relatedSelects.Load()
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
	relational.ResetForTests()

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
		case "related-select":
			return prepareRelationalRelatedTables(workerID, seedRows)
		case "mixed":
			return prepareRelationalPerformanceTables(workerID, seedRows)
		default:
			return fmt.Errorf("unsupported TSU_REL_SAT_MODE=%q; use read, insert, select-eq, select-like, related-select, or mixed", mode)
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
	relational.ResetForTests()

	reportRelationalPerformance(t, "relational saturation", setupDuration, elapsed, workers, seedRows, counters)
}

func TestSpecialRelationalReport(t *testing.T) {
	requireSpecialTests(t)

	duration := durationFromEnv("TSU_REL_REPORT_DURATION", defaultRelationalReportDuration)
	workers := intFromEnv("TSU_REL_REPORT_WORKERS", runtime.NumCPU())
	seedRows := intFromEnv("TSU_REL_REPORT_ROWS", defaultRelationalPerformanceRows)
	modes := relationalReportModes()

	results := make([]relationalReportResult, 0, len(modes))
	totalStart := time.Now()
	for _, mode := range modes {
		fmt.Fprintf(os.Stderr, "relational report: mode=%s workers=%d rows=%d duration=%s\n", mode, workers, seedRows, duration)
		result, err := runRelationalReportProfile(mode, duration, workers, seedRows)
		if err != nil {
			t.Fatalf("relational report mode %s: %v", mode, err)
		}
		results = append(results, result)
		fmt.Fprintf(os.Stderr, "relational report: mode=%s done throughput=%.2f/s setup=%s disk=%s\n",
			mode,
			result.actionsPerSec,
			result.setup.Round(time.Millisecond),
			formatBytes(result.diskBytes),
		)
	}

	t.Logf("\n%s", formatRelationalPerformanceReport(results, time.Since(totalStart)))
}

func runRelationalReportProfile(mode string, duration time.Duration, workers int, seedRows int) (relationalReportResult, error) {
	resetSpecialStorage()

	setupStart := time.Now()
	if err := setupRelationalSaturationMode(mode, workers, seedRows); err != nil {
		return relationalReportResult{}, err
	}
	setupDuration := time.Since(setupStart)

	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()

	counters := &relationalPerformanceCounters{}
	start := time.Now()
	err := runWorkers(ctx, workers, func(ctx context.Context, workerID int) error {
		return runRelationalSaturationWorker(ctx, workerID, seedRows, mode, counters)
	})
	elapsed := time.Since(start)
	relational.ResetForTests()
	if err != nil {
		return relationalReportResult{}, err
	}

	seconds := math.Max(elapsed.Seconds(), 0.001)
	return relationalReportResult{
		mode:           mode,
		setup:          setupDuration,
		measured:       elapsed,
		workers:        workers,
		rows:           seedRows,
		actions:        counters.totalActions(),
		inserts:        counters.inserts.Load(),
		reads:          counters.reads.Load(),
		eqSelects:      counters.equalitySelects.Load(),
		likeSelects:    counters.likeSelects.Load(),
		relatedSelects: counters.relatedSelects.Load(),
		rowsReturned:   counters.rowsReturned.Load(),
		diskBytes:      dirSize("db"),
		actionsPerSec:  float64(counters.totalActions()) / seconds,
		rowsPerSec:     float64(counters.rowsReturned.Load()) / seconds,
	}, nil
}

func setupRelationalSaturationMode(mode string, workers int, seedRows int) error {
	return runRelationalSetupWorkers(workers, func(workerID int) error {
		switch mode {
		case "insert":
			return createRelationalInsertTable(relationalPerformanceInsertTable(workerID))
		case "read":
			return prepareRelationalReadTable(workerID, seedRows, false, false)
		case "select-eq":
			return prepareRelationalReadTable(workerID, seedRows, true, false)
		case "select-like":
			return prepareRelationalReadTable(workerID, seedRows, false, true)
		case "related-select":
			return prepareRelationalRelatedTables(workerID, seedRows)
		case "mixed":
			return prepareRelationalPerformanceTables(workerID, seedRows)
		default:
			return fmt.Errorf("unsupported relational saturation mode %q; use read, insert, select-eq, select-like, related-select, or mixed", mode)
		}
	})
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
		if relationalSetupProgressEnabled() && rows >= 10 && (i+1)%(rows/10) == 0 {
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

func prepareRelationalRelatedTables(workerID int, orderRows int) error {
	usersTable := relationalRelatedUsersTable(workerID)
	ordersTable := relationalRelatedOrdersTable(workerID)

	fmt.Fprintf(os.Stderr, "relational perf setup: worker=%d create %s\n", workerID, usersTable)
	if _, err := relational.CreateTable(relational.Schema{
		Name: usersTable,
		Columns: []relational.Column{
			{Name: "id", Type: relational.ColumnTypeUint64},
			{Name: "name", Type: relational.ColumnTypeString, Size: 32},
			{Name: "tier", Type: relational.ColumnTypeUint64},
		},
	}); err != nil {
		return fmt.Errorf("CreateTable %s: %w", usersTable, err)
	}
	for i := 0; i < relationalRelatedUserCount; i++ {
		if _, err := relational.InsertRow(usersTable, map[string]any{
			"id":   uint64(i),
			"name": fmt.Sprintf("user_%03d", i),
			"tier": uint64(i % 4),
		}); err != nil {
			return fmt.Errorf("seed %s row %d: %w", usersTable, i, err)
		}
	}

	fmt.Fprintf(os.Stderr, "relational perf setup: worker=%d create %s\n", workerID, ordersTable)
	if _, err := relational.CreateTable(relational.Schema{
		Name: ordersTable,
		Columns: []relational.Column{
			{Name: "id", Type: relational.ColumnTypeUint64},
			{Name: "user_id", Type: relational.ColumnTypeRowRef, RefTable: usersTable},
			{Name: "total", Type: relational.ColumnTypeUint64},
			{Name: "status", Type: relational.ColumnTypeString, Size: 16},
		},
	}); err != nil {
		return fmt.Errorf("CreateTable %s: %w", ordersTable, err)
	}
	for i := 0; i < orderRows; i++ {
		if _, err := relational.InsertRow(ordersTable, map[string]any{
			"id":      uint64(i),
			"user_id": uint64(i % relationalRelatedUserCount),
			"total":   uint64(100 + i%900),
			"status":  fmt.Sprintf("status_%02d", i%8),
		}); err != nil {
			return fmt.Errorf("seed %s row %d: %w", ordersTable, i, err)
		}
		if relationalSetupProgressEnabled() && orderRows >= 10 && (i+1)%(orderRows/10) == 0 {
			fmt.Fprintf(os.Stderr, "relational perf setup: worker=%d seeded related %d/%d rows\n", workerID, i+1, orderRows)
		}
	}
	fmt.Fprintf(os.Stderr, "relational perf setup: worker=%d build %s.user_id index\n", workerID, ordersTable)
	if err := relational.CreateIndex(ordersTable, "user_id"); err != nil {
		return fmt.Errorf("CreateIndex %s.user_id: %w", ordersTable, err)
	}
	return nil
}

func runRelationalSaturationWorker(ctx context.Context, workerID int, seedRows int, mode string, counters *relationalPerformanceCounters) error {
	table := relationalPerformanceReadTable(workerID)
	insertTable := relationalPerformanceInsertTable(workerID)
	relatedOrdersTable := relationalRelatedOrdersTable(workerID)
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
		case "related-select":
			predicate := relational.Equal("user_id", uint64(seq%relationalRelatedUserCount))
			rows, err := relational.JoinRowRef(relatedOrdersTable, "user_id", &predicate)
			if err != nil {
				return fmt.Errorf("related select %s: %w", relatedOrdersTable, err)
			}
			counters.relatedSelects.Add(1)
			counters.rowsReturned.Add(int64(len(rows)))
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

func relationalReportModes() []string {
	raw := strings.TrimSpace(os.Getenv("TSU_REL_REPORT_MODES"))
	if raw == "" {
		return []string{"read", "insert", "select-eq", "select-like", "related-select", "mixed"}
	}

	parts := strings.Split(raw, ",")
	modes := make([]string, 0, len(parts))
	for _, part := range parts {
		mode := strings.ToLower(strings.TrimSpace(part))
		if mode != "" {
			modes = append(modes, mode)
		}
	}
	if len(modes) == 0 {
		return []string{"read", "insert", "select-eq", "select-like", "related-select", "mixed"}
	}
	return modes
}

func relationalSetupProgressEnabled() bool {
	return strings.TrimSpace(os.Getenv("TSU_REL_SETUP_PROGRESS")) != "0"
}

func formatRelationalPerformanceReport(results []relationalReportResult, total time.Duration) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Relational performance report\n")
	fmt.Fprintf(&b, "  total: %s\n", total.Round(time.Millisecond))
	fmt.Fprintf(&b, "  host_cpus: %d\n\n", runtime.NumCPU())
	fmt.Fprintf(&b, "%-15s %7s %8s %9s %11s %11s %11s %11s %11s %12s %10s\n",
		"mode", "workers", "rows", "setup", "actions/s", "reads/s", "inserts/s", "selects/s", "related/s", "rows/s", "disk")
	for _, result := range results {
		seconds := math.Max(result.measured.Seconds(), 0.001)
		selects := result.eqSelects + result.likeSelects
		fmt.Fprintf(&b, "%-15s %7d %8d %9s %11.2f %11.2f %11.2f %11.2f %11.2f %12.2f %10s\n",
			result.mode,
			result.workers,
			result.rows,
			result.setup.Round(time.Millisecond),
			result.actionsPerSec,
			float64(result.reads)/seconds,
			float64(result.inserts)/seconds,
			float64(selects)/seconds,
			float64(result.relatedSelects)/seconds,
			result.rowsPerSec,
			formatBytes(result.diskBytes),
		)
	}
	return b.String()
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

func relationalRelatedUsersTable(workerID int) string {
	return fmt.Sprintf("rel_perf_users_%02d", workerID)
}

func relationalRelatedOrdersTable(workerID int) string {
	return fmt.Sprintf("rel_perf_orders_%02d", workerID)
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
