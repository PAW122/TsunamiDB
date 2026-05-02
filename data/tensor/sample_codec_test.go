package tensor

import (
	"math"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	dataManager_v2 "github.com/PAW122/TsunamiDB/data/dataManager/v2"
	fileSystem_v1 "github.com/PAW122/TsunamiDB/data/fileSystem/v1"
)

func TestSchemaDefaultSampleEntrySizeTracksSchema(t *testing.T) {
	schema := Schema{
		Name:    "entry_size",
		Inputs:  make([]InputField, 0, 100),
		Results: make([]ResultField, 0, 3),
	}
	for i := 0; i < 100; i++ {
		schema.Inputs = append(schema.Inputs, InputField{
			Name: "p" + string(rune('a'+i%26)) + string(rune('a'+(i/26)%26)),
			Type: InputTypeFloat64,
		})
	}
	for i := 0; i < 3; i++ {
		schema.Results = append(schema.Results, ResultField{
			Key:  "r" + string(rune('a'+i)),
			Type: InputTypeString,
		})
	}

	got, err := validateSchema(schema)
	if err != nil {
		t.Fatalf("validateSchema: %v", err)
	}
	if got.SampleEntrySize >= 8<<10 {
		t.Fatalf("default sample entry size too large: got %d, want less than 8192", got.SampleEntrySize)
	}
	if got.SampleEntrySize < minSampleEntrySize(got) {
		t.Fatalf("default sample entry size %d below minimum %d", got.SampleEntrySize, minSampleEntrySize(got))
	}
}

func TestSampleFrameRoundTrip(t *testing.T) {
	schema := Schema{
		Name: "roundtrip",
		Inputs: []InputField{
			{Name: "float_value", Type: InputTypeFloat64},
			{Name: "int_value", Type: InputTypeInt64},
			{Name: "uint_value", Type: InputTypeUint64},
			{Name: "bool_value", Type: InputTypeBool},
			{Name: "string_value", Type: InputTypeString},
		},
		Results: []ResultField{
			{Key: "component", Type: InputTypeString},
		},
	}
	schema, err := validateSchema(schema)
	if err != nil {
		t.Fatalf("validateSchema: %v", err)
	}

	table := &Table{schema: schema, stats: newStats(schema)}
	sample := Sample{
		SampleID:       "sample_1",
		TestStatus:     TestStatusFail,
		LearningStatus: LearningStatusPositive,
		Input: map[string]any{
			"float_value":  12.5,
			"int_value":    int64(-7),
			"uint_value":   uint64(42),
			"bool_value":   true,
			"string_value": "E17",
		},
		Results: []ResultLabel{
			{Key: "component", Value: "power_supply"},
		},
	}
	input, _, err := table.validateSample(sample)
	if err != nil {
		t.Fatalf("validateSample: %v", err)
	}

	frame, err := encodeSampleFrame(schema, sample, input)
	if err != nil {
		t.Fatalf("encodeSampleFrame: %v", err)
	}
	got, err := decodeSampleFrame(schema, frame)
	if err != nil {
		t.Fatalf("decodeSampleFrame: %v", err)
	}

	if got.SampleID != sample.SampleID || got.TestStatus != sample.TestStatus || got.LearningStatus != sample.LearningStatus {
		t.Fatalf("metadata mismatch: got %#v want %#v", got, sample)
	}
	if got.Input["float_value"] != sample.Input["float_value"] ||
		got.Input["int_value"] != sample.Input["int_value"] ||
		got.Input["uint_value"] != sample.Input["uint_value"] ||
		got.Input["bool_value"] != sample.Input["bool_value"] ||
		got.Input["string_value"] != sample.Input["string_value"] {
		t.Fatalf("input mismatch: got %#v want %#v", got.Input, sample.Input)
	}
	if len(got.Results) != 1 || got.Results[0] != sample.Results[0] {
		t.Fatalf("results mismatch: got %#v want %#v", got.Results, sample.Results)
	}
}

func TestResultKeyFiltersLearnedInputs(t *testing.T) {
	schema, err := validateSchema(Schema{
		Name: "result_key_filter",
		Inputs: []InputField{
			{Name: "for_a", Type: InputTypeFloat64, ResultKey: "a"},
			{Name: "for_b", Type: InputTypeFloat64, ResultKey: "b"},
			{Name: "global", Type: InputTypeFloat64},
		},
		Results: []ResultField{
			{Key: "a", Type: InputTypeString},
			{Key: "b", Type: InputTypeString},
		},
	})
	if err != nil {
		t.Fatalf("validateSchema: %v", err)
	}

	table := newTable(schema, newStats(schema), tableFiles{})
	input := normalizedInput{float64(1), float64(2), float64(3)}
	table.stats.add(input, []ResultLabel{{Key: "a", Value: "class"}}, table.inputIndexesForResult)

	label := table.stats.LabelStats[labelID(ResultLabel{Key: "a", Value: "class"})]
	if label == nil {
		t.Fatal("label stats missing")
	}
	if label.Numerics[0] == nil || label.Numerics[2] == nil {
		t.Fatalf("expected result-specific and global stats, got %#v", label.Numerics)
	}
	if label.Numerics[1] != nil {
		t.Fatalf("unexpected stats for unrelated result input: %#v", label.Numerics[1])
	}
}

func TestRebuildChunkRecordsCapsLargeEntriesByBytes(t *testing.T) {
	if got := rebuildChunkRecords(85_000); got == 0 || got >= tensorRebuildChunkRecords {
		t.Fatalf("large entry chunk size = %d, want between 1 and %d", got, tensorRebuildChunkRecords)
	}
	if got := rebuildChunkRecords(128); got != tensorRebuildChunkRecords {
		t.Fatalf("small entry chunk size = %d, want %d", got, tensorRebuildChunkRecords)
	}
}

func TestSampleLogUsesCompactManifestEntries(t *testing.T) {
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

	schema := Schema{
		Name: "compact_log",
		Inputs: []InputField{
			{Name: "a", Type: InputTypeFloat64, ResultKey: "component"},
			{Name: "b", Type: InputTypeString, ResultKey: "component"},
		},
		Results: []ResultField{
			{Key: "component", Type: InputTypeString},
		},
	}
	table, err := CreateTable(schema)
	if err != nil {
		t.Fatalf("CreateTable: %v", err)
	}
	samples := []Sample{
		{
			SampleID:       "s1",
			TestStatus:     TestStatusFail,
			LearningStatus: LearningStatusPositive,
			Input:          map[string]any{"a": 1.25, "b": "long-enough-string-value"},
			Results:        []ResultLabel{{Key: "component", Value: "psu"}},
		},
		{
			SampleID:       "s2",
			TestStatus:     TestStatusFail,
			LearningStatus: LearningStatusPositive,
			Input:          map[string]any{"a": 2.5, "b": "another-long-enough-string-value"},
			Results:        []ResultLabel{{Key: "component", Value: "fan"}},
		},
	}
	if err := table.AddSamples(samples); err != nil {
		t.Fatalf("AddSamples: %v", err)
	}

	manifestPath := filepath.Join("db", "inc_tables", table.files.samples)
	info, err := os.Stat(manifestPath)
	if err != nil {
		t.Fatalf("stat manifest: %v", err)
	}
	wantManifestSize := int64(len(samples) * (int(tensorSampleManifestEntrySize) + 3))
	if info.Size() != wantManifestSize {
		t.Fatalf("manifest size=%d want=%d", info.Size(), wantManifestSize)
	}

	dataPath := filepath.Join("db", "data", table.files.sampleData)
	dataInfo, err := os.Stat(dataPath)
	if err != nil {
		t.Fatalf("stat sample data: %v", err)
	}
	if dataInfo.Size() <= info.Size() {
		t.Fatalf("sample data should hold variable payload, data=%d manifest=%d", dataInfo.Size(), info.Size())
	}

	if err := RebuildStats("compact_log"); err != nil {
		t.Fatalf("RebuildStats: %v", err)
	}
	reopened, err := OpenTable("compact_log")
	if err != nil {
		t.Fatalf("OpenTable: %v", err)
	}
	if _, err := reopened.Predict(map[string]any{"a": 1.1, "b": "long-enough-string-value"}, 1); err != nil {
		t.Fatalf("Predict: %v", err)
	}
}

func TestParallelRebuildStatsMatchesIncrementalStats(t *testing.T) {
	previousProcs := runtime.GOMAXPROCS(4)
	t.Cleanup(func() {
		runtime.GOMAXPROCS(previousProcs)
	})

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

	schema := Schema{
		Name: "parallel_rebuild",
		Inputs: []InputField{
			{Name: "temperature", Type: InputTypeFloat64, ResultKey: "component"},
			{Name: "voltage", Type: InputTypeFloat64, ResultKey: "severity"},
			{Name: "current", Type: InputTypeFloat64, ResultKey: "component"},
		},
		Results: []ResultField{
			{Key: "component", Type: InputTypeString},
			{Key: "severity", Type: InputTypeString},
		},
	}
	table, err := CreateTable(schema)
	if err != nil {
		t.Fatalf("CreateTable: %v", err)
	}

	samples := make([]Sample, 128)
	for i := range samples {
		samples[i] = Sample{
			SampleID:       "sample",
			TestStatus:     TestStatusFail,
			LearningStatus: LearningStatusPositive,
			Input: map[string]any{
				"temperature": float64(i%17) + 0.25,
				"voltage":     float64((i*3)%11) + 1.5,
				"current":     float64((i*7)%19) + 0.5,
			},
			Results: []ResultLabel{
				{Key: "component", Value: string(rune('a' + i%5))},
				{Key: "severity", Value: string(rune('a' + i%3))},
			},
		}
	}
	if err := table.AddSamples(samples); err != nil {
		t.Fatalf("AddSamples: %v", err)
	}

	if err := RebuildStats("parallel_rebuild"); err != nil {
		t.Fatalf("RebuildStats: %v", err)
	}
	reopened, err := OpenTable("parallel_rebuild")
	if err != nil {
		t.Fatalf("OpenTable: %v", err)
	}
	assertStatsClose(t, table.stats, reopened.stats)
}

func TestStatsMergeCategories(t *testing.T) {
	schema := Schema{
		Name:   "merge_categories",
		Inputs: make([]InputField, 9),
		Results: []ResultField{
			{Key: "result", Type: InputTypeString},
		},
	}
	for i := range schema.Inputs {
		schema.Inputs[i] = InputField{
			Name:      string(rune('a' + i)),
			Type:      InputTypeString,
			ResultKey: "result",
		}
	}

	combined := newTable(schema, newStats(schema), tableFiles{})
	left := newTable(schema, newStats(schema), tableFiles{})
	right := newTable(schema, newStats(schema), tableFiles{})
	for i := 0; i < 64; i++ {
		input := make(normalizedInput, len(schema.Inputs))
		for j := range input {
			input[j] = string(rune('a' + (i+j)%5))
		}
		results := []ResultLabel{{Key: "result", Value: string(rune('a' + i%4))}}
		combined.addStats(input, results)
		if i%2 == 0 {
			left.addStats(input, results)
		} else {
			right.addStats(input, results)
		}
	}

	left.stats.merge(right.stats, left.inputIndexesForResult)
	assertStatsClose(t, combined.stats, left.stats)
}

func assertStatsClose(t *testing.T, want, got *statsSnapshot) {
	t.Helper()
	if want.TotalCount != got.TotalCount {
		t.Fatalf("TotalCount=%d want %d", got.TotalCount, want.TotalCount)
	}
	if len(want.LabelStats) != len(got.LabelStats) {
		t.Fatalf("LabelStats len=%d want %d", len(got.LabelStats), len(want.LabelStats))
	}
	for id, wantLabel := range want.LabelStats {
		gotLabel := got.LabelStats[id]
		if gotLabel == nil {
			t.Fatalf("missing label %q", id)
		}
		if wantLabel.Count != gotLabel.Count {
			t.Fatalf("label %q count=%d want %d", id, gotLabel.Count, wantLabel.Count)
		}
		wantNumerics := numericStatsByIndex(wantLabel)
		gotNumerics := numericStatsByIndex(gotLabel)
		if len(wantNumerics) != len(gotNumerics) {
			t.Fatalf("label %q numeric len=%d want %d", id, len(gotNumerics), len(wantNumerics))
		}
		for index, wantStat := range wantNumerics {
			gotStat := gotNumerics[index]
			if gotStat == nil {
				t.Fatalf("label %q missing numeric index %d", id, index)
			}
			if wantStat.Count != gotStat.Count {
				t.Fatalf("label %q numeric %d count=%d want %d", id, index, gotStat.Count, wantStat.Count)
			}
			assertFloatClose(t, gotStat.Mean, wantStat.Mean, "mean")
			assertFloatClose(t, gotStat.M2, wantStat.M2, "m2")
			assertFloatClose(t, gotStat.Min, wantStat.Min, "min")
			assertFloatClose(t, gotStat.Max, wantStat.Max, "max")
		}
		wantCategories := categoryStatsByIndex(wantLabel)
		gotCategories := categoryStatsByIndex(gotLabel)
		if len(wantCategories) != len(gotCategories) {
			t.Fatalf("label %q category len=%d want %d", id, len(gotCategories), len(wantCategories))
		}
		for index, wantStat := range wantCategories {
			gotStat := gotCategories[index]
			if gotStat == nil {
				t.Fatalf("label %q missing category index %d", id, index)
			}
			if wantStat.Count != gotStat.Count {
				t.Fatalf("label %q category %d count=%d want %d", id, index, gotStat.Count, wantStat.Count)
			}
			if len(wantStat.Values) != len(gotStat.Values) {
				t.Fatalf("label %q category %d values len=%d want %d", id, index, len(gotStat.Values), len(wantStat.Values))
			}
			for value, wantCount := range wantStat.Values {
				if gotStat.Values[value] != wantCount {
					t.Fatalf("label %q category %d value %q count=%d want %d", id, index, value, gotStat.Values[value], wantCount)
				}
			}
		}
	}
}

func numericStatsByIndex(label *labelStats) map[int]*numericStats {
	out := make(map[int]*numericStats)
	label.forEachNumeric(func(index int, stat *numericStats) {
		out[index] = stat
	})
	return out
}

func categoryStatsByIndex(label *labelStats) map[int]*categoryStats {
	out := make(map[int]*categoryStats)
	label.forEachCategory(func(index int, stat *categoryStats) {
		out[index] = stat
	})
	return out
}

func assertFloatClose(t *testing.T, got, want float64, name string) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("%s=%g want %g", name, got, want)
	}
}
