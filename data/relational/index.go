package relational

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
)

type equalityIndex struct {
	Table  string              `json:"table"`
	Column string              `json:"column"`
	Values map[string][]uint64 `json:"values"`
}

type trigramIndex struct {
	Table  string              `json:"table"`
	Column string              `json:"column"`
	Values map[string][]uint64 `json:"values"`
}

func CreateIndex(table, column string) error {
	schema, err := LoadSchema(table)
	if err != nil {
		return err
	}

	column = strings.TrimSpace(column)
	columnIndex, indexedColumn, err := findColumn(*schema, column)
	if err != nil {
		return err
	}

	lock := tableLock(schema.Name)
	lock.Lock()
	defer lock.Unlock()

	index, err := buildEqualityIndexLocked(*schema, indexedColumn)
	if err != nil {
		return err
	}
	if err := writeEqualityIndex(index); err != nil {
		return err
	}

	schema.Columns[columnIndex].Indexed = true
	return persistSchema(*schema)
}

func CreateTrigramIndex(table, column string) error {
	schema, err := LoadSchema(table)
	if err != nil {
		return err
	}

	column = strings.TrimSpace(column)
	columnIndex, indexedColumn, err := findColumn(*schema, column)
	if err != nil {
		return err
	}
	if indexedColumn.Type != ColumnTypeString {
		return fmt.Errorf("%w: column %q is %s, want %s", ErrInvalidSchema, indexedColumn.Name, indexedColumn.Type, ColumnTypeString)
	}

	lock := tableLock(schema.Name)
	lock.Lock()
	defer lock.Unlock()

	index, err := buildTrigramIndexLocked(*schema, indexedColumn)
	if err != nil {
		return err
	}
	if err := writeTrigramIndex(index); err != nil {
		return err
	}

	schema.Columns[columnIndex].TrigramIndexed = true
	return persistSchema(*schema)
}

func RebuildIndexes(table string) error {
	schema, err := LoadSchema(table)
	if err != nil {
		return err
	}

	lock := tableLock(schema.Name)
	lock.Lock()
	defer lock.Unlock()

	return rebuildIndexesLocked(*schema)
}

func RebuildIndex(table, column string) error {
	return RebuildEqualityIndex(table, column)
}

func RebuildEqualityIndex(table, column string) error {
	schema, err := LoadSchema(table)
	if err != nil {
		return err
	}

	column = strings.TrimSpace(column)
	columnIndex, indexedColumn, err := findColumn(*schema, column)
	if err != nil {
		return err
	}

	lock := tableLock(schema.Name)
	lock.Lock()
	defer lock.Unlock()

	index, err := buildEqualityIndexLocked(*schema, indexedColumn)
	if err != nil {
		return err
	}
	if err := writeEqualityIndex(index); err != nil {
		return err
	}

	schema.Columns[columnIndex].Indexed = true
	return persistSchema(*schema)
}

func RebuildTrigramIndex(table, column string) error {
	schema, err := LoadSchema(table)
	if err != nil {
		return err
	}

	column = strings.TrimSpace(column)
	columnIndex, indexedColumn, err := findColumn(*schema, column)
	if err != nil {
		return err
	}
	if indexedColumn.Type != ColumnTypeString {
		return fmt.Errorf("%w: column %q is %s, want %s", ErrInvalidSchema, indexedColumn.Name, indexedColumn.Type, ColumnTypeString)
	}

	lock := tableLock(schema.Name)
	lock.Lock()
	defer lock.Unlock()

	index, err := buildTrigramIndexLocked(*schema, indexedColumn)
	if err != nil {
		return err
	}
	if err := writeTrigramIndex(index); err != nil {
		return err
	}

	schema.Columns[columnIndex].TrigramIndexed = true
	return persistSchema(*schema)
}

func rebuildIndexesLocked(schema Schema) error {
	for _, column := range schema.Columns {
		if column.Indexed {
			index, err := buildEqualityIndexLocked(schema, column)
			if err != nil {
				return err
			}
			if err := writeEqualityIndex(index); err != nil {
				return err
			}
		}
		if column.TrigramIndexed {
			index, err := buildTrigramIndexLocked(schema, column)
			if err != nil {
				return err
			}
			if err := writeTrigramIndex(index); err != nil {
				return err
			}
		}
	}
	return nil
}

func findColumn(schema Schema, name string) (int, Column, error) {
	if !safeNamePattern.MatchString(name) {
		return 0, Column{}, fmt.Errorf("%w: column name must match %s", ErrInvalidSchema, safeNamePattern.String())
	}
	for i, column := range schema.Columns {
		if column.Name == name {
			return i, column, nil
		}
	}
	return 0, Column{}, fmt.Errorf("%w: column %q not found", ErrInvalidSchema, name)
}

func loadOrBuildIndexesLocked(schema Schema) (map[string]equalityIndex, error) {
	indexes := make(map[string]equalityIndex)
	for _, column := range schema.Columns {
		if !column.Indexed {
			continue
		}

		index, err := loadEqualityIndex(schema.Name, column.Name)
		if errors.Is(err, os.ErrNotExist) {
			index, err = buildEqualityIndexLocked(schema, column)
			if err == nil {
				err = writeEqualityIndex(index)
			}
		}
		if err != nil {
			return nil, err
		}
		indexes[column.Name] = index
	}
	return indexes, nil
}

func loadOrBuildTrigramIndexesLocked(schema Schema) (map[string]trigramIndex, error) {
	indexes := make(map[string]trigramIndex)
	for _, column := range schema.Columns {
		if !column.TrigramIndexed {
			continue
		}

		index, err := loadTrigramIndex(schema.Name, column.Name)
		if errors.Is(err, os.ErrNotExist) {
			index, err = buildTrigramIndexLocked(schema, column)
			if err == nil {
				err = writeTrigramIndex(index)
			}
		}
		if err != nil {
			return nil, err
		}
		indexes[column.Name] = index
	}
	return indexes, nil
}

func appendRowToIndexes(schema Schema, indexes map[string]equalityIndex, values map[string]any, rowID uint64) error {
	for _, column := range schema.Columns {
		index, ok := indexes[column.Name]
		if !ok {
			continue
		}

		key, err := indexKey(column, values[column.Name])
		if err != nil {
			return err
		}
		index.Values[key] = append(index.Values[key], rowID)
		if err := writeEqualityIndex(index); err != nil {
			return err
		}
	}
	return nil
}

func appendRowToTrigramIndexes(schema Schema, indexes map[string]trigramIndex, values map[string]any, rowID uint64) error {
	for _, column := range schema.Columns {
		index, ok := indexes[column.Name]
		if !ok {
			continue
		}

		value, ok := values[column.Name].(string)
		if !ok {
			return fmt.Errorf("%w: column %q: string value required", ErrInvalidRow, column.Name)
		}
		for _, gram := range trigramsForIndex(value) {
			index.Values[gram] = append(index.Values[gram], rowID)
		}
		if err := writeTrigramIndex(index); err != nil {
			return err
		}
	}
	return nil
}

func buildEqualityIndexLocked(schema Schema, column Column) (equalityIndex, error) {
	index := equalityIndex{
		Table:  schema.Name,
		Column: column.Name,
		Values: make(map[string][]uint64),
	}

	file, err := openFile(tablePaths(schema.Name).rows, os.O_RDONLY, 0o644)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return equalityIndex{}, ErrTableNotFound
		}
		return equalityIndex{}, err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return equalityIndex{}, err
	}
	rowSize := int64(schema.RowSize)
	if rowSize <= 0 || info.Size()%rowSize != 0 {
		return equalityIndex{}, ErrCorruptRows
	}

	rowCount := uint64(info.Size() / rowSize)
	row := make([]byte, schema.RowSize)
	for rowID := uint64(0); rowID < rowCount; rowID++ {
		if _, err := io.ReadFull(file, row); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return equalityIndex{}, ErrCorruptRows
			}
			return equalityIndex{}, err
		}
		if row[0]&RowFlagActive == 0 {
			continue
		}

		values, err := DecodeRow(schema, row)
		if err != nil {
			return equalityIndex{}, err
		}
		key, err := indexKey(column, values[column.Name])
		if err != nil {
			return equalityIndex{}, err
		}
		index.Values[key] = append(index.Values[key], rowID)
	}
	return index, nil
}

func buildTrigramIndexLocked(schema Schema, column Column) (trigramIndex, error) {
	if column.Type != ColumnTypeString {
		return trigramIndex{}, fmt.Errorf("%w: column %q is %s, want %s", ErrInvalidSchema, column.Name, column.Type, ColumnTypeString)
	}

	index := trigramIndex{
		Table:  schema.Name,
		Column: column.Name,
		Values: make(map[string][]uint64),
	}

	file, err := openFile(tablePaths(schema.Name).rows, os.O_RDONLY, 0o644)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return trigramIndex{}, ErrTableNotFound
		}
		return trigramIndex{}, err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return trigramIndex{}, err
	}
	rowSize := int64(schema.RowSize)
	if rowSize <= 0 || info.Size()%rowSize != 0 {
		return trigramIndex{}, ErrCorruptRows
	}

	rowCount := uint64(info.Size() / rowSize)
	row := make([]byte, schema.RowSize)
	for rowID := uint64(0); rowID < rowCount; rowID++ {
		if _, err := io.ReadFull(file, row); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return trigramIndex{}, ErrCorruptRows
			}
			return trigramIndex{}, err
		}
		if row[0]&RowFlagActive == 0 {
			continue
		}

		values, err := DecodeRow(schema, row)
		if err != nil {
			return trigramIndex{}, err
		}
		value, ok := values[column.Name].(string)
		if !ok {
			return trigramIndex{}, fmt.Errorf("%w: column %q: string value required", ErrInvalidRow, column.Name)
		}
		for _, gram := range trigramsForIndex(value) {
			index.Values[gram] = append(index.Values[gram], rowID)
		}
	}
	return index, nil
}

func loadEqualityIndex(table, column string) (equalityIndex, error) {
	data, err := readFile(indexPath(table, column))
	if err != nil {
		return equalityIndex{}, err
	}

	var index equalityIndex
	if err := json.Unmarshal(data, &index); err != nil {
		return equalityIndex{}, err
	}
	if index.Table != table || index.Column != column || index.Values == nil {
		return equalityIndex{}, fmt.Errorf("%w: invalid index for %s.%s", ErrInvalidSchema, table, column)
	}
	return index, nil
}

func loadTrigramIndex(table, column string) (trigramIndex, error) {
	data, err := readFile(trigramIndexPath(table, column))
	if err != nil {
		return trigramIndex{}, err
	}

	var index trigramIndex
	if err := json.Unmarshal(data, &index); err != nil {
		return trigramIndex{}, err
	}
	if index.Table != table || index.Column != column || index.Values == nil {
		return trigramIndex{}, fmt.Errorf("%w: invalid trigram index for %s.%s", ErrInvalidSchema, table, column)
	}
	return index, nil
}

func writeEqualityIndex(index equalityIndex) error {
	if index.Values == nil {
		index.Values = make(map[string][]uint64)
	}

	data, err := marshalJSON(index, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	path := indexPath(index.Table, index.Column)
	tmp := path + ".tmp"
	if err := writeFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := renameFile(tmp, path); err != nil {
		_ = removeFile(tmp)
		return err
	}
	return nil
}

func writeTrigramIndex(index trigramIndex) error {
	if index.Values == nil {
		index.Values = make(map[string][]uint64)
	}

	data, err := marshalJSON(index, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	path := trigramIndexPath(index.Table, index.Column)
	tmp := path + ".tmp"
	if err := writeFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := renameFile(tmp, path); err != nil {
		_ = removeFile(tmp)
		return err
	}
	return nil
}

func indexKey(column Column, value any) (string, error) {
	switch column.Type {
	case ColumnTypeUint64, ColumnTypeBlobPtr, ColumnTypeRowRef:
		encoded, err := asUint64(value)
		if err != nil {
			return "", fmt.Errorf("%w: column %q: %v", ErrInvalidRow, column.Name, err)
		}
		return "u:" + strconv.FormatUint(encoded, 10), nil
	case ColumnTypeInt64:
		encoded, err := asInt64(value)
		if err != nil {
			return "", fmt.Errorf("%w: column %q: %v", ErrInvalidRow, column.Name, err)
		}
		return "i:" + strconv.FormatInt(encoded, 10), nil
	case ColumnTypeBool:
		encoded, ok := value.(bool)
		if !ok {
			return "", fmt.Errorf("%w: column %q: bool value required", ErrInvalidRow, column.Name)
		}
		if encoded {
			return "b:1", nil
		}
		return "b:0", nil
	case ColumnTypeFloat64:
		encoded, err := asFloat64(value)
		if err != nil {
			return "", fmt.Errorf("%w: column %q: %v", ErrInvalidRow, column.Name, err)
		}
		return "f:" + strconv.FormatUint(math.Float64bits(encoded), 16), nil
	case ColumnTypeString:
		encoded, ok := value.(string)
		if !ok {
			return "", fmt.Errorf("%w: column %q: string value required", ErrInvalidRow, column.Name)
		}
		return "s:" + base64.RawStdEncoding.EncodeToString([]byte(encoded)), nil
	default:
		return "", fmt.Errorf("%w: column %q: unsupported type %q", ErrInvalidSchema, column.Name, column.Type)
	}
}

func trigramsForIndex(value string) []string {
	if len(value) < 3 {
		return nil
	}

	seen := make(map[string]struct{}, len(value)-2)
	grams := make([]string, 0, len(value)-2)
	for i := 0; i+3 <= len(value); i++ {
		gram := value[i : i+3]
		if _, ok := seen[gram]; ok {
			continue
		}
		seen[gram] = struct{}{}
		grams = append(grams, gram)
	}
	return grams
}
