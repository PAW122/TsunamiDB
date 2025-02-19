//go:build debug

package debug

import (
	"fmt"
	"sync"
	"time"
)

// Struktura przechowująca statystyki czasu wykonania

type TimingStats struct {
	TotalTime time.Duration
	Count     int64
}

var (
	logMutex   sync.Mutex
	timingData sync.Map // Mapa dla statystyk czasu
)

func Log(log string) {
	fmt.Println("[DEBUG]", log)
}

// Mierzy czas wykonania i zapisuje do statystyk
func MeasureTime(name string) func() {
	start := time.Now()
	return func() {
		duration := time.Since(start)
		logMutex.Lock()
		if val, ok := timingData.Load(name); ok {
			stats := val.(*TimingStats)
			stats.TotalTime += duration
			stats.Count++
		} else {
			timingData.Store(name, &TimingStats{TotalTime: duration, Count: 1})
		}
		logMutex.Unlock()
		//fmt.Printf("[DEBUG] %s took %v\n", name, duration)
	}
}

// Wyświetla średnie czasy wykonania dla wszystkich funkcji
func PrintTimingStats() {
	fmt.Println("\n[DEBUG] Timing Stats:")
	timingData.Range(func(key, value interface{}) bool {
		stats := value.(*TimingStats)
		avgTime := stats.TotalTime / time.Duration(stats.Count)
		fmt.Printf("%s - avg: %v, total: %v, count: %d\n", key, avgTime, stats.TotalTime, stats.Count)
		return true
	})
}
