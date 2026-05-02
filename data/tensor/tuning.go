package tensor

import (
	"math"
	"sort"
)

const (
	defaultTuneIterations   = 12
	defaultTuneLearningRate = 0.02
	defaultTuneRegularize   = 0.999
	defaultTuneMinWeight    = 0.0
	defaultTuneMaxWeight    = 2.5
	defaultClassLayerRate   = 0.02
	defaultLabelBiasRate    = 4.0
	defaultMaxLabelBias     = 8.0
	tuneSummaryLimit        = 5
	tuneSummaryGroupLimit   = 12
)

func (t *Table) TuneWeights(samples []Sample, options TuneOptions) (TuneReport, error) {
	params := normalizeTuneOptions(options)
	progress := newTuneProgress(len(samples), params)
	report := TuneReport{
		Samples:        len(samples),
		Iterations:     params.Iterations,
		ErrorsByResult: make(map[string]int),
	}
	if len(samples) == 0 {
		progress.finish()
		return report, nil
	}
	if len(t.stats.LabelStats) == 0 {
		progress.finish()
		return report, ErrNoModelData
	}
	accuracy, err := t.tuneAccuracy(samples, progress.add)
	if err != nil {
		return report, err
	}
	report.AccuracyBefore = accuracy
	report.AccuracyAfter = accuracy

	for iteration := 0; iteration < params.Iterations; iteration++ {
		beforeGates := cloneResultGates(t.stats.ResultGates)
		beforeLabelGates := cloneResultGates(t.stats.LabelGates)
		epochErrors := make(map[string]int)
		epochCorrections := 0
		epochAdjustments := 0
		for i := range samples {
			var corrections, adjustments int
			if t.allFloatInputs {
				corrections, adjustments, err = t.tuneFloatSample(samples[i], params, epochErrors)
			} else {
				corrections, adjustments, err = t.tuneSample(samples[i], params, epochErrors)
			}
			if err != nil {
				return report, err
			}
			epochCorrections += corrections
			epochAdjustments += adjustments
			progress.add(1)
		}

		nextAccuracy, err := t.tuneAccuracy(samples, progress.add)
		if err != nil {
			return report, err
		}
		if nextAccuracy+1e-12 < report.AccuracyAfter {
			t.stats.ResultGates = beforeGates
			t.stats.LabelGates = beforeLabelGates
			params.LearningRate *= 0.5
			continue
		}
		report.AccuracyAfter = nextAccuracy
		report.Corrections += epochCorrections
		report.Adjustments += epochAdjustments
		for key, count := range epochErrors {
			report.ErrorsByResult[key] += count
		}
	}
	report.TopBoosted, report.TopSuppressed = t.gateWeightSummaries(t.stats.ResultGates, tuneSummaryLimit, false)
	report.TopClassBoosted, report.TopClassSuppressed = t.gateWeightSummaries(t.stats.LabelGates, tuneSummaryLimit, true)
	progress.finish()
	return report, nil
}

type tuneProgress struct {
	fn        func(completed, total int)
	completed int
	total     int
}

func newTuneProgress(samples int, options TuneOptions) tuneProgress {
	total := samples * (1 + 2*options.Iterations)
	if total < 1 {
		total = 1
	}
	progress := tuneProgress{
		fn:    options.Progress,
		total: total,
	}
	progress.notify()
	return progress
}

func (p *tuneProgress) add(delta int) {
	if delta <= 0 {
		return
	}
	p.completed += delta
	if p.completed > p.total {
		p.completed = p.total
	}
	p.notify()
}

func (p *tuneProgress) finish() {
	if p.completed >= p.total {
		return
	}
	p.completed = p.total
	p.notify()
}

func (p tuneProgress) notify() {
	if p.fn != nil {
		p.fn(p.completed, p.total)
	}
}

func normalizeTuneOptions(options TuneOptions) TuneOptions {
	if options.Iterations <= 0 {
		options.Iterations = defaultTuneIterations
	}
	if options.LearningRate <= 0 || math.IsNaN(options.LearningRate) || math.IsInf(options.LearningRate, 0) {
		options.LearningRate = defaultTuneLearningRate
	}
	if options.Regularization <= 0 || options.Regularization > 1 || math.IsNaN(options.Regularization) || math.IsInf(options.Regularization, 0) {
		options.Regularization = defaultTuneRegularize
	}
	if options.MinWeight < 0 || math.IsNaN(options.MinWeight) || math.IsInf(options.MinWeight, 0) {
		options.MinWeight = defaultTuneMinWeight
	}
	if options.MaxWeight <= 0 || math.IsNaN(options.MaxWeight) || math.IsInf(options.MaxWeight, 0) {
		options.MaxWeight = defaultTuneMaxWeight
	}
	if options.MaxWeight < options.MinWeight {
		options.MaxWeight = options.MinWeight
	}
	return options
}

func (t *Table) tuneFloatSample(sample Sample, options TuneOptions, errorsByResult map[string]int) (int, int, error) {
	input, results, err := t.validateFloatSample(sample)
	if err != nil {
		return 0, 0, err
	}
	ranks := t.floatLabelRanksPerResult(input)
	corrections, adjustments := t.tuneFloatRanks(input, results, ranks, options, errorsByResult)
	return corrections, adjustments, nil
}

func (t *Table) tuneSample(sample Sample, options TuneOptions, errorsByResult map[string]int) (int, int, error) {
	input, results, err := t.validateSample(sample)
	if err != nil {
		return 0, 0, err
	}
	ranks := t.labelRanksPerResult(input)
	corrections, adjustments := t.tuneRanks(input, results, ranks, options, errorsByResult)
	return corrections, adjustments, nil
}

func (t *Table) tuneFloatRanks(input []float64, want []ResultLabel, ranks []labelRank, options TuneOptions, errorsByResult map[string]int) (int, int) {
	corrections := 0
	adjustments := 0
	for _, expected := range want {
		expectedLabel := t.stats.LabelStats[labelID(expected)]
		if expectedLabel == nil {
			continue
		}
		wrongLabel, correct := t.rankCompetitor(expected, ranks)
		if !correct {
			corrections++
			errorsByResult[expected.Key]++
			adjustments += t.tuneLabelBias(expectedLabel, wrongLabel, options)
		}
		adjustments += t.tuneFloatMargin(input, expectedLabel, wrongLabel, options, true, !correct)
	}
	return corrections, adjustments
}

func (t *Table) tuneRanks(input normalizedInput, want []ResultLabel, ranks []labelRank, options TuneOptions, errorsByResult map[string]int) (int, int) {
	corrections := 0
	adjustments := 0
	for _, expected := range want {
		expectedLabel := t.stats.LabelStats[labelID(expected)]
		if expectedLabel == nil {
			continue
		}
		wrongLabel, correct := t.rankCompetitor(expected, ranks)
		if !correct {
			corrections++
			errorsByResult[expected.Key]++
			adjustments += t.tuneLabelBias(expectedLabel, wrongLabel, options)
		}
		adjustments += t.tuneMargin(input, expectedLabel, wrongLabel, options, true, !correct)
	}
	return corrections, adjustments
}

func (t *Table) tuneLabelBias(expectedLabel, wrongLabel *labelStats, options TuneOptions) int {
	adjustments := 0
	delta := options.LearningRate * defaultLabelBiasRate
	if t.stats.adjustLabelBias(expectedLabel, delta, options.Regularization, defaultMaxLabelBias) {
		adjustments++
	}
	if t.stats.adjustLabelBias(wrongLabel, -delta, options.Regularization, defaultMaxLabelBias) {
		adjustments++
	}
	return adjustments
}

func predictedResultsByKey(results []PredictedResult) map[string]PredictedResult {
	out := make(map[string]PredictedResult, len(results))
	for _, result := range results {
		out[result.Key] = result
	}
	return out
}

type labelRank struct {
	best        *labelStats
	bestScore   float64
	second      *labelStats
	secondScore float64
}

func (r *labelRank) offer(label *labelStats, score float64) {
	if label == nil {
		return
	}
	if betterLabel(label, score, r.best, r.bestScore) {
		r.second = r.best
		r.secondScore = r.bestScore
		r.best = label
		r.bestScore = score
		return
	}
	if r.best == label {
		return
	}
	if betterLabel(label, score, r.second, r.secondScore) {
		r.second = label
		r.secondScore = score
	}
}

func (t *Table) floatLabelRanksPerResult(input []float64) []labelRank {
	ranks := newLabelRanks(len(t.schema.Results))
	for _, label := range t.snapshotLabels() {
		resultIndex, ok := t.resultIndexByKey[label.Key]
		if !ok || resultIndex < 0 || resultIndex >= len(ranks) {
			continue
		}
		ranks[resultIndex].offer(label, t.scoreFloatLabelOnly(input, label))
	}
	return ranks
}

func (t *Table) labelRanksPerResult(input normalizedInput) []labelRank {
	ranks := newLabelRanks(len(t.schema.Results))
	for _, label := range t.snapshotLabels() {
		resultIndex, ok := t.resultIndexByKey[label.Key]
		if !ok || resultIndex < 0 || resultIndex >= len(ranks) {
			continue
		}
		ranks[resultIndex].offer(label, t.scoreLabelOnly(input, label))
	}
	return ranks
}

func newLabelRanks(count int) []labelRank {
	ranks := make([]labelRank, count)
	for i := range ranks {
		ranks[i].bestScore = math.Inf(-1)
		ranks[i].secondScore = math.Inf(-1)
	}
	return ranks
}

func (t *Table) rankCompetitor(expected ResultLabel, ranks []labelRank) (*labelStats, bool) {
	resultIndex, ok := t.resultIndexByKey[expected.Key]
	if !ok || resultIndex < 0 || resultIndex >= len(ranks) {
		return nil, false
	}
	rank := ranks[resultIndex]
	if rank.best == nil {
		return nil, false
	}
	if rank.best.Value != expected.Value {
		return rank.best, false
	}
	return rank.second, true
}

func (t *Table) tuneFloatMargin(input []float64, expectedLabel, wrongLabel *labelStats, options TuneOptions, adjustResult, adjustClass bool) int {
	adjustments := 0
	indexes := t.inputIndexesForResult(expectedLabel.Key)
	if len(expectedLabel.inputIndexes) != 0 {
		indexes = expectedLabel.inputIndexes
	}
	for pos, idx := range indexes {
		if idx < 0 || idx >= len(input) || idx >= len(t.schema.Inputs) {
			continue
		}
		good, goodOK := floatLabelInputContribution(t.stats, input[idx], expectedLabel, pos, idx)
		bad, badOK := floatLabelInputContribution(t.stats, input[idx], wrongLabel, pos, idx)
		if !goodOK && !badOK {
			continue
		}
		if adjustResult {
			delta := options.LearningRate * (good - bad)
			if t.stats.adjustResultInputWeight(expectedLabel.Key, idx, delta, options.Regularization, options.MinWeight, options.MaxWeight) {
				adjustments++
			}
		}
		if adjustClass {
			classRate := options.LearningRate * defaultClassLayerRate
			if t.stats.adjustLabelInputWeight(expectedLabel, idx, classRate*(good-bad), options.Regularization, options.MinWeight, options.MaxWeight) {
				adjustments++
			}
			if wrongLabel != nil && t.stats.adjustLabelInputWeight(wrongLabel, idx, -classRate*(good-bad), options.Regularization, options.MinWeight, options.MaxWeight) {
				adjustments++
			}
		}
	}
	return adjustments
}

func (t *Table) tuneMargin(input normalizedInput, expectedLabel, wrongLabel *labelStats, options TuneOptions, adjustResult, adjustClass bool) int {
	adjustments := 0
	indexes := t.inputIndexesForResult(expectedLabel.Key)
	if len(expectedLabel.inputIndexes) != 0 {
		indexes = expectedLabel.inputIndexes
	}
	for pos, idx := range indexes {
		if idx < 0 || idx >= len(input) || idx >= len(t.schema.Inputs) {
			continue
		}
		good, goodOK := labelInputContribution(t.stats, input[idx], expectedLabel, pos, idx)
		bad, badOK := labelInputContribution(t.stats, input[idx], wrongLabel, pos, idx)
		if !goodOK && !badOK {
			continue
		}
		if adjustResult {
			delta := options.LearningRate * (good - bad)
			if t.stats.adjustResultInputWeight(expectedLabel.Key, idx, delta, options.Regularization, options.MinWeight, options.MaxWeight) {
				adjustments++
			}
		}
		if adjustClass {
			classRate := options.LearningRate * defaultClassLayerRate
			if t.stats.adjustLabelInputWeight(expectedLabel, idx, classRate*(good-bad), options.Regularization, options.MinWeight, options.MaxWeight) {
				adjustments++
			}
			if wrongLabel != nil && t.stats.adjustLabelInputWeight(wrongLabel, idx, -classRate*(good-bad), options.Regularization, options.MinWeight, options.MaxWeight) {
				adjustments++
			}
		}
	}
	return adjustments
}

func labelInputContribution(stats *statsSnapshot, value any, label *labelStats, pos, idx int) (float64, bool) {
	if label == nil {
		return 0, false
	}
	if numeric, ok := numericValue(value); ok {
		return floatLabelInputContribution(stats, numeric, label, pos, idx)
	}
	stat := label.categoryAt(pos, idx)
	if stat == nil || stat.Count == 0 {
		return 0, false
	}
	category := categoryValue(value)
	frequency := float64(stat.Values[category]+1) / float64(stat.Count+uint64(len(stat.Values))+1)
	return math.Log(frequency), true
}

func floatLabelInputContribution(stats *statsSnapshot, value float64, label *labelStats, pos, idx int) (float64, bool) {
	if label == nil {
		return 0, false
	}
	stat := label.numericAt(pos, idx)
	if stat == nil || stat.Count == 0 {
		return 0, false
	}
	if stats != nil {
		return numericLogContribution(value, stat), true
	}
	return math.Log(0.05 + numericImpact(value, stat)), true
}

func (t *Table) tuneAccuracy(samples []Sample, progress func(int)) (float64, error) {
	correct := 0
	total := 0
	for i := range samples {
		if t.allFloatInputs {
			input, results, err := t.validateFloatSample(samples[i])
			if err != nil {
				return 0, err
			}
			prediction, err := t.predictFloatBestPerResult(input)
			if err != nil {
				return 0, err
			}
			got := predictedResultsByKey(prediction.Results)
			for _, expected := range results {
				total++
				if result, ok := got[expected.Key]; ok && result.Value == expected.Value {
					correct++
				}
			}
			if progress != nil {
				progress(1)
			}
			continue
		}
		input, results, err := t.validateSample(samples[i])
		if err != nil {
			return 0, err
		}
		prediction, err := t.predictBestPerResult(input)
		if err != nil {
			return 0, err
		}
		got := predictedResultsByKey(prediction.Results)
		for _, expected := range results {
			total++
			if result, ok := got[expected.Key]; ok && result.Value == expected.Value {
				correct++
			}
		}
		if progress != nil {
			progress(1)
		}
	}
	if total == 0 {
		return 0, nil
	}
	return float64(correct) / float64(total), nil
}

func cloneResultGates(source map[string]*resultGate) map[string]*resultGate {
	if len(source) == 0 {
		return nil
	}
	clone := make(map[string]*resultGate, len(source))
	for key, gate := range source {
		if gate == nil {
			continue
		}
		copied := &resultGate{}
		copied.Bias = gate.Bias
		if len(gate.InputWeights) != 0 {
			copied.InputWeights = make(map[int]float64, len(gate.InputWeights))
			for index, weight := range gate.InputWeights {
				copied.InputWeights[index] = weight
			}
		}
		clone[key] = copied
	}
	return clone
}

func (t *Table) gateWeightSummaries(gates map[string]*resultGate, limit int, labelLevel bool) (map[string][]GateWeightSummary, map[string][]GateWeightSummary) {
	if limit <= 0 || len(gates) == 0 {
		return nil, nil
	}
	boosted := make(map[string][]GateWeightSummary)
	suppressed := make(map[string][]GateWeightSummary)
	for gateKey, gate := range gates {
		if labelLevel && len(boosted)+len(suppressed) >= tuneSummaryGroupLimit {
			break
		}
		if gate == nil || len(gate.InputWeights) == 0 {
			continue
		}
		reportKey := gateKey
		if labelLevel {
			key, value := splitLabelID(gateKey)
			reportKey = key + "=" + value
		}
		items := make([]GateWeightSummary, 0, len(gate.InputWeights))
		for index, weight := range gate.InputWeights {
			if index < 0 || index >= len(t.schema.Inputs) || math.IsNaN(weight) || math.IsInf(weight, 0) {
				continue
			}
			items = append(items, GateWeightSummary{
				Input:  t.schema.Inputs[index].Name,
				Index:  index,
				Weight: weight,
			})
		}
		sort.Slice(items, func(i, j int) bool {
			if items[i].Weight == items[j].Weight {
				return items[i].Input < items[j].Input
			}
			return items[i].Weight > items[j].Weight
		})
		for _, item := range items {
			if item.Weight <= 1 {
				continue
			}
			boosted[reportKey] = append(boosted[reportKey], item)
			if len(boosted[reportKey]) >= limit {
				break
			}
		}
		sort.Slice(items, func(i, j int) bool {
			if items[i].Weight == items[j].Weight {
				return items[i].Input < items[j].Input
			}
			return items[i].Weight < items[j].Weight
		})
		for _, item := range items {
			if item.Weight >= 1 {
				continue
			}
			suppressed[reportKey] = append(suppressed[reportKey], item)
			if len(suppressed[reportKey]) >= limit {
				break
			}
		}
		if len(boosted[reportKey]) == 0 {
			delete(boosted, reportKey)
		}
		if len(suppressed[reportKey]) == 0 {
			delete(suppressed, reportKey)
		}
	}
	if len(boosted) == 0 {
		boosted = nil
	}
	if len(suppressed) == 0 {
		suppressed = nil
	}
	return boosted, suppressed
}
