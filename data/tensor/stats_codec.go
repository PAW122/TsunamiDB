package tensor

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
)

var statsPayloadMagic = [4]byte{'T', 'S', 'T', '1'}

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
	if len(payload) >= len(statsPayloadMagic) && bytes.Equal(payload[:4], statsPayloadMagic[:]) {
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
	}
	return buf.Bytes(), nil
}

func decodeStatsPayload(schema Schema, payload []byte) (*statsSnapshot, error) {
	reader := bytes.NewReader(payload)
	var magic [4]byte
	if _, err := reader.Read(magic[:]); err != nil {
		return nil, err
	}
	if magic != statsPayloadMagic {
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
		stats.LabelStats[labelID(ResultLabel{Key: key, Value: value})] = label
	}
	return stats, nil
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
