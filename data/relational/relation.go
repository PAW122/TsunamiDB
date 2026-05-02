package relational

import (
	"fmt"
	"strings"
)

func ReadRowRef(table string, rowID uint64, column string) (ReferencedRow, error) {
	schema, err := LoadSchema(table)
	if err != nil {
		return ReferencedRow{}, err
	}

	refColumn, err := rowRefColumn(*schema, column)
	if err != nil {
		return ReferencedRow{}, err
	}

	values, err := ReadRow(schema.Name, rowID)
	if err != nil {
		return ReferencedRow{}, err
	}

	refID, err := rowRefID(refColumn, values)
	if err != nil {
		return ReferencedRow{}, err
	}

	referenced, err := ReadRow(refColumn.RefTable, refID)
	if err != nil {
		return ReferencedRow{}, err
	}

	return ReferencedRow{
		Table:  refColumn.RefTable,
		RowID:  refID,
		Values: referenced,
	}, nil
}

func JoinRowRef(table string, refColumn string, predicate *Predicate) ([]JoinedRow, error) {
	schema, err := LoadSchema(table)
	if err != nil {
		return nil, err
	}

	column, err := rowRefColumn(*schema, refColumn)
	if err != nil {
		return nil, err
	}

	var rows []ScannedRow
	if predicate == nil {
		rows, err = ScanRows(schema.Name, nil)
	} else {
		rows, err = SelectRows(schema.Name, *predicate)
	}
	if err != nil {
		return nil, err
	}

	joined := make([]JoinedRow, 0, len(rows))
	for _, row := range rows {
		refID, err := rowRefID(column, row.Values)
		if err != nil {
			return nil, err
		}

		referenced, err := ReadRow(column.RefTable, refID)
		if err != nil {
			return nil, err
		}

		joined = append(joined, JoinedRow{
			RowID:     row.RowID,
			Values:    row.Values,
			RefColumn: column.Name,
			Referenced: ReferencedRow{
				Table:  column.RefTable,
				RowID:  refID,
				Values: referenced,
			},
		})
	}
	return joined, nil
}

func rowRefColumn(schema Schema, columnName string) (Column, error) {
	_, column, err := findColumn(schema, strings.TrimSpace(columnName))
	if err != nil {
		return Column{}, err
	}
	if column.Type != ColumnTypeRowRef {
		return Column{}, fmt.Errorf("%w: column %q is %s, want %s", ErrInvalidSchema, column.Name, column.Type, ColumnTypeRowRef)
	}
	if column.RefTable == "" {
		return Column{}, fmt.Errorf("%w: column %q does not declare ref_table", ErrInvalidSchema, column.Name)
	}
	return column, nil
}

func rowRefID(column Column, values map[string]any) (uint64, error) {
	value, ok := values[column.Name]
	if !ok {
		return 0, fmt.Errorf("%w: column %q missing from row", ErrInvalidRow, column.Name)
	}
	refID, ok := value.(uint64)
	if !ok {
		return 0, fmt.Errorf("%w: column %q row_ref decoded as %T", ErrInvalidRow, column.Name, value)
	}
	return refID, nil
}
