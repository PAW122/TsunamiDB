package tests

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
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
		inputCount     = 1000 // amount of different parameters
		resultCount    = 30
		classCount     = 30
		labelNoiseRate = 0.30
		aiLayerEnabled = true
	)

	sampleCount := envInt("TSU_TENSOR_ACCURACY_SAMPLES", 100_000)
	validationCount := envInt("TSU_TENSOR_ACCURACY_VALIDATION", envInt("TSU_TENSOR_ACCURACY_PRETESTS", 1_000))
	testCount := envInt("TSU_TENSOR_ACCURACY_TEST_SAMPLES", validationCount)
	chunkSize := envInt("TSU_TENSOR_ACCURACY_CHUNK", defaultTensorAccuracyChunk(inputCount, resultCount))
	useAILayer := envBool("TSU_TENSOR_AI_LAYER", aiLayerEnabled)

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
	fixture := newSyntheticTensorFixture(inputCount, resultCount, classCount, labelNoiseRate)

	table, err := tensor.CreateTable(schema)
	if err != nil {
		t.Fatalf("CreateTable: %v", err)
	}

	trainingRNG := rand.New(rand.NewSource(122))
	trainingProgress := newTestProgress("training", sampleCount)
	trainingProgress.Update(0)
	totalStarted := time.Now()
	trainingStarted := time.Now()
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
	trainingDuration := time.Since(trainingStarted)

	rebuildProgress := newTestProgress("rebuild", 1)
	rebuildProgress.Update(0)
	rebuildStarted := time.Now()
	if err := tensor.RebuildStats("tensor_accuracy"); err != nil {
		t.Fatalf("RebuildStats: %v", err)
	}
	rebuildProgress.Update(1)
	rebuildProgress.Finish()
	rebuildDuration := time.Since(rebuildStarted)
	table, err = tensor.OpenTable("tensor_accuracy")
	if err != nil {
		t.Fatalf("OpenTable after rebuild: %v", err)
	}

	validationRNG := rand.New(rand.NewSource(987654))
	validationSamples := make([]tensor.Sample, 0, validationCount)
	for i := 0; i < validationCount; i++ {
		validationSamples = append(validationSamples, fixture.sample(validationRNG, sampleCount+i))
	}
	testRNG := rand.New(rand.NewSource(654987))
	testSamples := make([]tensor.Sample, 0, testCount)
	for i := 0; i < testCount; i++ {
		testSamples = append(testSamples, fixture.sample(testRNG, sampleCount+validationCount+i))
	}

	pretestStarted := time.Now()
	beforeTune := evaluateTensorAccuracy(t, table, testSamples, resultCount, "pretest")
	pretestDuration := time.Since(pretestStarted)

	var aiReport tensor.AITrainReport
	var aiDuration time.Duration
	if useAILayer {
		aiTrainCount := envInt("TSU_TENSOR_AI_TRAIN_SAMPLES", minInt(sampleCount, 2_000))
		aiEpochs := envInt("TSU_TENSOR_AI_EPOCHS", 8)
		aiHidden := envInt("TSU_TENSOR_AI_HIDDEN", 64)
		aiRNG := rand.New(rand.NewSource(333777))
		aiTrainingSamples := make([]tensor.Sample, 0, aiTrainCount)
		for i := 0; i < aiTrainCount; i++ {
			aiTrainingSamples = append(aiTrainingSamples, fixture.sample(aiRNG, i))
		}

		var aiProgress *testProgress
		aiOptions := tensor.AITrainOptions{
			Epochs:            aiEpochs,
			BatchSize:         envInt("TSU_TENSOR_AI_BATCH", 32),
			HiddenSizes:       []int{aiHidden},
			LearningRate:      0.03,
			InputDropout:      0.10,
			ValidationSamples: validationSamples,
			Patience:          envInt("TSU_TENSOR_AI_PATIENCE", 4),
			Seed:              9901,
			Progress: func(completed, total int) {
				if aiProgress == nil {
					aiProgress = newTestProgress("ai", total)
				}
				aiProgress.Update(completed)
			},
		}
		aiStarted := time.Now()
		aiReport, err = table.TrainAI(aiTrainingSamples, aiOptions)
		if aiProgress != nil {
			aiProgress.Finish()
		}
		if err != nil {
			t.Fatalf("TrainAI: %v", err)
		}
		aiDuration = time.Since(aiStarted)
		if err := table.FlushAIModel(); err != nil {
			t.Fatalf("FlushAIModel: %v", err)
		}
		table, err = tensor.OpenTable("tensor_accuracy")
		if err != nil {
			t.Fatalf("OpenTable after TrainAI: %v", err)
		}
	}

	verifyStarted := time.Now()
	afterTune := evaluateTensorAccuracy(t, table, testSamples, resultCount, "verify")
	verifyDuration := time.Since(verifyStarted)
	totalDuration := time.Since(totalStarted)

	t.Logf("\n%s", formatTensorAccuracyReport(
		useAILayer, sampleCount, validationCount, testCount, inputCount, resultCount, labelNoiseRate,
		beforeTune.exactAccuracy, beforeTune.labelAccuracy,
		afterTune.exactAccuracy, afterTune.labelAccuracy,
		trainingDuration, rebuildDuration, pretestDuration, aiDuration, verifyDuration, totalDuration,
	))
	if useAILayer {
		t.Logf("\n%s", formatTensorAIReport(aiReport))
	} else {
		t.Log("tensor AI layer disabled")
	}

	exactAccuracyCeiling := math.Pow(0.995, float64(resultCount))
	if afterTune.exactAccuracy > exactAccuracyCeiling {
		t.Fatalf("exact accuracy %.4f is unrealistically high for noisy multi-label predictions with %d labels", afterTune.exactAccuracy, resultCount)
	}
	minLabelAccuracy := minTensorLabelAccuracy(classCount)
	if afterTune.labelAccuracy < minLabelAccuracy {
		t.Fatalf("label accuracy %.4f below expected threshold %.4f", afterTune.labelAccuracy, minLabelAccuracy)
	}
	if afterTune.labelAccuracy > 0.98 {
		t.Fatalf("label accuracy %.4f is unrealistically high for the noisy fixture", afterTune.labelAccuracy)
	}
	if useAILayer && afterTune.labelAccuracy+0.02 < beforeTune.labelAccuracy {
		t.Fatalf("AI label accuracy %.4f regressed too far from tensor baseline %.4f", afterTune.labelAccuracy, beforeTune.labelAccuracy)
	}
}

func minTensorLabelAccuracy(classCount int) float64 {
	if classCount <= 3 {
		return 0.80
	}
	return math.Max(0.50, 0.80*math.Sqrt(3/float64(classCount)))
}

func formatTensorAccuracyReport(
	aiLayer bool,
	sampleCount, validationCount, testCount, inputCount, resultCount int,
	labelNoiseRate, beforeExact, beforeLabel, afterExact, afterLabel float64,
	trainingDuration, rebuildDuration, pretestDuration, aiDuration, verifyDuration, totalDuration time.Duration,
) string {
	return fmt.Sprintf(`Tensor accuracy
  config:   ai_layer=%t samples=%d validation=%d test=%d inputs=%d results=%d label_noise=%.2f
  exact:    before=%.4f after=%.4f
  labels:   before=%.4f after=%.4f delta=%+.4f
  timing:   train=%s rebuild=%s pretest=%s ai=%s verify=%s total=%s`,
		aiLayer, sampleCount, validationCount, testCount, inputCount, resultCount, labelNoiseRate,
		beforeExact, afterExact,
		beforeLabel, afterLabel, afterLabel-beforeLabel,
		roundDuration(trainingDuration),
		roundDuration(rebuildDuration),
		roundDuration(pretestDuration),
		roundDuration(aiDuration),
		roundDuration(verifyDuration),
		roundDuration(totalDuration),
	)
}

func formatTensorAIReport(report tensor.AITrainReport) string {
	return fmt.Sprintf(`Tensor AI
  validation: labels=%.4f exact=%.4f loss=%.4f
  training:   samples=%d validation=%d epochs=%d best_epoch=%d hidden=%v device=%s
  model:      outputs=%d size=%d bytes`,
		report.ValidationLabelAccuracy, report.ValidationExactAccuracy, report.ValidationLoss,
		report.TrainingSamples, report.ValidationSamples, report.Epochs, report.BestEpoch, report.HiddenSizes, report.Device,
		report.OutputClasses, report.ModelSizeBytes,
	)
}

func formatTensorTuneReport(iterations int, before, after float64, corrections, adjustments int, errorsByResult map[string]int) string {
	return fmt.Sprintf(`Tensor tuning
  validation: before=%.4f after=%.4f delta=%+.4f
  work:       iterations=%d corrections=%d adjustments=%d
  top errors: %s`,
		before, after, after-before,
		iterations, corrections, adjustments,
		formatTopResultErrors(errorsByResult, 8),
	)
}

func formatTopResultErrors(errorsByResult map[string]int, limit int) string {
	if len(errorsByResult) == 0 || limit <= 0 {
		return "none"
	}
	type resultError struct {
		key   string
		count int
	}
	items := make([]resultError, 0, len(errorsByResult))
	for key, count := range errorsByResult {
		items = append(items, resultError{key: key, count: count})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].count == items[j].count {
			return items[i].key < items[j].key
		}
		return items[i].count > items[j].count
	})
	if len(items) > limit {
		items = items[:limit]
	}
	parts := make([]string, len(items))
	for i, item := range items {
		parts[i] = fmt.Sprintf("%s=%d", item.key, item.count)
	}
	return strings.Join(parts, ", ")
}

func roundDuration(duration time.Duration) time.Duration {
	return duration.Round(time.Millisecond)
}

type tensorAccuracyResult struct {
	exactAccuracy float64
	labelAccuracy float64
}

func evaluateTensorAccuracy(t *testing.T, table *tensor.Table, samples []tensor.Sample, resultCount int, stage string) tensorAccuracyResult {
	t.Helper()
	if len(samples) == 0 {
		return tensorAccuracyResult{}
	}

	exactMatches := 0
	labelMatches := 0
	totalLabels := len(samples) * resultCount
	progress := newTestProgress(stage, len(samples))
	progress.Update(0)
	for i, sample := range samples {
		prediction, err := table.Predict(sample.Input, resultCount)
		if err != nil {
			t.Fatalf("Predict %s %d: %v", stage, i, err)
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
		progress.Update(i + 1)
	}
	progress.Finish()
	return tensorAccuracyResult{
		exactAccuracy: float64(exactMatches) / float64(len(samples)),
		labelAccuracy: float64(labelMatches) / float64(totalLabels),
	}
}

type syntheticTensorFixture struct {
	inputNames     []string
	inputRanges    []syntheticInputRange
	resultKeys     []string
	resultProfiles []syntheticResultProfile
	classValues    []string
	noiseRate      float64
}

type syntheticInputRange struct {
	base        float64
	scale       float64
	signal      float64
	noise       float64
	ambient     float64
	load        float64
	batch       float64
	curve       float64
	interaction float64
	outlierRate float64
	corruptRate float64
}

type syntheticResultProfile struct {
	bias    float64
	ambient float64
	load    float64
	batch   float64
	noise   float64
}

type syntheticSampleContext struct {
	ambient float64
	load    float64
	batch   float64
}

func newSyntheticTensorFixture(inputCount, resultCount, classCount int, noiseRate float64) syntheticTensorFixture {
	fixture := syntheticTensorFixture{
		inputNames:     make([]string, inputCount),
		inputRanges:    make([]syntheticInputRange, inputCount),
		resultKeys:     make([]string, resultCount),
		resultProfiles: make([]syntheticResultProfile, resultCount),
		classValues:    make([]string, classCount),
		noiseRate:      noiseRate,
	}
	for i := 0; i < inputCount; i++ {
		fixture.inputNames[i] = fmt.Sprintf("p%03d", i)
		fixture.inputRanges[i] = syntheticRangeForInput(i, classCount)
	}
	for i := 0; i < resultCount; i++ {
		fixture.resultKeys[i] = fmt.Sprintf("result_%d", i)
		fixture.resultProfiles[i] = syntheticProfileForResult(i)
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
	context := syntheticContext(rng, id)
	classes := make([]int, resultCount)
	scores := make([]float64, resultCount)
	results := make([]tensor.ResultLabel, resultCount)
	for i := 0; i < resultCount; i++ {
		score := f.resultProfiles[i].score(context, rng)
		classes[i] = syntheticClassFromScore(score, classCount)
		scores[i] = score
		results[i] = tensor.ResultLabel{
			Key:   f.resultKeys[i],
			Value: f.classValues[classes[i]],
		}
	}

	input := make(map[string]any, inputCount)
	for i := 0; i < inputCount; i++ {
		resultID := i % resultCount
		inputRange := f.inputRanges[i]
		signalScore := scores[resultID]
		if rng.Float64() < f.noiseRate*inputRange.corruptRate {
			signalScore = rng.NormFloat64()*2.8 + randomSign(rng)*0.85
		}
		signalClass := syntheticClassFromScore(signalScore, classCount)
		position := syntheticClassPosition(signalClass, classCount)
		scorePosition := math.Tanh(scores[resultID])
		center := inputRange.base +
			inputRange.scale*inputRange.signal*position +
			inputRange.scale*inputRange.curve*(position*position-0.35) +
			inputRange.scale*inputRange.interaction*scorePosition*context.load +
			inputRange.scale*inputRange.ambient*context.ambient +
			inputRange.scale*inputRange.load*context.load +
			inputRange.scale*inputRange.batch*context.batch
		value := center + rng.NormFloat64()*inputRange.noise
		if rng.Float64() < inputRange.outlierRate {
			value += randomSign(rng) * inputRange.scale * (1.5 + rng.Float64()*2.5)
		}
		input[f.inputNames[i]] = value
	}

	return tensor.Sample{
		SampleID:       fmt.Sprintf("synthetic_%06d", id),
		TestStatus:     tensor.TestStatusFail,
		LearningStatus: tensor.LearningStatusPositive,
		Input:          input,
		Results:        results,
	}
}

func syntheticRangeForInput(index, classCount int) syntheticInputRange {
	magnitudes := [...]float64{0.05, 0.2, 1, 3.5, 12, 45, 160, 600}
	magnitude := magnitudes[index%len(magnitudes)]
	direction := 1.0
	if (index/len(magnitudes))%2 == 1 {
		direction = -1
	}
	scale := magnitude * (0.8 + float64((index/11)%7)*0.17)
	classResolutionScale := float64(maxInt(classCount-1, 1)) / 2
	signal := direction * (0.24 + float64((index/5)%9)*0.035) * classResolutionScale
	if index%19 == 0 {
		signal *= 0.25
	}
	if index%23 == 0 {
		signal *= -0.6
	}
	noise := scale * (0.34 + float64((index/17)%5)*0.08)
	corruptRate := 0.18 + float64((index*7)%17)*0.16
	if index%7 == 0 {
		corruptRate += 1.15
		signal *= 1.6
	}
	if index%11 == 0 {
		corruptRate += 0.85
		signal *= 1.35
	}
	return syntheticInputRange{
		base:        float64((index%37)-18)*magnitude*3.7 + float64(index%13)*0.01,
		scale:       scale,
		signal:      signal,
		noise:       noise,
		ambient:     signedSyntheticFactor(index, 3, 0.04, 0.18),
		load:        signedSyntheticFactor(index, 5, 0.03, 0.16),
		batch:       signedSyntheticFactor(index, 7, 0.02, 0.13),
		curve:       signedSyntheticFactor(index, 11, 0.00, 0.12),
		interaction: signedSyntheticFactor(index, 13, 0.00, 0.10),
		outlierRate: 0.003 + float64(index%5)*0.0015,
		corruptRate: corruptRate,
	}
}

func syntheticProfileForResult(index int) syntheticResultProfile {
	return syntheticResultProfile{
		bias:    signedSyntheticFactor(index, 2, 0.00, 0.25),
		ambient: signedSyntheticFactor(index, 3, 0.35, 0.95),
		load:    signedSyntheticFactor(index, 5, 0.25, 0.80),
		batch:   signedSyntheticFactor(index, 7, 0.15, 0.55),
		noise:   0.38 + float64(index%5)*0.05,
	}
}

func (p syntheticResultProfile) score(context syntheticSampleContext, rng *rand.Rand) float64 {
	return p.bias +
		p.ambient*context.ambient +
		p.load*context.load +
		p.batch*context.batch +
		rng.NormFloat64()*p.noise
}

func syntheticContext(rng *rand.Rand, id int) syntheticSampleContext {
	batchWave := math.Sin(float64(id)/97.0) * 0.45
	return syntheticSampleContext{
		ambient: rng.NormFloat64()*0.85 + batchWave,
		load:    rng.Float64()*2 - 1,
		batch:   batchWave + rng.NormFloat64()*0.18,
	}
}

func syntheticClassFromScore(score float64, classCount int) int {
	if classCount <= 1 {
		return 0
	}
	normalized := 0.5 + score/3.6
	if normalized < 0 {
		normalized = 0
	}
	if normalized >= 1 {
		normalized = math.Nextafter(1, 0)
	}
	return int(normalized * float64(classCount))
}

func syntheticClassPosition(class, classCount int) float64 {
	if classCount <= 1 {
		return 0
	}
	return float64(class)*2/float64(classCount-1) - 1
}

func signedSyntheticFactor(index, period int, min, spread float64) float64 {
	sign := 1.0
	if (index/period)%2 == 1 {
		sign = -1
	}
	return sign * (min + float64(index%period)*spread/float64(maxInt(period-1, 1)))
}

func randomSign(rng *rand.Rand) float64 {
	if rng.Intn(2) == 0 {
		return -1
	}
	return 1
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
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

func envBool(key string, fallback bool) bool {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
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
	enabled    bool
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
		enabled:    envBool("TSU_TENSOR_PROGRESS", true),
		updateRate: 200 * time.Millisecond,
	}
}

func (p *testProgress) Update(current int) {
	if !p.enabled {
		return
	}
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
	if !p.enabled {
		return
	}
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
