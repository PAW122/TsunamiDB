package tests

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	dataManager_v2 "github.com/PAW122/TsunamiDB/data/dataManager/v2"
	fileSystem_v1 "github.com/PAW122/TsunamiDB/data/fileSystem/v1"
	"github.com/PAW122/TsunamiDB/data/tensor"
)

func TestTensorAcuricy(t *testing.T) {
	requireTensorAccuracyTest(t)
	if testing.Short() {
		t.Skip("tensor accuracy test generates a large synthetic data set")
	}

	withTempTensorAccuracyDir(t)

	const (
		inputCount  = 100 // amount of different parameters
		resultCount = 3
		classCount  = 3
	)

	sampleCount := envInt("TSU_TENSOR_ACCURACY_SAMPLES", 100_000)
	pretestCount := envInt("TSU_TENSOR_ACCURACY_PRETESTS", 1_000)
	chunkSize := envInt("TSU_TENSOR_ACCURACY_CHUNK", defaultTensorAccuracyChunk(inputCount, resultCount))

	schema := tensor.Schema{
		Name:           "tensor_accuracy",
		IgnoreStatuses: []string{tensor.LearningStatusUnknown, tensor.LearningStatusNegative},
		Inputs:         make([]tensor.InputField, 0, inputCount),
		Results:        make([]tensor.ResultField, 0, resultCount),
	}
	for i := 0; i < inputCount; i++ {
		schema.Inputs = append(schema.Inputs, tensor.InputField{
			Name:      fmt.Sprintf("p%03d", i),
			Type:      tensor.InputTypeFloat64,
			ResultKey: fmt.Sprintf("result_%d", i%resultCount),
		})
	}
	for i := 0; i < resultCount; i++ {
		schema.Results = append(schema.Results, tensor.ResultField{
			Key:   fmt.Sprintf("result_%d", i),
			Type:  tensor.InputTypeString,
			Multi: true,
		})
	}
	fixture := newSyntheticTensorFixture(inputCount, resultCount, classCount)

	table, err := tensor.CreateTable(schema)
	if err != nil {
		t.Fatalf("CreateTable: %v", err)
	}

	trainingRNG := rand.New(rand.NewSource(122))
	trainingProgress := newTestProgress("training", sampleCount)
	trainingProgress.Update(0)
	for written := 0; written < sampleCount; {
		batchCount := chunkSize
		if remaining := sampleCount - written; remaining < batchCount {
			batchCount = remaining
		}
		batch := make([]tensor.Sample, 0, batchCount)
		for i := 0; i < batchCount; i++ {
			batch = append(batch, fixture.sample(trainingRNG, written+i))
		}
		if err := table.AddSamples(batch); err != nil {
			t.Fatalf("AddSamples batch starting at %d: %v", written, err)
		}
		written += batchCount
		trainingProgress.Update(written)
	}
	trainingProgress.Finish()

	rebuildProgress := newTestProgress("rebuild", 1)
	rebuildProgress.Update(0)
	if err := tensor.RebuildStats("tensor_accuracy"); err != nil {
		t.Fatalf("RebuildStats: %v", err)
	}
	rebuildProgress.Update(1)
	rebuildProgress.Finish()
	table, err = tensor.OpenTable("tensor_accuracy")
	if err != nil {
		t.Fatalf("OpenTable after rebuild: %v", err)
	}

	pretestRNG := rand.New(rand.NewSource(987654))
	exactMatches := 0
	labelMatches := 0
	totalLabels := pretestCount * resultCount
	pretestProgress := newTestProgress("pretest", pretestCount)
	pretestProgress.Update(0)
	for i := 0; i < pretestCount; i++ {
		sample := fixture.sample(pretestRNG, sampleCount+i)
		prediction, err := table.Predict(sample.Input, resultCount)
		if err != nil {
			t.Fatalf("Predict pretest %d: %v", i, err)
		}

		want := labelsSet(sample.Results)
		got := labelsSetFromPrediction(prediction.Results)
		matches := 0
		for label := range want {
			if got[label] {
				matches++
			}
		}
		labelMatches += matches
		if matches == len(want) && len(got) == len(want) {
			exactMatches++
		}
		pretestProgress.Update(i + 1)
	}
	pretestProgress.Finish()

	exactAccuracy := float64(exactMatches) / float64(pretestCount)
	labelAccuracy := float64(labelMatches) / float64(totalLabels)
	t.Logf("tensor accuracy samples=%d inputs=%d result_labels=%d pretests=%d exact_accuracy=%.4f label_accuracy=%.4f",
		sampleCount, inputCount, resultCount, pretestCount, exactAccuracy, labelAccuracy)

	if exactAccuracy < 0.95 {
		t.Fatalf("exact accuracy %.4f below expected threshold 0.95", exactAccuracy)
	}
	if labelAccuracy < 0.98 {
		t.Fatalf("label accuracy %.4f below expected threshold 0.98", labelAccuracy)
	}
}

type syntheticTensorFixture struct {
	inputNames  []string
	resultKeys  []string
	classValues []string
}

func newSyntheticTensorFixture(inputCount, resultCount, classCount int) syntheticTensorFixture {
	fixture := syntheticTensorFixture{
		inputNames:  make([]string, inputCount),
		resultKeys:  make([]string, resultCount),
		classValues: make([]string, classCount),
	}
	for i := 0; i < inputCount; i++ {
		fixture.inputNames[i] = fmt.Sprintf("p%03d", i)
	}
	for i := 0; i < resultCount; i++ {
		fixture.resultKeys[i] = fmt.Sprintf("result_%d", i)
	}
	for i := 0; i < classCount; i++ {
		fixture.classValues[i] = fmt.Sprintf("class_%d", i)
	}
	return fixture
}

func (f syntheticTensorFixture) sample(rng *rand.Rand, id int) tensor.Sample {
	inputCount := len(f.inputNames)
	resultCount := len(f.resultKeys)
	classCount := len(f.classValues)
	classes := make([]int, resultCount)
	results := make([]tensor.ResultLabel, resultCount)
	for i := 0; i < resultCount; i++ {
		classes[i] = rng.Intn(classCount)
		results[i] = tensor.ResultLabel{
			Key:   f.resultKeys[i],
			Value: f.classValues[classes[i]],
		}
	}

	input := make(map[string]any, inputCount)
	for i := 0; i < inputCount; i++ {
		resultID := i % resultCount
		center := (float64(classes[resultID]) - 1) * 8
		minorOffset := float64((i%11)-5) * 0.03
		input[f.inputNames[i]] = center + minorOffset + rng.NormFloat64()*0.7
	}

	return tensor.Sample{
		SampleID:       fmt.Sprintf("synthetic_%06d", id),
		TestStatus:     tensor.TestStatusFail,
		LearningStatus: tensor.LearningStatusPositive,
		Input:          input,
		Results:        results,
	}
}

func labelsSet(results []tensor.ResultLabel) map[string]bool {
	set := make(map[string]bool, len(results))
	for _, result := range results {
		set[result.Key+"\x00"+result.Value] = true
	}
	return set
}

func labelsSetFromPrediction(results []tensor.PredictedResult) map[string]bool {
	set := make(map[string]bool, len(results))
	for _, result := range results {
		set[result.Key+"\x00"+result.Value] = true
	}
	return set
}

func envInt(key string, fallback int) int {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func defaultTensorAccuracyChunk(inputCount, resultCount int) int {
	featuresPerSample := inputCount + resultCount
	switch {
	case featuresPerSample >= 10_000:
		return 128
	case featuresPerSample >= 2_000:
		return 64
	default:
		return 1_000
	}
}

func withTempTensorAccuracyDir(t *testing.T) {
	t.Helper()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	keepDir := os.Getenv("TSU_TENSOR_KEEP_DIR") == "1"
	tmp := ""
	if keepDir {
		tmp = filepath.Join(os.TempDir(), "tsunamidb-tensor-accuracy")
		if err := removeAllWithRetry(tmp); err != nil {
			fallback, mkErr := os.MkdirTemp(os.TempDir(), "tsunamidb-tensor-accuracy-")
			if mkErr != nil {
				t.Fatalf("clean kept tensor accuracy dir: %v; create fallback dir: %v", err, mkErr)
			}
			t.Logf("kept tensor accuracy dir is locked, using fallback dir: %s (clean error: %v)", fallback, err)
			tmp = fallback
		}
		if err := os.MkdirAll(tmp, 0o755); err != nil {
			t.Fatalf("create kept tensor accuracy dir: %v", err)
		}
	} else {
		tmp = t.TempDir()
	}

	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir temp: %v", err)
	}
	t.Cleanup(func() {
		dataManager_v2.ShutdownWorkersForTests()
		fileSystem_v1.ShutdownForTests()
		_ = os.Chdir(wd)
		if keepDir {
			t.Logf("tensor accuracy db path: %s", filepath.Join(tmp, "db"))
		}
	})

	if err := os.MkdirAll(filepath.Join(tmp, "db"), 0o755); err != nil {
		t.Fatalf("create temp db dir: %v", err)
	}
}

func removeAllWithRetry(path string) error {
	var err error
	for i := 0; i < 5; i++ {
		err = os.RemoveAll(path)
		if err == nil {
			return nil
		}
		time.Sleep(time.Duration(i+1) * 100 * time.Millisecond)
	}
	return err
}

func requireTensorAccuracyTest(t *testing.T) {
	t.Helper()
	if os.Getenv("TSU_TENSOR_ACCURACY_TEST") != "1" && os.Getenv("TSU_SPECIAL_TESTS") != "1" {
		t.Skip("tensor accuracy test is disabled; set TSU_TENSOR_ACCURACY_TEST=1 to run it")
	}
}

type testProgress struct {
	stage      string
	total      int
	lastPrint  time.Time
	lastValue  int
	lastWidth  int
	updateRate time.Duration
}

func newTestProgress(stage string, total int) *testProgress {
	if total <= 0 {
		total = 1
	}
	return &testProgress{
		stage:      stage,
		total:      total,
		updateRate: 200 * time.Millisecond,
	}
}

func (p *testProgress) Update(current int) {
	if current < 0 {
		current = 0
	}
	if current > p.total {
		current = p.total
	}

	now := time.Now()
	if current != 0 && current != p.total && current == p.lastValue {
		return
	}
	if current != 0 && current != p.total && now.Sub(p.lastPrint) < p.updateRate {
		return
	}

	p.lastPrint = now
	p.lastValue = current
	p.print(current, false)
}

func (p *testProgress) Finish() {
	if p.lastValue == p.total {
		fmt.Fprintln(os.Stderr)
		return
	}
	p.print(p.total, true)
}

func (p *testProgress) print(current int, finish bool) {
	const barWidth = 28

	filled := current * barWidth / p.total
	bar := make([]byte, barWidth)
	for i := range bar {
		if i < filled {
			bar[i] = '#'
		} else {
			bar[i] = '-'
		}
	}

	percent := current * 100 / p.total
	line := fmt.Sprintf("%-8s [%s] %3d%% (%d/%d)", p.stage, string(bar), percent, current, p.total)
	padding := ""
	if p.lastWidth > len(line) {
		padding = fmt.Sprintf("%*s", p.lastWidth-len(line), "")
	}
	p.lastWidth = len(line)

	if finish {
		fmt.Fprintf(os.Stderr, "\r%s%s\n", line, padding)
		return
	}
	fmt.Fprintf(os.Stderr, "\r%s%s", line, padding)
}
