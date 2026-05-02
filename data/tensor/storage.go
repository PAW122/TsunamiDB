package tensor

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"

	dataManager_v2 "github.com/PAW122/TsunamiDB/data/dataManager/v2"
	defragmentationManager "github.com/PAW122/TsunamiDB/data/defragmentationManager"
	fileSystem_v1 "github.com/PAW122/TsunamiDB/data/fileSystem/v1"
	encoder_v1 "github.com/PAW122/TsunamiDB/encoding/v1"
)

const baseDir = "db/tensor"
const tensorKVFile = "tensor.kv"
const tensorSampleManifestEntrySize = 16
const tensorRebuildChunkRecords = 1024
const tensorRebuildChunkBytes = 16 << 20

var validName = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

type tableFiles struct {
	schema          string
	samples         string
	sampleData      string
	stats           string
	aiModel         string
	sampleEntrySize uint64
	legacySchema    string
	legacySamples   string
	legacyStats     string
}

func paths(table string) tableFiles {
	return tableFiles{
		schema:        "tensor:" + table + ":schema",
		samples:       "tensor_" + table + "_sample_manifest.tbl",
		sampleData:    "tensor_" + table + "_sample_data.bin",
		stats:         "tensor:" + table + ":stats",
		aiModel:       "tensor:" + table + ":ai_model",
		legacySchema:  filepath.Join(baseDir, table+".schema"),
		legacySamples: filepath.Join(baseDir, table+".samples"),
		legacyStats:   filepath.Join(baseDir, table+".stats"),
	}
}

func CreateTable(schema Schema) (*Table, error) {
	calculated, err := validateSchema(schema)
	if err != nil {
		return nil, err
	}
	files := paths(calculated.Name)
	files.sampleEntrySize = tensorSampleManifestEntrySize
	if tensorKVExists(files.schema) {
		return nil, ErrTableExists
	}

	if err := writeKVJSON(files.schema, calculated); err != nil {
		return nil, err
	}
	if err := dataManager_v2.DeleteIncTableFile(files.samples); err != nil {
		return nil, err
	}

	stats := newStats(calculated)
	if err := writeStatsSnapshot(files.stats, calculated, stats); err != nil {
		return nil, err
	}
	return newTable(calculated, stats, files), nil
}

func OpenTable(name string) (*Table, error) {
	if !validName.MatchString(name) {
		return nil, ErrInvalidSchema
	}
	files := paths(name)

	var schema Schema
	if err := readKVJSON(files.schema, &schema); err != nil {
		if !errors.Is(err, ErrTableNotFound) {
			return nil, err
		}
		raw, readErr := os.ReadFile(files.legacySchema)
		if readErr != nil {
			if errors.Is(readErr, os.ErrNotExist) {
				return nil, ErrTableNotFound
			}
			return nil, readErr
		}
		if err := json.Unmarshal(raw, &schema); err != nil {
			return nil, err
		}
	}
	calculated, err := validateSchema(schema)
	if err != nil {
		return nil, err
	}
	files.sampleEntrySize = tensorSampleManifestEntrySize

	stats := newStats(calculated)
	statsLoaded := false
	if err := readStatsSnapshot(files.stats, calculated, stats); err != nil {
		if !errors.Is(err, ErrTableNotFound) {
			if jsonErr := readKVJSON(files.stats, stats); jsonErr != nil {
				return nil, err
			}
			statsLoaded = true
		}
	} else {
		statsLoaded = true
	}
	if !statsLoaded {
		if raw, readErr := os.ReadFile(files.legacyStats); readErr == nil && len(raw) > 0 {
			if err := json.Unmarshal(raw, stats); err != nil {
				return nil, err
			}
		} else if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
			return nil, readErr
		}
	}

	table := newTable(calculated, stats, files)
	var model AIModel
	if err := readKVJSON(files.aiModel, &model); err == nil {
		table.aiModel = &model
	} else if !errors.Is(err, ErrTableNotFound) {
		return nil, err
	}
	return table, nil
}

func tensorKVExists(key string) bool {
	_, err := fileSystem_v1.GetElementByKey(tensorKVFile, key)
	return err == nil
}

func writeKVJSON(key string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return writeKVBytes(key, raw)
}

func writeKVBytes(key string, raw []byte) error {
	encoded, _ := encoder_v1.Encode(raw, false)
	startPtr, endPtr, err := dataManager_v2.SaveDataToFileAsync(encoded, tensorKVFile)
	if err != nil {
		return err
	}
	prev, existed, err := fileSystem_v1.SaveElementByKey(tensorKVFile, key, int(startPtr), int(endPtr), false)
	if err != nil {
		return err
	}
	if existed {
		if prev.FileName != tensorKVFile || prev.StartPtr != int(startPtr) || prev.EndPtr != int(endPtr) {
			defragmentationManager.MarkAsFree(prev.Key, prev.FileName, int64(prev.StartPtr), int64(prev.EndPtr))
			fileSystem_v1.RecordDefragFree()
		} else {
			fileSystem_v1.RecordDefragSkip()
		}
	}
	return nil
}

func readKVJSON(key string, value any) error {
	raw, err := readKVBytes(key)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, value)
}

func readKVBytes(key string) ([]byte, error) {
	meta, err := fileSystem_v1.GetElementByKey(tensorKVFile, key)
	if err != nil {
		return nil, ErrTableNotFound
	}
	raw, err := dataManager_v2.ReadDataFromFileAsync(meta.FileName, int64(meta.StartPtr), int64(meta.EndPtr))
	if err != nil {
		return nil, err
	}
	decoded := encoder_v1.Decode(raw)
	if decoded.Data == "" {
		return nil, ErrTableNotFound
	}
	return []byte(decoded.Data), nil
}

func encodeSampleIncEntry(entrySize uint64, frame []byte) ([]byte, error) {
	if uint64(len(frame)) > entrySize {
		return nil, ErrSampleTooLarge
	}
	entry := encoder_v1.EncodeIncEntry(entrySize, frame)
	if entry == nil {
		return nil, ErrSampleTooLarge
	}
	return entry, nil
}

func encodeSampleManifestEntry(startPtr, endPtr int64) ([]byte, error) {
	if startPtr < 0 || endPtr <= startPtr {
		return nil, ErrInvalidSample
	}
	var payload [tensorSampleManifestEntrySize]byte
	binary.LittleEndian.PutUint64(payload[0:8], uint64(startPtr))
	binary.LittleEndian.PutUint64(payload[8:16], uint64(endPtr))
	entry := encoder_v1.EncodeIncEntry(tensorSampleManifestEntrySize, payload[:])
	if entry == nil {
		return nil, ErrSampleTooLarge
	}
	return entry, nil
}

func decodeSampleManifestEntry(raw []byte) (int64, int64, bool, error) {
	entry, err := encoder_v1.DecodeIncEntry(tensorSampleManifestEntrySize, raw)
	if err != nil {
		return 0, 0, false, err
	}
	if entry.SkipBit || len(entry.Data) == 0 {
		return 0, 0, false, nil
	}
	if len(entry.Data) != tensorSampleManifestEntrySize {
		return 0, 0, false, errors.New("tensor: invalid sample manifest entry")
	}
	startPtr := int64(binary.LittleEndian.Uint64(entry.Data[0:8]))
	endPtr := int64(binary.LittleEndian.Uint64(entry.Data[8:16]))
	if startPtr < 0 || endPtr <= startPtr {
		return 0, 0, false, errors.New("tensor: invalid sample manifest pointers")
	}
	return startPtr, endPtr, true, nil
}

func decodeSampleIncEntry(entrySize uint64, raw []byte) ([]byte, error) {
	entry, err := encoder_v1.DecodeIncEntry(entrySize, raw)
	if err != nil {
		return nil, err
	}
	if entry.SkipBit || len(entry.Data) == 0 {
		return nil, nil
	}
	return entry.Data, nil
}

func openLegacySamples(path string) (*os.File, error) {
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrTableNotFound
		}
		return nil, err
	}
	return file, nil
}
