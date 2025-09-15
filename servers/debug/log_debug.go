//go:build debug

package debug

import (
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"
)

type timingEntry struct {
	totalNs int64
	count   int64
}

var (
	timingData sync.Map
	once       sync.Once
)

func Log(log string) {
	fmt.Println("[DEBUG]", log)
}

// Użycie: defer debug.MeasureTime("Nazwa")()
func MeasureTime(name string) func() {
	start := time.Now()
	once.Do(StartStatsLogger)

	entry := getOrCreateEntry(name)
	return func() {
		duration := time.Since(start)
		atomic.AddInt64(&entry.totalNs, duration.Nanoseconds())
		atomic.AddInt64(&entry.count, 1)
	}
}

// Rejestruje czas trwania danego bloku i zapisuje go do statystyk
func MeasureBlock(name string, fn func()) {
	start := time.Now()
	fn()
	duration := time.Since(start)
	entry := getOrCreateEntry(name)
	atomic.AddInt64(&entry.totalNs, duration.Nanoseconds())
	atomic.AddInt64(&entry.count, 1)
}

func getOrCreateEntry(name string) *timingEntry {
	val, _ := timingData.LoadOrStore(name, &timingEntry{})
	return val.(*timingEntry)
}

// Startuje cykliczny logger co 5s
func StartStatsLogger() {
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			log.Println("---------- [DEBUG] Function Timing Stats ----------")
			timingData.Range(func(key, val any) bool {
				name := key.(string)
				entry := val.(*timingEntry)
				total := time.Duration(atomic.LoadInt64(&entry.totalNs))
				count := atomic.LoadInt64(&entry.count)
				var avg time.Duration
				if count > 0 {
					avg = total / time.Duration(count)
				}
				log.Printf("%-30s | calls: %6d | avg: %8s | total: %s\n", name, count, avg, total)
				return true
			})
			log.Println("---------------------------------------------------")
		}
	}()
}

// Wyświetla średnie czasy wykonania dla wszystkich funkcji
func PrintTimingStats() {
	fmt.Println("\n[DEBUG] Timing Stats:")
	timingData.Range(func(key, value interface{}) bool {
		entry := value.(*timingEntry)
		total := time.Duration(atomic.LoadInt64(&entry.totalNs))
		count := atomic.LoadInt64(&entry.count)
		var avg time.Duration
		if count > 0 {
			avg = total / time.Duration(count)
		}
		fmt.Printf("%s - avg: %v, total: %v, count: %d\n", key, avg, total, count)
		return true
	})
}
