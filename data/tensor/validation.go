package tensor

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
)

func validateSchema(schema Schema) (Schema, error) {
	if !validName.MatchString(schema.Name) || len(schema.Inputs) == 0 {
		return Schema{}, ErrInvalidSchema
	}

	inputs := make(map[string]struct{}, len(schema.Inputs))
	for _, input := range schema.Inputs {
		if !validName.MatchString(input.Name) || !isInputType(input.Type) {
			return Schema{}, fmt.Errorf("%w: input %q", ErrInvalidSchema, input.Name)
		}
		if input.ResultKey != "" && !validName.MatchString(input.ResultKey) {
			return Schema{}, fmt.Errorf("%w: input %q result_key", ErrInvalidSchema, input.Name)
		}
		if _, exists := inputs[input.Name]; exists {
			return Schema{}, fmt.Errorf("%w: duplicate input %q", ErrInvalidSchema, input.Name)
		}
		inputs[input.Name] = struct{}{}
	}

	results := make(map[string]struct{}, len(schema.Results))
	for _, result := range schema.Results {
		if !validName.MatchString(result.Key) || result.Type != InputTypeString {
			return Schema{}, fmt.Errorf("%w: result %q", ErrInvalidSchema, result.Key)
		}
		if _, exists := results[result.Key]; exists {
			return Schema{}, fmt.Errorf("%w: duplicate result %q", ErrInvalidSchema, result.Key)
		}
		results[result.Key] = struct{}{}
	}
	for _, input := range schema.Inputs {
		if input.ResultKey == "" {
			continue
		}
		if _, exists := results[input.ResultKey]; !exists {
			return Schema{}, fmt.Errorf("%w: input %q references unknown result_key %q", ErrInvalidSchema, input.Name, input.ResultKey)
		}
	}

	if len(schema.IgnoreStatuses) == 0 {
		schema.IgnoreStatuses = []string{LearningStatusUnknown}
	}
	minEntrySize := minSampleEntrySize(schema)
	if schema.SampleEntrySize == 0 {
		schema.SampleEntrySize = DefaultSampleEntrySize
		if schema.SampleEntrySize < minEntrySize {
			schema.SampleEntrySize = minEntrySize
		}
	}
	if schema.SampleEntrySize < minEntrySize {
		return Schema{}, fmt.Errorf("%w: sample_entry_size too small", ErrInvalidSchema)
	}
	return schema, nil
}

func minSampleEntrySize(schema Schema) uint64 {
	size := uint64(256)
	for _, input := range schema.Inputs {
		switch input.Type {
		case InputTypeFloat64, InputTypeInt64, InputTypeUint64:
			size += 8
		case InputTypeBool:
			size++
		case InputTypeString:
			size += 64
		}
	}
	size += uint64(len(schema.Results)) * 16
	if size < 1024 {
		return 1024
	}
	return size
}

func isInputType(kind string) bool {
	switch kind {
	case InputTypeFloat64, InputTypeInt64, InputTypeUint64, InputTypeBool, InputTypeString:
		return true
	default:
		return false
	}
}

func (t *Table) validateInput(input map[string]any) (normalizedInput, error) {
	if input == nil {
		return nil, ErrInvalidInput
	}
	normalized := make(normalizedInput, len(t.schema.Inputs))
	for i, field := range t.schema.Inputs {
		value, ok := input[field.Name]
		if !ok {
			return nil, fmt.Errorf("%w: missing input %q", ErrInvalidInput, field.Name)
		}
		typed, err := normalizeValue(value, field.Type)
		if err != nil {
			return nil, fmt.Errorf("%w: input %q: %v", ErrInvalidInput, field.Name, err)
		}
		normalized[i] = typed
	}
	return normalized, nil
}

func (t *Table) validateFloatInput(input map[string]any) ([]float64, error) {
	if input == nil {
		return nil, ErrInvalidInput
	}
	normalized := make([]float64, len(t.schema.Inputs))
	for i, field := range t.schema.Inputs {
		value, ok := input[field.Name]
		if !ok {
			return nil, fmt.Errorf("%w: missing input %q", ErrInvalidInput, field.Name)
		}
		typed, err := toFloat64(value)
		if err != nil {
			return nil, fmt.Errorf("%w: input %q: %v", ErrInvalidInput, field.Name, err)
		}
		normalized[i] = typed
	}
	return normalized, nil
}

func (t *Table) validateSample(sample Sample) (normalizedInput, []ResultLabel, error) {
	input, err := t.validateInput(sample.Input)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", ErrInvalidSample, err)
	}
	if len(sample.Results) == 0 && t.shouldLearn(sample.LearningStatus) {
		return nil, nil, fmt.Errorf("%w: positive learning sample needs results", ErrInvalidSample)
	}
	for _, result := range sample.Results {
		if !validName.MatchString(result.Key) || result.Value == "" {
			return nil, nil, fmt.Errorf("%w: invalid result", ErrInvalidSample)
		}
	}
	return input, sample.Results, nil
}

func (t *Table) validateFloatSample(sample Sample) ([]float64, []ResultLabel, error) {
	if sample.Input == nil {
		return nil, nil, ErrInvalidInput
	}
	input := make([]float64, len(t.schema.Inputs))
	for i, field := range t.schema.Inputs {
		value, ok := sample.Input[field.Name]
		if !ok {
			return nil, nil, fmt.Errorf("%w: missing input %q", ErrInvalidInput, field.Name)
		}
		typed, err := toFloat64(value)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: input %q: %v", ErrInvalidInput, field.Name, err)
		}
		input[i] = typed
	}
	if len(sample.Results) == 0 && t.shouldLearn(sample.LearningStatus) {
		return nil, nil, fmt.Errorf("%w: positive learning sample needs results", ErrInvalidSample)
	}
	for _, result := range sample.Results {
		if !validName.MatchString(result.Key) || result.Value == "" {
			return nil, nil, fmt.Errorf("%w: invalid result", ErrInvalidSample)
		}
	}
	return input, sample.Results, nil
}

func normalizeValue(value any, kind string) (any, error) {
	switch kind {
	case InputTypeFloat64:
		return toFloat64(value)
	case InputTypeInt64:
		v, ok := toInt64(value)
		if !ok {
			return nil, fmt.Errorf("want int64-compatible value")
		}
		return v, nil
	case InputTypeUint64:
		v, ok := toUint64(value)
		if !ok {
			return nil, fmt.Errorf("want uint64-compatible value")
		}
		return v, nil
	case InputTypeBool:
		v, ok := value.(bool)
		if !ok {
			return nil, fmt.Errorf("want bool")
		}
		return v, nil
	case InputTypeString:
		v, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("want string")
		}
		return v, nil
	default:
		return nil, ErrInvalidSchema
	}
}

func numericValue(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, !math.IsNaN(v) && !math.IsInf(v, 0)
	case int64:
		return float64(v), true
	case uint64:
		return float64(v), true
	default:
		return 0, false
	}
}

func categoryValue(value any) string {
	switch v := value.(type) {
	case bool:
		if v {
			return "true"
		}
		return "false"
	case string:
		return v
	default:
		return fmt.Sprint(v)
	}
}

func toFloat64(value any) (float64, error) {
	switch v := value.(type) {
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return 0, fmt.Errorf("not finite")
		}
		return v, nil
	case float32:
		out := float64(v)
		if math.IsNaN(out) || math.IsInf(out, 0) {
			return 0, fmt.Errorf("not finite")
		}
		return out, nil
	case int:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case uint64:
		return float64(v), nil
	case json.Number:
		return strconv.ParseFloat(v.String(), 64)
	default:
		return 0, fmt.Errorf("want float64-compatible value")
	}
}

func toInt64(value any) (int64, bool) {
	switch v := value.(type) {
	case int:
		return int64(v), true
	case int64:
		return v, true
	case int32:
		return int64(v), true
	case float64:
		if math.Trunc(v) == v && v >= float64(-1<<63) && v <= float64(1<<63-1) {
			return int64(v), true
		}
	}
	return 0, false
}

func toUint64(value any) (uint64, bool) {
	switch v := value.(type) {
	case uint:
		return uint64(v), true
	case uint64:
		return v, true
	case uint32:
		return uint64(v), true
	case int:
		return uint64(v), v >= 0
	case int64:
		return uint64(v), v >= 0
	case float64:
		if math.Trunc(v) == v && v >= 0 && v <= float64(^uint64(0)) {
			return uint64(v), true
		}
	}
	return 0, false
}
