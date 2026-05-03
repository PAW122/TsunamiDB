package relational

import "errors"

const (
	baseDir       = "./db/rel"
	RowHeaderSize = uint64(8)
)

const (
	RowFlagActive = byte(1)
	RowVersion    = uint32(1)
)

const (
	ColumnTypeUint64  = "uint64"
	ColumnTypeInt64   = "int64"
	ColumnTypeBool    = "bool"
	ColumnTypeFloat64 = "float64"
	ColumnTypeString  = "string"
	ColumnTypeBlobPtr = "blob_ptr"
	ColumnTypeRowRef  = "row_ref"
)

var (
	ErrInvalidSchema    = errors.New("relational: invalid schema")
	ErrTableExists      = errors.New("relational: table already exists")
	ErrTableNotFound    = errors.New("relational: table not found")
	ErrRowNotFound      = errors.New("relational: row not found")
	ErrInvalidRow       = errors.New("relational: invalid row")
	ErrInvalidPredicate = errors.New("relational: invalid predicate")
	ErrInvalidSQL       = errors.New("relational: invalid sql")
	ErrCorruptRows      = errors.New("relational: corrupt rows file")
)

type Schema struct {
	Name          string   `json:"name"`
	RowSize       uint64   `json:"row_size"`
	RowHeaderSize uint64   `json:"row_header_size"`
	Columns       []Column `json:"columns"`
}

type Column struct {
	Name           string `json:"name"`
	Type           string `json:"type"`
	Offset         uint64 `json:"offset"`
	Size           uint64 `json:"size"`
	Indexed        bool   `json:"indexed,omitempty"`
	TrigramIndexed bool   `json:"trigram_indexed,omitempty"`
	RefTable       string `json:"ref_table,omitempty"`
}

type RowPredicate = func(row map[string]any) bool

const (
	PredicateOpEqual = "eq"
	PredicateOpLike  = "like"
)

type Predicate struct {
	Column string `json:"column"`
	Op     string `json:"op"`
	Value  any    `json:"value"`
}

func Equal(column string, value any) Predicate {
	return Predicate{
		Column: column,
		Op:     PredicateOpEqual,
		Value:  value,
	}
}

func Like(column string, pattern string) Predicate {
	return Predicate{
		Column: column,
		Op:     PredicateOpLike,
		Value:  pattern,
	}
}

type ScannedRow struct {
	RowID  uint64         `json:"row_id"`
	Values map[string]any `json:"values"`
}

type ReferencedRow struct {
	Table  string         `json:"table"`
	RowID  uint64         `json:"row_id"`
	Values map[string]any `json:"values"`
}

type JoinedRow struct {
	RowID      uint64         `json:"row_id"`
	Values     map[string]any `json:"values"`
	RefColumn  string         `json:"ref_column"`
	Referenced ReferencedRow  `json:"referenced"`
}

type CompactionResult struct {
	Table      string            `json:"table"`
	RowsBefore uint64            `json:"rows_before"`
	RowsAfter  uint64            `json:"rows_after"`
	Removed    uint64            `json:"removed"`
	RowIDMap   map[uint64]uint64 `json:"row_id_map"`
}

type SQLResult struct {
	Operation    string       `json:"operation"`
	Table        string       `json:"table,omitempty"`
	RowID        *uint64      `json:"row_id,omitempty"`
	RowsAffected uint64       `json:"rows_affected,omitempty"`
	Rows         []ScannedRow `json:"rows,omitempty"`
	Schema       *Schema      `json:"schema,omitempty"`
}
