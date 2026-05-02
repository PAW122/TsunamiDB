package tensor

import (
	"os"
	"testing"

	dataManager_v2 "github.com/PAW122/TsunamiDB/data/dataManager/v2"
	fileSystem_v1 "github.com/PAW122/TsunamiDB/data/fileSystem/v1"
)

func TestTrainAIPredictsAndPersistsModel(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir temp: %v", err)
	}
	t.Cleanup(func() {
		dataManager_v2.ShutdownWorkersForTests()
		fileSystem_v1.ShutdownForTests()
		_ = os.Chdir(wd)
	})

	table, err := CreateTable(Schema{
		Name: "ai_model",
		Inputs: []InputField{
			{Name: "signal", Type: InputTypeFloat64, ResultKey: "class"},
			{Name: "context", Type: InputTypeFloat64},
		},
		Results: []ResultField{
			{Key: "class", Type: InputTypeString},
		},
	})
	if err != nil {
		t.Fatalf("CreateTable: %v", err)
	}

	training := makeAISamples(80, 0)
	validation := makeAISamples(20, 1000)
	report, err := table.TrainAI(training, AITrainOptions{
		Epochs:            50,
		BatchSize:         8,
		LearningRate:      0.08,
		InputDropout:      0,
		ValidationSamples: validation,
		Patience:          10,
		Seed:              42,
	})
	if err != nil {
		t.Fatalf("TrainAI: %v", err)
	}
	if report.ValidationLabelAccuracy < 0.95 {
		t.Fatalf("validation label accuracy=%g, want >= 0.95; report=%#v", report.ValidationLabelAccuracy, report)
	}

	assertAIPrediction(t, table, -3, "cold")
	assertAIPrediction(t, table, 3, "hot")
	if err := table.FlushAIModel(); err != nil {
		t.Fatalf("FlushAIModel: %v", err)
	}
	reopened, err := OpenTable("ai_model")
	if err != nil {
		t.Fatalf("OpenTable: %v", err)
	}
	assertAIPrediction(t, reopened, 3, "hot")
	prediction, err := reopened.Predict(map[string]any{"signal": 3.0, "context": 1.0}, 1)
	if err != nil {
		t.Fatalf("Predict with AI model: %v", err)
	}
	if len(prediction.Results) != 1 || prediction.Results[0].Value != "hot" {
		t.Fatalf("Predict with AI model=%#v, want hot", prediction.Results)
	}
}

func TestTrainAIHiddenLayerLearnsNonLinearBoundary(t *testing.T) {
	schema, err := validateSchema(Schema{
		Name: "ai_xor",
		Inputs: []InputField{
			{Name: "x", Type: InputTypeFloat64, ResultKey: "class"},
			{Name: "y", Type: InputTypeFloat64, ResultKey: "class"},
		},
		Results: []ResultField{
			{Key: "class", Type: InputTypeString},
		},
	})
	if err != nil {
		t.Fatalf("validateSchema: %v", err)
	}
	table := newTable(schema, newStats(schema), tableFiles{})

	samples := make([]Sample, 0, 160)
	points := []struct {
		x, y  float64
		class string
	}{
		{-1, -1, "same"},
		{-1, 1, "different"},
		{1, -1, "different"},
		{1, 1, "same"},
	}
	for i := 0; i < 40; i++ {
		for _, point := range points {
			jitter := float64(i%5) * 0.01
			samples = append(samples, Sample{
				LearningStatus: LearningStatusPositive,
				Input: map[string]any{
					"x": point.x + jitter,
					"y": point.y - jitter,
				},
				Results: []ResultLabel{{Key: "class", Value: point.class}},
			})
		}
	}

	report, err := table.TrainAI(samples, AITrainOptions{
		Epochs:          200,
		BatchSize:       8,
		HiddenSizes:     []int{8},
		LearningRate:    0.05,
		InputDropout:    0,
		ValidationSplit: 0,
		Patience:        200,
		Seed:            7,
	})
	if err != nil {
		t.Fatalf("TrainAI: %v", err)
	}
	if report.TrainLabelAccuracy < 0.95 {
		t.Fatalf("train accuracy=%g, want >= 0.95; report=%#v", report.TrainLabelAccuracy, report)
	}
	assertAIXORPrediction(t, table, -1, -1, "same")
	assertAIXORPrediction(t, table, -1, 1, "different")
	assertAIXORPrediction(t, table, 1, -1, "different")
	assertAIXORPrediction(t, table, 1, 1, "same")
}

func makeAISamples(count, offset int) []Sample {
	samples := make([]Sample, 0, count)
	for i := 0; i < count; i++ {
		value := -2.0 - float64(i%5)*0.05
		class := "cold"
		if i%2 == 1 {
			value = 2.0 + float64(i%5)*0.05
			class = "hot"
		}
		samples = append(samples, Sample{
			SampleID:       "ai_sample",
			LearningStatus: LearningStatusPositive,
			Input: map[string]any{
				"signal":  value,
				"context": float64((i + offset) % 7),
			},
			Results: []ResultLabel{{Key: "class", Value: class}},
		})
	}
	return samples
}

func assertAIXORPrediction(t *testing.T, table *Table, x, y float64, want string) {
	t.Helper()
	prediction, err := table.PredictAI(map[string]any{
		"x": x,
		"y": y,
	})
	if err != nil {
		t.Fatalf("PredictAI: %v", err)
	}
	if len(prediction.Results) != 1 || prediction.Results[0].Value != want {
		t.Fatalf("PredictAI(%g,%g)=%#v, want %q", x, y, prediction.Results, want)
	}
}

func assertAIPrediction(t *testing.T, table *Table, signal float64, want string) {
	t.Helper()
	prediction, err := table.PredictAI(map[string]any{
		"signal":  signal,
		"context": 1.0,
	})
	if err != nil {
		t.Fatalf("PredictAI: %v", err)
	}
	if len(prediction.Results) != 1 {
		t.Fatalf("prediction results=%#v, want one result", prediction.Results)
	}
	if prediction.Results[0].Value != want {
		t.Fatalf("PredictAI(%g)=%q, want %q; prediction=%#v", signal, prediction.Results[0].Value, want, prediction)
	}
}
