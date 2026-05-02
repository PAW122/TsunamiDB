package relational

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
)

func InsertRow(table string, values map[string]any) (uint64, error) {
	schema, err := LoadSchema(table)
	if err != nil {
		return 0, err
	}

	row, err := EncodeRow(*schema, values)
	if err != nil {
		return 0, err
	}

	lock := tableLock(schema.Name)
	lock.Lock()
	defer lock.Unlock()

	paths := tablePaths(schema.Name)
	file, err := openRowsFile(paths.rows, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return 0, err
	}

	info, err := file.Stat()
	if err != nil {
		return 0, err
	}
	rowSize := int64(schema.RowSize)
	if rowSize <= 0 || info.Size()%rowSize != 0 {
		return 0, ErrCorruptRows
	}

	rowID := uint64(info.Size() / rowSize)
	indexes, err := loadOrBuildIndexesLocked(*schema)
	if err != nil {
		return 0, err
	}
	trigramIndexes, err := loadOrBuildTrigramIndexesLocked(*schema)
	if err != nil {
		return 0, err
	}
	if _, err := file.WriteAt(row, info.Size()); err != nil {
		return 0, err
	}
	if len(indexes) > 0 || len(trigramIndexes) > 0 {
		decoded, err := DecodeRow(*schema, row)
		if err != nil {
			return 0, err
		}
		if err := appendRowToIndexes(*schema, indexes, decoded, rowID); err != nil {
			return 0, err
		}
		if err := appendRowToTrigramIndexes(*schema, trigramIndexes, decoded, rowID); err != nil {
			return 0, err
		}
	}
	return rowID, nil
}

func ReadRow(table string, rowID uint64) (map[string]any, error) {
	schema, err := LoadSchema(table)
	if err != nil {
		return nil, err
	}

	lock := tableLock(schema.Name)
	lock.RLock()
	defer lock.RUnlock()

	paths := tablePaths(schema.Name)
	file, err := openRowsFile(paths.rows, os.O_RDWR, 0o644)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrTableNotFound
		}
		return nil, err
	}

	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	rowSize := int64(schema.RowSize)
	if rowSize <= 0 || info.Size()%rowSize != 0 {
		return nil, ErrCorruptRows
	}

	if rowID > uint64(math.MaxInt64)/schema.RowSize {
		return nil, ErrRowNotFound
	}
	offset := int64(rowID * schema.RowSize)
	if offset >= info.Size() {
		return nil, ErrRowNotFound
	}

	row := make([]byte, schema.RowSize)
	if _, err := file.ReadAt(row, offset); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, ErrCorruptRows
		}
		return nil, err
	}
	if row[0]&RowFlagActive == 0 {
		return nil, ErrRowNotFound
	}

	return DecodeRow(*schema, row)
}

func UpdateRow(table string, rowID uint64, values map[string]any) error {
	schema, err := LoadSchema(table)
	if err != nil {
		return err
	}

	lock := tableLock(schema.Name)
	lock.Lock()
	defer lock.Unlock()

	paths := tablePaths(schema.Name)
	file, err := openRowsFile(paths.rows, os.O_RDWR, 0o644)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrTableNotFound
		}
		return err
	}

	info, err := file.Stat()
	if err != nil {
		return err
	}
	rowSize := int64(schema.RowSize)
	if rowSize <= 0 || info.Size()%rowSize != 0 {
		return ErrCorruptRows
	}
	if rowID > uint64(math.MaxInt64)/schema.RowSize {
		return ErrRowNotFound
	}
	offset := int64(rowID * schema.RowSize)
	if offset >= info.Size() {
		return ErrRowNotFound
	}

	existingRow := make([]byte, schema.RowSize)
	if _, err := file.ReadAt(existingRow, offset); err != nil {
		if errors.Is(err, io.EOF) {
			return ErrCorruptRows
		}
		return err
	}
	if existingRow[0]&RowFlagActive == 0 {
		return ErrRowNotFound
	}

	merged, err := DecodeRow(*schema, existingRow)
	if err != nil {
		return err
	}
	for column, value := range values {
		merged[column] = value
	}

	updatedRow, err := EncodeRow(*schema, merged)
	if err != nil {
		return err
	}
	if _, err := file.WriteAt(updatedRow, offset); err != nil {
		return err
	}

	return rebuildIndexesLocked(*schema)
}

func DeleteRow(table string, rowID uint64) error {
	schema, err := LoadSchema(table)
	if err != nil {
		return err
	}

	lock := tableLock(schema.Name)
	lock.Lock()
	defer lock.Unlock()

	paths := tablePaths(schema.Name)
	file, err := openRowsFile(paths.rows, os.O_RDWR, 0o644)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrTableNotFound
		}
		return err
	}

	info, err := file.Stat()
	if err != nil {
		return err
	}
	rowSize := int64(schema.RowSize)
	if rowSize <= 0 || info.Size()%rowSize != 0 {
		return ErrCorruptRows
	}
	if rowID > uint64(math.MaxInt64)/schema.RowSize {
		return ErrRowNotFound
	}
	offset := int64(rowID * schema.RowSize)
	if offset >= info.Size() {
		return ErrRowNotFound
	}

	flag := []byte{0}
	if _, err := file.ReadAt(flag, offset); err != nil {
		if errors.Is(err, io.EOF) {
			return ErrCorruptRows
		}
		return err
	}
	if flag[0]&RowFlagActive == 0 {
		return ErrRowNotFound
	}
	if _, err := file.WriteAt([]byte{0}, offset); err != nil {
		return err
	}

	free, err := openFile(paths.free, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(free, "%d\n", rowID); err != nil {
		_ = free.Close()
		return err
	}
	if err := free.Close(); err != nil {
		return err
	}

	return rebuildIndexesLocked(*schema)
}

func Scan(table string, predicate RowPredicate) ([]map[string]any, error) {
	scanned, err := ScanRows(table, predicate)
	if err != nil {
		return nil, err
	}

	rows := make([]map[string]any, len(scanned))
	for i := range scanned {
		rows[i] = scanned[i].Values
	}
	return rows, nil
}

func ScanRows(table string, predicate RowPredicate) ([]ScannedRow, error) {
	schema, err := LoadSchema(table)
	if err != nil {
		return nil, err
	}

	lock := tableLock(schema.Name)
	lock.RLock()
	defer lock.RUnlock()

	return scanRowsLocked(*schema, func(values map[string]any) (bool, error) {
		if predicate == nil {
			return true, nil
		}
		return predicate(values), nil
	})
}

func Select(table string, predicate Predicate) ([]map[string]any, error) {
	scanned, err := SelectRows(table, predicate)
	if err != nil {
		return nil, err
	}

	rows := make([]map[string]any, len(scanned))
	for i := range scanned {
		rows[i] = scanned[i].Values
	}
	return rows, nil
}

func SelectRows(table string, predicate Predicate) ([]ScannedRow, error) {
	schema, err := LoadSchema(table)
	if err != nil {
		return nil, err
	}

	op := strings.ToLower(strings.TrimSpace(predicate.Op))
	switch op {
	case "", PredicateOpEqual, "=", "==":
		column, wantedKey, err := prepareEqualityPredicate(*schema, predicate)
		if err != nil {
			return nil, err
		}

		lock := tableLock(schema.Name)
		lock.RLock()
		defer lock.RUnlock()

		matchesPredicate := func(values map[string]any) (bool, error) {
			key, err := indexKey(column, values[column.Name])
			if err != nil {
				return false, err
			}
			return key == wantedKey, nil
		}

		if column.Indexed {
			index, err := loadEqualityIndex(schema.Name, column.Name)
			if errors.Is(err, os.ErrNotExist) {
				index, err = buildEqualityIndexLocked(*schema, column)
				if err == nil {
					err = writeEqualityIndex(index)
				}
			}
			if err != nil {
				return nil, err
			}
			return readRowsByIDsLocked(*schema, index.Values[wantedKey], matchesPredicate)
		}

		return scanRowsLocked(*schema, matchesPredicate)
	case PredicateOpLike:
		column, pattern, err := prepareLikePredicate(*schema, predicate)
		if err != nil {
			return nil, err
		}

		lock := tableLock(schema.Name)
		lock.RLock()
		defer lock.RUnlock()

		return selectLikeRowsLocked(*schema, column, pattern)
	default:
		return nil, fmt.Errorf("%w: unsupported op %q", ErrInvalidPredicate, predicate.Op)
	}
}

func prepareEqualityPredicate(schema Schema, predicate Predicate) (Column, string, error) {
	_, column, err := findColumn(schema, strings.TrimSpace(predicate.Column))
	if err != nil {
		return Column{}, "", err
	}

	key, err := indexKey(column, predicate.Value)
	if err != nil {
		return Column{}, "", fmt.Errorf("%w: column %q: equality value does not match %s", ErrInvalidPredicate, column.Name, column.Type)
	}
	return column, key, nil
}

func prepareLikePredicate(schema Schema, predicate Predicate) (Column, string, error) {
	_, column, err := findColumn(schema, strings.TrimSpace(predicate.Column))
	if err != nil {
		return Column{}, "", err
	}
	if column.Type != ColumnTypeString {
		return Column{}, "", fmt.Errorf("%w: column %q is %s, want %s for LIKE", ErrInvalidPredicate, column.Name, column.Type, ColumnTypeString)
	}
	pattern, ok := predicate.Value.(string)
	if !ok {
		return Column{}, "", fmt.Errorf("%w: column %q: LIKE pattern must be a string", ErrInvalidPredicate, column.Name)
	}
	return column, pattern, nil
}

func selectLikeRowsLocked(schema Schema, column Column, pattern string) ([]ScannedRow, error) {
	matchesPredicate := func(values map[string]any) (bool, error) {
		value, ok := values[column.Name].(string)
		if !ok {
			return false, fmt.Errorf("%w: column %q: string value required", ErrInvalidRow, column.Name)
		}
		return likeMatch(value, pattern), nil
	}

	grams := trigramsFromLikePattern(pattern)
	if column.TrigramIndexed && len(grams) > 0 {
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
		return readRowsByIDsLocked(schema, candidateRowIDsForTrigrams(index, grams), matchesPredicate)
	}

	return scanRowsLocked(schema, matchesPredicate)
}

func scanRowsLocked(schema Schema, predicate func(map[string]any) (bool, error)) ([]ScannedRow, error) {
	paths := tablePaths(schema.Name)
	file, err := openRowsFile(paths.rows, os.O_RDWR, 0o644)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrTableNotFound
		}
		return nil, err
	}

	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	rowSize := int64(schema.RowSize)
	if rowSize <= 0 || info.Size()%rowSize != 0 {
		return nil, ErrCorruptRows
	}

	rowCount := uint64(info.Size() / rowSize)
	matches := make([]ScannedRow, 0)
	row := make([]byte, schema.RowSize)
	for rowID := uint64(0); rowID < rowCount; rowID++ {
		offset := int64(rowID * schema.RowSize)
		if _, err := file.ReadAt(row, offset); err != nil {
			if errors.Is(err, io.EOF) {
				return nil, ErrCorruptRows
			}
			return nil, err
		}

		if row[0]&RowFlagActive == 0 {
			continue
		}

		values, err := DecodeRow(schema, row)
		if err != nil {
			return nil, err
		}
		if predicate != nil {
			ok, err := predicate(values)
			if err != nil {
				return nil, err
			}
			if !ok {
				continue
			}
		}

		matches = append(matches, ScannedRow{
			RowID:  rowID,
			Values: values,
		})
	}
	return matches, nil
}

func readRowsByIDsLocked(schema Schema, rowIDs []uint64, predicate func(map[string]any) (bool, error)) ([]ScannedRow, error) {
	if len(rowIDs) == 0 {
		return nil, nil
	}

	paths := tablePaths(schema.Name)
	file, err := openRowsFile(paths.rows, os.O_RDWR, 0o644)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrTableNotFound
		}
		return nil, err
	}

	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	rowSize := int64(schema.RowSize)
	if rowSize <= 0 || info.Size()%rowSize != 0 {
		return nil, ErrCorruptRows
	}

	matches := make([]ScannedRow, 0, len(rowIDs))
	row := make([]byte, schema.RowSize)
	for _, rowID := range rowIDs {
		if rowID > uint64(math.MaxInt64)/schema.RowSize {
			return nil, ErrCorruptRows
		}
		offset := int64(rowID * schema.RowSize)
		if offset >= info.Size() {
			return nil, ErrCorruptRows
		}

		if _, err := file.ReadAt(row, offset); err != nil {
			if errors.Is(err, io.EOF) {
				return nil, ErrCorruptRows
			}
			return nil, err
		}

		if row[0]&RowFlagActive == 0 {
			continue
		}

		values, err := DecodeRow(schema, row)
		if err != nil {
			return nil, err
		}
		if predicate != nil {
			ok, err := predicate(values)
			if err != nil {
				return nil, err
			}
			if !ok {
				continue
			}
		}

		matches = append(matches, ScannedRow{
			RowID:  rowID,
			Values: values,
		})
	}
	return matches, nil
}

func EncodeRow(schema Schema, values map[string]any) ([]byte, error) {
	if schema.RowHeaderSize != RowHeaderSize {
		return nil, fmt.Errorf("%w: unsupported row header size %d", ErrInvalidSchema, schema.RowHeaderSize)
	}
	if schema.RowSize < schema.RowHeaderSize {
		return nil, fmt.Errorf("%w: row size is smaller than row header", ErrInvalidSchema)
	}

	knownColumns := make(map[string]Column, len(schema.Columns))
	for _, column := range schema.Columns {
		knownColumns[column.Name] = column
	}
	for name := range values {
		if _, ok := knownColumns[name]; !ok {
			return nil, fmt.Errorf("%w: unknown column %q", ErrInvalidRow, name)
		}
	}

	row := make([]byte, schema.RowSize)
	row[0] = RowFlagActive
	binary.LittleEndian.PutUint32(row[1:5], RowVersion)

	for _, column := range schema.Columns {
		value, ok := values[column.Name]
		if !ok || value == nil {
			continue
		}

		start := schema.RowHeaderSize + column.Offset
		end := start + column.Size
		if end > uint64(len(row)) {
			return nil, fmt.Errorf("%w: column %q exceeds row size", ErrInvalidSchema, column.Name)
		}
		if err := encodeColumn(row[start:end], column, value); err != nil {
			return nil, err
		}
	}

	return row, nil
}

func DecodeRow(schema Schema, row []byte) (map[string]any, error) {
	if schema.RowHeaderSize != RowHeaderSize {
		return nil, fmt.Errorf("%w: unsupported row header size %d", ErrInvalidSchema, schema.RowHeaderSize)
	}
	if schema.RowSize < schema.RowHeaderSize {
		return nil, fmt.Errorf("%w: row size is smaller than row header", ErrInvalidSchema)
	}
	if uint64(len(row)) != schema.RowSize {
		return nil, fmt.Errorf("%w: row length is %d, want %d", ErrInvalidRow, len(row), schema.RowSize)
	}
	if row[0]&RowFlagActive == 0 {
		return nil, fmt.Errorf("%w: row is not active", ErrInvalidRow)
	}
	if version := binary.LittleEndian.Uint32(row[1:5]); version != RowVersion {
		return nil, fmt.Errorf("%w: row version is %d, want %d", ErrInvalidRow, version, RowVersion)
	}

	values := make(map[string]any, len(schema.Columns))
	for _, column := range schema.Columns {
		start := schema.RowHeaderSize + column.Offset
		end := start + column.Size
		if end > uint64(len(row)) {
			return nil, fmt.Errorf("%w: column %q exceeds row size", ErrInvalidSchema, column.Name)
		}
		value, err := decodeColumn(row[start:end], column)
		if err != nil {
			return nil, err
		}
		values[column.Name] = value
	}
	return values, nil
}

func encodeColumn(dst []byte, column Column, value any) error {
	switch column.Type {
	case ColumnTypeUint64, ColumnTypeBlobPtr, ColumnTypeRowRef:
		encoded, err := asUint64(value)
		if err != nil {
			return fmt.Errorf("%w: column %q: %v", ErrInvalidRow, column.Name, err)
		}
		binary.LittleEndian.PutUint64(dst, encoded)
	case ColumnTypeInt64:
		encoded, err := asInt64(value)
		if err != nil {
			return fmt.Errorf("%w: column %q: %v", ErrInvalidRow, column.Name, err)
		}
		binary.LittleEndian.PutUint64(dst, uint64(encoded))
	case ColumnTypeBool:
		encoded, ok := value.(bool)
		if !ok {
			return fmt.Errorf("%w: column %q: bool value required", ErrInvalidRow, column.Name)
		}
		if encoded {
			dst[0] = 1
		}
	case ColumnTypeFloat64:
		encoded, err := asFloat64(value)
		if err != nil {
			return fmt.Errorf("%w: column %q: %v", ErrInvalidRow, column.Name, err)
		}
		binary.LittleEndian.PutUint64(dst, math.Float64bits(encoded))
	case ColumnTypeString:
		encoded, ok := value.(string)
		if !ok {
			return fmt.Errorf("%w: column %q: string value required", ErrInvalidRow, column.Name)
		}
		if len(encoded) > len(dst) {
			return fmt.Errorf("%w: column %q: string is %d bytes, max %d", ErrInvalidRow, column.Name, len(encoded), len(dst))
		}
		copy(dst, encoded)
	default:
		return fmt.Errorf("%w: column %q: unsupported type %q", ErrInvalidSchema, column.Name, column.Type)
	}
	return nil
}

func decodeColumn(src []byte, column Column) (any, error) {
	switch column.Type {
	case ColumnTypeUint64, ColumnTypeBlobPtr, ColumnTypeRowRef:
		return binary.LittleEndian.Uint64(src), nil
	case ColumnTypeInt64:
		return int64(binary.LittleEndian.Uint64(src)), nil
	case ColumnTypeBool:
		return src[0] != 0, nil
	case ColumnTypeFloat64:
		return math.Float64frombits(binary.LittleEndian.Uint64(src)), nil
	case ColumnTypeString:
		return string(bytes.TrimRight(src, "\x00")), nil
	default:
		return nil, fmt.Errorf("%w: column %q: unsupported type %q", ErrInvalidSchema, column.Name, column.Type)
	}
}

func asUint64(value any) (uint64, error) {
	switch v := value.(type) {
	case uint64:
		return v, nil
	case uint:
		return uint64(v), nil
	case uint32:
		return uint64(v), nil
	case uint16:
		return uint64(v), nil
	case uint8:
		return uint64(v), nil
	case int:
		if v < 0 {
			return 0, errors.New("unsigned value cannot be negative")
		}
		return uint64(v), nil
	case int64:
		if v < 0 {
			return 0, errors.New("unsigned value cannot be negative")
		}
		return uint64(v), nil
	case int32:
		if v < 0 {
			return 0, errors.New("unsigned value cannot be negative")
		}
		return uint64(v), nil
	case int16:
		if v < 0 {
			return 0, errors.New("unsigned value cannot be negative")
		}
		return uint64(v), nil
	case int8:
		if v < 0 {
			return 0, errors.New("unsigned value cannot be negative")
		}
		return uint64(v), nil
	case json.Number:
		parsed, err := strconv.ParseUint(v.String(), 10, 64)
		if err != nil {
			return 0, errors.New("uint64 value required")
		}
		return parsed, nil
	default:
		return 0, errors.New("uint64 value required")
	}
}

func asInt64(value any) (int64, error) {
	switch v := value.(type) {
	case int64:
		return v, nil
	case int:
		return int64(v), nil
	case int32:
		return int64(v), nil
	case int16:
		return int64(v), nil
	case int8:
		return int64(v), nil
	case uint:
		if uint64(v) > math.MaxInt64 {
			return 0, errors.New("int64 value required")
		}
		return int64(v), nil
	case uint64:
		if v > math.MaxInt64 {
			return 0, errors.New("int64 value required")
		}
		return int64(v), nil
	case uint32:
		return int64(v), nil
	case uint16:
		return int64(v), nil
	case uint8:
		return int64(v), nil
	case json.Number:
		parsed, err := strconv.ParseInt(v.String(), 10, 64)
		if err != nil {
			return 0, errors.New("int64 value required")
		}
		return parsed, nil
	default:
		return 0, errors.New("int64 value required")
	}
}

func asFloat64(value any) (float64, error) {
	switch v := value.(type) {
	case float64:
		return v, nil
	case float32:
		return float64(v), nil
	case int:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case int32:
		return float64(v), nil
	case int16:
		return float64(v), nil
	case int8:
		return float64(v), nil
	case uint:
		return float64(v), nil
	case uint64:
		return float64(v), nil
	case uint32:
		return float64(v), nil
	case uint16:
		return float64(v), nil
	case uint8:
		return float64(v), nil
	case json.Number:
		parsed, err := strconv.ParseFloat(v.String(), 64)
		if err != nil {
			return 0, errors.New("float64 value required")
		}
		return parsed, nil
	default:
		return 0, errors.New("float64 value required")
	}
}

func trigramsFromLikePattern(pattern string) []string {
	seen := make(map[string]struct{})
	grams := make([]string, 0)
	addFragment := func(fragment string) {
		for _, gram := range trigramsForIndex(fragment) {
			if _, ok := seen[gram]; ok {
				continue
			}
			seen[gram] = struct{}{}
			grams = append(grams, gram)
		}
	}

	start := 0
	for i := 0; i < len(pattern); i++ {
		if pattern[i] != '%' && pattern[i] != '_' {
			continue
		}
		addFragment(pattern[start:i])
		start = i + 1
	}
	addFragment(pattern[start:])
	return grams
}

func candidateRowIDsForTrigrams(index trigramIndex, grams []string) []uint64 {
	if len(grams) == 0 {
		return nil
	}

	best := -1
	for i, gram := range grams {
		rows, ok := index.Values[gram]
		if !ok {
			return nil
		}
		if best == -1 || len(rows) < len(index.Values[grams[best]]) {
			best = i
		}
	}

	candidates := append([]uint64(nil), index.Values[grams[best]]...)
	for i, gram := range grams {
		if i == best {
			continue
		}
		candidates = intersectSortedUint64(candidates, index.Values[gram])
		if len(candidates) == 0 {
			return nil
		}
	}
	return candidates
}

func intersectSortedUint64(left, right []uint64) []uint64 {
	out := make([]uint64, 0)
	i, j := 0, 0
	for i < len(left) && j < len(right) {
		switch {
		case left[i] == right[j]:
			if len(out) == 0 || out[len(out)-1] != left[i] {
				out = append(out, left[i])
			}
			i++
			j++
		case left[i] < right[j]:
			i++
		default:
			j++
		}
	}
	return out
}

func likeMatch(value, pattern string) bool {
	previous := make([]bool, len(pattern)+1)
	previous[0] = true
	for j := 1; j <= len(pattern); j++ {
		previous[j] = previous[j-1] && pattern[j-1] == '%'
	}

	for i := 1; i <= len(value); i++ {
		current := make([]bool, len(pattern)+1)
		for j := 1; j <= len(pattern); j++ {
			switch pattern[j-1] {
			case '%':
				current[j] = current[j-1] || previous[j]
			case '_':
				current[j] = previous[j-1]
			default:
				current[j] = previous[j-1] && value[i-1] == pattern[j-1]
			}
		}
		previous = current
	}
	return previous[len(pattern)]
}
