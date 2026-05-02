package tensor

import (
	"math"
	"strings"
)

type statsSnapshot struct {
	Table       string                 `json:"table"`
	TotalCount  uint64                 `json:"total_count"`
	LabelStats  map[string]*labelStats `json:"label_stats"`
	ResultGates map[string]*resultGate `json:"result_gates,omitempty"`
	LabelGates  map[string]*resultGate `json:"label_gates,omitempty"`
	IgnoreIndex map[string]bool        `json:"ignore_index,omitempty"`
}

type resultGate struct {
	InputWeights map[int]float64 `json:"input_weights,omitempty"`
	Bias         float64         `json:"bias,omitempty"`
}

type labelStats struct {
	Key        string                 `json:"key"`
	Value      string                 `json:"value"`
	Count      uint64                 `json:"count"`
	Numerics   map[int]*numericStats  `json:"numerics,omitempty"`
	Categories map[int]*categoryStats `json:"categories,omitempty"`
	Weights    map[int]float64        `json:"weights,omitempty"`

	inputIndexes  []int
	numericDense  []*numericStats
	categoryDense []*categoryStats
}

type numericStats struct {
	Count uint64  `json:"count"`
	Mean  float64 `json:"mean"`
	M2    float64 `json:"m2"`
	Min   float64 `json:"min"`
	Max   float64 `json:"max"`
}

type categoryStats struct {
	Count  uint64            `json:"count"`
	Values map[string]uint64 `json:"values"`
}

type normalizedInput []any

func newStats(schema Schema) *statsSnapshot {
	ignore := make(map[string]bool, len(schema.IgnoreStatuses))
	for _, status := range schema.IgnoreStatuses {
		ignore[status] = true
	}
	return &statsSnapshot{
		Table:       schema.Name,
		LabelStats:  make(map[string]*labelStats),
		ResultGates: make(map[string]*resultGate),
		LabelGates:  make(map[string]*resultGate),
		IgnoreIndex: ignore,
	}
}

func (s *statsSnapshot) reset(schema Schema) {
	*s = *newStats(schema)
}

func (t *Table) shouldLearn(learningStatus string) bool {
	if learningStatus == "" {
		learningStatus = LearningStatusUnknown
	}
	if t.stats.IgnoreIndex[learningStatus] {
		return false
	}
	return learningStatus == LearningStatusPositive
}

func (s *statsSnapshot) add(input normalizedInput, results []ResultLabel, indexesForResult func(string) []int) {
	if len(results) == 0 {
		return
	}
	s.TotalCount++
	for _, result := range results {
		id := labelID(result)
		label := s.LabelStats[id]
		if label == nil {
			label = newLabelStats(result.Key, result.Value, indexesForResult(result.Key))
			s.LabelStats[id] = label
		}
		label.Count++
		indexes := label.inputIndexes
		if len(indexes) == 0 {
			indexes = indexesForResult(result.Key)
			label.prepareDense(indexes)
		}
		for pos, idx := range indexes {
			if idx < 0 || idx >= len(input) {
				continue
			}
			value := input[idx]
			if numeric, ok := numericValue(value); ok {
				label.addNumericAt(pos, idx, numeric)
				continue
			}
			label.addCategoryAt(pos, idx, categoryValue(value))
		}
	}
}

func (s *statsSnapshot) merge(other *statsSnapshot, indexesForResult func(string) []int) {
	if other == nil {
		return
	}
	s.TotalCount += other.TotalCount
	for _, source := range other.LabelStats {
		if source == nil {
			continue
		}
		id := labelID(ResultLabel{Key: source.Key, Value: source.Value})
		target := s.LabelStats[id]
		if target == nil {
			target = newLabelStats(source.Key, source.Value, indexesForResult(source.Key))
			s.LabelStats[id] = target
		}
		target.merge(source)
	}
	s.mergeResultGates(other)
	s.mergeLabelGates(other)
}

func newLabelStats(key, value string, indexes []int) *labelStats {
	label := &labelStats{
		Key:   key,
		Value: value,
	}
	label.prepareDense(indexes)
	if !label.usesDenseOnly() {
		label.Numerics = make(map[int]*numericStats)
		label.Categories = make(map[int]*categoryStats)
	}
	return label
}

func (l *labelStats) merge(source *labelStats) {
	if source == nil {
		return
	}
	l.Count += source.Count
	l.mergeNumerics(source)
	l.mergeCategories(source)
	l.mergeWeights(source)
}

func (l *labelStats) mergeNumerics(source *labelStats) {
	if len(source.numericDense) != 0 {
		wrote := false
		for pos, stat := range source.numericDense {
			if stat != nil && stat.Count != 0 {
				wrote = true
				l.mergeNumericAt(pos, source.inputIndexes[pos], stat)
			}
		}
		if wrote || len(source.Numerics) == 0 {
			return
		}
	}
	for index, stat := range source.Numerics {
		if stat != nil && stat.Count != 0 {
			l.mergeNumericAt(-1, index, stat)
		}
	}
}

func (l *labelStats) mergeNumericAt(pos, index int, source *numericStats) {
	if source == nil || source.Count == 0 {
		return
	}
	if l.usesDenseOnly() {
		if len(l.numericDense) == 0 && len(l.inputIndexes) != 0 {
			l.numericDense = make([]*numericStats, len(l.inputIndexes))
		}
		targetPos := l.densePosition(pos, index)
		if targetPos >= 0 {
			if l.numericDense[targetPos] == nil {
				l.numericDense[targetPos] = cloneNumericStats(source)
				return
			}
			l.numericDense[targetPos].merge(source)
			return
		}
	}
	if l.Numerics == nil {
		l.Numerics = make(map[int]*numericStats)
	}
	if l.Numerics[index] == nil {
		l.Numerics[index] = cloneNumericStats(source)
		return
	}
	l.Numerics[index].merge(source)
}

func (l *labelStats) mergeCategories(source *labelStats) {
	if len(source.categoryDense) != 0 {
		wrote := false
		for pos, stat := range source.categoryDense {
			if stat != nil && stat.Count != 0 {
				wrote = true
				l.mergeCategoryAt(pos, source.inputIndexes[pos], stat)
			}
		}
		if wrote || len(source.Categories) == 0 {
			return
		}
	}
	for index, stat := range source.Categories {
		if stat != nil && stat.Count != 0 {
			l.mergeCategoryAt(-1, index, stat)
		}
	}
}

func (l *labelStats) mergeCategoryAt(pos, index int, source *categoryStats) {
	if source == nil || source.Count == 0 {
		return
	}
	if l.usesDenseOnly() {
		if len(l.categoryDense) == 0 && len(l.inputIndexes) != 0 {
			l.categoryDense = make([]*categoryStats, len(l.inputIndexes))
		}
		targetPos := l.densePosition(pos, index)
		if targetPos >= 0 {
			if l.categoryDense[targetPos] == nil {
				l.categoryDense[targetPos] = cloneCategoryStats(source)
				return
			}
			l.categoryDense[targetPos].merge(source)
			return
		}
	}
	if l.Categories == nil {
		l.Categories = make(map[int]*categoryStats)
	}
	if l.Categories[index] == nil {
		l.Categories[index] = cloneCategoryStats(source)
		return
	}
	l.Categories[index].merge(source)
}

func (l *labelStats) densePosition(pos, index int) int {
	if pos >= 0 && pos < len(l.inputIndexes) && l.inputIndexes[pos] == index {
		return pos
	}
	for i, inputIndex := range l.inputIndexes {
		if inputIndex == index {
			return i
		}
	}
	return -1
}

func (l *labelStats) prepareDense(indexes []int) {
	if len(indexes) == 0 {
		return
	}
	l.inputIndexes = append(l.inputIndexes[:0], indexes...)
	l.numericDense = make([]*numericStats, len(indexes))
	l.categoryDense = make([]*categoryStats, len(indexes))
	for pos, idx := range indexes {
		if stat := l.Numerics[idx]; stat != nil {
			l.numericDense[pos] = stat
		}
		if stat := l.Categories[idx]; stat != nil {
			l.categoryDense[pos] = stat
		}
	}
}

func (l *labelStats) usesDenseOnly() bool {
	return len(l.inputIndexes) > 8
}

func (l *labelStats) addNumericAt(pos, index int, value float64) {
	if l.usesDenseOnly() && pos >= 0 && pos < len(l.numericDense) {
		stat := l.numericDense[pos]
		if stat == nil {
			stat = &numericStats{Min: value, Max: value}
			l.numericDense[pos] = stat
		}
		stat.add(value)
		return
	}
	l.addNumeric(index, value)
}

func (l *labelStats) addNumeric(index int, value float64) {
	stat := l.Numerics[index]
	if stat == nil {
		stat = &numericStats{Min: value, Max: value}
		l.Numerics[index] = stat
	}
	stat.add(value)
}

func (n *numericStats) add(value float64) {
	n.Count++
	delta := value - n.Mean
	n.Mean += delta / float64(n.Count)
	n.M2 += delta * (value - n.Mean)
	if value < n.Min {
		n.Min = value
	}
	if value > n.Max {
		n.Max = value
	}
}

func (n *numericStats) merge(source *numericStats) {
	if source == nil || source.Count == 0 {
		return
	}
	if n.Count == 0 {
		*n = *source
		return
	}
	total := n.Count + source.Count
	delta := source.Mean - n.Mean
	n.Mean += delta * float64(source.Count) / float64(total)
	n.M2 += source.M2 + delta*delta*float64(n.Count)*float64(source.Count)/float64(total)
	n.Count = total
	if source.Min < n.Min {
		n.Min = source.Min
	}
	if source.Max > n.Max {
		n.Max = source.Max
	}
}

func cloneNumericStats(source *numericStats) *numericStats {
	if source == nil {
		return nil
	}
	clone := *source
	return &clone
}

func (l *labelStats) numericAt(pos, index int) *numericStats {
	if pos >= 0 && pos < len(l.numericDense) {
		if stat := l.numericDense[pos]; stat != nil {
			return stat
		}
	}
	return l.Numerics[index]
}

func (l *labelStats) numericStatsCount() int {
	if len(l.numericDense) == 0 {
		return len(l.Numerics)
	}
	count := 0
	for _, stat := range l.numericDense {
		if stat != nil && stat.Count != 0 {
			count++
		}
	}
	if count == 0 {
		return len(l.Numerics)
	}
	return count
}

func (l *labelStats) forEachNumeric(fn func(int, *numericStats)) {
	if len(l.numericDense) != 0 {
		wrote := false
		for pos, stat := range l.numericDense {
			if stat != nil && stat.Count != 0 {
				wrote = true
				fn(l.inputIndexes[pos], stat)
			}
		}
		if wrote || len(l.Numerics) == 0 {
			return
		}
	}
	for index, stat := range l.Numerics {
		if stat != nil && stat.Count != 0 {
			fn(index, stat)
		}
	}
}

func (l *labelStats) addCategoryAt(pos, index int, value string) {
	if l.usesDenseOnly() && pos >= 0 && pos < len(l.categoryDense) {
		stat := l.categoryDense[pos]
		if stat == nil {
			stat = &categoryStats{Values: make(map[string]uint64)}
			l.categoryDense[pos] = stat
		}
		stat.Count++
		stat.Values[value]++
		return
	}
	l.addCategory(index, value)
}

func (l *labelStats) addCategory(index int, value string) {
	stat := l.Categories[index]
	if stat == nil {
		stat = &categoryStats{Values: make(map[string]uint64)}
		l.Categories[index] = stat
	}
	stat.Count++
	stat.Values[value]++
}

func (c *categoryStats) merge(source *categoryStats) {
	if source == nil || source.Count == 0 {
		return
	}
	if c.Values == nil {
		c.Values = make(map[string]uint64, len(source.Values))
	}
	c.Count += source.Count
	for value, count := range source.Values {
		c.Values[value] += count
	}
}

func cloneCategoryStats(source *categoryStats) *categoryStats {
	if source == nil {
		return nil
	}
	clone := &categoryStats{
		Count:  source.Count,
		Values: make(map[string]uint64, len(source.Values)),
	}
	for value, count := range source.Values {
		clone.Values[value] = count
	}
	return clone
}

func (l *labelStats) mergeWeights(source *labelStats) {
	if len(source.Weights) == 0 {
		return
	}
	if l.Weights == nil {
		l.Weights = make(map[int]float64, len(source.Weights))
	}
	for index, weight := range source.Weights {
		l.Weights[index] = weight
	}
}

func (s *statsSnapshot) mergeResultGates(source *statsSnapshot) {
	if source == nil {
		return
	}
	s.mergeGates(source.ResultGates, true)
}

func (s *statsSnapshot) mergeLabelGates(source *statsSnapshot) {
	if source == nil {
		return
	}
	s.mergeGates(source.LabelGates, false)
}

func (s *statsSnapshot) mergeGates(source map[string]*resultGate, resultLevel bool) {
	if len(source) == 0 {
		return
	}
	targetGates := s.ResultGates
	if !resultLevel {
		targetGates = s.LabelGates
	}
	if targetGates == nil {
		targetGates = make(map[string]*resultGate, len(source))
		if resultLevel {
			s.ResultGates = targetGates
		} else {
			s.LabelGates = targetGates
		}
	}
	for key, sourceGate := range source {
		if sourceGate == nil || (len(sourceGate.InputWeights) == 0 && sourceGate.Bias == 0) {
			continue
		}
		target := targetGates[key]
		if target == nil {
			target = &resultGate{}
			if len(sourceGate.InputWeights) != 0 {
				target.InputWeights = make(map[int]float64, len(sourceGate.InputWeights))
			}
			targetGates[key] = target
		}
		target.Bias = sourceGate.Bias
		if len(sourceGate.InputWeights) != 0 && target.InputWeights == nil {
			target.InputWeights = make(map[int]float64, len(sourceGate.InputWeights))
		}
		for index, weight := range sourceGate.InputWeights {
			target.InputWeights[index] = weight
		}
	}
}

func (s *statsSnapshot) effectiveInputWeight(label *labelStats, index int) float64 {
	if label == nil {
		return 1
	}
	return s.resultInputWeight(label.Key, index) * s.labelInputWeight(label.Key, label.Value, index)
}

func (s *statsSnapshot) resultInputWeight(resultKey string, index int) float64 {
	return gateInputWeight(s.ResultGates, resultKey, index)
}

func (s *statsSnapshot) labelInputWeight(resultKey, value string, index int) float64 {
	return gateInputWeight(s.LabelGates, labelID(ResultLabel{Key: resultKey, Value: value}), index)
}

func (s *statsSnapshot) labelBias(resultKey, value string) float64 {
	return gateBias(s.LabelGates, labelID(ResultLabel{Key: resultKey, Value: value}))
}

func gateInputWeight(gates map[string]*resultGate, key string, index int) float64 {
	if len(gates) == 0 {
		return 1
	}
	gate := gates[key]
	if gate == nil || len(gate.InputWeights) == 0 {
		return 1
	}
	weight, ok := gate.InputWeights[index]
	if !ok {
		return 1
	}
	if math.IsNaN(weight) || math.IsInf(weight, 0) {
		return 1
	}
	if weight < 0 {
		return 0
	}
	return weight
}

func gateBias(gates map[string]*resultGate, key string) float64 {
	if len(gates) == 0 {
		return 0
	}
	gate := gates[key]
	if gate == nil || math.IsNaN(gate.Bias) || math.IsInf(gate.Bias, 0) {
		return 0
	}
	return gate.Bias
}

func (s *statsSnapshot) adjustResultInputWeight(resultKey string, index int, delta, regularization, minWeight, maxWeight float64) bool {
	return s.adjustGateInputWeight(s.ResultGates, true, resultKey, index, delta, regularization, minWeight, maxWeight)
}

func (s *statsSnapshot) adjustLabelInputWeight(label *labelStats, index int, delta, regularization, minWeight, maxWeight float64) bool {
	if label == nil {
		return false
	}
	return s.adjustGateInputWeight(s.LabelGates, false, labelID(ResultLabel{Key: label.Key, Value: label.Value}), index, delta, regularization, minWeight, maxWeight)
}

func (s *statsSnapshot) adjustLabelBias(label *labelStats, delta, regularization, maxAbsBias float64) bool {
	if label == nil {
		return false
	}
	return s.adjustGateBias(s.LabelGates, false, labelID(ResultLabel{Key: label.Key, Value: label.Value}), delta, regularization, maxAbsBias)
}

func (s *statsSnapshot) adjustGateInputWeight(gates map[string]*resultGate, resultLevel bool, key string, index int, delta, regularization, minWeight, maxWeight float64) bool {
	if s == nil || key == "" || index < 0 || math.IsNaN(delta) || math.IsInf(delta, 0) {
		return false
	}
	if regularization <= 0 || regularization > 1 || math.IsNaN(regularization) || math.IsInf(regularization, 0) {
		regularization = 1
	}
	if minWeight < 0 || math.IsNaN(minWeight) || math.IsInf(minWeight, 0) {
		minWeight = 0
	}
	if maxWeight < minWeight {
		maxWeight = minWeight
	}
	current := gateInputWeight(gates, key, index)
	next := (current + delta) * regularization
	if next < minWeight {
		next = minWeight
	}
	if next > maxWeight {
		next = maxWeight
	}
	if next == current {
		return false
	}
	if gates == nil {
		gates = make(map[string]*resultGate)
		if resultLevel {
			s.ResultGates = gates
		} else {
			s.LabelGates = gates
		}
	}
	gate := gates[key]
	if gate == nil {
		gate = &resultGate{}
		gates[key] = gate
	}
	if gate.InputWeights == nil {
		gate.InputWeights = make(map[int]float64)
	}
	gate.InputWeights[index] = next
	return true
}

func (s *statsSnapshot) adjustGateBias(gates map[string]*resultGate, resultLevel bool, key string, delta, regularization, maxAbsBias float64) bool {
	if s == nil || key == "" || math.IsNaN(delta) || math.IsInf(delta, 0) {
		return false
	}
	if regularization <= 0 || regularization > 1 || math.IsNaN(regularization) || math.IsInf(regularization, 0) {
		regularization = 1
	}
	if maxAbsBias <= 0 || math.IsNaN(maxAbsBias) || math.IsInf(maxAbsBias, 0) {
		maxAbsBias = 1
	}
	current := gateBias(gates, key)
	next := (current + delta) * regularization
	if next < -maxAbsBias {
		next = -maxAbsBias
	}
	if next > maxAbsBias {
		next = maxAbsBias
	}
	if next == current {
		return false
	}
	if gates == nil {
		gates = make(map[string]*resultGate)
		if resultLevel {
			s.ResultGates = gates
		} else {
			s.LabelGates = gates
		}
	}
	gate := gates[key]
	if gate == nil {
		gate = &resultGate{}
		gates[key] = gate
	}
	gate.Bias = next
	return true
}

func (l *labelStats) categoryAt(pos, index int) *categoryStats {
	if pos >= 0 && pos < len(l.categoryDense) {
		if stat := l.categoryDense[pos]; stat != nil {
			return stat
		}
	}
	return l.Categories[index]
}

func (l *labelStats) categoryStatsCount() int {
	if len(l.categoryDense) == 0 {
		return len(l.Categories)
	}
	count := 0
	for _, stat := range l.categoryDense {
		if stat != nil && stat.Count != 0 {
			count++
		}
	}
	if count == 0 {
		return len(l.Categories)
	}
	return count
}

func (l *labelStats) forEachCategory(fn func(int, *categoryStats)) {
	if len(l.categoryDense) != 0 {
		wrote := false
		for pos, stat := range l.categoryDense {
			if stat != nil && stat.Count != 0 {
				wrote = true
				fn(l.inputIndexes[pos], stat)
			}
		}
		if wrote || len(l.Categories) == 0 {
			return
		}
	}
	for index, stat := range l.Categories {
		if stat != nil && stat.Count != 0 {
			fn(index, stat)
		}
	}
}

func (n numericStats) variance() float64 {
	if n.Count < 2 {
		return 1
	}
	v := n.M2 / float64(n.Count-1)
	if v < 1e-9 || math.IsNaN(v) || math.IsInf(v, 0) {
		return 1e-9
	}
	return v
}

func numericLogContribution(value float64, stat *numericStats) float64 {
	if stat == nil || stat.Count == 0 {
		return 0
	}
	return math.Log(0.05 + numericImpact(value, stat))
}

func numericImpact(value float64, stat *numericStats) float64 {
	if stat == nil || stat.Count == 0 {
		return 0
	}
	variance := stat.variance()
	z := (value - stat.Mean) / math.Sqrt(variance)
	return 1 / (1 + math.Abs(z))
}

func labelID(result ResultLabel) string {
	return result.Key + "\x00" + result.Value
}

func splitLabelID(id string) (string, string) {
	key, value, ok := strings.Cut(id, "\x00")
	if !ok {
		return id, ""
	}
	return key, value
}
