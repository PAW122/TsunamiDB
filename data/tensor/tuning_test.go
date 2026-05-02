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

	weight := table.stats.resultInputWeight("result", 0)
	if weight >= 1 {
		t.Fatalf("weight=%g, want below 1 after tuning low-impact expected label", weight)
	}
	if len(report.TopSuppressed["result"]) == 0 {
		t.Fatalf("missing suppressed gate summary: %#v", report)
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
}
