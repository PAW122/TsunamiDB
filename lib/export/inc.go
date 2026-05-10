package export

import (
	"bytes"
	"encoding/binary"
	stderrors "errors"
	"fmt"

	fileSystem_v1 "github.com/PAW122/TsunamiDB/data/fileSystem/v1"
	incindex "github.com/PAW122/TsunamiDB/data/incIndex"
	subServer "github.com/PAW122/TsunamiDB/servers/subscriptions"
	"github.com/PAW122/TsunamiDB/types"
)

type SaveIncMode string

const (
	SaveIncModeAppend    SaveIncMode = "append"
	SaveIncModeOverwrite SaveIncMode = "overwrite"
)

type IncCountFrom string

const (
	IncCountFromTop    IncCountFrom = "top"
	IncCountFromBottom IncCountFrom = "bottom"
)

type ReadIncType string

const (
	ReadIncByID         ReadIncType = "by_id"
	ReadIncByKey        ReadIncType = "by_key"
	ReadIncFirstEntries ReadIncType = "first_entries"
	ReadIncLastEntries  ReadIncType = "last_entries"
)

type SaveIncOptions struct {
	MaxEntrySize uint64
	ID           *uint64
	Mode         SaveIncMode
	CountFrom    IncCountFrom
	EntryKey     string
}

type SaveIncResult struct {
	ID      uint64
	Warning string
}

type ReadIncOptions struct {
	Type     ReadIncType
	ID       uint64
	EntryKey string
	Amount   uint64
}

type IncEntry struct {
	ID   uint64 `json:"id"`
	Data []byte `json:"data"`
}

func SaveInc(key, table string, data []byte, options SaveIncOptions) (SaveIncResult, error) {
	if key == "" || table == "" {
		return SaveIncResult{}, fmt.Errorf("Invalid key or table value")
	}

	mode := options.Mode
	if mode == "" {
		mode = SaveIncModeAppend
	}
	if mode != SaveIncModeAppend && mode != SaveIncModeOverwrite {
		return SaveIncResult{}, fmt.Errorf("invalid save_inc mode: %s", mode)
	}

	countFrom := options.CountFrom
	if countFrom == "" {
		countFrom = IncCountFromTop
	}
	if countFrom != IncCountFromTop && countFrom != IncCountFromBottom {
		return SaveIncResult{}, fmt.Errorf("invalid save_inc count_from: %s", countFrom)
	}

	incTable, warning, err := loadOrCreateIncTable(key, table, options.MaxEntrySize, len(data))
	if err != nil {
		return SaveIncResult{}, err
	}
	if uint64(len(data)) > incTable.EntrySize {
		return SaveIncResult{}, fmt.Errorf("Body size exceeds entry size")
	}

	if options.EntryKey != "" && (options.ID == nil || mode != SaveIncModeOverwrite) {
		if _, exists, err := incindex.Lookup(incTable.TableFileName, options.EntryKey); err != nil {
			return SaveIncResult{}, err
		} else if exists {
			return SaveIncResult{}, incindex.ErrDuplicateKey
		}
	}

	encoded := encodeIncEntry(incTable.EntrySize, data)
	if encoded == nil {
		return SaveIncResult{}, fmt.Errorf("cannot encode incremental entry")
	}

	var id uint64
	if options.ID == nil {
		id, err = saveIncData(encoded, incTable.TableFileName, incTable.EntrySize)
		if err == nil && options.EntryKey != "" {
			err = incindex.Insert(incTable.TableFileName, id, options.EntryKey)
		}
		if err == nil {
			go subServer.NotifyTableIncTableSubscribers(table, key, "add", id, data)
		}
	} else if mode == SaveIncModeOverwrite {
		id, err = saveIncDataOverwrite(encoded, incTable.TableFileName, incTable.EntrySize, *options.ID, string(countFrom))
		if err == nil && options.EntryKey != "" {
			err = incindex.Set(incTable.TableFileName, id, options.EntryKey)
		}
		if err == nil {
			go subServer.NotifyTableIncTableSubscribers(table, key, "overwrite", id, data)
		}
	} else {
		id, err = saveIncDataPut(encoded, incTable.TableFileName, incTable.EntrySize, *options.ID, string(countFrom))
		if err == nil && options.EntryKey != "" {
			err = incindex.Insert(incTable.TableFileName, id, options.EntryKey)
		}
		if err == nil {
			go subServer.NotifyTableIncTableSubscribers(table, key, "insert", id, data)
		}
	}
	if err != nil {
		return SaveIncResult{}, err
	}

	return SaveIncResult{ID: id, Warning: warning}, nil
}

func ReadInc(key, table string, options ReadIncOptions) ([]IncEntry, error) {
	if key == "" || table == "" {
		return nil, fmt.Errorf("Invalid key or table value")
	}

	readType := options.Type
	if readType == "" {
		readType = ReadIncByID
	}
	switch readType {
	case ReadIncByID, ReadIncByKey, ReadIncFirstEntries, ReadIncLastEntries:
	default:
		return nil, fmt.Errorf("unsupported read_inc type: %s", readType)
	}

	incTable, err := readIncTableMetadata(key, table)
	if err != nil {
		return nil, err
	}

	switch readType {
	case ReadIncByID:
		return readIncSingle(incTable, options.ID)
	case ReadIncByKey:
		if options.EntryKey == "" {
			return nil, fmt.Errorf("Missing entry_key")
		}
		id, ok, err := incindex.Lookup(incTable.TableFileName, options.EntryKey)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("Entry not found")
		}
		return readIncSingle(incTable, id)
	case ReadIncFirstEntries:
		raw, err := readIncDataFirst(incTable.TableFileName, options.Amount, incTable.EntrySize)
		if err != nil {
			return nil, err
		}
		return decodeIncEntries(incTable.EntrySize, raw, func(i int, _ int) uint64 { return uint64(i) })
	case ReadIncLastEntries:
		total, err := getIncRecordCount(incTable.TableFileName, incTable.EntrySize)
		if err != nil {
			return nil, err
		}
		raw, err := readIncDataLast(incTable.TableFileName, options.Amount, incTable.EntrySize)
		if err != nil {
			return nil, err
		}
		return decodeIncEntries(incTable.EntrySize, raw, func(i int, _ int) uint64 {
			return total - 1 - uint64(i)
		})
	}
	return nil, fmt.Errorf("unsupported read_inc type: %s", readType)
}

func loadOrCreateIncTable(key, table string, maxEntrySize uint64, bodyLen int) (types.IncTableEntryData, string, error) {
	fsData, err := getElementByKey(table, key)
	if err == nil {
		incTable, err := readIncTableMetadataFromFS(table, fsData)
		if err != nil {
			return types.IncTableEntryData{}, "", err
		}
		if maxEntrySize != 0 && maxEntrySize != incTable.EntrySize {
			return incTable, fmt.Sprintf("max_entry_size (%d) does not match existing table (%d); value ignored", maxEntrySize, incTable.EntrySize), nil
		}
		return incTable, "", nil
	}

	if maxEntrySize == 0 && bodyLen > 0 {
		return types.IncTableEntryData{}, "", fmt.Errorf("max_entry_size is required for a new incremental table")
	}

	incTable := types.IncTableEntryData{
		EntrySize:     maxEntrySize,
		TableFileName: fmt.Sprintf("inc_table_%s.tbl", key),
	}
	raw, err := incTableMetadataToBytes(incTable)
	if err != nil {
		return types.IncTableEntryData{}, "", err
	}
	encoded, _ := encode(raw, false)
	startPtr, endPtr, err := saveDataToFileAsync(encoded, table)
	if err != nil {
		return types.IncTableEntryData{}, "", err
	}
	prevMeta, existed, err := saveElementByKey(table, key, int(startPtr), int(endPtr), false)
	if err != nil {
		return types.IncTableEntryData{}, "", err
	}
	if existed {
		if prevMeta.FileName != table || prevMeta.StartPtr != int(startPtr) || prevMeta.EndPtr != int(endPtr) {
			markAsFree(prevMeta.Key, prevMeta.FileName, int64(prevMeta.StartPtr), int64(prevMeta.EndPtr))
			recordDefragFree()
		} else {
			recordDefragSkip()
		}
	}
	return incTable, "", nil
}

func readIncTableMetadata(key, table string) (types.IncTableEntryData, error) {
	fsData, err := getElementByKey(table, key)
	if err != nil {
		return types.IncTableEntryData{}, err
	}
	return readIncTableMetadataFromFS(table, fsData)
}

func readIncTableMetadataFromFS(table string, fsData *fileSystem_v1.GetElement_output) (types.IncTableEntryData, error) {
	data, err := readDataFromFileAsync(table, int64(fsData.StartPtr), int64(fsData.EndPtr))
	if err != nil {
		return types.IncTableEntryData{}, err
	}
	decodedObj := decode(data)
	return incTableMetadataFromBytes([]byte(decodedObj.Data))
}

func incTableMetadataToBytes(s types.IncTableEntryData) ([]byte, error) {
	var buf bytes.Buffer
	if err := binary.Write(&buf, binary.LittleEndian, s.EntrySize); err != nil {
		return nil, err
	}
	nameBytes := []byte(s.TableFileName)
	if len(nameBytes) > int(^uint32(0)) {
		return nil, stderrors.New("TableFileName too long")
	}
	if err := binary.Write(&buf, binary.LittleEndian, uint32(len(nameBytes))); err != nil {
		return nil, err
	}
	if _, err := buf.Write(nameBytes); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func incTableMetadataFromBytes(raw []byte) (types.IncTableEntryData, error) {
	var out types.IncTableEntryData
	reader := bytes.NewReader(raw)
	if err := binary.Read(reader, binary.LittleEndian, &out.EntrySize); err != nil {
		return out, err
	}
	var nameLen uint32
	if err := binary.Read(reader, binary.LittleEndian, &nameLen); err != nil {
		return out, err
	}
	if nameLen > uint32(reader.Len()) {
		return out, stderrors.New("corrupted payload: nameLen exceeds buffer")
	}
	nameBytes := make([]byte, nameLen)
	if _, err := reader.Read(nameBytes); err != nil {
		return out, err
	}
	out.TableFileName = string(nameBytes)
	return out, nil
}

func readIncSingle(incTable types.IncTableEntryData, id uint64) ([]IncEntry, error) {
	raw, err := readIncDataByID(incTable.TableFileName, id, incTable.EntrySize)
	if err != nil {
		return nil, err
	}
	entries, err := decodeIncEntries(incTable.EntrySize, raw, func(int, int) uint64 { return id })
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("Entry not found")
	}
	return entries, nil
}

func decodeIncEntries(entrySize uint64, raw []byte, idFor func(i, count int) uint64) ([]IncEntry, error) {
	recordSize := int(entrySize) + 3
	if recordSize <= 0 {
		return nil, fmt.Errorf("Invalid entry size")
	}
	if len(raw)%recordSize != 0 {
		return nil, fmt.Errorf("Corrupted read: data len=%d not divisible by recordSize=%d", len(raw), recordSize)
	}

	count := len(raw) / recordSize
	entries := make([]IncEntry, 0, count)
	for i := 0; i < count; i++ {
		chunk := raw[i*recordSize : (i+1)*recordSize]
		decoded, err := decodeIncEntry(entrySize, chunk)
		if err != nil {
			return nil, err
		}
		if decoded.SkipBit {
			continue
		}
		entries = append(entries, IncEntry{
			ID:   idFor(i, count),
			Data: append([]byte(nil), decoded.Data...),
		})
	}
	return entries, nil
}
