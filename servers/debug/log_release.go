//go:build !debug

package debug

func Log(log string) {}

func MeasureTime(name string) func() {
	return func() {} // Pusta funkcja
}

func MeasureBlock(name string, fn func()) {
	fn()
}

func PrintTimingStats() {}
