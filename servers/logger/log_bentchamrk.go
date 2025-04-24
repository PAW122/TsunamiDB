//go:build bentchmark

package logger

import (
	"fmt"
	"log"
	"os"
	"sync"
	"time"
)

var (
	logFile    *os.File
	logger     *log.Logger
	enabled    bool
	logChannel chan string
	wg         sync.WaitGroup
	timingData sync.Map
)

type TimingStats struct {
	TotalTime time.Duration
	Count     int64
}

func InitLogger(filePath string, enable bool) error {
	enabled = enable
	if !enabled {
		return nil
	}

	var err error
	logFile, err = os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}

	logger = log.New(logFile, "", log.LstdFlags)

	logChannel = make(chan string, 1000)
	wg.Add(1)
	go logWorker()

	return nil
}

func logWorker() {
	defer wg.Done()
	for log := range logChannel {
		if logger != nil {
			logger.Println(log)
		}
	}
}

func Log(format string, v ...interface{}) {
	if !enabled {
		return
	}

	log := fmt.Sprintf(format, v...)
	select {
	case logChannel <- log:
	default:

	}
}

func MeasureTime(name string) func() {
	if !enabled {
		return func() {}
	}

	start := time.Now()
	return func() {
		duration := time.Since(start)

		if val, ok := timingData.Load(name); ok {
			stats := val.(*TimingStats)
			stats.TotalTime += duration
			stats.Count++
		} else {
			timingData.Store(name, &TimingStats{TotalTime: duration, Count: 1})
		}

		Log("%s took %v", name, duration)
	}
}

func PrintTimingStats() {
	if !enabled {
		return
	}

	fmt.Println("\n[LOGGER] Timing Stats:")
	timingData.Range(func(key, value interface{}) bool {
		stats := value.(*TimingStats)
		avgTime := stats.TotalTime / time.Duration(stats.Count)
		fmt.Printf("%s - avg: %v, total: %v, count: %d\n", key, avgTime, stats.TotalTime, stats.Count)
		Log("%s - avg: %v, total: %v, count: %d", key, avgTime, stats.TotalTime, stats.Count)
		return true
	})
}

func CloseLogger() {
	if !enabled {
		return
	}

	close(logChannel)
	wg.Wait()

	if logFile != nil {
		logFile.Close()
	}
}
