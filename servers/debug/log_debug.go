//go:build debug

package debug

import (
	"fmt"
	"log"
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
	once       sync.Once
)

func Log(log string) {
	fmt.Println("[DEBUG]", log)
}

// Użycie: defer debug.MeasureTime("Nazwa")()
func MeasureTime(name string) func() {
	start := time.Now()
	once.Do(StartStatsLogger)

	return func() {
		duration := time.Since(start)
		logMutex.Lock()
		val, _ := timingData.LoadOrStore(name, &TimingStats{})
		stats := val.(*TimingStats)
		stats.TotalTime += duration
		stats.Count++
		logMutex.Unlock()
	}
}

// Rejestruje czas trwania danego bloku i zapisuje go do statystyk
func MeasureBlock(name string, fn func()) {
	start := time.Now()
	fn()
	duration := time.Since(start)

	logMutex.Lock()
	defer logMutex.Unlock()

	if val, ok := timingData.Load(name); ok {
		stats := val.(*TimingStats)
		stats.TotalTime += duration
		stats.Count++
	} else {
		timingData.Store(name, &TimingStats{
			TotalTime: duration,
			Count:     1,
		})
	}
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
				stats := val.(*TimingStats)

				var avg time.Duration
				if stats.Count > 0 {
					avg = stats.TotalTime / time.Duration(stats.Count)
				}

				log.Printf("%-30s | calls: %6d | avg: %8s | total: %s\n",
					name, stats.Count, avg, stats.TotalTime)

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
		stats := value.(*TimingStats)
		avgTime := stats.TotalTime / time.Duration(stats.Count)
		fmt.Printf("%s - avg: %v, total: %v, count: %d\n", key, avgTime, stats.TotalTime, stats.Count)
		return true
	})
}
