package TsuClient

import (
	"encoding/json"

	"github.com/PAW122/TsunamiDB/data/relational"
	debug "github.com/PAW122/TsunamiDB/servers/debug"
)

const (
	RelationalColumnTypeUint64  = relational.ColumnTypeUint64
	RelationalColumnTypeInt64   = relational.ColumnTypeInt64
	RelationalColumnTypeBool    = relational.ColumnTypeBool
	RelationalColumnTypeFloat64 = relational.ColumnTypeFloat64
	RelationalColumnTypeString  = relational.ColumnTypeString
	RelationalColumnTypeBlobPtr = relational.ColumnTypeBlobPtr
	RelationalColumnTypeRowRef  = relational.ColumnTypeRowRef

	RelationalPredicateOpEqual = relational.PredicateOpEqual
	RelationalPredicateOpLike  = relational.PredicateOpLike
)

type RelationalSchema = relational.Schema
type RelationalColumn = relational.Column
type RelationalPredicate = relational.Predicate
type RelationalScannedRow = relational.ScannedRow
type RelationalReferencedRow = relational.ReferencedRow
type RelationalJoinedRow = relational.JoinedRow
type RelationalCompactionResult = relational.CompactionResult
type RelationalSQLResult = relational.SQLResult

func RelationalEqual(column string, value any) RelationalPredicate {
	return relational.Equal(column, value)
}

func RelationalLike(column string, pattern string) RelationalPredicate {
	return relational.Like(column, pattern)
}

func CreateRelationalTable(schema RelationalSchema) (*RelationalSchema, error) {
	defer debug.MeasureTime("[lib.dbclient] [relational-create-table]")()
	return relational.CreateTable(schema)
}

func LoadRelationalSchema(table string) (*RelationalSchema, error) {
	defer debug.MeasureTime("[lib.dbclient] [relational-load-schema]")()
	return relational.LoadSchema(table)
}

func ListRelationalTables() ([]RelationalSchema, error) {
	defer debug.MeasureTime("[lib.dbclient] [relational-list-tables]")()
	return relational.ListTables()
}

func InsertRelationalRow(table string, values map[string]any) (uint64, error) {
	defer debug.MeasureTime("[lib.dbclient] [relational-insert-row]")()
	return relational.InsertRow(table, values)
}

func ReadRelationalRow(table string, rowID uint64) (map[string]any, error) {
	defer debug.MeasureTime("[lib.dbclient] [relational-read-row]")()
	return relational.ReadRow(table, rowID)
}

func UpdateRelationalRow(table string, rowID uint64, values map[string]any) error {
	defer debug.MeasureTime("[lib.dbclient] [relational-update-row]")()
	return relational.UpdateRow(table, rowID, values)
}

func DeleteRelationalRow(table string, rowID uint64) error {
	defer debug.MeasureTime("[lib.dbclient] [relational-delete-row]")()
	return relational.DeleteRow(table, rowID)
}

func SelectRelationalRows(table string, predicate RelationalPredicate) ([]RelationalScannedRow, error) {
	defer debug.MeasureTime("[lib.dbclient] [relational-select-rows]")()
	return relational.SelectRows(table, predicate)
}

func ScanRelationalRows(table string) ([]RelationalScannedRow, error) {
	defer debug.MeasureTime("[lib.dbclient] [relational-scan-rows]")()
	return relational.ScanRows(table, nil)
}

func CreateRelationalIndex(table, column string) error {
	defer debug.MeasureTime("[lib.dbclient] [relational-create-index]")()
	return relational.CreateIndex(table, column)
}

func CreateRelationalTrigramIndex(table, column string) error {
	defer debug.MeasureTime("[lib.dbclient] [relational-create-trigram-index]")()
	return relational.CreateTrigramIndex(table, column)
}

func ReadRelationalRowRef(table string, rowID uint64, column string) (RelationalReferencedRow, error) {
	defer debug.MeasureTime("[lib.dbclient] [relational-read-row-ref]")()
	return relational.ReadRowRef(table, rowID, column)
}

func JoinRelationalRowRef(table string, refColumn string, predicate *RelationalPredicate) ([]RelationalJoinedRow, error) {
	defer debug.MeasureTime("[lib.dbclient] [relational-join-row-ref]")()
	return relational.JoinRowRef(table, refColumn, predicate)
}

func CompactRelationalTable(table string) (RelationalCompactionResult, error) {
	defer debug.MeasureTime("[lib.dbclient] [relational-compact-table]")()
	return relational.CompactTable(table)
}

func ExecuteRelationalSQL(query string) (*RelationalSQLResult, error) {
	defer debug.MeasureTime("[lib.dbclient] [relational-sql]")()
	return relational.ExecuteSQL(query)
}

func ExecuteRelationalSQLJSON(query string) ([]byte, error) {
	result, err := ExecuteRelationalSQL(query)
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}
