package tensor

import "math"

type statsSnapshot struct {
	Table       string                 `json:"table"`
	TotalCount  uint64                 `json:"total_count"`
	LabelStats  map[string]*labelStats `json:"label_stats"`
	IgnoreIndex map[string]bool        `json:"ignore_index,omitempty"`
}

type labelStats struct {
	Key        string                 `json:"key"`
	Value      string                 `json:"value"`
	Count      uint64                 `json:"count"`
	Numerics   map[int]*numericStats  `json:"numerics,omitempty"`
	Categories map[int]*categoryStats `json:"categories,omitempty"`

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

func labelID(result ResultLabel) string {
	return result.Key + "\x00" + result.Value
}
