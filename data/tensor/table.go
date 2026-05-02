package tensor

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"runtime"
	"sort"
	"sync"

	dataManager_v2 "github.com/PAW122/TsunamiDB/data/dataManager/v2"
)

type Table struct {
	schema             Schema
	stats              *statsSnapshot
	files              tableFiles
	globalInputs       []int
	inputsByResult     map[string][]int
	resultIndexByKey   map[string]int
	inputResultIndex   []int
	inputDensePosition []int
	inputSignalWeights []float64
	allFloatInputs     bool
	aiModel            *AIModel
}

func (t *Table) Schema() Schema {
	return t.schema
}

func (t *Table) FlushStats() error {
	return writeStatsSnapshot(t.files.stats, t.schema, t.stats)
}

func (t *Table) FlushAIModel() error {
	if t == nil || t.aiModel == nil {
		return ErrNoModelData
	}
	return writeKVJSON(t.files.aiModel, t.aiModel)
}

func newTable(schema Schema, stats *statsSnapshot, files tableFiles) *Table {
	t := &Table{
		schema:             schema,
		stats:              stats,
		files:              files,
		inputsByResult:     make(map[string][]int),
		resultIndexByKey:   make(map[string]int, len(schema.Results)),
		inputResultIndex:   make([]int, len(schema.Inputs)),
		inputDensePosition: make([]int, len(schema.Inputs)),
	}
	for i := range t.inputResultIndex {
		t.inputResultIndex[i] = -1
		t.inputDensePosition[i] = -1
	}
	for i, result := range schema.Results {
		t.resultIndexByKey[result.Key] = i
	}
	t.allFloatInputs = true
	for i, field := range schema.Inputs {
		if field.Type != InputTypeFloat64 {
			t.allFloatInputs = false
		}
		if field.ResultKey == "" {
			t.globalInputs = append(t.globalInputs, i)
			continue
		}
		if resultIndex, ok := t.resultIndexByKey[field.ResultKey]; ok {
			t.inputResultIndex[i] = resultIndex
			t.inputDensePosition[i] = len(t.inputsByResult[field.ResultKey])
		}
		t.inputsByResult[field.ResultKey] = append(t.inputsByResult[field.ResultKey], i)
	}
	t.prepareStatsLayout()
	t.prepareInputSignalWeights()
	return t
}

func (t *Table) prepareStatsLayout() {
	if t == nil || t.stats == nil {
		return
	}
	for _, label := range t.stats.LabelStats {
		label.prepareDense(t.inputIndexesForResult(label.Key))
	}
}

func (t *Table) prepareInputSignalWeights() {
	if t == nil || t.stats == nil || len(t.schema.Inputs) == 0 {
		return
	}
	t.inputSignalWeights = make([]float64, len(t.schema.Inputs))
	for i := range t.inputSignalWeights {
		t.inputSignalWeights[i] = 1
	}
	if len(t.stats.LabelStats) == 0 {
		return
	}
	for _, result := range t.schema.Results {
		for _, inputIndex := range t.inputIndexesForResult(result.Key) {
			if inputIndex < 0 || inputIndex >= len(t.inputSignalWeights) {
				continue
			}
			t.inputSignalWeights[inputIndex] = t.numericInputSignalWeight(result.Key, inputIndex)
		}
	}
}

func (t *Table) numericInputSignalWeight(resultKey string, inputIndex int) float64 {
	total := 0.0
	weightedMean := 0.0
	labels := 0
	for _, label := range t.stats.LabelStats {
		if label == nil || label.Key != resultKey {
			continue
		}
		stat := label.numericAt(label.densePosition(-1, inputIndex), inputIndex)
		if stat == nil || stat.Count == 0 {
			continue
		}
		count := float64(stat.Count)
		total += count
		weightedMean += count * stat.Mean
		labels++
	}
	if labels < 2 || total <= 0 {
		return 1
	}
	weightedMean /= total

	within := 0.0
	between := 0.0
	for _, label := range t.stats.LabelStats {
		if label == nil || label.Key != resultKey {
			continue
		}
		stat := label.numericAt(label.densePosition(-1, inputIndex), inputIndex)
		if stat == nil || stat.Count == 0 {
			continue
		}
		count := float64(stat.Count)
		within += count * stat.variance()
		diff := stat.Mean - weightedMean
		between += count * diff * diff
	}
	within /= total
	between /= total
	if within < 0 || between < 0 || math.IsNaN(within) || math.IsNaN(between) || math.IsInf(within, 0) || math.IsInf(between, 0) {
		return 1
	}
	signal := between / (between + within + 1e-12)
	if signal < 0 {
		signal = 0
	}
	if signal > 1 {
		signal = 1
	}
	weight := 0.10 + 1.90*math.Sqrt(signal)
	if weight < 0.05 {
		return 0.05
	}
	if weight > 2 {
		return 2
	}
	return weight
}

func (t *Table) effectiveInputWeight(label *labelStats, index int) float64 {
	weight := t.stats.effectiveInputWeight(label, index)
	if index >= 0 && index < len(t.inputSignalWeights) {
		weight *= t.inputSignalWeights[index]
	}
	return weight
}

func (t *Table) inputIndexesForResult(key string) []int {
	if t.inputsByResult == nil && t.globalInputs == nil {
		rebuilt := newTable(t.schema, t.stats, t.files)
		t.globalInputs = rebuilt.globalInputs
		t.inputsByResult = rebuilt.inputsByResult
		t.resultIndexByKey = rebuilt.resultIndexByKey
		t.inputResultIndex = rebuilt.inputResultIndex
		t.inputDensePosition = rebuilt.inputDensePosition
		t.inputSignalWeights = rebuilt.inputSignalWeights
		t.allFloatInputs = rebuilt.allFloatInputs
	}
	specific := t.inputsByResult[key]
	if len(t.globalInputs) == 0 {
		return specific
	}
	if len(specific) == 0 {
		return t.globalInputs
	}
	indexes := make([]int, 0, len(t.globalInputs)+len(specific))
	indexes = append(indexes, t.globalInputs...)
	indexes = append(indexes, specific...)
	return indexes
}

func (t *Table) addStats(input normalizedInput, results []ResultLabel) {
	if len(results) == 0 {
		return
	}
	if len(t.globalInputs) == 0 && len(t.resultIndexByKey) != 0 {
		if t.addStatsByResultInputs(input, results) {
			return
		}
	}
	t.stats.add(input, results, t.inputIndexesForResult)
}

func (t *Table) addStatsByResultInputs(input normalizedInput, results []ResultLabel) bool {
	resultCount := len(t.schema.Results)
	if resultCount == 0 {
		return false
	}

	var fixedSeen [512]bool
	seen := fixedSeen[:]
	if resultCount > len(fixedSeen) {
		seen = make([]bool, resultCount)
	}
	for _, result := range results {
		idx, ok := t.resultIndexByKey[result.Key]
		if !ok || idx < 0 || idx >= resultCount || seen[idx] {
			return false
		}
		seen[idx] = true
	}

	var fixedLabels [512]*labelStats
	labelsByResult := fixedLabels[:]
	if resultCount > len(fixedLabels) {
		labelsByResult = make([]*labelStats, resultCount)
	}
	labelsByResult = labelsByResult[:resultCount]

	t.stats.TotalCount++
	for _, result := range results {
		id := labelID(result)
		label := t.stats.LabelStats[id]
		if label == nil {
			label = newLabelStats(result.Key, result.Value, t.inputIndexesForResult(result.Key))
			t.stats.LabelStats[id] = label
		}
		label.Count++
		labelsByResult[t.resultIndexByKey[result.Key]] = label
	}

	for idx, value := range input {
		if idx < 0 || idx >= len(t.inputResultIndex) {
			continue
		}
		resultIndex := t.inputResultIndex[idx]
		if resultIndex < 0 || resultIndex >= len(labelsByResult) {
			continue
		}
		label := labelsByResult[resultIndex]
		if label == nil {
			continue
		}
		position := t.inputDensePosition[idx]
		if numeric, ok := numericValue(value); ok {
			label.addNumericAt(position, idx, numeric)
			continue
		}
		label.addCategoryAt(position, idx, categoryValue(value))
	}
	return true
}

func (t *Table) addFloatStats(input []float64, results []ResultLabel) {
	if len(results) == 0 {
		return
	}
	if len(t.globalInputs) == 0 && len(t.resultIndexByKey) != 0 {
		if t.addFloatStatsByResultInputs(input, results) {
			return
		}
	}
	normalized := make(normalizedInput, len(input))
	for i, value := range input {
		normalized[i] = value
	}
	t.stats.add(normalized, results, t.inputIndexesForResult)
}

func (t *Table) addFloatStatsByResultInputs(input []float64, results []ResultLabel) bool {
	resultCount := len(t.schema.Results)
	if resultCount == 0 {
		return false
	}

	var fixedSeen [512]bool
	seen := fixedSeen[:]
	if resultCount > len(fixedSeen) {
		seen = make([]bool, resultCount)
	}
	for _, result := range results {
		idx, ok := t.resultIndexByKey[result.Key]
		if !ok || idx < 0 || idx >= resultCount || seen[idx] {
			return false
		}
		seen[idx] = true
	}

	var fixedLabels [512]*labelStats
	labelsByResult := fixedLabels[:]
	if resultCount > len(fixedLabels) {
		labelsByResult = make([]*labelStats, resultCount)
	}
	labelsByResult = labelsByResult[:resultCount]

	t.stats.TotalCount++
	for _, result := range results {
		id := labelID(result)
		label := t.stats.LabelStats[id]
		if label == nil {
			label = newLabelStats(result.Key, result.Value, t.inputIndexesForResult(result.Key))
			t.stats.LabelStats[id] = label
		}
		label.Count++
		labelsByResult[t.resultIndexByKey[result.Key]] = label
	}

	for idx, value := range input {
		if idx < 0 || idx >= len(t.inputResultIndex) {
			continue
		}
		resultIndex := t.inputResultIndex[idx]
		if resultIndex < 0 || resultIndex >= len(labelsByResult) {
			continue
		}
		label := labelsByResult[resultIndex]
		if label == nil {
			continue
		}
		label.addNumericAt(t.inputDensePosition[idx], idx, value)
	}
	return true
}

func (t *Table) AddSample(sample Sample) error {
	input, results, err := t.validateSample(sample)
	if err != nil {
		return err
	}

	frame, err := encodeSampleFrame(t.schema, sample, input)
	if err != nil {
		return err
	}
	startPtr, endPtr, err := dataManager_v2.SaveDataAppendToFileAsync(frame, t.files.sampleData)
	if err != nil {
		return err
	}
	entry, err := encodeSampleManifestEntry(startPtr, endPtr)
	if err != nil {
		return err
	}
	if _, err := dataManager_v2.SaveIncDataToFileAsync(entry, t.files.samples, t.files.sampleEntrySize); err != nil {
		return err
	}

	if t.shouldLearn(sample.LearningStatus) {
		t.addStats(input, results)
	}
	return nil
}

func (t *Table) AddSamples(samples []Sample) error {
	if len(samples) == 0 {
		return nil
	}
	if t.allFloatInputs {
		return t.addFloatSamples(samples)
	}

	type learnedSample struct {
		input   normalizedInput
		results []ResultLabel
	}
	type preparedSample struct {
		input   normalizedInput
		results []ResultLabel
		frame   []byte
		learn   bool
	}
	prepared := make([]preparedSample, len(samples))
	if err := t.parallelForErr(len(samples), func(start, end int) error {
		for i := start; i < end; i++ {
			sample := samples[i]
			input, results, err := t.validateSample(sample)
			if err != nil {
				return err
			}
			frame, err := encodeSampleFrame(t.schema, sample, input)
			if err != nil {
				return err
			}
			prepared[i] = preparedSample{
				input:   input,
				results: results,
				frame:   frame,
				learn:   t.shouldLearn(sample.LearningStatus),
			}
		}
		return nil
	}); err != nil {
		return err
	}

	var payload bytes.Buffer
	frameSizes := make([]int, 0, len(samples))
	learned := make([]learnedSample, 0, len(samples))
	totalSize := 0
	for _, sample := range prepared {
		totalSize += len(sample.frame)
	}
	payload.Grow(totalSize)
	for _, sample := range prepared {
		if sample.learn {
			learned = append(learned, learnedSample{input: sample.input, results: sample.results})
		}
		frameSizes = append(frameSizes, len(sample.frame))
		payload.Write(sample.frame)
	}

	startPtr, _, err := dataManager_v2.SaveDataAppendToFileAsync(payload.Bytes(), t.files.sampleData)
	if err != nil {
		return err
	}

	entries := make([][]byte, 0, len(frameSizes))
	offset := startPtr
	for _, size := range frameSizes {
		endPtr := offset + int64(size)
		entry, err := encodeSampleManifestEntry(offset, endPtr)
		if err != nil {
			return err
		}
		entries = append(entries, entry)
		offset = endPtr
	}
	if _, err := dataManager_v2.SaveIncDataBatchToFileAsync(entries, t.files.samples, t.files.sampleEntrySize); err != nil {
		return err
	}

	for _, sample := range learned {
		t.addStats(sample.input, sample.results)
	}
	return nil
}

func (t *Table) addFloatSamples(samples []Sample) error {
	type learnedSample struct {
		input   []float64
		results []ResultLabel
	}
	type preparedSample struct {
		input   []float64
		results []ResultLabel
		frame   []byte
		learn   bool
	}
	prepared := make([]preparedSample, len(samples))
	if err := t.parallelForErr(len(samples), func(start, end int) error {
		for i := start; i < end; i++ {
			sample := samples[i]
			input, results, err := t.validateFloatSample(sample)
			if err != nil {
				return err
			}
			frame, err := encodeFloatSampleFrame(t.schema, sample, input, t.resultIndexByKey)
			if err != nil {
				return err
			}
			prepared[i] = preparedSample{
				input:   input,
				results: results,
				frame:   frame,
				learn:   t.shouldLearn(sample.LearningStatus),
			}
		}
		return nil
	}); err != nil {
		return err
	}

	var payload bytes.Buffer
	frameSizes := make([]int, 0, len(samples))
	learned := make([]learnedSample, 0, len(samples))
	totalSize := 0
	for _, sample := range prepared {
		totalSize += len(sample.frame)
	}
	payload.Grow(totalSize)
	for _, sample := range prepared {
		if sample.learn {
			learned = append(learned, learnedSample{input: sample.input, results: sample.results})
		}
		frameSizes = append(frameSizes, len(sample.frame))
		payload.Write(sample.frame)
	}

	startPtr, _, err := dataManager_v2.SaveDataAppendToFileAsync(payload.Bytes(), t.files.sampleData)
	if err != nil {
		return err
	}

	entries := make([][]byte, 0, len(frameSizes))
	offset := startPtr
	for _, size := range frameSizes {
		endPtr := offset + int64(size)
		entry, err := encodeSampleManifestEntry(offset, endPtr)
		if err != nil {
			return err
		}
		entries = append(entries, entry)
		offset = endPtr
	}
	if _, err := dataManager_v2.SaveIncDataBatchToFileAsync(entries, t.files.samples, t.files.sampleEntrySize); err != nil {
		return err
	}

	for _, sample := range learned {
		t.addFloatStats(sample.input, sample.results)
	}
	return nil
}

func (t *Table) Predict(input map[string]any, topN int) (Prediction, error) {
	if t.aiModel != nil {
		return t.PredictAI(input)
	}
	if t.allFloatInputs {
		normalized, err := t.validateFloatInput(input)
		if err != nil {
			return Prediction{}, err
		}
		if topN <= 0 {
			topN = 10
		}
		if len(t.stats.LabelStats) == 0 {
			return Prediction{}, ErrNoModelData
		}
		if topN >= len(t.schema.Results) && len(t.schema.Results) > 0 {
			return t.predictFloatBestPerResult(normalized)
		}
		return t.predictFloatTopN(normalized, topN)
	}

	normalized, err := t.validateInput(input)
	if err != nil {
		return Prediction{}, err
	}
	if topN <= 0 {
		topN = 10
	}
	if len(t.stats.LabelStats) == 0 {
		return Prediction{}, ErrNoModelData
	}
	if topN >= len(t.schema.Results) && len(t.schema.Results) > 0 {
		return t.predictBestPerResult(normalized)
	}

	type scoredLabel struct {
		label  *labelStats
		result PredictedResult
	}
	labels := t.snapshotLabels()
	scored := make([]scoredLabel, len(labels))
	t.parallelFor(len(labels), func(start, end int) {
		for i := start; i < end; i++ {
			label := labels[i]
			scored[i] = scoredLabel{
				label: label,
				result: PredictedResult{
					Key:     label.Key,
					Value:   label.Value,
					Score:   t.scoreLabelOnly(normalized, label),
					Samples: label.Count,
				},
			}
		}
	})

	sort.Slice(scored, func(i, j int) bool {
		if scored[i].result.Score == scored[j].result.Score {
			if scored[i].result.Key == scored[j].result.Key {
				return scored[i].result.Value < scored[j].result.Value
			}
			return scored[i].result.Key < scored[j].result.Key
		}
		return scored[i].result.Score > scored[j].result.Score
	})

	if len(scored) > topN {
		scored = scored[:topN]
	}
	results := make([]PredictedResult, len(scored))
	for i := range scored {
		results[i] = t.scoreLabel(normalized, scored[i].label)
	}
	normalizeProbabilities(results)
	return Prediction{Results: results}, nil
}

func (t *Table) predictFloatTopN(input []float64, topN int) (Prediction, error) {
	type scoredLabel struct {
		label  *labelStats
		result PredictedResult
	}
	labels := t.snapshotLabels()
	scored := make([]scoredLabel, len(labels))
	t.parallelFor(len(labels), func(start, end int) {
		for i := start; i < end; i++ {
			label := labels[i]
			scored[i] = scoredLabel{
				label: label,
				result: PredictedResult{
					Key:     label.Key,
					Value:   label.Value,
					Score:   t.scoreFloatLabelOnly(input, label),
					Samples: label.Count,
				},
			}
		}
	})
	sort.Slice(scored, func(i, j int) bool {
		if scored[i].result.Score == scored[j].result.Score {
			if scored[i].result.Key == scored[j].result.Key {
				return scored[i].result.Value < scored[j].result.Value
			}
			return scored[i].result.Key < scored[j].result.Key
		}
		return scored[i].result.Score > scored[j].result.Score
	})
	if len(scored) > topN {
		scored = scored[:topN]
	}
	results := make([]PredictedResult, len(scored))
	for i := range scored {
		results[i] = t.scoreFloatLabel(input, scored[i].label)
	}
	normalizeProbabilities(results)
	return Prediction{Results: results}, nil
}

func (t *Table) predictFloatBestPerResult(input []float64) (Prediction, error) {
	resultCount := len(t.schema.Results)
	bestByResult := make([]*labelStats, resultCount)
	bestScores := newNegativeInfSlice(resultCount)
	labels := t.snapshotLabels()
	workers := t.workerCount(len(labels))

	if workers == 1 {
		for _, label := range labels {
			resultIndex, ok := t.resultIndexByKey[label.Key]
			if !ok || resultIndex < 0 || resultIndex >= len(bestByResult) {
				continue
			}
			score := t.scoreFloatLabelOnly(input, label)
			if betterLabel(label, score, bestByResult[resultIndex], bestScores[resultIndex]) {
				bestByResult[resultIndex] = label
				bestScores[resultIndex] = score
			}
		}
	} else {
		localBest := make([][]*labelStats, workers)
		localScores := make([][]float64, workers)
		var wg sync.WaitGroup
		for worker := 0; worker < workers; worker++ {
			start, end := workerRange(len(labels), workers, worker)
			localBest[worker] = make([]*labelStats, resultCount)
			localScores[worker] = newNegativeInfSlice(resultCount)
			wg.Add(1)
			go func(worker, start, end int) {
				defer wg.Done()
				best := localBest[worker]
				scores := localScores[worker]
				for i := start; i < end; i++ {
					label := labels[i]
					resultIndex, ok := t.resultIndexByKey[label.Key]
					if !ok || resultIndex < 0 || resultIndex >= len(best) {
						continue
					}
					score := t.scoreFloatLabelOnly(input, label)
					if betterLabel(label, score, best[resultIndex], scores[resultIndex]) {
						best[resultIndex] = label
						scores[resultIndex] = score
					}
				}
			}(worker, start, end)
		}
		wg.Wait()
		for worker := 0; worker < workers; worker++ {
			for resultIndex, label := range localBest[worker] {
				if label == nil {
					continue
				}
				score := localScores[worker][resultIndex]
				if betterLabel(label, score, bestByResult[resultIndex], bestScores[resultIndex]) {
					bestByResult[resultIndex] = label
					bestScores[resultIndex] = score
				}
			}
		}
	}

	results := make([]PredictedResult, 0, len(bestByResult))
	for _, label := range bestByResult {
		if label != nil {
			results = append(results, t.scoreFloatLabel(input, label))
		}
	}
	normalizeProbabilities(results)
	return Prediction{Results: results}, nil
}

func (t *Table) predictBestPerResult(input normalizedInput) (Prediction, error) {
	resultCount := len(t.schema.Results)
	bestByResult := make([]*labelStats, resultCount)
	bestScores := newNegativeInfSlice(resultCount)
	labels := t.snapshotLabels()
	workers := t.workerCount(len(labels))

	if workers == 1 {
		for _, label := range labels {
			resultIndex, ok := t.resultIndexByKey[label.Key]
			if !ok || resultIndex < 0 || resultIndex >= len(bestByResult) {
				continue
			}
			score := t.scoreLabelOnly(input, label)
			if betterLabel(label, score, bestByResult[resultIndex], bestScores[resultIndex]) {
				bestByResult[resultIndex] = label
				bestScores[resultIndex] = score
			}
		}
	} else {
		localBest := make([][]*labelStats, workers)
		localScores := make([][]float64, workers)
		var wg sync.WaitGroup
		for worker := 0; worker < workers; worker++ {
			start, end := workerRange(len(labels), workers, worker)
			localBest[worker] = make([]*labelStats, resultCount)
			localScores[worker] = newNegativeInfSlice(resultCount)
			wg.Add(1)
			go func(worker, start, end int) {
				defer wg.Done()
				best := localBest[worker]
				scores := localScores[worker]
				for i := start; i < end; i++ {
					label := labels[i]
					resultIndex, ok := t.resultIndexByKey[label.Key]
					if !ok || resultIndex < 0 || resultIndex >= len(best) {
						continue
					}
					score := t.scoreLabelOnly(input, label)
					if betterLabel(label, score, best[resultIndex], scores[resultIndex]) {
						best[resultIndex] = label
						scores[resultIndex] = score
					}
				}
			}(worker, start, end)
		}
		wg.Wait()
		for worker := 0; worker < workers; worker++ {
			for resultIndex, label := range localBest[worker] {
				if label == nil {
					continue
				}
				score := localScores[worker][resultIndex]
				if betterLabel(label, score, bestByResult[resultIndex], bestScores[resultIndex]) {
					bestByResult[resultIndex] = label
					bestScores[resultIndex] = score
				}
			}
		}
	}

	results := make([]PredictedResult, 0, len(bestByResult))
	for _, label := range bestByResult {
		if label != nil {
			results = append(results, t.scoreLabel(input, label))
		}
	}
	normalizeProbabilities(results)
	return Prediction{Results: results}, nil
}

func (t *Table) scoreFloatLabelOnly(input []float64, label *labelStats) float64 {
	denominator := float64(maxUint64(t.stats.TotalCount, 1))
	score := math.Log((float64(label.Count) + 1) / (denominator + float64(len(t.stats.LabelStats))))
	score += t.stats.labelBias(label.Key, label.Value)
	indexes := t.inputIndexesForResult(label.Key)
	if len(label.inputIndexes) != 0 {
		indexes = label.inputIndexes
	}

	for pos, idx := range indexes {
		if idx < 0 || idx >= len(input) || idx >= len(t.schema.Inputs) {
			continue
		}
		stat := label.numericAt(pos, idx)
		if stat == nil || stat.Count == 0 {
			continue
		}
		contribution := numericLogContribution(input[idx], stat)
		score += t.effectiveInputWeight(label, idx) * contribution
	}
	return score
}

func (t *Table) scoreLabelOnly(input normalizedInput, label *labelStats) float64 {
	denominator := float64(maxUint64(t.stats.TotalCount, 1))
	score := math.Log((float64(label.Count) + 1) / (denominator + float64(len(t.stats.LabelStats))))
	score += t.stats.labelBias(label.Key, label.Value)
	indexes := t.inputIndexesForResult(label.Key)
	if len(label.inputIndexes) != 0 {
		indexes = label.inputIndexes
	}

	for pos, idx := range indexes {
		if idx < 0 || idx >= len(input) || idx >= len(t.schema.Inputs) {
			continue
		}
		value := input[idx]
		if numeric, ok := numericValue(value); ok {
			stat := label.numericAt(pos, idx)
			if stat == nil || stat.Count == 0 {
				continue
			}
			contribution := numericLogContribution(numeric, stat)
			score += t.effectiveInputWeight(label, idx) * contribution
			continue
		}

		stat := label.categoryAt(pos, idx)
		if stat == nil || stat.Count == 0 {
			continue
		}
		category := categoryValue(value)
		frequency := float64(stat.Values[category]+1) / float64(stat.Count+uint64(len(stat.Values))+1)
		score += t.effectiveInputWeight(label, idx) * math.Log(frequency)
	}
	return score
}

func (t *Table) scoreFloatLabel(input []float64, label *labelStats) PredictedResult {
	denominator := float64(maxUint64(t.stats.TotalCount, 1))
	score := math.Log((float64(label.Count) + 1) / (denominator + float64(len(t.stats.LabelStats))))
	score += t.stats.labelBias(label.Key, label.Value)
	indexes := t.inputIndexesForResult(label.Key)
	if len(label.inputIndexes) != 0 {
		indexes = label.inputIndexes
	}
	influences := make([]Influence, 0, len(indexes))

	for pos, idx := range indexes {
		if idx < 0 || idx >= len(input) || idx >= len(t.schema.Inputs) {
			continue
		}
		field := t.schema.Inputs[idx]
		stat := label.numericAt(pos, idx)
		if stat == nil || stat.Count == 0 {
			continue
		}
		weight := t.effectiveInputWeight(label, idx)
		impact := numericImpact(input[idx], stat)
		contribution := numericLogContribution(input[idx], stat)
		score += weight * contribution
		influences = append(influences, Influence{
			Input:  field.Name,
			Impact: impact * weight,
			Reason: fmt.Sprintf("numeric distance from mean %.4f, weight %.3f", stat.Mean, weight),
		})
	}

	sort.Slice(influences, func(i, j int) bool {
		if influences[i].Impact == influences[j].Impact {
			return influences[i].Input < influences[j].Input
		}
		return influences[i].Impact > influences[j].Impact
	})
	if len(influences) > 5 {
		influences = influences[:5]
	}

	return PredictedResult{
		Key:        label.Key,
		Value:      label.Value,
		Score:      score,
		Samples:    label.Count,
		Influences: influences,
	}
}

func (t *Table) scoreLabel(input normalizedInput, label *labelStats) PredictedResult {
	denominator := float64(maxUint64(t.stats.TotalCount, 1))
	score := math.Log((float64(label.Count) + 1) / (denominator + float64(len(t.stats.LabelStats))))
	score += t.stats.labelBias(label.Key, label.Value)
	indexes := t.inputIndexesForResult(label.Key)
	influences := make([]Influence, 0, len(indexes))

	if len(label.inputIndexes) != 0 {
		indexes = label.inputIndexes
	}
	for pos, idx := range indexes {
		if idx < 0 || idx >= len(input) || idx >= len(t.schema.Inputs) {
			continue
		}
		field := t.schema.Inputs[idx]
		value := input[idx]
		if numeric, ok := numericValue(value); ok {
			stat := label.numericAt(pos, idx)
			if stat == nil || stat.Count == 0 {
				continue
			}
			weight := t.effectiveInputWeight(label, idx)
			impact := numericImpact(numeric, stat)
			contribution := numericLogContribution(numeric, stat)
			contribution *= weight
			score += contribution
			influences = append(influences, Influence{
				Input:  field.Name,
				Impact: impact * weight,
				Reason: fmt.Sprintf("numeric distance from mean %.4f, weight %.3f", stat.Mean, weight),
			})
			continue
		}

		stat := label.categoryAt(pos, idx)
		if stat == nil || stat.Count == 0 {
			continue
		}
		category := categoryValue(value)
		frequency := float64(stat.Values[category]+1) / float64(stat.Count+uint64(len(stat.Values))+1)
		weight := t.effectiveInputWeight(label, idx)
		score += weight * math.Log(frequency)
		influences = append(influences, Influence{
			Input:  field.Name,
			Impact: frequency * weight,
			Reason: fmt.Sprintf("categorical frequency matched learned cases, weight %.3f", weight),
		})
	}

	sort.Slice(influences, func(i, j int) bool {
		if influences[i].Impact == influences[j].Impact {
			return influences[i].Input < influences[j].Input
		}
		return influences[i].Impact > influences[j].Impact
	})
	if len(influences) > 5 {
		influences = influences[:5]
	}

	return PredictedResult{
		Key:        label.Key,
		Value:      label.Value,
		Score:      score,
		Samples:    label.Count,
		Influences: influences,
	}
}

func RebuildStats(table string) error {
	t, err := OpenTable(table)
	if err != nil {
		return err
	}
	t.stats.reset(t.schema)

	if err := t.rebuildStatsFromIncTable(); err == nil {
		return t.FlushStats()
	} else if !errors.Is(err, ErrTableNotFound) {
		return err
	}

	file, err := openLegacySamples(t.files.legacySamples)
	if err != nil {
		if errors.Is(err, ErrTableNotFound) {
			return nil
		}
		return err
	}
	defer file.Close()

	reader := bufio.NewReaderSize(file, 4<<20)
	if first, err := reader.Peek(1); err == nil && len(first) > 0 && first[0] == '{' {
		return t.rebuildStatsFromJSONLines(reader)
	} else if err != nil && !errors.Is(err, io.EOF) {
		return err
	}

	for {
		frame, err := readSampleFrame(reader)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		if t.allFloatInputs {
			learningStatus, input, results, err := decodeFloatSampleFrameForStats(t.schema, frame)
			if err != nil {
				return err
			}
			if t.shouldLearn(learningStatus) {
				t.addFloatStats(input, results)
			}
		} else {
			learningStatus, input, results, err := decodeSampleFrameForStats(t.schema, frame)
			if err != nil {
				return err
			}
			if t.shouldLearn(learningStatus) {
				t.addStats(input, results)
			}
		}
	}
	return t.FlushStats()
}

func (t *Table) rebuildStatsFromJSONLines(reader io.Reader) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		var sample Sample
		if err := json.Unmarshal(scanner.Bytes(), &sample); err != nil {
			return err
		}
		input, results, err := t.validateSample(sample)
		if err != nil {
			return err
		}
		if t.shouldLearn(sample.LearningStatus) {
			t.addStats(input, results)
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return t.FlushStats()
}

func (t *Table) rebuildStatsFromIncTable() error {
	count, err := dataManager_v2.GetIncRecordCount(t.files.samples, t.files.sampleEntrySize)
	if err != nil {
		return ErrTableNotFound
	}
	if count == 0 {
		return nil
	}

	recordSize := int(t.files.sampleEntrySize) + 3
	chunkRecords := rebuildChunkRecords(t.files.sampleEntrySize)
	for start := uint64(0); start < count; start += chunkRecords {
		amount := chunkRecords
		if remaining := count - start; remaining < amount {
			amount = remaining
		}
		raw, err := dataManager_v2.ReadIncDataFromFileAsync_Range(t.files.samples, start, amount, t.files.sampleEntrySize)
		if err != nil {
			return err
		}
		if len(raw)%recordSize != 0 {
			return errors.New("tensor: corrupted inc table sample log")
		}
		spans := make([]sampleSpan, 0, len(raw)/recordSize)
		for offset := 0; offset < len(raw); offset += recordSize {
			startPtr, endPtr, ok, err := decodeSampleManifestEntry(raw[offset : offset+recordSize])
			if err != nil {
				return err
			}
			if !ok {
				continue
			}
			spans = append(spans, sampleSpan{start: startPtr, end: endPtr})
		}
		if err := t.rebuildStatsFromSpans(spans); err != nil {
			return err
		}
	}
	return nil
}

type sampleSpan struct {
	start int64
	end   int64
}

func (t *Table) rebuildStatsFromSpans(spans []sampleSpan) error {
	workers := t.workerCount(len(spans))
	if workers == 1 {
		return t.rebuildStatsFromSpansSerial(spans)
	}

	localStats := make([]*statsSnapshot, workers)
	var (
		wg      sync.WaitGroup
		errOnce sync.Once
		first   error
	)
	for worker := 0; worker < workers; worker++ {
		start, end := workerRange(len(spans), workers, worker)
		local := newTable(t.schema, newStats(t.schema), t.files)
		localStats[worker] = local.stats

		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := local.rebuildStatsFromSpansSerial(spans[start:end]); err != nil {
				errOnce.Do(func() {
					first = err
				})
			}
		}()
	}
	wg.Wait()
	if first != nil {
		return first
	}
	for _, stats := range localStats {
		t.stats.merge(stats, t.inputIndexesForResult)
	}
	return nil
}

func (t *Table) rebuildStatsFromSpansSerial(spans []sampleSpan) error {
	for i := 0; i < len(spans); {
		groupStart := spans[i].start
		groupEnd := spans[i].end
		j := i + 1
		for j < len(spans) && spans[j].start == groupEnd {
			groupEnd = spans[j].end
			j++
		}

		raw, err := dataManager_v2.ReadDataFromFileAsync(t.files.sampleData, groupStart, groupEnd)
		if err != nil {
			return err
		}
		for k := i; k < j; k++ {
			localStart := spans[k].start - groupStart
			localEnd := spans[k].end - groupStart
			if localStart < 0 || localEnd <= localStart || localEnd > int64(len(raw)) {
				return errors.New("tensor: invalid sample data span")
			}
			frame := raw[localStart:localEnd]
			if t.allFloatInputs {
				learningStatus, input, results, err := decodeFloatSampleFrameForStats(t.schema, frame)
				if err != nil {
					return err
				}
				if t.shouldLearn(learningStatus) {
					t.addFloatStats(input, results)
				}
			} else {
				learningStatus, input, results, err := decodeSampleFrameForStats(t.schema, frame)
				if err != nil {
					return err
				}
				if t.shouldLearn(learningStatus) {
					t.addStats(input, results)
				}
			}
		}
		i = j
	}
	return nil
}

func rebuildChunkRecords(entrySize uint64) uint64 {
	recordSize := entrySize + 3
	if recordSize == 0 {
		return 1
	}
	byBytes := uint64(tensorRebuildChunkBytes) / recordSize
	if byBytes == 0 {
		return 1
	}
	if byBytes > tensorRebuildChunkRecords {
		return tensorRebuildChunkRecords
	}
	return byBytes
}

func normalizeProbabilities(results []PredictedResult) {
	if len(results) == 0 {
		return
	}
	maxScore := results[0].Score
	for _, result := range results[1:] {
		if result.Score > maxScore {
			maxScore = result.Score
		}
	}

	sum := 0.0
	for i := range results {
		results[i].Probability = math.Exp(results[i].Score - maxScore)
		sum += results[i].Probability
	}
	if sum == 0 || math.IsNaN(sum) || math.IsInf(sum, 0) {
		p := 1 / float64(len(results))
		for i := range results {
			results[i].Probability = p
		}
		return
	}
	for i := range results {
		results[i].Probability /= sum
	}
}

func (t *Table) snapshotLabels() []*labelStats {
	labels := make([]*labelStats, 0, len(t.stats.LabelStats))
	for _, label := range t.stats.LabelStats {
		labels = append(labels, label)
	}
	return labels
}

func (t *Table) parallelFor(items int, fn func(start, end int)) {
	workers := t.workerCount(items)
	if workers == 1 {
		fn(0, items)
		return
	}
	var wg sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		start, end := workerRange(items, workers, worker)
		wg.Add(1)
		go func() {
			defer wg.Done()
			fn(start, end)
		}()
	}
	wg.Wait()
}

func (t *Table) parallelForErr(items int, fn func(start, end int) error) error {
	workers := t.workerCount(items)
	if workers == 1 {
		return fn(0, items)
	}

	var (
		wg      sync.WaitGroup
		errOnce sync.Once
		first   error
	)
	for worker := 0; worker < workers; worker++ {
		start, end := workerRange(items, workers, worker)
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := fn(start, end); err != nil {
				errOnce.Do(func() {
					first = err
				})
			}
		}()
	}
	wg.Wait()
	return first
}

func (t *Table) workerCount(items int) int {
	if items < 32 {
		return 1
	}
	workers := runtime.GOMAXPROCS(0)
	if workers < 2 {
		return 1
	}
	if workers > items {
		workers = items
	}
	return workers
}

func workerRange(items, workers, worker int) (int, int) {
	start := items * worker / workers
	end := items * (worker + 1) / workers
	return start, end
}

func newNegativeInfSlice(size int) []float64 {
	values := make([]float64, size)
	for i := range values {
		values[i] = math.Inf(-1)
	}
	return values
}

func betterLabel(candidate *labelStats, candidateScore float64, current *labelStats, currentScore float64) bool {
	return current == nil ||
		candidateScore > currentScore ||
		(candidateScore == currentScore && candidate.Value < current.Value)
}

func maxUint64(a, b uint64) uint64 {
	if a > b {
		return a
	}
	return b
}
