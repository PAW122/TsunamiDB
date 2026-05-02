package relational

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

var schemaCache sync.Map // map[string]Schema

func LoadSchema(table string) (*Schema, error) {
	table = strings.TrimSpace(table)
	if !safeNamePattern.MatchString(table) {
		return nil, fmt.Errorf("%w: table name must match %s", ErrInvalidSchema, safeNamePattern.String())
	}
	cacheKey := schemaCacheKey(table)
	if cached, ok := schemaCache.Load(cacheKey); ok {
		schema := cached.(Schema)
		return &schema, nil
	}

	data, err := readFile(tablePaths(table).schema)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrTableNotFound
		}
		return nil, err
	}

	var schema Schema
	if err := json.Unmarshal(data, &schema); err != nil {
		return nil, err
	}
	if schema.Name != table {
		return nil, fmt.Errorf("%w: schema name %q does not match table %q", ErrInvalidSchema, schema.Name, table)
	}

	calculated, err := CalculateSchema(schema)
	if err != nil {
		return nil, err
	}
	if schema.RowHeaderSize != calculated.RowHeaderSize || schema.RowSize != calculated.RowSize {
		return nil, fmt.Errorf("%w: persisted row layout does not match calculated layout", ErrInvalidSchema)
	}
	for i := range schema.Columns {
		if schema.Columns[i].Offset != calculated.Columns[i].Offset ||
			schema.Columns[i].Size != calculated.Columns[i].Size ||
			schema.Columns[i].Type != calculated.Columns[i].Type ||
			schema.Columns[i].RefTable != calculated.Columns[i].RefTable {
			return nil, fmt.Errorf("%w: persisted column layout does not match calculated layout", ErrInvalidSchema)
		}
	}
	schemaCache.Store(cacheKey, calculated)
	return &calculated, nil
}

func persistSchema(schema Schema) error {
	data, err := marshalJSON(schema, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	path := tablePaths(schema.Name).schema
	tmp := path + ".tmp"
	if err := writeFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := renameFile(tmp, path); err != nil {
		_ = removeFile(tmp)
		return err
	}
	schemaCache.Store(schemaCacheKey(schema.Name), schema)
	return nil
}

func schemaCacheKey(table string) string {
	path := tablePaths(table).schema
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return abs
}

func resetSchemaCacheForTests() {
	schemaCache = sync.Map{}
}

func CalculateSchema(schema Schema) (Schema, error) {
	schema.Name = strings.TrimSpace(schema.Name)
	if !safeNamePattern.MatchString(schema.Name) {
		return Schema{}, fmt.Errorf("%w: table name must match %s", ErrInvalidSchema, safeNamePattern.String())
	}
	if len(schema.Columns) == 0 {
		return Schema{}, fmt.Errorf("%w: at least one column is required", ErrInvalidSchema)
	}

	out := Schema{
		Name:          schema.Name,
		RowHeaderSize: RowHeaderSize,
		Columns:       make([]Column, len(schema.Columns)),
	}

	seen := make(map[string]struct{}, len(schema.Columns))
	offset := uint64(0)
	for i, column := range schema.Columns {
		column.Name = strings.TrimSpace(column.Name)
		column.Type = strings.TrimSpace(column.Type)
		column.RefTable = strings.TrimSpace(column.RefTable)

		if !safeNamePattern.MatchString(column.Name) {
			return Schema{}, fmt.Errorf("%w: column %d name must match %s", ErrInvalidSchema, i, safeNamePattern.String())
		}
		if _, ok := seen[column.Name]; ok {
			return Schema{}, fmt.Errorf("%w: duplicate column %q", ErrInvalidSchema, column.Name)
		}
		seen[column.Name] = struct{}{}

		size, normalizedType, err := columnSize(column.Type, column.Size)
		if err != nil {
			return Schema{}, fmt.Errorf("%w: column %q: %v", ErrInvalidSchema, column.Name, err)
		}

		column.Type = normalizedType
		if column.TrigramIndexed && normalizedType != ColumnTypeString {
			return Schema{}, fmt.Errorf("%w: column %q: trigram indexes require string columns", ErrInvalidSchema, column.Name)
		}
		if column.RefTable != "" {
			if normalizedType != ColumnTypeRowRef {
				return Schema{}, fmt.Errorf("%w: column %q: ref_table is only valid for row_ref columns", ErrInvalidSchema, column.Name)
			}
			if !safeNamePattern.MatchString(column.RefTable) {
				return Schema{}, fmt.Errorf("%w: column %q ref_table must match %s", ErrInvalidSchema, column.Name, safeNamePattern.String())
			}
		}
		column.Offset = offset
		column.Size = size
		out.Columns[i] = column
		offset += size
	}

	out.RowSize = out.RowHeaderSize + offset
	return out, nil
}

func columnSize(columnType string, requested uint64) (uint64, string, error) {
	if strings.HasPrefix(columnType, "string[") && strings.HasSuffix(columnType, "]") {
		rawSize := strings.TrimSuffix(strings.TrimPrefix(columnType, "string["), "]")
		parsed, err := strconv.ParseUint(rawSize, 10, 64)
		if err != nil || parsed == 0 {
			return 0, "", fmt.Errorf("invalid string size %q", rawSize)
		}
		if requested != 0 && requested != parsed {
			return 0, "", fmt.Errorf("size %d does not match type size %d", requested, parsed)
		}
		return parsed, ColumnTypeString, nil
	}

	switch columnType {
	case ColumnTypeUint64, ColumnTypeInt64, ColumnTypeFloat64, ColumnTypeBlobPtr, ColumnTypeRowRef:
		return fixedColumnSize(columnType, requested, 8)
	case ColumnTypeBool:
		return fixedColumnSize(columnType, requested, 1)
	case ColumnTypeString:
		if requested == 0 {
			return 0, "", errors.New("string columns require size")
		}
		return requested, ColumnTypeString, nil
	default:
		return 0, "", fmt.Errorf("unsupported type %q", columnType)
	}
}

func fixedColumnSize(columnType string, requested, fixed uint64) (uint64, string, error) {
	if requested != 0 && requested != fixed {
		return 0, "", fmt.Errorf("%s columns must be %d bytes", columnType, fixed)
	}
	return fixed, columnType, nil
}
