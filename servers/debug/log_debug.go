//go:build debug

package debug

import (
	"fmt"
	"time"
)

func Log(log string) {
	fmt.Println("[DEBUG]", log)
}

/*
mierzy czas od dodania w kodzie:
defer debug.MeasureTime("name")()
do końca funkcji
*/
func MeasureTime(name string) func() {
	start := time.Now()
	return func() {
		fmt.Printf("[DEBUG] %s took %v\n", name, time.Since(start))
	}
}
