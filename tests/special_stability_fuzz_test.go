package tests

import (
	"context"
	"fmt"
	"math/rand"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

func TestSpecialStabilityFuzz(t *testing.T) {
	requireSpecialTests(t)
	resetSpecialStorage()

	duration := durationFromEnv("TSU_STABILITY_DURATION", defaultStabilityDuration)
	workers := intFromEnv("TSU_STABILITY_WORKERS", runtime.NumCPU())
	payloadLimit := intFromEnv("TSU_STABILITY_MAX_PAYLOAD_BYTES", 4096)
	seed := int64FromEnv("TSU_STABILITY_SEED", time.Now().UnixNano())

	t.Logf("stability fuzz seed=%d duration=%s workers=%d", seed, duration, workers)

	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()
	counters := &operationCounters{}
	var globalSeq atomic.Int64

	err := runWorkers(ctx, workers, func(ctx context.Context, workerID int) error {
		rng := rand.New(rand.NewSource(seed + int64(workerID)*7919))
		for ctx.Err() == nil {
			seq := globalSeq.Add(1)
			payload := randomPayload(rng, payloadLimit)
			table := fmt.Sprintf("special_fuzz_%02d", workerID%8)
			key := fmt.Sprintf("fuzz_%02d_%012d", workerID, seq)

			switch rng.Intn(3) {
			case 0:
				if err := exerciseGoClient(table, key, payload, counters); err != nil {
					return fmt.Errorf("go client action failed: %w", err)
				}
			case 1:
				if err := exerciseHTTPRoutes(table, key, payload, rng, counters); err != nil {
					return fmt.Errorf("api route action failed: %w", err)
				}
			default:
				if err := exerciseDLLCandidate(table, key, payload, counters); err != nil {
					return fmt.Errorf("dll candidate action failed: %w", err)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	t.Logf(
		"stability summary: actions=%d writes=%d reads=%d encrypted_writes=%d encrypted_reads=%d frees=%d api=%d go_lib=%d dll_candidate=%d disk=%s",
		counters.totalActions(),
		counters.writes.Load(),
		counters.reads.Load(),
		counters.encryptedWrites.Load(),
		counters.encryptedReads.Load(),
		counters.frees.Load(),
		counters.apiActions.Load(),
		counters.goLibActions.Load(),
		counters.dllCandidateActions.Load(),
		formatBytes(mustDirSize(t, "db")),
	)
}
