package relational

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type sqlTokenKind int

const (
	sqlTokenEOF sqlTokenKind = iota
	sqlTokenIdentifier
	sqlTokenNumber
	sqlTokenString
	sqlTokenSymbol
)

type sqlToken struct {
	kind sqlTokenKind
	lit  string
}

type sqlParser struct {
	tokens []sqlToken
	pos    int
}

type sqlWhere struct {
	column string
	op     string
	value  any
}

type sqlOrder struct {
	column string
	desc   bool
}

// ExecuteSQL translates a compact SQL subset onto the fixed-row relational API.
// Supported statements:
//   - CREATE TABLE table (column type [INDEXED] [TRIGRAM] [REFERENCES table], ...)
//   - CREATE [TRIGRAM] INDEX [name] ON table (column)
//   - INSERT INTO table (column, ...) VALUES (value, ...)
//   - SELECT *|column,... FROM table [WHERE column = value|column LIKE value|row_id = value] [ORDER BY column [ASC|DESC]]
//   - UPDATE table SET column = value, ... [WHERE ...]
//   - DELETE FROM table [WHERE ...]
//   - SHOW TABLES
func ExecuteSQL(query string) (*SQLResult, error) {
	tokens, err := lexSQL(query)
	if err != nil {
		return nil, err
	}
	parser := &sqlParser{tokens: tokens}
	result, err := parser.parseStatement()
	if err != nil {
		return nil, err
	}
	if err := parser.expectEnd(); err != nil {
		return nil, err
	}
	return result, nil
}

func lexSQL(query string) ([]sqlToken, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("%w: empty query", ErrInvalidSQL)
	}

	tokens := make([]sqlToken, 0, len(query)/2)
	for i := 0; i < len(query); {
		r, size := rune(query[i]), 1
		if r >= utf8.RuneSelf {
			r, size = utf8.DecodeRuneInString(query[i:])
		}

		if unicode.IsSpace(r) {
			i += size
			continue
		}

		if isSQLIdentifierStart(r) {
			start := i
			i += size
			for i < len(query) {
				next, nextSize := rune(query[i]), 1
				if next >= utf8.RuneSelf {
					next, nextSize = utf8.DecodeRuneInString(query[i:])
				}
				if !isSQLIdentifierPart(next) {
					break
				}
				i += nextSize
			}
			tokens = append(tokens, sqlToken{kind: sqlTokenIdentifier, lit: query[start:i]})
			continue
		}

		if unicode.IsDigit(r) || (r == '-' && i+1 < len(query) && query[i+1] >= '0' && query[i+1] <= '9') {
			start := i
			i += size
			for i < len(query) && query[i] >= '0' && query[i] <= '9' {
				i++
			}
			if i < len(query) && query[i] == '.' {
				i++
				for i < len(query) && query[i] >= '0' && query[i] <= '9' {
					i++
				}
			}
			tokens = append(tokens, sqlToken{kind: sqlTokenNumber, lit: query[start:i]})
			continue
		}

		if r == '\'' {
			value, next, err := scanSQLString(query, i)
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, sqlToken{kind: sqlTokenString, lit: value})
			i = next
			continue
		}

		switch r {
		case '(', ')', ',', ';', '=', '*', '[', ']':
			tokens = append(tokens, sqlToken{kind: sqlTokenSymbol, lit: string(r)})
			i += size
		default:
			return nil, fmt.Errorf("%w: unexpected character %q", ErrInvalidSQL, r)
		}
	}

	tokens = append(tokens, sqlToken{kind: sqlTokenEOF})
	return tokens, nil
}

func scanSQLString(query string, start int) (string, int, error) {
	var builder strings.Builder
	for i := start + 1; i < len(query); i++ {
		if query[i] != '\'' {
			builder.WriteByte(query[i])
			continue
		}
		if i+1 < len(query) && query[i+1] == '\'' {
			builder.WriteByte('\'')
			i++
			continue
		}
		return builder.String(), i + 1, nil
	}
	return "", 0, fmt.Errorf("%w: unterminated string literal", ErrInvalidSQL)
}

func isSQLIdentifierStart(r rune) bool {
	return unicode.IsLetter(r) || r == '_'
}

func isSQLIdentifierPart(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

func (p *sqlParser) parseStatement() (*SQLResult, error) {
	switch {
	case p.matchKeyword("CREATE"):
		return p.parseCreate()
	case p.matchKeyword("INSERT"):
		return p.parseInsert()
	case p.matchKeyword("SELECT"):
		return p.parseSelect()
	case p.matchKeyword("UPDATE"):
		return p.parseUpdate()
	case p.matchKeyword("DELETE"):
		return p.parseDelete()
	case p.matchKeyword("SHOW"):
		return p.parseShow()
	default:
		return nil, fmt.Errorf("%w: unsupported statement %q", ErrInvalidSQL, p.current().lit)
	}
}

func (p *sqlParser) parseShow() (*SQLResult, error) {
	if err := p.expectKeyword("TABLES"); err != nil {
		return nil, err
	}
	tables, err := ListTables()
	if err != nil {
		return nil, err
	}

	rows := make([]ScannedRow, 0, len(tables))
	for i, schema := range tables {
		var indexedColumns uint64
		var trigramColumns uint64
		for _, column := range schema.Columns {
			if column.Indexed {
				indexedColumns++
			}
			if column.TrigramIndexed {
				trigramColumns++
			}
		}
		rows = append(rows, ScannedRow{
			RowID: uint64(i),
			Values: map[string]any{
				"table":           schema.Name,
				"columns":         uint64(len(schema.Columns)),
				"row_size":        schema.RowSize,
				"indexed_columns": indexedColumns,
				"trigram_indexes": trigramColumns,
			},
		})
	}
	return &SQLResult{Operation: "show_tables", Rows: rows, RowsAffected: uint64(len(rows))}, nil
}

func (p *sqlParser) parseCreate() (*SQLResult, error) {
	switch {
	case p.matchKeyword("TABLE"):
		return p.parseCreateTable()
	case p.matchKeyword("TRIGRAM"):
		if err := p.expectKeyword("INDEX"); err != nil {
			return nil, err
		}
		return p.parseCreateIndex(true)
	case p.matchKeyword("INDEX"):
		return p.parseCreateIndex(false)
	default:
		return nil, fmt.Errorf("%w: expected TABLE or INDEX after CREATE", ErrInvalidSQL)
	}
}

func (p *sqlParser) parseCreateTable() (*SQLResult, error) {
	table, err := p.expectIdentifier()
	if err != nil {
		return nil, err
	}
	if err := p.expectSymbol("("); err != nil {
		return nil, err
	}

	columns := make([]Column, 0)
	for {
		if p.matchSymbol(")") {
			break
		}
		name, err := p.expectIdentifier()
		if err != nil {
			return nil, err
		}
		column, err := p.parseColumnDefinition(name)
		if err != nil {
			return nil, err
		}
		columns = append(columns, column)
		if p.matchSymbol(",") {
			continue
		}
		if err := p.expectSymbol(")"); err != nil {
			return nil, err
		}
		break
	}

	schema, err := CreateTable(Schema{Name: table, Columns: columns})
	if err != nil {
		return nil, err
	}
	return &SQLResult{Operation: "create_table", Table: table, Schema: schema}, nil
}

func (p *sqlParser) parseColumnDefinition(name string) (Column, error) {
	typeName, err := p.expectIdentifier()
	if err != nil {
		return Column{}, err
	}

	column := Column{Name: name}
	switch strings.ToLower(typeName) {
	case "uint64", "unsigned", "bigserial":
		column.Type = ColumnTypeUint64
	case "int64", "integer", "int", "bigint":
		column.Type = ColumnTypeInt64
	case "bool", "boolean":
		column.Type = ColumnTypeBool
	case "float64", "float", "double", "real":
		column.Type = ColumnTypeFloat64
	case "blob_ptr":
		column.Type = ColumnTypeBlobPtr
	case "row_ref":
		column.Type = ColumnTypeRowRef
	case "string", "varchar", "text":
		column.Type = ColumnTypeString
		if p.matchSymbol("(") {
			size, err := p.expectUintLiteral()
			if err != nil {
				return Column{}, err
			}
			column.Size = size
			if err := p.expectSymbol(")"); err != nil {
				return Column{}, err
			}
		} else if p.matchSymbol("[") {
			size, err := p.expectUintLiteral()
			if err != nil {
				return Column{}, err
			}
			column.Size = size
			if err := p.expectSymbol("]"); err != nil {
				return Column{}, err
			}
		}
	default:
		return Column{}, fmt.Errorf("%w: unsupported column type %q", ErrInvalidSQL, typeName)
	}

	for {
		switch {
		case p.matchKeyword("INDEXED"), p.matchKeyword("INDEX"):
			column.Indexed = true
		case p.matchKeyword("TRIGRAM_INDEXED"):
			column.TrigramIndexed = true
		case p.matchKeyword("TRIGRAM"):
			column.TrigramIndexed = true
			_ = p.matchKeyword("INDEX")
			_ = p.matchKeyword("INDEXED")
		case p.matchKeyword("REFERENCES"):
			refTable, err := p.expectIdentifier()
			if err != nil {
				return Column{}, err
			}
			column.RefTable = refTable
		case p.matchKeyword("PRIMARY"):
			if err := p.expectKeyword("KEY"); err != nil {
				return Column{}, err
			}
			column.Indexed = true
		case p.matchKeyword("NOT"):
			if err := p.expectKeyword("NULL"); err != nil {
				return Column{}, err
			}
		case p.matchKeyword("NULL"):
		default:
			return column, nil
		}
	}
}

func (p *sqlParser) parseCreateIndex(trigram bool) (*SQLResult, error) {
	if !p.checkKeyword("ON") {
		if _, err := p.expectIdentifier(); err != nil {
			return nil, err
		}
	}
	if err := p.expectKeyword("ON"); err != nil {
		return nil, err
	}
	table, err := p.expectIdentifier()
	if err != nil {
		return nil, err
	}
	if err := p.expectSymbol("("); err != nil {
		return nil, err
	}
	column, err := p.expectIdentifier()
	if err != nil {
		return nil, err
	}
	if err := p.expectSymbol(")"); err != nil {
		return nil, err
	}

	if trigram {
		err = CreateTrigramIndex(table, column)
	} else {
		err = CreateIndex(table, column)
	}
	if err != nil {
		return nil, err
	}
	operation := "create_index"
	if trigram {
		operation = "create_trigram_index"
	}
	return &SQLResult{Operation: operation, Table: table}, nil
}

func (p *sqlParser) parseInsert() (*SQLResult, error) {
	if err := p.expectKeyword("INTO"); err != nil {
		return nil, err
	}
	table, err := p.expectIdentifier()
	if err != nil {
		return nil, err
	}

	columns := make([]string, 0)
	if p.matchSymbol("(") {
		for {
			column, err := p.expectIdentifier()
			if err != nil {
				return nil, err
			}
			columns = append(columns, column)
			if p.matchSymbol(",") {
				continue
			}
			if err := p.expectSymbol(")"); err != nil {
				return nil, err
			}
			break
		}
	}
	if err := p.expectKeyword("VALUES"); err != nil {
		return nil, err
	}
	if err := p.expectSymbol("("); err != nil {
		return nil, err
	}
	values := make([]any, 0)
	for {
		value, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		values = append(values, value)
		if p.matchSymbol(",") {
			continue
		}
		if err := p.expectSymbol(")"); err != nil {
			return nil, err
		}
		break
	}

	if len(columns) == 0 {
		schema, err := LoadSchema(table)
		if err != nil {
			return nil, err
		}
		for _, column := range schema.Columns {
			columns = append(columns, column.Name)
		}
	}
	if len(columns) != len(values) {
		return nil, fmt.Errorf("%w: INSERT column count does not match value count", ErrInvalidSQL)
	}

	row := make(map[string]any, len(columns))
	for i, column := range columns {
		row[column] = values[i]
	}
	rowID, err := InsertRow(table, row)
	if err != nil {
		return nil, err
	}
	return &SQLResult{Operation: "insert", Table: table, RowID: &rowID, RowsAffected: 1}, nil
}

func (p *sqlParser) parseSelect() (*SQLResult, error) {
	projection, err := p.parseProjection()
	if err != nil {
		return nil, err
	}
	if err := p.expectKeyword("FROM"); err != nil {
		return nil, err
	}
	table, err := p.expectIdentifier()
	if err != nil {
		return nil, err
	}
	where, err := p.parseOptionalWhere()
	if err != nil {
		return nil, err
	}
	order, err := p.parseOptionalOrderBy()
	if err != nil {
		return nil, err
	}

	rows, err := selectRowsForSQL(table, where)
	if err != nil {
		return nil, err
	}
	if order != nil {
		if err := sortSQLRows(table, rows, *order); err != nil {
			return nil, err
		}
	}
	if len(projection) > 0 {
		if err := projectSQLRows(table, rows, projection); err != nil {
			return nil, err
		}
	}
	return &SQLResult{Operation: "select", Table: table, Rows: rows, RowsAffected: uint64(len(rows))}, nil
}

func (p *sqlParser) parseUpdate() (*SQLResult, error) {
	table, err := p.expectIdentifier()
	if err != nil {
		return nil, err
	}
	if err := p.expectKeyword("SET"); err != nil {
		return nil, err
	}
	values := make(map[string]any)
	for {
		column, err := p.expectIdentifier()
		if err != nil {
			return nil, err
		}
		if err := p.expectSymbol("="); err != nil {
			return nil, err
		}
		value, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		values[column] = value
		if p.matchSymbol(",") {
			continue
		}
		break
	}
	where, err := p.parseOptionalWhere()
	if err != nil {
		return nil, err
	}

	rows, err := selectRowsForSQL(table, where)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		if err := UpdateRow(table, row.RowID, values); err != nil {
			return nil, err
		}
	}
	return &SQLResult{Operation: "update", Table: table, RowsAffected: uint64(len(rows))}, nil
}

func (p *sqlParser) parseDelete() (*SQLResult, error) {
	if err := p.expectKeyword("FROM"); err != nil {
		return nil, err
	}
	table, err := p.expectIdentifier()
	if err != nil {
		return nil, err
	}
	where, err := p.parseOptionalWhere()
	if err != nil {
		return nil, err
	}

	rows, err := selectRowsForSQL(table, where)
	if err != nil {
		return nil, err
	}
	var affected uint64
	for _, row := range rows {
		if err := DeleteRow(table, row.RowID); err != nil {
			return nil, err
		}
		affected++
	}
	return &SQLResult{Operation: "delete", Table: table, RowsAffected: affected}, nil
}

func (p *sqlParser) parseProjection() ([]string, error) {
	if p.matchSymbol("*") {
		return nil, nil
	}

	columns := make([]string, 0)
	for {
		column, err := p.expectIdentifier()
		if err != nil {
			return nil, err
		}
		columns = append(columns, column)
		if p.matchSymbol(",") {
			continue
		}
		return columns, nil
	}
}

func (p *sqlParser) parseOptionalWhere() (*sqlWhere, error) {
	if !p.matchKeyword("WHERE") {
		return nil, nil
	}
	column, err := p.expectIdentifier()
	if err != nil {
		return nil, err
	}

	op := PredicateOpEqual
	switch {
	case p.matchSymbol("="):
		op = PredicateOpEqual
	case p.matchKeyword("LIKE"):
		op = PredicateOpLike
	default:
		return nil, fmt.Errorf("%w: expected = or LIKE in WHERE", ErrInvalidSQL)
	}

	value, err := p.parseValue()
	if err != nil {
		return nil, err
	}
	return &sqlWhere{column: column, op: op, value: value}, nil
}

func (p *sqlParser) parseOptionalOrderBy() (*sqlOrder, error) {
	if !p.matchKeyword("ORDER") {
		if !p.matchKeyword("OREDER") {
			return nil, nil
		}
	}
	if err := p.expectKeyword("BY"); err != nil {
		return nil, err
	}
	column, err := p.expectIdentifier()
	if err != nil {
		return nil, err
	}

	order := &sqlOrder{column: column}
	switch {
	case p.matchKeyword("ASC"):
	case p.matchKeyword("DESC"):
		order.desc = true
	}
	return order, nil
}

func (p *sqlParser) parseValue() (any, error) {
	token := p.current()
	switch token.kind {
	case sqlTokenString:
		p.pos++
		return token.lit, nil
	case sqlTokenNumber:
		p.pos++
		return json.Number(token.lit), nil
	case sqlTokenIdentifier:
		p.pos++
		switch strings.ToLower(token.lit) {
		case "true":
			return true, nil
		case "false":
			return false, nil
		case "null":
			return nil, nil
		default:
			return nil, fmt.Errorf("%w: unsupported bare value %q", ErrInvalidSQL, token.lit)
		}
	default:
		return nil, fmt.Errorf("%w: expected value", ErrInvalidSQL)
	}
}

func selectRowsForSQL(table string, where *sqlWhere) ([]ScannedRow, error) {
	if where == nil {
		return ScanRows(table, nil)
	}
	if strings.EqualFold(where.column, "row_id") {
		rowID, err := sqlUint64Value(where.value)
		if err != nil {
			return nil, fmt.Errorf("%w: row_id requires uint64 value", ErrInvalidSQL)
		}
		values, err := ReadRow(table, rowID)
		if errors.Is(err, ErrRowNotFound) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		return []ScannedRow{{RowID: rowID, Values: values}}, nil
	}
	return SelectRows(table, Predicate{Column: where.column, Op: where.op, Value: where.value})
}

func sqlUint64Value(value any) (uint64, error) {
	switch v := value.(type) {
	case json.Number:
		return strconv.ParseUint(v.String(), 10, 64)
	case uint64:
		return v, nil
	case int:
		if v < 0 {
			return 0, strconv.ErrSyntax
		}
		return uint64(v), nil
	default:
		return 0, strconv.ErrSyntax
	}
}

func projectSQLRows(table string, rows []ScannedRow, columns []string) error {
	schema, err := LoadSchema(table)
	if err != nil {
		return err
	}

	for _, column := range columns {
		if strings.EqualFold(column, "row_id") {
			continue
		}
		if _, _, err := findColumn(*schema, column); err != nil {
			return err
		}
	}

	for i := range rows {
		projected := make(map[string]any, len(columns))
		for _, column := range columns {
			if strings.EqualFold(column, "row_id") {
				projected["row_id"] = rows[i].RowID
				continue
			}
			projected[column] = rows[i].Values[column]
		}
		rows[i].Values = projected
	}
	return nil
}

func sortSQLRows(table string, rows []ScannedRow, order sqlOrder) error {
	if len(rows) < 2 {
		return validateSQLOrderColumn(table, order.column)
	}
	if err := validateSQLOrderColumn(table, order.column); err != nil {
		return err
	}

	var compareErr error
	sort.SliceStable(rows, func(i, j int) bool {
		if compareErr != nil {
			return false
		}
		cmp, err := compareSQLOrderValues(sqlRowOrderValue(rows[i], order.column), sqlRowOrderValue(rows[j], order.column))
		if err != nil {
			compareErr = err
			return false
		}
		if order.desc {
			return cmp > 0
		}
		return cmp < 0
	})
	return compareErr
}

func validateSQLOrderColumn(table, column string) error {
	if strings.EqualFold(column, "row_id") {
		return nil
	}
	schema, err := LoadSchema(table)
	if err != nil {
		return err
	}
	_, _, err = findColumn(*schema, column)
	return err
}

func sqlRowOrderValue(row ScannedRow, column string) any {
	if strings.EqualFold(column, "row_id") {
		return row.RowID
	}
	return row.Values[column]
}

func compareSQLOrderValues(left, right any) (int, error) {
	if cmp, ok := compareSQLNumbers(left, right); ok {
		return cmp, nil
	}
	if leftTime, leftOK := parseSQLDateValue(left); leftOK {
		rightTime, rightOK := parseSQLDateValue(right)
		if !rightOK {
			return 0, fmt.Errorf("%w: cannot compare date value %T", ErrInvalidSQL, right)
		}
		switch {
		case leftTime.Before(rightTime):
			return -1, nil
		case leftTime.After(rightTime):
			return 1, nil
		default:
			return 0, nil
		}
	}
	if leftText, leftOK := left.(string); leftOK {
		rightText, rightOK := right.(string)
		if !rightOK {
			return 0, fmt.Errorf("%w: cannot compare string with %T", ErrInvalidSQL, right)
		}
		return strings.Compare(leftText, rightText), nil
	}
	return 0, fmt.Errorf("%w: ORDER BY does not support value type %T", ErrInvalidSQL, left)
}

func compareSQLNumbers(left, right any) (int, bool) {
	if leftFloat, ok := left.(float64); ok {
		rightFloat, rightOK := sqlValueAsFloat64(right)
		if !rightOK {
			return 0, false
		}
		return compareFloat64(leftFloat, rightFloat), true
	}
	if rightFloat, ok := right.(float64); ok {
		leftFloat, leftOK := sqlValueAsFloat64(left)
		if !leftOK {
			return 0, false
		}
		return compareFloat64(leftFloat, rightFloat), true
	}

	switch l := left.(type) {
	case uint64:
		switch r := right.(type) {
		case uint64:
			return compareUint64(l, r), true
		case int64:
			if r < 0 {
				return 1, true
			}
			return compareUint64(l, uint64(r)), true
		}
	case int64:
		switch r := right.(type) {
		case int64:
			return compareInt64(l, r), true
		case uint64:
			if l < 0 {
				return -1, true
			}
			return compareUint64(uint64(l), r), true
		}
	}
	return 0, false
}

func sqlValueAsFloat64(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case uint64:
		return float64(v), true
	case int64:
		return float64(v), true
	default:
		return 0, false
	}
}

func compareFloat64(left, right float64) int {
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}

func compareUint64(left, right uint64) int {
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}

func compareInt64(left, right int64) int {
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}

func parseSQLDateValue(value any) (time.Time, bool) {
	text, ok := value.(string)
	if !ok {
		return time.Time{}, false
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02",
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"2006-01-02T15:04:05",
		"2006-01-02T15:04",
	} {
		parsed, err := time.Parse(layout, text)
		if err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func (p *sqlParser) current() sqlToken {
	if p.pos >= len(p.tokens) {
		return sqlToken{kind: sqlTokenEOF}
	}
	return p.tokens[p.pos]
}

func (p *sqlParser) checkKeyword(keyword string) bool {
	token := p.current()
	return token.kind == sqlTokenIdentifier && strings.EqualFold(token.lit, keyword)
}

func (p *sqlParser) matchKeyword(keyword string) bool {
	if !p.checkKeyword(keyword) {
		return false
	}
	p.pos++
	return true
}

func (p *sqlParser) expectKeyword(keyword string) error {
	if p.matchKeyword(keyword) {
		return nil
	}
	return fmt.Errorf("%w: expected keyword %s", ErrInvalidSQL, keyword)
}

func (p *sqlParser) matchSymbol(symbol string) bool {
	token := p.current()
	if token.kind != sqlTokenSymbol || token.lit != symbol {
		return false
	}
	p.pos++
	return true
}

func (p *sqlParser) expectSymbol(symbol string) error {
	if p.matchSymbol(symbol) {
		return nil
	}
	return fmt.Errorf("%w: expected %q", ErrInvalidSQL, symbol)
}

func (p *sqlParser) expectIdentifier() (string, error) {
	token := p.current()
	if token.kind != sqlTokenIdentifier {
		return "", fmt.Errorf("%w: expected identifier", ErrInvalidSQL)
	}
	p.pos++
	return token.lit, nil
}

func (p *sqlParser) expectUintLiteral() (uint64, error) {
	token := p.current()
	if token.kind != sqlTokenNumber {
		return 0, fmt.Errorf("%w: expected unsigned integer", ErrInvalidSQL)
	}
	if strings.Contains(token.lit, ".") || strings.HasPrefix(token.lit, "-") {
		return 0, fmt.Errorf("%w: expected unsigned integer", ErrInvalidSQL)
	}
	p.pos++
	parsed, err := strconv.ParseUint(token.lit, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: invalid unsigned integer %q", ErrInvalidSQL, token.lit)
	}
	return parsed, nil
}

func (p *sqlParser) expectEnd() error {
	_ = p.matchSymbol(";")
	if p.current().kind != sqlTokenEOF {
		return fmt.Errorf("%w: unexpected token %q", ErrInvalidSQL, p.current().lit)
	}
	return nil
}
