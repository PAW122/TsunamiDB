package relational

import (
	"errors"
	"fmt"
	"io"
	"os"
)

func CompactTable(table string) (CompactionResult, error) {
	schema, err := LoadSchema(table)
	if err != nil {
		return CompactionResult{}, err
	}

	lock := tableLock(schema.Name)
	lock.Lock()
	defer lock.Unlock()

	return compactTableLocked(*schema)
}

func compactTableLocked(schema Schema) (CompactionResult, error) {
	paths := tablePaths(schema.Name)
	if err := closeRowsFile(paths.rows); err != nil {
		return CompactionResult{}, err
	}
	input, err := openFile(paths.rows, os.O_RDONLY, 0o644)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return CompactionResult{}, ErrTableNotFound
		}
		return CompactionResult{}, err
	}

	info, err := input.Stat()
	if err != nil {
		_ = input.Close()
		return CompactionResult{}, err
	}
	rowSize := int64(schema.RowSize)
	if rowSize <= 0 || info.Size()%rowSize != 0 {
		_ = input.Close()
		return CompactionResult{}, ErrCorruptRows
	}

	result := CompactionResult{
		Table:      schema.Name,
		RowsBefore: uint64(info.Size() / rowSize),
		RowIDMap:   make(map[uint64]uint64),
	}

	equalityIndexes := emptyEqualityIndexes(schema)
	trigramIndexes := emptyTrigramIndexes(schema)

	tmpRows := paths.rows + ".compact.tmp"
	output, err := openFile(tmpRows, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		_ = input.Close()
		return CompactionResult{}, err
	}
	tmpRowsReady := false
	defer func() {
		if !tmpRowsReady {
			_ = removeFile(tmpRows)
		}
	}()

	row := make([]byte, schema.RowSize)
	for oldRowID := uint64(0); oldRowID < result.RowsBefore; oldRowID++ {
		if _, err := io.ReadFull(input, row); err != nil {
			_ = output.Close()
			_ = input.Close()
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return CompactionResult{}, ErrCorruptRows
			}
			return CompactionResult{}, err
		}
		if row[0]&RowFlagActive == 0 {
			continue
		}

		values, err := DecodeRow(schema, row)
		if err != nil {
			_ = output.Close()
			_ = input.Close()
			return CompactionResult{}, err
		}

		newRowID := result.RowsAfter
		if _, err := output.Write(row); err != nil {
			_ = output.Close()
			_ = input.Close()
			return CompactionResult{}, err
		}
		result.RowIDMap[oldRowID] = newRowID
		result.RowsAfter++

		if err := appendRowToEqualityIndexValues(schema, equalityIndexes, values, newRowID); err != nil {
			_ = output.Close()
			_ = input.Close()
			return CompactionResult{}, err
		}
		if err := appendRowToTrigramIndexValues(schema, trigramIndexes, values, newRowID); err != nil {
			_ = output.Close()
			_ = input.Close()
			return CompactionResult{}, err
		}
	}
	result.Removed = result.RowsBefore - result.RowsAfter

	if err := output.Close(); err != nil {
		_ = input.Close()
		return CompactionResult{}, err
	}
	if err := input.Close(); err != nil {
		return CompactionResult{}, err
	}

	if err := replaceRowsFile(paths.rows, tmpRows); err != nil {
		return CompactionResult{}, err
	}
	tmpRowsReady = true

	if err := writeFile(paths.free, nil, 0o644); err != nil {
		return CompactionResult{}, err
	}
	if err := writeEqualityIndexes(equalityIndexes); err != nil {
		return CompactionResult{}, err
	}
	if err := writeTrigramIndexes(trigramIndexes); err != nil {
		return CompactionResult{}, err
	}

	return result, nil
}

func emptyEqualityIndexes(schema Schema) map[string]equalityIndex {
	indexes := make(map[string]equalityIndex)
	for _, column := range schema.Columns {
		if !column.Indexed {
			continue
		}
		indexes[column.Name] = equalityIndex{
			Table:  schema.Name,
			Column: column.Name,
			Values: make(map[string][]uint64),
		}
	}
	return indexes
}

func emptyTrigramIndexes(schema Schema) map[string]trigramIndex {
	indexes := make(map[string]trigramIndex)
	for _, column := range schema.Columns {
		if !column.TrigramIndexed {
			continue
		}
		indexes[column.Name] = trigramIndex{
			Table:  schema.Name,
			Column: column.Name,
			Values: make(map[string][]uint64),
		}
	}
	return indexes
}

func appendRowToEqualityIndexValues(schema Schema, indexes map[string]equalityIndex, values map[string]any, rowID uint64) error {
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
		indexes[column.Name] = index
	}
	return nil
}

func appendRowToTrigramIndexValues(schema Schema, indexes map[string]trigramIndex, values map[string]any, rowID uint64) error {
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
		indexes[column.Name] = index
	}
	return nil
}

func writeEqualityIndexes(indexes map[string]equalityIndex) error {
	for _, index := range indexes {
		if err := writeEqualityIndex(index); err != nil {
			return err
		}
	}
	return nil
}

func writeTrigramIndexes(indexes map[string]trigramIndex) error {
	for _, index := range indexes {
		if err := writeTrigramIndex(index); err != nil {
			return err
		}
	}
	return nil
}

func replaceRowsFile(path, tmp string) error {
	backup := path + ".compact.bak"
	if err := removeFile(backup); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := renameFile(path, backup); err != nil {
		return err
	}
	if err := renameFile(tmp, path); err != nil {
		_ = renameFile(backup, path)
		return err
	}
	if err := removeFile(backup); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
