package tensor

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"

	encoder_v1 "github.com/PAW122/TsunamiDB/encoding/v1"
)

var (
	samplePayloadMagic        = [4]byte{'T', 'S', 'M', '1'}
	samplePayloadMagicCompact = [4]byte{'T', 'S', 'M', '2'}
)

func encodeSampleFrame(schema Schema, sample Sample, input normalizedInput) ([]byte, error) {
	return encodeSamplePayload(schema, sample, input)
}

func encodeFloatSampleFrame(schema Schema, sample Sample, input []float64, resultIndexes map[string]int) ([]byte, error) {
	var buf bytes.Buffer
	buf.Write(samplePayloadMagicCompact[:])

	writeString(&buf, sample.SampleID)
	writeString(&buf, sample.TestStatus)
	writeString(&buf, sample.LearningStatus)

	if len(input) != len(schema.Inputs) {
		return nil, ErrInvalidSample
	}
	for i, value := range input {
		if i >= len(schema.Inputs) || schema.Inputs[i].Type != InputTypeFloat64 || math.IsNaN(value) || math.IsInf(value, 0) {
			return nil, ErrInvalidSample
		}
		var tmp [4]byte
		binary.LittleEndian.PutUint32(tmp[:], math.Float32bits(float32(value)))
		buf.Write(tmp[:])
	}

	writeUvarint(&buf, uint64(len(sample.Results)))
	for _, result := range sample.Results {
		if idx, ok := resultIndexes[result.Key]; ok {
			writeUvarint(&buf, uint64(idx+1))
		} else {
			writeUvarint(&buf, 0)
			writeString(&buf, result.Key)
		}
		writeString(&buf, result.Value)
	}

	return buf.Bytes(), nil
}

func encodeSamplePayload(schema Schema, sample Sample, input normalizedInput) ([]byte, error) {
	var buf bytes.Buffer
	buf.Write(samplePayloadMagicCompact[:])

	writeString(&buf, sample.SampleID)
	writeString(&buf, sample.TestStatus)
	writeString(&buf, sample.LearningStatus)

	for i, field := range schema.Inputs {
		if i < 0 || i >= len(input) {
			return nil, fmt.Errorf("%w: input %q", ErrInvalidSample, field.Name)
		}
		value := input[i]
		switch field.Type {
		case InputTypeFloat64:
			n, ok := value.(float64)
			if !ok || math.IsNaN(n) || math.IsInf(n, 0) {
				return nil, fmt.Errorf("%w: input %q", ErrInvalidSample, field.Name)
			}
			var tmp [4]byte
			binary.LittleEndian.PutUint32(tmp[:], math.Float32bits(float32(n)))
			buf.Write(tmp[:])
		case InputTypeInt64:
			n, ok := value.(int64)
			if !ok {
				return nil, fmt.Errorf("%w: input %q", ErrInvalidSample, field.Name)
			}
			var tmp [8]byte
			binary.LittleEndian.PutUint64(tmp[:], uint64(n))
			buf.Write(tmp[:])
		case InputTypeUint64:
			n, ok := value.(uint64)
			if !ok {
				return nil, fmt.Errorf("%w: input %q", ErrInvalidSample, field.Name)
			}
			var tmp [8]byte
			binary.LittleEndian.PutUint64(tmp[:], n)
			buf.Write(tmp[:])
		case InputTypeBool:
			v, ok := value.(bool)
			if !ok {
				return nil, fmt.Errorf("%w: input %q", ErrInvalidSample, field.Name)
			}
			if v {
				buf.WriteByte(1)
			} else {
				buf.WriteByte(0)
			}
		case InputTypeString:
			v, ok := value.(string)
			if !ok {
				return nil, fmt.Errorf("%w: input %q", ErrInvalidSample, field.Name)
			}
			writeString(&buf, v)
		default:
			return nil, ErrInvalidSchema
		}
	}

	writeUvarint(&buf, uint64(len(sample.Results)))
	resultIndexes := resultKeyIndexes(schema)
	for _, result := range sample.Results {
		if idx, ok := resultIndexes[result.Key]; ok {
			writeUvarint(&buf, uint64(idx+1))
		} else {
			writeUvarint(&buf, 0)
			writeString(&buf, result.Key)
		}
		writeString(&buf, result.Value)
	}

	return buf.Bytes(), nil
}

func decodeSampleFrame(schema Schema, frame []byte) (Sample, error) {
	if len(frame) >= 4 && (bytes.Equal(frame[:4], samplePayloadMagic[:]) || bytes.Equal(frame[:4], samplePayloadMagicCompact[:])) {
		return decodeSamplePayload(schema, frame)
	}
	decoded := encoder_v1.Decode(frame)
	return decodeSamplePayload(schema, []byte(decoded.Data))
}

func decodeSampleFrameForStats(schema Schema, frame []byte) (string, normalizedInput, []ResultLabel, error) {
	if len(frame) >= 4 && (bytes.Equal(frame[:4], samplePayloadMagic[:]) || bytes.Equal(frame[:4], samplePayloadMagicCompact[:])) {
		return decodeSamplePayloadForStats(schema, frame)
	}
	decoded := encoder_v1.Decode(frame)
	return decodeSamplePayloadForStats(schema, []byte(decoded.Data))
}

func decodeFloatSampleFrameForStats(schema Schema, frame []byte) (string, []float64, []ResultLabel, error) {
	if len(frame) >= 4 && (bytes.Equal(frame[:4], samplePayloadMagic[:]) || bytes.Equal(frame[:4], samplePayloadMagicCompact[:])) {
		return decodeFloatSamplePayloadForStats(schema, frame)
	}
	decoded := encoder_v1.Decode(frame)
	return decodeFloatSamplePayloadForStats(schema, []byte(decoded.Data))
}

func decodeSamplePayload(schema Schema, payload []byte) (Sample, error) {
	reader := bytes.NewReader(payload)
	var magic [4]byte
	if _, err := io.ReadFull(reader, magic[:]); err != nil {
		return Sample{}, err
	}
	compact := false
	switch magic {
	case samplePayloadMagicCompact:
		compact = true
	case samplePayloadMagic:
	default:
		return Sample{}, errors.New("invalid tensor sample payload")
	}

	sampleID, err := readString(reader)
	if err != nil {
		return Sample{}, err
	}
	testStatus, err := readString(reader)
	if err != nil {
		return Sample{}, err
	}
	learningStatus, err := readString(reader)
	if err != nil {
		return Sample{}, err
	}

	input := make(map[string]any, len(schema.Inputs))
	for _, field := range schema.Inputs {
		switch field.Type {
		case InputTypeFloat64:
			if compact {
				raw, err := readUint32(reader)
				if err != nil {
					return Sample{}, err
				}
				input[field.Name] = float64(math.Float32frombits(raw))
			} else {
				raw, err := readUint64(reader)
				if err != nil {
					return Sample{}, err
				}
				input[field.Name] = math.Float64frombits(raw)
			}
		case InputTypeInt64:
			raw, err := readUint64(reader)
			if err != nil {
				return Sample{}, err
			}
			input[field.Name] = int64(raw)
		case InputTypeUint64:
			raw, err := readUint64(reader)
			if err != nil {
				return Sample{}, err
			}
			input[field.Name] = raw
		case InputTypeBool:
			raw, err := reader.ReadByte()
			if err != nil {
				return Sample{}, err
			}
			input[field.Name] = raw != 0
		case InputTypeString:
			value, err := readString(reader)
			if err != nil {
				return Sample{}, err
			}
			input[field.Name] = value
		default:
			return Sample{}, ErrInvalidSchema
		}
	}

	resultCount, err := binary.ReadUvarint(reader)
	if err != nil {
		return Sample{}, err
	}
	results := make([]ResultLabel, 0, resultCount)
	for i := uint64(0); i < resultCount; i++ {
		keyIndex, err := binary.ReadUvarint(reader)
		if err != nil {
			return Sample{}, err
		}
		key := ""
		if keyIndex == 0 {
			key, err = readString(reader)
			if err != nil {
				return Sample{}, err
			}
		} else {
			idx := int(keyIndex - 1)
			if idx < 0 || idx >= len(schema.Results) {
				return Sample{}, ErrInvalidSample
			}
			key = schema.Results[idx].Key
		}
		value, err := readString(reader)
		if err != nil {
			return Sample{}, err
		}
		results = append(results, ResultLabel{Key: key, Value: value})
	}

	return Sample{
		SampleID:       sampleID,
		TestStatus:     testStatus,
		LearningStatus: learningStatus,
		Input:          input,
		Results:        results,
	}, nil
}

func decodeSamplePayloadForStats(schema Schema, payload []byte) (string, normalizedInput, []ResultLabel, error) {
	reader := bytes.NewReader(payload)
	var magic [4]byte
	if _, err := io.ReadFull(reader, magic[:]); err != nil {
		return "", nil, nil, err
	}
	compact := false
	switch magic {
	case samplePayloadMagicCompact:
		compact = true
	case samplePayloadMagic:
	default:
		return "", nil, nil, errors.New("invalid tensor sample payload")
	}

	if _, err := readString(reader); err != nil {
		return "", nil, nil, err
	}
	if _, err := readString(reader); err != nil {
		return "", nil, nil, err
	}
	learningStatus, err := readString(reader)
	if err != nil {
		return "", nil, nil, err
	}

	input := make(normalizedInput, len(schema.Inputs))
	for i, field := range schema.Inputs {
		switch field.Type {
		case InputTypeFloat64:
			if compact {
				raw, err := readUint32(reader)
				if err != nil {
					return "", nil, nil, err
				}
				input[i] = float64(math.Float32frombits(raw))
			} else {
				raw, err := readUint64(reader)
				if err != nil {
					return "", nil, nil, err
				}
				input[i] = math.Float64frombits(raw)
			}
		case InputTypeInt64:
			raw, err := readUint64(reader)
			if err != nil {
				return "", nil, nil, err
			}
			input[i] = int64(raw)
		case InputTypeUint64:
			raw, err := readUint64(reader)
			if err != nil {
				return "", nil, nil, err
			}
			input[i] = raw
		case InputTypeBool:
			raw, err := reader.ReadByte()
			if err != nil {
				return "", nil, nil, err
			}
			input[i] = raw != 0
		case InputTypeString:
			value, err := readString(reader)
			if err != nil {
				return "", nil, nil, err
			}
			input[i] = value
		default:
			return "", nil, nil, ErrInvalidSchema
		}
	}

	resultCount, err := binary.ReadUvarint(reader)
	if err != nil {
		return "", nil, nil, err
	}
	results := make([]ResultLabel, 0, resultCount)
	for i := uint64(0); i < resultCount; i++ {
		keyIndex, err := binary.ReadUvarint(reader)
		if err != nil {
			return "", nil, nil, err
		}
		key := ""
		if keyIndex == 0 {
			key, err = readString(reader)
			if err != nil {
				return "", nil, nil, err
			}
		} else {
			idx := int(keyIndex - 1)
			if idx < 0 || idx >= len(schema.Results) {
				return "", nil, nil, ErrInvalidSample
			}
			key = schema.Results[idx].Key
		}
		value, err := readString(reader)
		if err != nil {
			return "", nil, nil, err
		}
		results = append(results, ResultLabel{Key: key, Value: value})
	}
	return learningStatus, input, results, nil
}

func decodeFloatSamplePayloadForStats(schema Schema, payload []byte) (string, []float64, []ResultLabel, error) {
	reader := bytes.NewReader(payload)
	var magic [4]byte
	if _, err := io.ReadFull(reader, magic[:]); err != nil {
		return "", nil, nil, err
	}
	compact := false
	switch magic {
	case samplePayloadMagicCompact:
		compact = true
	case samplePayloadMagic:
	default:
		return "", nil, nil, errors.New("invalid tensor sample payload")
	}

	if _, err := readString(reader); err != nil {
		return "", nil, nil, err
	}
	if _, err := readString(reader); err != nil {
		return "", nil, nil, err
	}
	learningStatus, err := readString(reader)
	if err != nil {
		return "", nil, nil, err
	}

	input := make([]float64, len(schema.Inputs))
	for i, field := range schema.Inputs {
		if field.Type != InputTypeFloat64 {
			return "", nil, nil, ErrInvalidSchema
		}
		if compact {
			raw, err := readUint32(reader)
			if err != nil {
				return "", nil, nil, err
			}
			input[i] = float64(math.Float32frombits(raw))
			continue
		}
		raw, err := readUint64(reader)
		if err != nil {
			return "", nil, nil, err
		}
		input[i] = math.Float64frombits(raw)
	}

	resultCount, err := binary.ReadUvarint(reader)
	if err != nil {
		return "", nil, nil, err
	}
	results := make([]ResultLabel, 0, resultCount)
	for i := uint64(0); i < resultCount; i++ {
		keyIndex, err := binary.ReadUvarint(reader)
		if err != nil {
			return "", nil, nil, err
		}
		key := ""
		if keyIndex == 0 {
			key, err = readString(reader)
			if err != nil {
				return "", nil, nil, err
			}
		} else {
			idx := int(keyIndex - 1)
			if idx < 0 || idx >= len(schema.Results) {
				return "", nil, nil, ErrInvalidSample
			}
			key = schema.Results[idx].Key
		}
		value, err := readString(reader)
		if err != nil {
			return "", nil, nil, err
		}
		results = append(results, ResultLabel{Key: key, Value: value})
	}
	return learningStatus, input, results, nil
}

func readSampleFrame(reader bufioReader) ([]byte, error) {
	header := make([]byte, 3)
	if _, err := io.ReadFull(reader, header[:2]); err != nil {
		return nil, err
	}
	if header[0] != 1 {
		return nil, errors.New("invalid kv frame version")
	}
	pointerSize := header[1] & 0x7F
	if pointerSize != 1 && pointerSize != 2 && pointerSize != 4 && pointerSize != 8 {
		return nil, errors.New("invalid kv pointer size")
	}
	if _, err := io.ReadFull(reader, header[2:3]); err != nil {
		return nil, err
	}

	endBytes := make([]byte, pointerSize)
	if _, err := io.ReadFull(reader, endBytes); err != nil {
		return nil, err
	}
	startPtr := uint64(header[2])
	var endPtr uint64
	switch pointerSize {
	case 1:
		endPtr = uint64(endBytes[0])
	case 2:
		endPtr = uint64(binary.LittleEndian.Uint16(endBytes))
	case 4:
		endPtr = uint64(binary.LittleEndian.Uint32(endBytes))
	case 8:
		endPtr = binary.LittleEndian.Uint64(endBytes)
	}
	if endPtr < startPtr {
		return nil, errors.New("invalid kv frame pointers")
	}

	dataLen := endPtr - startPtr
	if dataLen > uint64(int(^uint(0)>>1)) {
		return nil, errors.New("kv frame too large")
	}
	data := make([]byte, int(dataLen))
	if _, err := io.ReadFull(reader, data); err != nil {
		return nil, err
	}

	frame := make([]byte, 0, len(header)+len(endBytes)+len(data))
	frame = append(frame, header...)
	frame = append(frame, endBytes...)
	frame = append(frame, data...)
	return frame, nil
}

type bufioReader interface {
	io.Reader
	io.ByteReader
}

func writeString(buf *bytes.Buffer, value string) {
	writeUvarint(buf, uint64(len(value)))
	buf.WriteString(value)
}

func readString(reader *bytes.Reader) (string, error) {
	size, err := binary.ReadUvarint(reader)
	if err != nil {
		return "", err
	}
	if size > uint64(reader.Len()) {
		return "", io.ErrUnexpectedEOF
	}
	raw := make([]byte, int(size))
	if _, err := io.ReadFull(reader, raw); err != nil {
		return "", err
	}
	return string(raw), nil
}

func writeUvarint(buf *bytes.Buffer, value uint64) {
	var tmp [10]byte
	n := binary.PutUvarint(tmp[:], value)
	buf.Write(tmp[:n])
}

func readUint64(reader *bytes.Reader) (uint64, error) {
	var raw [8]byte
	if _, err := io.ReadFull(reader, raw[:]); err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint64(raw[:]), nil
}

func readUint32(reader *bytes.Reader) (uint32, error) {
	var raw [4]byte
	if _, err := io.ReadFull(reader, raw[:]); err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(raw[:]), nil
}

func resultKeyIndexes(schema Schema) map[string]int {
	indexes := make(map[string]int, len(schema.Results))
	for i, result := range schema.Results {
		indexes[result.Key] = i
	}
	return indexes
}
