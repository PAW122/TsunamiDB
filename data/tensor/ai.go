package tensor

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"sort"
)

const (
	aiModelVersion          = 1
	defaultAIEpochs         = 20
	defaultAIBatchSize      = 32
	defaultAILearningRate   = 0.03
	defaultAIWeightDecay    = 1e-4
	defaultAIInputDropout   = 0.10
	defaultAIValidation     = 0.10
	defaultAIPatience       = 4
	defaultAINormalizeClamp = 6
	defaultAIActivation     = "relu"
	defaultAIDevice         = "cpu"
)

type aiRawExample struct {
	input   []float64
	results map[string]string
}

type aiExample struct {
	features []float64
	labels   []int
}

type aiEvalResult struct {
	labelAccuracy float64
	exactAccuracy float64
	loss          float64
}

func (t *Table) TrainAI(samples []Sample, options AITrainOptions) (AITrainReport, error) {
	if !t.allFloatInputs {
		return AITrainReport{}, fmt.Errorf("%w: AI training currently requires float64 inputs", ErrInvalidSchema)
	}
	params := normalizeAITrainOptions(options)
	rawTrain, classValues, err := t.prepareAIRawExamples(samples, nil)
	if err != nil {
		return AITrainReport{}, err
	}
	if len(rawTrain) == 0 {
		return AITrainReport{}, ErrInvalidSample
	}
	t.addAIClassValuesFromStats(classValues)

	resultKeys := make([]string, len(t.schema.Results))
	for i, result := range t.schema.Results {
		resultKeys[i] = result.Key
		if len(classValues[result.Key]) == 0 {
			return AITrainReport{}, fmt.Errorf("%w: result %q has no AI classes", ErrInvalidSample, result.Key)
		}
	}
	classIndex := aiClassIndex(classValues)

	rng := rand.New(rand.NewSource(params.Seed))
	shuffleRawExamples(rawTrain, rng)
	rawValidation := make([]aiRawExample, 0)
	if len(params.ValidationSamples) != 0 {
		rawValidation, _, err = t.prepareAIRawExamples(params.ValidationSamples, classIndex)
		if err != nil {
			return AITrainReport{}, err
		}
	} else if params.ValidationSplit > 0 && len(rawTrain) > 1 {
		validationCount := int(math.Round(float64(len(rawTrain)) * params.ValidationSplit))
		if validationCount < 1 {
			validationCount = 1
		}
		if validationCount >= len(rawTrain) {
			validationCount = len(rawTrain) - 1
		}
		rawValidation = append(rawValidation, rawTrain[len(rawTrain)-validationCount:]...)
		rawTrain = rawTrain[:len(rawTrain)-validationCount]
	}

	mean, std := aiNormalization(rawTrain, len(t.schema.Inputs))
	train := aiBuildExamples(rawTrain, resultKeys, classIndex, mean, std)
	validation := aiBuildExamples(rawValidation, resultKeys, classIndex, mean, std)
	outputs := aiOutputCount(resultKeys, classValues)
	layers := newAIMLPLayers(len(t.schema.Inputs), params.HiddenSizes, outputs, rng)
	offsets := aiOutputOffsets(resultKeys, classValues)

	bestLayers := cloneDenseLayers(layers)
	bestEval := aiEvalResult{labelAccuracy: math.Inf(-1), loss: math.Inf(1)}
	bestEpoch := 0
	completedEpochs := 0
	noImprovement := 0
	totalBatches := params.Epochs * maxAIInt(1, (len(train)+params.BatchSize-1)/params.BatchSize)
	completedBatches := 0
	notifyAIProgress(params.Progress, completedBatches, totalBatches)

	indices := make([]int, len(train))
	for i := range indices {
		indices[i] = i
	}
	for epoch := 1; epoch <= params.Epochs; epoch++ {
		completedEpochs = epoch
		rng.Shuffle(len(indices), func(i, j int) {
			indices[i], indices[j] = indices[j], indices[i]
		})
		for start := 0; start < len(indices); start += params.BatchSize {
			end := start + params.BatchSize
			if end > len(indices) {
				end = len(indices)
			}
			aiTrainBatch(layers, train, indices[start:end], resultKeys, classValues, offsets, params, rng)
			completedBatches++
			notifyAIProgress(params.Progress, completedBatches, totalBatches)
		}

		scoreSet := train
		if len(validation) != 0 {
			scoreSet = validation
		}
		current := aiEvaluateLayers(layers, scoreSet, resultKeys, classValues, offsets)
		if current.labelAccuracy > bestEval.labelAccuracy+1e-12 ||
			(current.labelAccuracy == bestEval.labelAccuracy && current.loss < bestEval.loss) {
			bestEval = current
			bestLayers = cloneDenseLayers(layers)
			bestEpoch = epoch
			noImprovement = 0
		} else {
			noImprovement++
			if noImprovement >= params.Patience {
				break
			}
		}
	}
	notifyAIProgress(params.Progress, totalBatches, totalBatches)

	trainEval := aiEvaluateLayers(bestLayers, train, resultKeys, classValues, offsets)
	validationEval := bestEval
	if len(validation) == 0 {
		validationEval = trainEval
	}
	model := &AIModel{
		Version:     aiModelVersion,
		InputMean:   mean,
		InputStd:    std,
		ResultKeys:  append([]string(nil), resultKeys...),
		ClassValues: cloneAIClassValues(classValues),
		Layers:      bestLayers,
		Activation:  defaultAIActivation,
		Device:      params.Device,
		Metrics: AIMetrics{
			Samples:                 len(rawTrain) + len(rawValidation),
			TrainingSamples:         len(train),
			ValidationSamples:       len(validation),
			Epochs:                  completedEpochs,
			BestEpoch:               bestEpoch,
			TrainLabelAccuracy:      trainEval.labelAccuracy,
			ValidationLabelAccuracy: validationEval.labelAccuracy,
			ValidationExactAccuracy: validationEval.exactAccuracy,
			ValidationLoss:          validationEval.loss,
			Device:                  params.Device,
		},
	}
	t.aiModel = model

	modelSize := 0
	if raw, err := json.Marshal(model); err == nil {
		modelSize = len(raw)
	}
	return AITrainReport{
		Samples:                 model.Metrics.Samples,
		TrainingSamples:         model.Metrics.TrainingSamples,
		ValidationSamples:       model.Metrics.ValidationSamples,
		Epochs:                  model.Metrics.Epochs,
		BestEpoch:               model.Metrics.BestEpoch,
		TrainLabelAccuracy:      model.Metrics.TrainLabelAccuracy,
		ValidationLabelAccuracy: model.Metrics.ValidationLabelAccuracy,
		ValidationExactAccuracy: model.Metrics.ValidationExactAccuracy,
		ValidationLoss:          model.Metrics.ValidationLoss,
		ResultKeys:              append([]string(nil), resultKeys...),
		OutputClasses:           outputs,
		HiddenSizes:             append([]int(nil), params.HiddenSizes...),
		Device:                  params.Device,
		ModelSizeBytes:          modelSize,
	}, nil
}

func (t *Table) PredictAI(input map[string]any) (Prediction, error) {
	if t == nil || t.aiModel == nil {
		return Prediction{}, ErrNoModelData
	}
	values, err := t.validateFloatInput(input)
	if err != nil {
		return Prediction{}, err
	}
	model := t.aiModel
	if len(model.Layers) == 0 || len(model.InputMean) != len(values) || len(model.InputStd) != len(values) {
		return Prediction{}, ErrNoModelData
	}
	features := normalizeAIInput(values, model.InputMean, model.InputStd)
	logits := aiForward(model.Layers, features)
	offsets := aiOutputOffsets(model.ResultKeys, model.ClassValues)
	results := make([]PredictedResult, 0, len(model.ResultKeys))
	for _, key := range model.ResultKeys {
		classes := model.ClassValues[key]
		offset := offsets[key]
		if len(classes) == 0 || offset+len(classes) > len(logits) {
			continue
		}
		best := 0
		bestScore := logits[offset]
		for i := 1; i < len(classes); i++ {
			score := logits[offset+i]
			if score > bestScore {
				best = i
				bestScore = score
			}
		}
		probability := aiSoftmaxProbability(logits[offset:offset+len(classes)], best)
		samples := uint64(0)
		if t.stats != nil {
			if label := t.stats.LabelStats[labelID(ResultLabel{Key: key, Value: classes[best]})]; label != nil {
				samples = label.Count
			}
		}
		results = append(results, PredictedResult{
			Key:         key,
			Value:       classes[best],
			Probability: probability,
			Score:       bestScore,
			Samples:     samples,
		})
	}
	return Prediction{Results: results}, nil
}

func normalizeAITrainOptions(options AITrainOptions) AITrainOptions {
	if options.Epochs <= 0 {
		options.Epochs = defaultAIEpochs
	}
	if options.BatchSize <= 0 {
		options.BatchSize = defaultAIBatchSize
	}
	if len(options.HiddenSizes) == 0 {
		options.HiddenSizes = []int{64}
	} else {
		cleaned := make([]int, 0, len(options.HiddenSizes))
		for _, size := range options.HiddenSizes {
			if size > 0 {
				cleaned = append(cleaned, size)
			}
		}
		options.HiddenSizes = cleaned
	}
	if options.LearningRate <= 0 || math.IsNaN(options.LearningRate) || math.IsInf(options.LearningRate, 0) {
		options.LearningRate = defaultAILearningRate
	}
	if options.WeightDecay < 0 || math.IsNaN(options.WeightDecay) || math.IsInf(options.WeightDecay, 0) {
		options.WeightDecay = defaultAIWeightDecay
	}
	if options.InputDropout < 0 || options.InputDropout >= 1 || math.IsNaN(options.InputDropout) || math.IsInf(options.InputDropout, 0) {
		options.InputDropout = defaultAIInputDropout
	}
	if options.ValidationSplit < 0 || options.ValidationSplit >= 1 || math.IsNaN(options.ValidationSplit) || math.IsInf(options.ValidationSplit, 0) {
		options.ValidationSplit = defaultAIValidation
	}
	if len(options.ValidationSamples) != 0 {
		options.ValidationSplit = 0
	}
	if options.Patience <= 0 {
		options.Patience = defaultAIPatience
	}
	if options.Seed == 0 {
		options.Seed = 1
	}
	if options.Device == "" || options.Device == "auto" || options.Device == "gpu" {
		options.Device = defaultAIDevice
	}
	return options
}

func (t *Table) prepareAIRawExamples(samples []Sample, known map[string]map[string]int) ([]aiRawExample, map[string][]string, error) {
	classSets := make(map[string]map[string]struct{}, len(t.schema.Results))
	for _, result := range t.schema.Results {
		classSets[result.Key] = make(map[string]struct{})
	}
	examples := make([]aiRawExample, 0, len(samples))
	for i := range samples {
		sample := samples[i]
		if sample.LearningStatus != "" && !t.shouldLearn(sample.LearningStatus) {
			continue
		}
		input, results, err := t.validateFloatSample(sample)
		if err != nil {
			return nil, nil, err
		}
		if len(results) == 0 {
			continue
		}
		resultMap := make(map[string]string, len(results))
		for _, result := range results {
			if _, ok := t.resultIndexByKey[result.Key]; !ok {
				return nil, nil, fmt.Errorf("%w: unknown result %q", ErrInvalidSample, result.Key)
			}
			if known != nil {
				if _, ok := known[result.Key][result.Value]; !ok {
					return nil, nil, fmt.Errorf("%w: unknown AI class %q=%q", ErrInvalidSample, result.Key, result.Value)
				}
			}
			resultMap[result.Key] = result.Value
			classSets[result.Key][result.Value] = struct{}{}
		}
		examples = append(examples, aiRawExample{input: input, results: resultMap})
	}
	classValues := make(map[string][]string, len(classSets))
	for key, values := range classSets {
		classValues[key] = make([]string, 0, len(values))
		for value := range values {
			classValues[key] = append(classValues[key], value)
		}
		sort.Strings(classValues[key])
	}
	return examples, classValues, nil
}

func (t *Table) addAIClassValuesFromStats(classValues map[string][]string) {
	if t == nil || t.stats == nil || len(t.stats.LabelStats) == 0 {
		return
	}
	seen := make(map[string]map[string]struct{}, len(classValues))
	for key, values := range classValues {
		seen[key] = make(map[string]struct{}, len(values))
		for _, value := range values {
			seen[key][value] = struct{}{}
		}
	}
	for _, label := range t.stats.LabelStats {
		if label == nil || label.Key == "" || label.Value == "" {
			continue
		}
		if seen[label.Key] == nil {
			seen[label.Key] = make(map[string]struct{})
		}
		if _, ok := seen[label.Key][label.Value]; ok {
			continue
		}
		seen[label.Key][label.Value] = struct{}{}
		classValues[label.Key] = append(classValues[label.Key], label.Value)
	}
	for key := range classValues {
		sort.Strings(classValues[key])
	}
}

func aiClassIndex(classValues map[string][]string) map[string]map[string]int {
	index := make(map[string]map[string]int, len(classValues))
	for key, values := range classValues {
		index[key] = make(map[string]int, len(values))
		for i, value := range values {
			index[key][value] = i
		}
	}
	return index
}

func aiBuildExamples(raw []aiRawExample, resultKeys []string, classIndex map[string]map[string]int, mean, std []float64) []aiExample {
	examples := make([]aiExample, 0, len(raw))
	for _, source := range raw {
		labels := make([]int, len(resultKeys))
		for i := range labels {
			labels[i] = -1
		}
		for i, key := range resultKeys {
			if value, ok := source.results[key]; ok {
				labels[i] = classIndex[key][value]
			}
		}
		examples = append(examples, aiExample{
			features: normalizeAIInput(source.input, mean, std),
			labels:   labels,
		})
	}
	return examples
}

func aiNormalization(examples []aiRawExample, inputCount int) ([]float64, []float64) {
	mean := make([]float64, inputCount)
	std := make([]float64, inputCount)
	if len(examples) == 0 {
		for i := range std {
			std[i] = 1
		}
		return mean, std
	}
	for _, sample := range examples {
		for i, value := range sample.input {
			mean[i] += value
		}
	}
	for i := range mean {
		mean[i] /= float64(len(examples))
	}
	for _, sample := range examples {
		for i, value := range sample.input {
			diff := value - mean[i]
			std[i] += diff * diff
		}
	}
	for i := range std {
		std[i] = math.Sqrt(std[i] / float64(len(examples)))
		if std[i] < 1e-9 || math.IsNaN(std[i]) || math.IsInf(std[i], 0) {
			std[i] = 1
		}
	}
	return mean, std
}

func normalizeAIInput(input, mean, std []float64) []float64 {
	out := make([]float64, len(input))
	for i, value := range input {
		scale := 1.0
		if i < len(std) && std[i] > 1e-9 {
			scale = std[i]
		}
		center := 0.0
		if i < len(mean) {
			center = mean[i]
		}
		normalized := (value - center) / scale
		if normalized > defaultAINormalizeClamp {
			normalized = defaultAINormalizeClamp
		} else if normalized < -defaultAINormalizeClamp {
			normalized = -defaultAINormalizeClamp
		}
		out[i] = normalized
	}
	return out
}

func newAIDenseLayer(in, out int, rng *rand.Rand) DenseLayer {
	layer := DenseLayer{
		In:      in,
		Out:     out,
		Weights: make([]float64, in*out),
		Bias:    make([]float64, out),
	}
	scale := math.Sqrt(2 / float64(in+out))
	for i := range layer.Weights {
		layer.Weights[i] = rng.NormFloat64() * scale
	}
	return layer
}

func newAIMLPLayers(inputSize int, hiddenSizes []int, outputSize int, rng *rand.Rand) []DenseLayer {
	sizes := make([]int, 0, len(hiddenSizes)+2)
	sizes = append(sizes, inputSize)
	sizes = append(sizes, hiddenSizes...)
	sizes = append(sizes, outputSize)
	layers := make([]DenseLayer, 0, len(sizes)-1)
	for i := 0; i < len(sizes)-1; i++ {
		layers = append(layers, newAIDenseLayer(sizes[i], sizes[i+1], rng))
	}
	return layers
}

func aiTrainBatch(layers []DenseLayer, examples []aiExample, indices []int, resultKeys []string, classValues map[string][]string, offsets map[string]int, options AITrainOptions, rng *rand.Rand) {
	if len(layers) == 0 || len(indices) == 0 {
		return
	}
	gradW := make([][]float64, len(layers))
	gradB := make([][]float64, len(layers))
	for i := range layers {
		gradW[i] = make([]float64, len(layers[i].Weights))
		gradB[i] = make([]float64, len(layers[i].Bias))
	}
	labelsSeen := 0
	for _, index := range indices {
		example := examples[index]
		input := append([]float64(nil), example.features...)
		if options.InputDropout > 0 {
			keepScale := 1 / (1 - options.InputDropout)
			for i := range input {
				if rng.Float64() < options.InputDropout {
					input[i] = 0
				} else {
					input[i] *= keepScale
				}
			}
		}
		activations, preActivations := aiForwardTrace(layers, input)
		logits := activations[len(activations)-1]
		present := 0
		for _, label := range example.labels {
			if label >= 0 {
				present++
			}
		}
		if present == 0 {
			continue
		}
		sampleScale := 1 / float64(present)
		outputDelta := make([]float64, len(logits))
		for resultIndex, key := range resultKeys {
			expected := example.labels[resultIndex]
			if expected < 0 {
				continue
			}
			offset := offsets[key]
			size := len(classValues[key])
			grad := aiSoftmaxGradient(logits[offset:offset+size], expected)
			for class := 0; class < size; class++ {
				outputDelta[offset+class] += grad[class] * sampleScale
			}
			labelsSeen++
		}
		aiAccumulateGradients(layers, activations, preActivations, outputDelta, gradW, gradB)
	}
	if labelsSeen == 0 {
		return
	}
	scale := 1 / float64(len(indices))
	for layerIndex := range layers {
		for i := range layers[layerIndex].Weights {
			layers[layerIndex].Weights[i] -= options.LearningRate * (gradW[layerIndex][i]*scale + options.WeightDecay*layers[layerIndex].Weights[i])
		}
		for i := range layers[layerIndex].Bias {
			layers[layerIndex].Bias[i] -= options.LearningRate * gradB[layerIndex][i] * scale
		}
	}
}

func aiEvaluateLayers(layers []DenseLayer, examples []aiExample, resultKeys []string, classValues map[string][]string, offsets map[string]int) aiEvalResult {
	if len(examples) == 0 {
		return aiEvalResult{}
	}
	correct := 0
	total := 0
	exact := 0
	totalLoss := 0.0
	for _, example := range examples {
		logits := aiForward(layers, example.features)
		sampleCorrect := 0
		sampleTotal := 0
		for resultIndex, key := range resultKeys {
			expected := example.labels[resultIndex]
			if expected < 0 {
				continue
			}
			offset := offsets[key]
			size := len(classValues[key])
			group := logits[offset : offset+size]
			best := aiArgmax(group)
			if best == expected {
				correct++
				sampleCorrect++
			}
			totalLoss += aiCrossEntropy(group, expected)
			total++
			sampleTotal++
		}
		if sampleTotal > 0 && sampleCorrect == sampleTotal {
			exact++
		}
	}
	if total == 0 {
		return aiEvalResult{}
	}
	return aiEvalResult{
		labelAccuracy: float64(correct) / float64(total),
		exactAccuracy: float64(exact) / float64(len(examples)),
		loss:          totalLoss / float64(total),
	}
}

func aiForwardTrace(layers []DenseLayer, input []float64) ([][]float64, [][]float64) {
	activations := make([][]float64, len(layers)+1)
	preActivations := make([][]float64, len(layers))
	activations[0] = input
	for i, layer := range layers {
		z := make([]float64, layer.Out)
		aiForwardDenseInto(layer, activations[i], z)
		preActivations[i] = z
		if i == len(layers)-1 {
			activations[i+1] = z
			continue
		}
		a := make([]float64, len(z))
		for j, value := range z {
			if value > 0 {
				a[j] = value
			}
		}
		activations[i+1] = a
	}
	return activations, preActivations
}

func aiAccumulateGradients(layers []DenseLayer, activations, preActivations [][]float64, outputDelta []float64, gradW, gradB [][]float64) {
	delta := outputDelta
	for layerIndex := len(layers) - 1; layerIndex >= 0; layerIndex-- {
		layer := layers[layerIndex]
		input := activations[layerIndex]
		for out, value := range delta {
			gradB[layerIndex][out] += value
			weightBase := out * layer.In
			for in := 0; in < layer.In; in++ {
				gradW[layerIndex][weightBase+in] += value * input[in]
			}
		}
		if layerIndex == 0 {
			break
		}
		prev := make([]float64, layer.In)
		for in := 0; in < layer.In; in++ {
			sum := 0.0
			for out, value := range delta {
				sum += layer.Weights[out*layer.In+in] * value
			}
			if preActivations[layerIndex-1][in] <= 0 {
				sum = 0
			}
			prev[in] = sum
		}
		delta = prev
	}
}

func aiForward(layers []DenseLayer, input []float64) []float64 {
	activations := append([]float64(nil), input...)
	for i, layer := range layers {
		next := make([]float64, layer.Out)
		aiForwardDenseInto(layer, activations, next)
		if i != len(layers)-1 {
			for j := range next {
				if next[j] < 0 {
					next[j] = 0
				}
			}
		}
		activations = next
	}
	return activations
}

func aiForwardDenseInto(layer DenseLayer, input, output []float64) {
	for out := 0; out < layer.Out; out++ {
		sum := layer.Bias[out]
		weightBase := out * layer.In
		for in := 0; in < layer.In; in++ {
			sum += layer.Weights[weightBase+in] * input[in]
		}
		output[out] = sum
	}
}

func aiSoftmaxGradient(logits []float64, expected int) []float64 {
	grad := make([]float64, len(logits))
	maxValue := logits[0]
	for _, value := range logits[1:] {
		if value > maxValue {
			maxValue = value
		}
	}
	sum := 0.0
	for i, value := range logits {
		prob := math.Exp(value - maxValue)
		grad[i] = prob
		sum += prob
	}
	if sum == 0 || math.IsNaN(sum) || math.IsInf(sum, 0) {
		p := 1 / float64(len(grad))
		for i := range grad {
			grad[i] = p
		}
	} else {
		for i := range grad {
			grad[i] /= sum
		}
	}
	if expected >= 0 && expected < len(grad) {
		grad[expected] -= 1
	}
	return grad
}

func aiCrossEntropy(logits []float64, expected int) float64 {
	if expected < 0 || expected >= len(logits) {
		return 0
	}
	maxValue := logits[0]
	for _, value := range logits[1:] {
		if value > maxValue {
			maxValue = value
		}
	}
	sum := 0.0
	for _, value := range logits {
		sum += math.Exp(value - maxValue)
	}
	if sum <= 0 || math.IsNaN(sum) || math.IsInf(sum, 0) {
		return 0
	}
	return -logits[expected] + maxValue + math.Log(sum)
}

func aiSoftmaxProbability(logits []float64, index int) float64 {
	if index < 0 || index >= len(logits) {
		return 0
	}
	maxValue := logits[0]
	for _, value := range logits[1:] {
		if value > maxValue {
			maxValue = value
		}
	}
	sum := 0.0
	for _, value := range logits {
		sum += math.Exp(value - maxValue)
	}
	if sum <= 0 || math.IsNaN(sum) || math.IsInf(sum, 0) {
		return 1 / float64(len(logits))
	}
	return math.Exp(logits[index]-maxValue) / sum
}

func aiArgmax(values []float64) int {
	best := 0
	bestValue := values[0]
	for i := 1; i < len(values); i++ {
		if values[i] > bestValue {
			best = i
			bestValue = values[i]
		}
	}
	return best
}

func aiOutputOffsets(resultKeys []string, classValues map[string][]string) map[string]int {
	offsets := make(map[string]int, len(resultKeys))
	offset := 0
	for _, key := range resultKeys {
		offsets[key] = offset
		offset += len(classValues[key])
	}
	return offsets
}

func aiOutputCount(resultKeys []string, classValues map[string][]string) int {
	total := 0
	for _, key := range resultKeys {
		total += len(classValues[key])
	}
	return total
}

func cloneDenseLayer(source DenseLayer) DenseLayer {
	return DenseLayer{
		In:      source.In,
		Out:     source.Out,
		Weights: append([]float64(nil), source.Weights...),
		Bias:    append([]float64(nil), source.Bias...),
	}
}

func cloneDenseLayers(source []DenseLayer) []DenseLayer {
	clone := make([]DenseLayer, len(source))
	for i := range source {
		clone[i] = cloneDenseLayer(source[i])
	}
	return clone
}

func cloneAIClassValues(source map[string][]string) map[string][]string {
	clone := make(map[string][]string, len(source))
	for key, values := range source {
		clone[key] = append([]string(nil), values...)
	}
	return clone
}

func shuffleRawExamples(examples []aiRawExample, rng *rand.Rand) {
	rng.Shuffle(len(examples), func(i, j int) {
		examples[i], examples[j] = examples[j], examples[i]
	})
}

func notifyAIProgress(fn func(completed, total int), completed, total int) {
	if fn != nil {
		fn(completed, total)
	}
}

func maxAIInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
