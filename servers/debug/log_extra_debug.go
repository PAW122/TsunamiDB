//go:build debug_extra

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

func LogExtra(args ...interface{}) {
	// Użyj fmt.Sprintln, aby połączyć argumenty w jeden string
	log := fmt.Sprintln(args...)
	fmt.Print("[DEBUG EXTRA] ", log) // fmt.Print, aby uniknąć dodatkowego nowego wiersza
}
