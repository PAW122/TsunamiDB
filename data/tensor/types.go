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

type TuneOptions struct {
	Iterations     int                        `json:"iterations,omitempty"`
	LearningRate   float64                    `json:"learning_rate,omitempty"`
	Regularization float64                    `json:"regularization,omitempty"`
	MinWeight      float64                    `json:"min_weight,omitempty"`
	MaxWeight      float64                    `json:"max_weight,omitempty"`
	Progress       func(completed, total int) `json:"-"`
}

type TuneReport struct {
	Samples            int                            `json:"samples"`
	Iterations         int                            `json:"iterations"`
	AccuracyBefore     float64                        `json:"accuracy_before"`
	AccuracyAfter      float64                        `json:"accuracy_after"`
	Corrections        int                            `json:"corrections"`
	Adjustments        int                            `json:"adjustments"`
	ErrorsByResult     map[string]int                 `json:"errors_by_result,omitempty"`
	TopBoosted         map[string][]GateWeightSummary `json:"top_boosted,omitempty"`
	TopSuppressed      map[string][]GateWeightSummary `json:"top_suppressed,omitempty"`
	TopClassBoosted    map[string][]GateWeightSummary `json:"top_class_boosted,omitempty"`
	TopClassSuppressed map[string][]GateWeightSummary `json:"top_class_suppressed,omitempty"`
}

type AITrainOptions struct {
	Epochs            int                        `json:"epochs,omitempty"`
	BatchSize         int                        `json:"batch_size,omitempty"`
	HiddenSizes       []int                      `json:"hidden_sizes,omitempty"`
	LearningRate      float64                    `json:"learning_rate,omitempty"`
	WeightDecay       float64                    `json:"weight_decay,omitempty"`
	InputDropout      float64                    `json:"input_dropout,omitempty"`
	ValidationSplit   float64                    `json:"validation_split,omitempty"`
	ValidationSamples []Sample                   `json:"-"`
	Patience          int                        `json:"patience,omitempty"`
	Seed              int64                      `json:"seed,omitempty"`
	Device            string                     `json:"device,omitempty"`
	Progress          func(completed, total int) `json:"-"`
}

type AITrainReport struct {
	Samples                 int      `json:"samples"`
	TrainingSamples         int      `json:"training_samples"`
	ValidationSamples       int      `json:"validation_samples"`
	Epochs                  int      `json:"epochs"`
	BestEpoch               int      `json:"best_epoch"`
	TrainLabelAccuracy      float64  `json:"train_label_accuracy"`
	ValidationLabelAccuracy float64  `json:"validation_label_accuracy"`
	ValidationExactAccuracy float64  `json:"validation_exact_accuracy"`
	ValidationLoss          float64  `json:"validation_loss"`
	ResultKeys              []string `json:"result_keys,omitempty"`
	OutputClasses           int      `json:"output_classes"`
	HiddenSizes             []int    `json:"hidden_sizes,omitempty"`
	Device                  string   `json:"device"`
	ModelSizeBytes          int      `json:"model_size_bytes"`
}

type AIMetrics struct {
	Samples                 int     `json:"samples"`
	TrainingSamples         int     `json:"training_samples"`
	ValidationSamples       int     `json:"validation_samples"`
	Epochs                  int     `json:"epochs"`
	BestEpoch               int     `json:"best_epoch"`
	TrainLabelAccuracy      float64 `json:"train_label_accuracy"`
	ValidationLabelAccuracy float64 `json:"validation_label_accuracy"`
	ValidationExactAccuracy float64 `json:"validation_exact_accuracy"`
	ValidationLoss          float64 `json:"validation_loss"`
	Device                  string  `json:"device,omitempty"`
}

type AIModel struct {
	Version     uint32              `json:"version"`
	InputMean   []float64           `json:"input_mean"`
	InputStd    []float64           `json:"input_std"`
	ResultKeys  []string            `json:"result_keys"`
	ClassValues map[string][]string `json:"class_values"`
	Layers      []DenseLayer        `json:"layers"`
	Activation  string              `json:"activation,omitempty"`
	Device      string              `json:"device,omitempty"`
	Metrics     AIMetrics           `json:"metrics"`
}

type DenseLayer struct {
	In      int       `json:"in"`
	Out     int       `json:"out"`
	Weights []float64 `json:"weights"`
	Bias    []float64 `json:"bias"`
}

type GateWeightSummary struct {
	Input  string  `json:"input"`
	Index  int     `json:"index"`
	Weight float64 `json:"weight"`
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
