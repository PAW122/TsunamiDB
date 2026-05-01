package debug

import "testing"

func TestNoopDebugFunctions(t *testing.T) {
	Log("message")
	called := false
	MeasureBlock("block", func() {
		called = true
	})
	if !called {
		t.Fatalf("MeasureBlock did not execute callback")
	}
	done := MeasureTime("timer")
	done()
	PrintTimingStats()
	LogExtra("extra", 1)
}
