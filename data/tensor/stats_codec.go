package tensor

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
)

var (
	statsPayloadMagicV1 = [4]byte{'T', 'S', 'T', '1'}
	statsPayloadMagicV2 = [4]byte{'T', 'S', 'T', '2'}
	statsPayloadMagicV3 = [4]byte{'T', 'S', 'T', '3'}
	statsPayloadMagicV4 = [4]byte{'T', 'S', 'T', '4'}
	statsPayloadMagic   = [4]byte{'T', 'S', 'T', '5'}
)

func writeStatsSnapshot(key string, schema Schema, stats *statsSnapshot) error {
	payload, err := encodeStatsPayload(schema, stats)
	if err != nil {
		return err
	}
	return writeKVBytes(key, payload)
}

func readStatsSnapshot(key string, schema Schema, stats *statsSnapshot) error {
	payload, err := readKVBytes(key)
	if err != nil {
		return err
	}
	if len(payload) >= len(statsPayloadMagic) &&
		(bytes.Equal(payload[:4], statsPayloadMagic[:]) ||
			bytes.Equal(payload[:4], statsPayloadMagicV4[:]) ||
			bytes.Equal(payload[:4], statsPayloadMagicV3[:]) ||
			bytes.Equal(payload[:4], statsPayloadMagicV2[:]) ||
			bytes.Equal(payload[:4], statsPayloadMagicV1[:])) {
		decoded, err := decodeStatsPayload(schema, payload)
		if err != nil {
			return err
		}
		*stats = *decoded
		return nil
	}
	return errors.New("tensor: unsupported stats snapshot")
}

func encodeStatsPayload(schema Schema, stats *statsSnapshot) ([]byte, error) {
	var buf bytes.Buffer
	buf.Write(statsPayloadMagic[:])
	writeString(&buf, stats.Table)
	writeUvarint(&buf, stats.TotalCount)
	writeUvarint(&buf, uint64(len(stats.LabelStats)))

	resultIndexes := resultKeyIndexes(schema)
	for _, label := range stats.LabelStats {
		writeIndexedString(&buf, label.Key, resultIndexes)
		writeString(&buf, label.Value)
		writeUvarint(&buf, label.Count)

		numericCount := label.numericStatsCount()
		writeUvarint(&buf, uint64(numericCount))
		label.forEachNumeric(func(index int, stat *numericStats) {
			writeUvarint(&buf, uint64(index+1))
			writeUvarint(&buf, stat.Count)
			writeFloat64(&buf, stat.Mean)
			writeFloat64(&buf, stat.M2)
			writeFloat64(&buf, stat.Min)
			writeFloat64(&buf, stat.Max)
		})

		categoryCount := label.categoryStatsCount()
		writeUvarint(&buf, uint64(categoryCount))
		label.forEachCategory(func(index int, stat *categoryStats) {
			writeUvarint(&buf, uint64(index+1))
			writeUvarint(&buf, stat.Count)
			writeUvarint(&buf, uint64(len(stat.Values)))
			for value, count := range stat.Values {
				writeString(&buf, value)
				writeUvarint(&buf, count)
			}
		})

		writeUvarint(&buf, uint64(len(label.Weights)))
		for index, weight := range label.Weights {
			writeUvarint(&buf, uint64(index+1))
			writeFloat64(&buf, weight)
		}
	}

	writeGateSet(&buf, stats.ResultGates, resultIndexes)
	writeLabelGateSet(&buf, stats.LabelGates, resultIndexes)
	return buf.Bytes(), nil
}

func writeGateSet(buf *bytes.Buffer, gates map[string]*resultGate, resultIndexes map[string]int) {
	writeUvarint(buf, uint64(len(gates)))
	for key, gate := range gates {
		writeIndexedString(buf, key, resultIndexes)
		writeInputWeights(buf, gate)
	}
}

func writeLabelGateSet(buf *bytes.Buffer, gates map[string]*resultGate, resultIndexes map[string]int) {
	writeUvarint(buf, uint64(len(gates)))
	for id, gate := range gates {
		key, value := splitLabelID(id)
		writeIndexedString(buf, key, resultIndexes)
		writeString(buf, value)
		writeInputWeights(buf, gate)
	}
}

func writeInputWeights(buf *bytes.Buffer, gate *resultGate) {
	if gate == nil {
		writeUvarint(buf, 0)
		writeFloat64(buf, 0)
		return
	}
	writeUvarint(buf, uint64(len(gate.InputWeights)))
	for index, weight := range gate.InputWeights {
		writeUvarint(buf, uint64(index+1))
		writeFloat64(buf, weight)
	}
	writeFloat64(buf, gate.Bias)
}

func decodeStatsPayload(schema Schema, payload []byte) (*statsSnapshot, error) {
	reader := bytes.NewReader(payload)
	var magic [4]byte
	if _, err := reader.Read(magic[:]); err != nil {
		return nil, err
	}
	withWeights := false
	withGates := false
	withLabelGates := false
	withGateBias := false
	switch magic {
	case statsPayloadMagic:
		withWeights = true
		withGates = true
		withLabelGates = true
		withGateBias = true
	case statsPayloadMagicV4:
		withWeights = true
		withGates = true
		withLabelGates = true
	case statsPayloadMagicV3:
		withWeights = true
		withGates = true
	case statsPayloadMagicV2:
		withWeights = true
	case statsPayloadMagicV1:
	default:
		return nil, errors.New("tensor: invalid stats payload")
	}

	table, err := readString(reader)
	if err != nil {
		return nil, err
	}
	total, err := binary.ReadUvarint(reader)
	if err != nil {
		return nil, err
	}
	labelCount, err := binary.ReadUvarint(reader)
	if err != nil {
		return nil, err
	}

	stats := newStats(schema)
	stats.Table = table
	stats.TotalCount = total
	resultNames := resultKeyNames(schema)
	legacyWeightSums := make(map[string]map[int]float64)
	legacyWeightCounts := make(map[string]map[int]int)
	for i := uint64(0); i < labelCount; i++ {
		key, err := readIndexedString(reader, resultNames)
		if err != nil {
			return nil, err
		}
		value, err := readString(reader)
		if err != nil {
			return nil, err
		}
		count, err := binary.ReadUvarint(reader)
		if err != nil {
			return nil, err
		}
		label := &labelStats{
			Key:        key,
			Value:      value,
			Count:      count,
			Numerics:   make(map[int]*numericStats),
			Categories: make(map[int]*categoryStats),
		}

		numericCount, err := binary.ReadUvarint(reader)
		if err != nil {
			return nil, err
		}
		for j := uint64(0); j < numericCount; j++ {
			index, err := readInputIndex(reader, len(schema.Inputs))
			if err != nil {
				return nil, err
			}
			statCount, err := binary.ReadUvarint(reader)
			if err != nil {
				return nil, err
			}
			mean, err := readFloat64(reader)
			if err != nil {
				return nil, err
			}
			m2, err := readFloat64(reader)
			if err != nil {
				return nil, err
			}
			min, err := readFloat64(reader)
			if err != nil {
				return nil, err
			}
			max, err := readFloat64(reader)
			if err != nil {
				return nil, err
			}
			label.Numerics[index] = &numericStats{Count: statCount, Mean: mean, M2: m2, Min: min, Max: max}
		}

		categoryCount, err := binary.ReadUvarint(reader)
		if err != nil {
			return nil, err
		}
		for j := uint64(0); j < categoryCount; j++ {
			index, err := readInputIndex(reader, len(schema.Inputs))
			if err != nil {
				return nil, err
			}
			statCount, err := binary.ReadUvarint(reader)
			if err != nil {
				return nil, err
			}
			valueCount, err := binary.ReadUvarint(reader)
			if err != nil {
				return nil, err
			}
			stat := &categoryStats{Count: statCount, Values: make(map[string]uint64, valueCount)}
			for k := uint64(0); k < valueCount; k++ {
				value, err := readString(reader)
				if err != nil {
					return nil, err
				}
				count, err := binary.ReadUvarint(reader)
				if err != nil {
					return nil, err
				}
				stat.Values[value] = count
			}
			label.Categories[index] = stat
		}
		if withWeights {
			weightCount, err := binary.ReadUvarint(reader)
			if err != nil {
				return nil, err
			}
			if weightCount != 0 {
				label.Weights = make(map[int]float64, weightCount)
			}
			for j := uint64(0); j < weightCount; j++ {
				index, err := readInputIndex(reader, len(schema.Inputs))
				if err != nil {
					return nil, err
				}
				weight, err := readFloat64(reader)
				if err != nil {
					return nil, err
				}
				label.Weights[index] = weight
				if !withGates && !math.IsNaN(weight) && !math.IsInf(weight, 0) {
					if legacyWeightSums[key] == nil {
						legacyWeightSums[key] = make(map[int]float64)
						legacyWeightCounts[key] = make(map[int]int)
					}
					legacyWeightSums[key][index] += weight
					legacyWeightCounts[key][index]++
				}
			}
		}
		stats.LabelStats[labelID(ResultLabel{Key: key, Value: value})] = label
	}
	if withGates {
		gates, err := readGateSet(reader, resultNames, len(schema.Inputs), withGateBias)
		if err != nil {
			return nil, err
		}
		stats.ResultGates = gates
		if withLabelGates {
			labelGates, err := readLabelGateSet(reader, resultNames, len(schema.Inputs), withGateBias)
			if err != nil {
				return nil, err
			}
			stats.LabelGates = labelGates
		}
	} else {
		seedResultGatesFromLegacyWeights(stats, legacyWeightSums, legacyWeightCounts)
	}
	return stats, nil
}

func readGateSet(reader *bytes.Reader, resultNames []string, inputCount int, withBias bool) (map[string]*resultGate, error) {
	gateCount, err := binary.ReadUvarint(reader)
	if err != nil {
		return nil, err
	}
	gates := make(map[string]*resultGate, gateCount)
	for i := uint64(0); i < gateCount; i++ {
		key, err := readIndexedString(reader, resultNames)
		if err != nil {
			return nil, err
		}
		gate, err := readInputWeights(reader, inputCount, withBias)
		if err != nil {
			return nil, err
		}
		gates[key] = gate
	}
	return gates, nil
}

func readLabelGateSet(reader *bytes.Reader, resultNames []string, inputCount int, withBias bool) (map[string]*resultGate, error) {
	gateCount, err := binary.ReadUvarint(reader)
	if err != nil {
		return nil, err
	}
	gates := make(map[string]*resultGate, gateCount)
	for i := uint64(0); i < gateCount; i++ {
		key, err := readIndexedString(reader, resultNames)
		if err != nil {
			return nil, err
		}
		value, err := readString(reader)
		if err != nil {
			return nil, err
		}
		gate, err := readInputWeights(reader, inputCount, withBias)
		if err != nil {
			return nil, err
		}
		gates[labelID(ResultLabel{Key: key, Value: value})] = gate
	}
	return gates, nil
}

func readInputWeights(reader *bytes.Reader, inputCount int, withBias bool) (*resultGate, error) {
	weightCount, err := binary.ReadUvarint(reader)
	if err != nil {
		return nil, err
	}
	gate := &resultGate{}
	if weightCount != 0 {
		gate.InputWeights = make(map[int]float64, weightCount)
	}
	for j := uint64(0); j < weightCount; j++ {
		index, err := readInputIndex(reader, inputCount)
		if err != nil {
			return nil, err
		}
		weight, err := readFloat64(reader)
		if err != nil {
			return nil, err
		}
		gate.InputWeights[index] = weight
	}
	if withBias {
		bias, err := readFloat64(reader)
		if err != nil {
			return nil, err
		}
		gate.Bias = bias
	}
	return gate, nil
}

func seedResultGatesFromLegacyWeights(stats *statsSnapshot, sums map[string]map[int]float64, counts map[string]map[int]int) {
	if stats == nil || len(sums) == 0 {
		return
	}
	if stats.ResultGates == nil {
		stats.ResultGates = make(map[string]*resultGate, len(sums))
	}
	for key, byInput := range sums {
		gate := stats.ResultGates[key]
		if gate == nil {
			gate = &resultGate{InputWeights: make(map[int]float64, len(byInput))}
			stats.ResultGates[key] = gate
		}
		if gate.InputWeights == nil {
			gate.InputWeights = make(map[int]float64, len(byInput))
		}
		for index, sum := range byInput {
			count := counts[key][index]
			if count <= 0 {
				continue
			}
			gate.InputWeights[index] = sum / float64(count)
		}
	}
}

func writeIndexedString(buf *bytes.Buffer, value string, indexes map[string]int) {
	if idx, ok := indexes[value]; ok {
		writeUvarint(buf, uint64(idx+1))
		return
	}
	writeUvarint(buf, 0)
	writeString(buf, value)
}

func readIndexedString(reader *bytes.Reader, names []string) (string, error) {
	idx, err := binary.ReadUvarint(reader)
	if err != nil {
		return "", err
	}
	if idx == 0 {
		return readString(reader)
	}
	pos := int(idx - 1)
	if pos < 0 || pos >= len(names) {
		return "", errors.New("tensor: indexed string out of range")
	}
	return names[pos], nil
}

func readInputIndex(reader *bytes.Reader, inputCount int) (int, error) {
	raw, err := binary.ReadUvarint(reader)
	if err != nil {
		return 0, err
	}
	if raw == 0 {
		return 0, errors.New("tensor: input index cannot be zero")
	}
	index := int(raw - 1)
	if index < 0 || index >= inputCount {
		return 0, errors.New("tensor: input index out of range")
	}
	return index, nil
}

func writeFloat64(buf *bytes.Buffer, value float64) {
	var tmp [8]byte
	binary.LittleEndian.PutUint64(tmp[:], math.Float64bits(value))
	buf.Write(tmp[:])
}

func readFloat64(reader *bytes.Reader) (float64, error) {
	raw, err := readUint64(reader)
	if err != nil {
		return 0, err
	}
	return math.Float64frombits(raw), nil
}

func resultKeyNames(schema Schema) []string {
	names := make([]string, len(schema.Results))
	for i, result := range schema.Results {
		names[i] = result.Key
	}
	return names
}
