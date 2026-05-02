package relational

import (
	"errors"
	"os"
)

// CreateTable validates a schema, calculates fixed column offsets, and writes
// db/rel/<table>.schema. Empty .rows and .free files are created for the next
// implementation steps.
func CreateTable(schema Schema) (*Schema, error) {
	calculated, err := CalculateSchema(schema)
	if err != nil {
		return nil, err
	}

	if err := mkdirAll(baseDir, 0o755); err != nil {
		return nil, err
	}

	paths := tablePaths(calculated.Name)
	if _, err := statFile(paths.schema); err == nil {
		return nil, ErrTableExists
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	if err := persistSchema(calculated); err != nil {
		return nil, err
	}

	if err := ensureEmptyFile(paths.rows); err != nil {
		return nil, err
	}
	if err := ensureEmptyFile(paths.free); err != nil {
		return nil, err
	}
	for _, column := range calculated.Columns {
		if column.Indexed {
			index := equalityIndex{
				Table:  calculated.Name,
				Column: column.Name,
				Values: make(map[string][]uint64),
			}
			if err := writeEqualityIndex(index); err != nil {
				return nil, err
			}
		}
		if column.TrigramIndexed {
			index := trigramIndex{
				Table:  calculated.Name,
				Column: column.Name,
				Values: make(map[string][]uint64),
			}
			if err := writeTrigramIndex(index); err != nil {
				return nil, err
			}
		}
	}

	return &calculated, nil
}
