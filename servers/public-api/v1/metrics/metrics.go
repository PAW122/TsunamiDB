package metrics

import (
	"math"
	"sync/atomic"
	"time"
)

type Snapshot struct {
	StartedAt       time.Time
	TotalRequests   uint64
	TotalDuration   time.Duration
	AverageResponse time.Duration
	LastRequestAt   time.Time
}

var (
	startTime     = time.Now()
	totalDuration atomic.Int64
	requestCount  atomic.Uint64
	lastRequest   atomic.Int64
)

func RecordRequest(duration time.Duration) {
	if duration < 0 {
		duration = 0
	}
	totalDuration.Add(duration.Nanoseconds())
	requestCount.Add(1)
	lastRequest.Store(time.Now().UnixNano())
}

func SnapshotStats() Snapshot {
	totalNs := totalDuration.Load()
	count := requestCount.Load()
	lastNs := lastRequest.Load()

	snap := Snapshot{
		StartedAt:     startTime,
		TotalRequests: count,
		TotalDuration: time.Duration(totalNs),
	}

	if count > 0 {
		divisor := int64(count)
		if count > math.MaxInt64 {
			divisor = math.MaxInt64
		}
		snap.AverageResponse = time.Duration(totalNs / divisor)
	}

	if lastNs > 0 {
		snap.LastRequestAt = time.Unix(0, lastNs)
	}

	return snap
}

func ResetForTests() {
	totalDuration.Store(0)
	requestCount.Store(0)
	lastRequest.Store(0)
	startTime = time.Now()
}
