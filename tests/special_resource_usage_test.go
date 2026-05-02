package tests

import (
	"context"
	"fmt"
	"runtime"
	"testing"
	"time"
)

func TestSpecialResourceUsage(t *testing.T) {
	requireSpecialTests(t)
	resetSpecialStorage()

	duration := durationFromEnv("TSU_RESOURCE_DURATION", defaultResourceDuration)
	workers := intFromEnv("TSU_RESOURCE_WORKERS", runtime.NumCPU())
	payloadSize := intFromEnv("TSU_PAYLOAD_BYTES", defaultPayloadBytes)
	sampleEvery := durationFromEnv("TSU_RESOURCE_SAMPLE_EVERY", time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()

	counters := &operationCounters{}
	sampler := newResourceSampler(sampleEvery)
	sampler.start(ctx)

	payload := deterministicPayload(payloadSize)
	err := runWorkers(ctx, workers, func(ctx context.Context, workerID int) error {
		seq := 0
		table := fmt.Sprintf("special_resource_%02d", workerID)
		for ctx.Err() == nil {
			key := fmt.Sprintf("resource_%02d_%08d", workerID, seq)
			if err := saveReadFreeCycle(table, key, payload, seq%10 == 0, counters); err != nil {
				return err
			}
			seq++
		}
		return nil
	})
	sampler.stop()
	if err != nil {
		t.Fatal(err)
	}

	reportResourceSummary(t, "resource", duration, counters, sampler.snapshots())
}
