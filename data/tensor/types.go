package tensor

import "errors"

const (
	InputTypeFloat64 = "float64"
	InputTypeInt64   = "int64"
	InputTypeUint64  = "uint64"
	InputTypeBool    = "bool"
	InputTypeString  = "string"

	DefaultSampleEntrySize uint64 = 1 << 10

	TestStatusPass = "pass"
	TestStatusFail = "fail"

	LearningStatusPositive = "positive"
	LearningStatusNegative = "negative"
	LearningStatusUnknown  = "unknown"
)

var (
	ErrInvalidSchema  = errors.New("tensor: invalid schema")
	ErrTableExists    = errors.New("tensor: table already exists")
	ErrTableNotFound  = errors.New("tensor: table not found")
	ErrInvalidSample  = errors.New("tensor: invalid sample")
	ErrInvalidInput   = errors.New("tensor: invalid input")
	ErrNoModelData    = errors.New("tensor: no model data")
	ErrSampleTooLarge = errors.New("tensor: sample exceeds inc table entry size")
)

type Schema struct {
	Name            string        `json:"name"`
	IgnoreStatuses  []string      `json:"ignore_statuses,omitempty"`
	SampleEntrySize uint64        `json:"sample_entry_size,omitempty"`
	Inputs          []InputField  `json:"inputs"`
	Results         []ResultField `json:"results,omitempty"`
}

type InputField struct {
	Name      string `json:"name"`
	Type      string `json:"type"`
	ResultKey string `json:"result_key,omitempty"`
}

type ResultField struct {
	Key   string `json:"key"`
	Type  string `json:"type"`
	Multi bool   `json:"multi,omitempty"`
}

type ResultLabel struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type Sample struct {
	SampleID       string         `json:"sample_id,omitempty"`
	TestStatus     string         `json:"test_status,omitempty"`
	LearningStatus string         `json:"learning_status,omitempty"`
	Input          map[string]any `json:"input"`
	Results        []ResultLabel  `json:"results,omitempty"`
}

type Prediction struct {
	Results []PredictedResult `json:"results"`
}

type PredictedResult struct {
	Key         string      `json:"key"`
	Value       string      `json:"value"`
	Probability float64     `json:"probability"`
	Score       float64     `json:"score"`
	Samples     uint64      `json:"samples"`
	Influences  []Influence `json:"influences,omitempty"`
}

type Influence struct {
	Input  string  `json:"input"`
	Impact float64 `json:"impact"`
	Reason string  `json:"reason"`
}
