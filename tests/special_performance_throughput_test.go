package tests

import (
	"context"
	"fmt"
	"math"
	"runtime"
	"testing"
	"time"
)

func TestSpecialPerformanceThroughput(t *testing.T) {
	requireSpecialTests(t)
	resetSpecialStorage()

	duration := durationFromEnv("TSU_PERF_DURATION", defaultPerformanceTime)
	workers := intFromEnv("TSU_PERF_WORKERS", runtime.NumCPU())
	payloadSize := intFromEnv("TSU_PAYLOAD_BYTES", defaultPayloadBytes)
	payload := deterministicPayload(payloadSize)

	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()

	counters := &operationCounters{}
	start := time.Now()
	err := runWorkers(ctx, workers, func(ctx context.Context, workerID int) error {
		seq := 0
		table := fmt.Sprintf("special_perf_%02d", workerID)
		for ctx.Err() == nil {
			key := fmt.Sprintf("perf_%02d_%012d", workerID, seq)
			if err := saveReadFreeCycle(table, key, payload, seq%20 == 0, counters); err != nil {
				return err
			}
			seq++
		}
		return nil
	})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}

	seconds := math.Max(elapsed.Seconds(), 0.001)
	t.Logf("performance duration=%s workers=%d payload=%s", elapsed.Round(time.Millisecond), workers, formatBytes(int64(len(payload))))
	t.Logf("total actions=%d throughput=%.2f actions/s", counters.totalActions(), float64(counters.totalActions())/seconds)
	t.Logf("writes=%d %.2f/s reads=%d %.2f/s frees=%d %.2f/s",
		counters.writes.Load(), float64(counters.writes.Load())/seconds,
		counters.reads.Load(), float64(counters.reads.Load())/seconds,
		counters.frees.Load(), float64(counters.frees.Load())/seconds,
	)
	t.Logf("logical write IO=%s logical read IO=%s disk used=%s",
		formatBytes(counters.logicalWriteBytes.Load()),
		formatBytes(counters.logicalReadBytes.Load()),
		formatBytes(mustDirSize(t, "db")),
	)
}
