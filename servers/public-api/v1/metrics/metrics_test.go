package metrics

import (
	"math"
	"testing"
	"time"
)

func TestSnapshotStatsEmpty(t *testing.T) {
	ResetForTests()

	snap := SnapshotStats()
	if snap.TotalRequests != 0 {
		t.Fatalf("expected no requests, got %d", snap.TotalRequests)
	}
	if snap.TotalDuration != 0 {
		t.Fatalf("expected zero duration, got %s", snap.TotalDuration)
	}
	if snap.AverageResponse != 0 {
		t.Fatalf("expected zero average, got %s", snap.AverageResponse)
	}
	if !snap.LastRequestAt.IsZero() {
		t.Fatalf("expected zero last request, got %s", snap.LastRequestAt)
	}
	if snap.StartedAt.IsZero() {
		t.Fatalf("expected start time to be set")
	}
}

func TestRecordRequestTracksTotalsAndClampsNegativeDuration(t *testing.T) {
	ResetForTests()

	RecordRequest(10 * time.Millisecond)
	RecordRequest(-5 * time.Second)

	snap := SnapshotStats()
	if snap.TotalRequests != 2 {
		t.Fatalf("expected two requests, got %d", snap.TotalRequests)
	}
	if snap.TotalDuration != 10*time.Millisecond {
		t.Fatalf("expected total duration to ignore negative input, got %s", snap.TotalDuration)
	}
	if snap.AverageResponse != 5*time.Millisecond {
		t.Fatalf("expected average 5ms, got %s", snap.AverageResponse)
	}
	if snap.LastRequestAt.IsZero() {
		t.Fatalf("expected last request timestamp")
	}
}

func TestSnapshotStatsCapsAverageDivisorAtMaxInt64(t *testing.T) {
	ResetForTests()
	totalDuration.Store(100)
	requestCount.Store(uint64(math.MaxInt64) + 10)

	snap := SnapshotStats()
	if snap.AverageResponse != 0 {
		t.Fatalf("expected truncated average of 0, got %s", snap.AverageResponse)
	}
}
