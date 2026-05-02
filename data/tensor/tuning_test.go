package tensor

import (
	"math"
	"testing"
)

func TestTuneWeightsAdjustsAndStatsPayloadPreservesResultGates(t *testing.T) {
	schema, err := validateSchema(Schema{
		Name: "tune_weights",
		Inputs: []InputField{
			{Name: "signal", Type: InputTypeFloat64, ResultKey: "result"},
		},
		Results: []ResultField{
			{Key: "result", Type: InputTypeString},
		},
	})
	if err != nil {
		t.Fatalf("validateSchema: %v", err)
	}

	table := newTable(schema, newStats(schema), tableFiles{})
	for i := 0; i < 20; i++ {
		table.addFloatStats([]float64{0}, []ResultLabel{{Key: "result", Value: "a"}})
		table.addFloatStats([]float64{10}, []ResultLabel{{Key: "result", Value: "b"}})
	}

	report, err := table.TuneWeights([]Sample{
		{
			LearningStatus: LearningStatusPositive,
			Input:          map[string]any{"signal": 10.0},
			Results:        []ResultLabel{{Key: "result", Value: "a"}},
		},
	}, TuneOptions{})
	if err != nil {
		t.Fatalf("TuneWeights: %v", err)
	}
	if report.Corrections == 0 || report.Adjustments == 0 {
		t.Fatalf("unexpected tune report: %#v", report)
	}
	if report.TrainingCycles == 0 || report.AINeurons == 0 || report.AIWeights == 0 || report.AIBiases == 0 {
		t.Fatalf("missing AI layer stats in tune report: %#v", report)
	}

	weight := table.stats.resultInputWeight("result", 0)
	if weight >= 1 {
		t.Fatalf("weight=%g, want below 1 after tuning low-impact expected label", weight)
	}
	if len(report.TopSuppressed["result"]) == 0 {
		t.Fatalf("missing suppressed gate summary: %#v", report)
	}
	bias := table.stats.labelBias("result", "a")
	if bias <= 0 {
		t.Fatalf("label bias=%g, want positive after correcting expected label", bias)
	}

	payload, err := encodeStatsPayload(schema, table.stats)
	if err != nil {
		t.Fatalf("encodeStatsPayload: %v", err)
	}
	decoded, err := decodeStatsPayload(schema, payload)
	if err != nil {
		t.Fatalf("decodeStatsPayload: %v", err)
	}
	if math.Abs(decoded.resultInputWeight("result", 0)-weight) > 1e-12 {
		t.Fatalf("decoded weight=%g want %g", decoded.resultInputWeight("result", 0), weight)
	}
	if math.Abs(decoded.labelBias("result", "a")-bias) > 1e-12 {
		t.Fatalf("decoded bias=%g want %g", decoded.labelBias("result", "a"), bias)
	}
}

func TestTuneWeightsReportsProgress(t *testing.T) {
	schema, err := validateSchema(Schema{
		Name: "tune_progress",
		Inputs: []InputField{
			{Name: "signal", Type: InputTypeFloat64, ResultKey: "result"},
		},
		Results: []ResultField{
			{Key: "result", Type: InputTypeString},
		},
	})
	if err != nil {
		t.Fatalf("validateSchema: %v", err)
	}

	table := newTable(schema, newStats(schema), tableFiles{})
	table.addFloatStats([]float64{0}, []ResultLabel{{Key: "result", Value: "a"}})
	table.addFloatStats([]float64{10}, []ResultLabel{{Key: "result", Value: "b"}})

	type progressEvent struct {
		completed int
		total     int
	}
	var events []progressEvent
	_, err = table.TuneWeights([]Sample{
		{
			LearningStatus: LearningStatusPositive,
			Input:          map[string]any{"signal": 0.0},
			Results:        []ResultLabel{{Key: "result", Value: "a"}},
		},
	}, TuneOptions{
		Iterations: 2,
		Progress: func(completed, total int) {
			events = append(events, progressEvent{completed: completed, total: total})
		},
	})
	if err != nil {
		t.Fatalf("TuneWeights: %v", err)
	}
	if len(events) == 0 {
		t.Fatal("missing progress events")
	}
	if events[0] != (progressEvent{completed: 0, total: 5}) {
		t.Fatalf("first progress event=%+v, want completed=0 total=5", events[0])
	}
	last := events[len(events)-1]
	if last != (progressEvent{completed: 5, total: 5}) {
		t.Fatalf("last progress event=%+v, want completed=5 total=5", last)
	}
	finalEvents := 0
	for i, event := range events {
		if event.total != 5 {
			t.Fatalf("event %d total=%d, want 5", i, event.total)
		}
		if i > 0 && event.completed < events[i-1].completed {
			t.Fatalf("progress regressed at event %d: %+v after %+v", i, event, events[i-1])
		}
		if event.completed == event.total {
			finalEvents++
		}
	}
	if finalEvents != 1 {
		t.Fatalf("final progress event count=%d, want 1: %+v", finalEvents, events)
	}
}
