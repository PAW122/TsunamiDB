package mysqlcompat

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/PAW122/TsunamiDB/data/relational"
)

var (
	reWhitespace       = regexp.MustCompile(`\s+`)
	reTrailingLimit    = regexp.MustCompile(`(?i)\s+LIMIT\s+\d+(\s*,\s*\d+|\s+OFFSET\s+\d+)?\s*$`)
	reQualifiedName    = regexp.MustCompile(`\b[A-Za-z][A-Za-z0-9_]*\.`)
	reCreateEngine     = regexp.MustCompile(`(?i)\)\s+ENGINE\s*=.*$`)
	reAlertTypo        = regexp.MustCompile(`(?i)^ALERT\s+TABLE\b`)
	reIntWidthUnsigned = regexp.MustCompile(`(?i)\b(?:BIGINT|INT|INTEGER)\s*\(\s*\d+\s*\)\s+UNSIGNED\b`)
	reBigintUnsigned   = regexp.MustCompile(`(?i)\b(?:BIGINT|INT|INTEGER)\s+UNSIGNED\b`)
	reIntWidth         = regexp.MustCompile(`(?i)\b(?:BIGINT|INT|INTEGER)\s*\(\s*\d+\s*\)`)
	reTinyintBool      = regexp.MustCompile(`(?i)\bTINYINT\s*\(\s*1\s*\)`)
	rePlainText        = regexp.MustCompile(`(?i)\bTEXT\b`)
	reAutoIncrement    = regexp.MustCompile(`(?i)\s+AUTO_INCREMENT\b`)
	reDefaultValue     = regexp.MustCompile(`(?i)\s+DEFAULT\s+(NULL|'[^']*'|[^\s,]+)`)
	reCharsetClause    = regexp.MustCompile(`(?i)\s+(?:CHARACTER\s+SET|CHARSET|COLLATE)\s+[A-Za-z0-9_]+`)
	reColumnComment    = regexp.MustCompile(`(?i)\s+COMMENT\s+'[^']*'`)
	reShowFullTables   = regexp.MustCompile(`(?i)^SHOW\s+FULL\s+TABLES(?:\s+FROM\s+\S+)?(?:\s+LIKE\s+'([^']*)')?$`)
	reShowTables       = regexp.MustCompile(`(?i)^SHOW\s+TABLES(?:\s+FROM\s+\S+)?(?:\s+LIKE\s+'([^']*)')?$`)
	reShowTableStatus  = regexp.MustCompile(`(?i)^SHOW\s+TABLE\s+STATUS(?:\s+FROM\s+\S+)?(?:\s+LIKE\s+'([^']*)')?$`)
	reShowCreateTable  = regexp.MustCompile(`(?i)^SHOW\s+CREATE\s+TABLE\s+(.+)$`)
	reShowColumns      = regexp.MustCompile(`(?i)^(SHOW\s+(?:FULL\s+)?(?:COLUMNS|FIELDS)\s+FROM|DESCRIBE|DESC)\s+(.+?)(?:\s+FROM\s+\S+)?(?:\s+LIKE\s+'([^']*)')?$`)
	reShowIndexes      = regexp.MustCompile(`(?i)^SHOW\s+(?:INDEX|INDEXES|KEYS)\s+FROM\s+(.+?)(?:\s+FROM\s+\S+)?(?:\s+WHERE\s+.+)?$`)
	reShowVariables    = regexp.MustCompile(`(?i)^SHOW\s+(?:GLOBAL\s+|SESSION\s+)?VARIABLES(?:\s+LIKE\s+'([^']*)')?$`)
	reSelectFromDual   = regexp.MustCompile(`(?i)^SELECT\s+(.+?)\s+FROM\s+DUAL$`)
	reSelectAlias      = regexp.MustCompile(`(?i)^(.+?)\s+AS\s+(.+)$`)
	reShowRoutines     = regexp.MustCompile(`(?i)^SHOW\s+(?:PROCEDURE|FUNCTION)\s+STATUS(?:\s+WHERE\s+.+)?$`)
	reShowTriggers     = regexp.MustCompile(`(?i)^SHOW\s+TRIGGERS(?:\s+FROM\s+\S+)?(?:\s+LIKE\s+'[^']*')?(?:\s+WHERE\s+.+)?$`)
	reShowEvents       = regexp.MustCompile(`(?i)^SHOW\s+EVENTS(?:\s+FROM\s+\S+)?(?:\s+LIKE\s+'[^']*')?(?:\s+WHERE\s+.+)?$`)
	reUse              = regexp.MustCompile(`(?i)^USE\s+(.+)$`)
)

func executeCompatQuery(db, query string) (*queryResult, error) {
	normalized := normalizeSQL(query)
	if normalized == "" {
		return &queryResult{}, nil
	}
	upper := strings.ToUpper(normalized)

	switch {
	case upper == "BEGIN" || upper == "COMMIT" || upper == "ROLLBACK" || upper == "START TRANSACTION":
		return &queryResult{}, nil
	case strings.HasPrefix(upper, "SET ") || strings.HasPrefix(upper, "DO "):
		return &queryResult{}, nil
	case reUse.MatchString(normalized):
		return &queryResult{}, nil
	case upper == "SHOW DATABASES":
		return singleTextColumn("Database", defaultDatabase), nil
	case reShowVariables.MatchString(normalized):
		return showVariables(normalized), nil
	case upper == "SHOW WARNINGS" || upper == "SHOW ERRORS":
		return emptyResult("Level", "Code", "Message"), nil
	case upper == "SHOW STATUS" || strings.HasPrefix(upper, "SHOW SESSION STATUS") || strings.HasPrefix(upper, "SHOW GLOBAL STATUS"):
		return emptyResult("Variable_name", "Value"), nil
	case reShowRoutines.MatchString(normalized):
		return emptyRoutineStatusResult(), nil
	case reShowTriggers.MatchString(normalized):
		return emptyTriggersResult(), nil
	case reShowEvents.MatchString(normalized):
		return emptyEventsResult(), nil
	case upper == "SHOW ENGINES":
		return &queryResult{
			columns: []column{
				{name: "Engine", typ: columnTypeVarString, length: 64},
				{name: "Support", typ: columnTypeVarString, length: 16},
				{name: "Comment", typ: columnTypeVarString, length: 256},
				{name: "Transactions", typ: columnTypeVarString, length: 8},
				{name: "XA", typ: columnTypeVarString, length: 8},
				{name: "Savepoints", typ: columnTypeVarString, length: 8},
			},
			rows: [][]any{{"TsunamiDB", "DEFAULT", "TsunamiDB fixed-row relational storage", "NO", "NO", "NO"}},
		}, nil
	case upper == "SHOW CHARACTER SET" || upper == "SHOW CHARSET":
		return &queryResult{
			columns: []column{
				{name: "Charset", typ: columnTypeVarString, length: 64},
				{name: "Description", typ: columnTypeVarString, length: 256},
				{name: "Default collation", typ: columnTypeVarString, length: 64},
				{name: "Maxlen", typ: columnTypeLongLong, flags: columnFlagUnsigned, length: 20},
			},
			rows: [][]any{{"utf8mb4", "UTF-8 Unicode", "utf8mb4_general_ci", uint64(4)}},
		}, nil
	case upper == "SHOW COLLATION":
		return &queryResult{
			columns: []column{
				{name: "Collation", typ: columnTypeVarString, length: 64},
				{name: "Charset", typ: columnTypeVarString, length: 64},
				{name: "Id", typ: columnTypeLongLong, flags: columnFlagUnsigned, length: 20},
				{name: "Default", typ: columnTypeVarString, length: 8},
				{name: "Compiled", typ: columnTypeVarString, length: 8},
				{name: "Sortlen", typ: columnTypeLongLong, flags: columnFlagUnsigned, length: 20},
			},
			rows: [][]any{{"utf8mb4_general_ci", "utf8mb4", uint64(45), "Yes", "Yes", uint64(1)}},
		}, nil
	case strings.HasPrefix(upper, "SELECT ") && !strings.Contains(upper, " FROM "):
		if result, ok := selectSessionValue(db, normalized); ok {
			return result, nil
		}
	case reSelectFromDual.MatchString(normalized):
		dualSelect := reSelectFromDual.ReplaceAllString(normalized, "SELECT $1")
		if result, ok := selectSessionValue(db, dualSelect); ok {
			return result, nil
		}
	case strings.Contains(upper, "INFORMATION_SCHEMA."):
		return informationSchemaResult(normalized), nil
	case reShowFullTables.MatchString(normalized):
		return showTables(db, normalized, true)
	case reShowTables.MatchString(normalized):
		return showTables(db, normalized, false)
	case reShowTableStatus.MatchString(normalized):
		return showTableStatus(normalized)
	case reShowCreateTable.MatchString(normalized):
		return showCreateTable(normalized)
	case reShowColumns.MatchString(normalized):
		return showColumns(normalized)
	case reShowIndexes.MatchString(normalized):
		return showIndexes(normalized)
	}

	sql := normalizeRelationalSQL(normalized)
	result, err := relational.ExecuteSQL(sql)
	if err != nil {
		return nil, err
	}
	return sqlResultToMySQL(sql, result)
}

func normalizeSQL(query string) string {
	query = strings.TrimSpace(query)
	query = stripMySQLComments(query)
	for {
		trimmed := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(query), ";"))
		if trimmed == query {
			break
		}
		query = trimmed
	}
	query = strings.TrimSpace(query)
	query = unquoteMySQLIdentifiers(query)
	query = reWhitespace.ReplaceAllString(query, " ")
	return strings.TrimSpace(query)
}

func stripMySQLComments(query string) string {
	var out strings.Builder
	for i := 0; i < len(query); i++ {
		switch {
		case query[i] == '\'':
			next := copySQLStringLiteral(&out, query, i)
			i = next - 1
		case query[i] == '`':
			next := copyBacktickIdentifier(&out, query, i)
			i = next - 1
		case query[i] == '#':
			i = skipLineComment(query, i) - 1
		case i+1 < len(query) && query[i] == '-' && query[i+1] == '-' && isLineCommentStart(query, i+2):
			i = skipLineComment(query, i) - 1
		case i+1 < len(query) && query[i] == '/' && query[i+1] == '*':
			next := skipOrExpandBlockComment(&out, query, i)
			i = next - 1
		default:
			out.WriteByte(query[i])
		}
	}
	return out.String()
}

func copySQLStringLiteral(out *strings.Builder, query string, start int) int {
	out.WriteByte(query[start])
	for i := start + 1; i < len(query); i++ {
		out.WriteByte(query[i])
		if query[i] != '\'' {
			continue
		}
		if i+1 < len(query) && query[i+1] == '\'' {
			i++
			out.WriteByte(query[i])
			continue
		}
		return i + 1
	}
	return len(query)
}

func copyBacktickIdentifier(out *strings.Builder, query string, start int) int {
	out.WriteByte(query[start])
	for i := start + 1; i < len(query); i++ {
		out.WriteByte(query[i])
		if query[i] != '`' {
			continue
		}
		if i+1 < len(query) && query[i+1] == '`' {
			i++
			out.WriteByte(query[i])
			continue
		}
		return i + 1
	}
	return len(query)
}

func isLineCommentStart(query string, pos int) bool {
	return pos >= len(query) || query[pos] == ' ' || query[pos] == '\t' || query[pos] == '\r' || query[pos] == '\n'
}

func skipLineComment(query string, start int) int {
	for i := start; i < len(query); i++ {
		if query[i] == '\r' || query[i] == '\n' {
			return i + 1
		}
	}
	return len(query)
}

func skipOrExpandBlockComment(out *strings.Builder, query string, start int) int {
	end := strings.Index(query[start+2:], "*/")
	if end < 0 {
		return len(query)
	}
	commentEnd := start + 2 + end
	if start+2 < len(query) && query[start+2] == '!' {
		contentStart := start + 3
		for contentStart < commentEnd && query[contentStart] >= '0' && query[contentStart] <= '9' {
			contentStart++
		}
		out.WriteByte(' ')
		out.WriteString(strings.TrimSpace(query[contentStart:commentEnd]))
		out.WriteByte(' ')
	}
	return commentEnd + 2
}

func unquoteMySQLIdentifiers(query string) string {
	var out strings.Builder
	for i := 0; i < len(query); i++ {
		if query[i] != '`' {
			out.WriteByte(query[i])
			continue
		}
		for i++; i < len(query); i++ {
			if query[i] == '`' {
				if i+1 < len(query) && query[i+1] == '`' {
					out.WriteByte('`')
					i++
					continue
				}
				break
			}
			out.WriteByte(query[i])
		}
	}
	return out.String()
}

func normalizeRelationalSQL(query string) string {
	query = reTrailingLimit.ReplaceAllString(query, "")
	query = reQualifiedName.ReplaceAllString(query, "")
	query = reAlertTypo.ReplaceAllString(query, "ALTER TABLE")
	query = reCreateEngine.ReplaceAllString(query, ")")
	query = reIntWidthUnsigned.ReplaceAllString(query, "uint64")
	query = reBigintUnsigned.ReplaceAllString(query, "uint64")
	query = reIntWidth.ReplaceAllString(query, "int64")
	query = reTinyintBool.ReplaceAllString(query, "bool")
	query = rePlainText.ReplaceAllString(query, "string(255)")
	query = reAutoIncrement.ReplaceAllString(query, "")
	query = reDefaultValue.ReplaceAllString(query, "")
	query = reCharsetClause.ReplaceAllString(query, "")
	query = reColumnComment.ReplaceAllString(query, "")
	return strings.TrimSpace(query)
}

func singleTextColumn(name, value string) *queryResult {
	return &queryResult{
		columns: []column{{name: name, typ: columnTypeVarString, length: 1024}},
		rows:    [][]any{{value}},
	}
}

func emptyResult(names ...string) *queryResult {
	columns := make([]column, len(names))
	for i, name := range names {
		columns[i] = column{name: name, typ: columnTypeVarString, length: 1024}
	}
	return &queryResult{columns: columns}
}

func emptyRoutineStatusResult() *queryResult {
	return emptyResult(
		"Db",
		"Name",
		"Type",
		"Definer",
		"Modified",
		"Created",
		"Security_type",
		"Comment",
		"character_set_client",
		"collation_connection",
		"Database Collation",
	)
}

func emptyTriggersResult() *queryResult {
	return emptyResult(
		"Trigger",
		"Event",
		"Table",
		"Statement",
		"Timing",
		"Created",
		"sql_mode",
		"Definer",
		"character_set_client",
		"collation_connection",
		"Database Collation",
	)
}

func emptyEventsResult() *queryResult {
	return emptyResult(
		"Db",
		"Name",
		"Definer",
		"Time zone",
		"Type",
		"Execute at",
		"Interval value",
		"Interval field",
		"Starts",
		"Ends",
		"Status",
		"Originator",
		"character_set_client",
		"collation_connection",
		"Database Collation",
	)
}

func showVariables(query string) *queryResult {
	variables := map[string]string{
		"autocommit":               "ON",
		"character_set_client":     "utf8mb4",
		"character_set_connection": "utf8mb4",
		"character_set_results":    "utf8mb4",
		"collation_connection":     "utf8mb4_general_ci",
		"lower_case_table_names":   "0",
		"max_allowed_packet":       "67108864",
		"sql_mode":                 "",
		"system_time_zone":         "UTC",
		"time_zone":                "SYSTEM",
		"tx_isolation":             "READ-COMMITTED",
		"version":                  "5.7.0-TsunamiDB",
		"version_comment":          "TsunamiDB MySQL compatibility endpoint",
	}

	filter := ""
	if matches := reShowVariables.FindStringSubmatch(query); len(matches) == 2 {
		filter = strings.ReplaceAll(matches[1], "%", "")
		filter = strings.ToLower(filter)
	}

	names := make([]string, 0, len(variables))
	for name := range variables {
		if filter == "" || strings.Contains(strings.ToLower(name), filter) {
			names = append(names, name)
		}
	}
	sort.Strings(names)

	rows := make([][]any, 0, len(names))
	for _, name := range names {
		rows = append(rows, []any{name, variables[name]})
	}
	return &queryResult{
		columns: []column{
			{name: "Variable_name", typ: columnTypeVarString, length: 256},
			{name: "Value", typ: columnTypeVarString, length: 4096},
		},
		rows: rows,
	}
}

func selectSessionValue(db, query string) (*queryResult, bool) {
	expr := strings.TrimSpace(query[len("SELECT"):])
	expr = reTrailingLimit.ReplaceAllString(expr, "")
	if expr == "" {
		return nil, false
	}

	parts := splitSelectExpressions(expr)
	columns := make([]column, 0, len(parts))
	row := make([]any, 0, len(parts))
	for _, part := range parts {
		name, value := evalSessionSelectItem(db, part)
		columns = append(columns, columnForResultValue(name, value))
		row = append(row, value)
	}
	return &queryResult{columns: columns, rows: [][]any{row}}, true
}

func splitSelectExpressions(expr string) []string {
	var parts []string
	start := 0
	depth := 0
	inString := false
	for i := 0; i < len(expr); i++ {
		switch expr[i] {
		case '\'':
			if inString && i+1 < len(expr) && expr[i+1] == '\'' {
				i++
				continue
			}
			inString = !inString
		case '(':
			if !inString {
				depth++
			}
		case ')':
			if !inString && depth > 0 {
				depth--
			}
		case ',':
			if !inString && depth == 0 {
				parts = append(parts, strings.TrimSpace(expr[start:i]))
				start = i + 1
			}
		}
	}
	parts = append(parts, strings.TrimSpace(expr[start:]))
	return parts
}

func evalSessionSelectItem(db, item string) (string, any) {
	item = strings.TrimSpace(item)
	expr, alias := splitSelectAlias(item)
	value, ok := evalSessionExpression(db, expr)
	if !ok {
		value = ""
	}
	name := alias
	if name == "" {
		name = expr
	}
	return name, value
}

func splitSelectAlias(item string) (string, string) {
	matches := reSelectAlias.FindStringSubmatch(item)
	if len(matches) == 3 {
		return strings.TrimSpace(matches[1]), cleanAlias(matches[2])
	}
	return strings.TrimSpace(item), ""
}

func cleanAlias(alias string) string {
	alias = strings.TrimSpace(alias)
	alias = strings.Trim(alias, "`\"'")
	return alias
}

func evalSessionExpression(db, expr string) (any, bool) {
	expr = strings.TrimSpace(expr)
	exprUpper := strings.ToUpper(expr)
	switch {
	case strings.HasPrefix(exprUpper, "IFNULL(") && strings.HasSuffix(expr, ")"):
		return evalFirstNonEmptySessionExpression(db, expr[len("IFNULL("):len(expr)-1])
	case strings.HasPrefix(exprUpper, "COALESCE(") && strings.HasSuffix(expr, ")"):
		return evalFirstNonEmptySessionExpression(db, expr[len("COALESCE("):len(expr)-1])
	case exprUpper == "VERSION()":
		return "5.7.0-TsunamiDB", true
	case exprUpper == "@@VERSION":
		return "5.7.0-TsunamiDB", true
	case strings.Contains(exprUpper, "@@VERSION_COMMENT"):
		return "TsunamiDB MySQL compatibility endpoint", true
	case exprUpper == "DATABASE()" || exprUpper == "SCHEMA()":
		return db, true
	case exprUpper == "CONNECTION_ID()":
		return uint64(1), true
	case exprUpper == "USER()" || exprUpper == "CURRENT_USER()":
		return "tsunamidb@localhost", true
	case exprUpper == "1":
		return uint64(1), true
	case strings.HasPrefix(exprUpper, "@@"):
		return sessionVariableValue(strings.TrimPrefix(strings.ToLower(expr), "@@")), true
	case strings.EqualFold(expr, "NULL"):
		return nil, true
	case len(expr) >= 2 && expr[0] == '\'' && expr[len(expr)-1] == '\'':
		return strings.ReplaceAll(expr[1:len(expr)-1], "''", "'"), true
	default:
		if strings.Contains(expr, ".") {
			value, err := strconv.ParseFloat(expr, 64)
			return value, err == nil
		}
		value, err := strconv.ParseInt(expr, 10, 64)
		if err == nil {
			if value >= 0 {
				return uint64(value), true
			}
			return value, true
		}
		return nil, false
	}
}

func evalFirstNonEmptySessionExpression(db, args string) (any, bool) {
	var fallback any
	var hasFallback bool
	for _, part := range splitSelectExpressions(args) {
		value, ok := evalSessionExpression(db, part)
		if !ok {
			continue
		}
		if !hasFallback {
			fallback = value
			hasFallback = true
		}
		if value != nil && mysqlTextValue(value) != "" {
			return value, true
		}
	}
	return fallback, hasFallback
}

func sessionVariableValue(name string) string {
	name = strings.TrimPrefix(name, "session.")
	name = strings.TrimPrefix(name, "global.")
	switch name {
	case "character_set_client", "character_set_connection", "character_set_results":
		return "utf8mb4"
	case "character_set_database", "character_set_server":
		return "utf8mb4"
	case "collation_connection":
		return "utf8mb4_general_ci"
	case "collation_database", "collation_server":
		return "utf8mb4_general_ci"
	case "autocommit":
		return "1"
	case "hostname":
		return "localhost"
	case "interactive_timeout", "net_read_timeout", "net_write_timeout", "wait_timeout":
		return "28800"
	case "license":
		return "MIT"
	case "lower_case_table_names":
		return "0"
	case "max_allowed_packet":
		return "67108864"
	case "port":
		return "3307"
	case "protocol_version":
		return "10"
	case "sql_mode":
		return ""
	case "system_time_zone":
		return "UTC"
	case "time_zone":
		return "SYSTEM"
	case "transaction_isolation", "tx_isolation":
		return "READ-COMMITTED"
	case "version":
		return "5.7.0-TsunamiDB"
	case "version_comment":
		return "TsunamiDB MySQL compatibility endpoint"
	default:
		return ""
	}
}

func informationSchemaResult(query string) *queryResult {
	upper := strings.ToUpper(query)
	switch {
	case strings.Contains(upper, "INFORMATION_SCHEMA.SCHEMATA"):
		return &queryResult{
			columns: []column{{name: "SCHEMA_NAME", typ: columnTypeVarString, length: 256}},
			rows:    [][]any{{defaultDatabase}},
		}
	case strings.Contains(upper, "INFORMATION_SCHEMA.EVENTS"):
		return emptyResult(
			"EVENT_CATALOG",
			"EVENT_SCHEMA",
			"EVENT_NAME",
			"DEFINER",
			"TIME_ZONE",
			"EVENT_BODY",
			"EVENT_DEFINITION",
			"EVENT_TYPE",
			"EXECUTE_AT",
			"INTERVAL_VALUE",
			"INTERVAL_FIELD",
			"SQL_MODE",
			"STARTS",
			"ENDS",
			"STATUS",
			"ON_COMPLETION",
			"CREATED",
			"LAST_ALTERED",
			"LAST_EXECUTED",
			"EVENT_COMMENT",
			"ORIGINATOR",
			"CHARACTER_SET_CLIENT",
			"COLLATION_CONNECTION",
			"DATABASE_COLLATION",
			"Db",
			"Name",
		)
	case strings.Contains(upper, "INFORMATION_SCHEMA.TRIGGERS"):
		return emptyResult(
			"TRIGGER_CATALOG",
			"TRIGGER_SCHEMA",
			"TRIGGER_NAME",
			"EVENT_MANIPULATION",
			"EVENT_OBJECT_CATALOG",
			"EVENT_OBJECT_SCHEMA",
			"EVENT_OBJECT_TABLE",
			"ACTION_ORDER",
			"ACTION_CONDITION",
			"ACTION_STATEMENT",
			"ACTION_ORIENTATION",
			"ACTION_TIMING",
			"ACTION_REFERENCE_OLD_TABLE",
			"ACTION_REFERENCE_NEW_TABLE",
			"ACTION_REFERENCE_OLD_ROW",
			"ACTION_REFERENCE_NEW_ROW",
			"CREATED",
			"SQL_MODE",
			"DEFINER",
			"CHARACTER_SET_CLIENT",
			"COLLATION_CONNECTION",
			"DATABASE_COLLATION",
		)
	case strings.Contains(upper, "INFORMATION_SCHEMA.ROUTINES"):
		return emptyResult(
			"SPECIFIC_NAME",
			"ROUTINE_CATALOG",
			"ROUTINE_SCHEMA",
			"ROUTINE_NAME",
			"ROUTINE_TYPE",
			"DATA_TYPE",
			"ROUTINE_BODY",
			"ROUTINE_DEFINITION",
			"EXTERNAL_NAME",
			"EXTERNAL_LANGUAGE",
			"PARAMETER_STYLE",
			"IS_DETERMINISTIC",
			"SQL_DATA_ACCESS",
			"SQL_PATH",
			"SECURITY_TYPE",
			"CREATED",
			"LAST_ALTERED",
			"SQL_MODE",
			"ROUTINE_COMMENT",
			"DEFINER",
			"CHARACTER_SET_CLIENT",
			"COLLATION_CONNECTION",
			"DATABASE_COLLATION",
		)
	case strings.Contains(upper, "INFORMATION_SCHEMA.REFERENTIAL_CONSTRAINTS"):
		return emptyResult(
			"CONSTRAINT_CATALOG",
			"CONSTRAINT_SCHEMA",
			"CONSTRAINT_NAME",
			"UNIQUE_CONSTRAINT_CATALOG",
			"UNIQUE_CONSTRAINT_SCHEMA",
			"UNIQUE_CONSTRAINT_NAME",
			"MATCH_OPTION",
			"UPDATE_RULE",
			"DELETE_RULE",
			"TABLE_NAME",
			"REFERENCED_TABLE_NAME",
		)
	case strings.Contains(upper, "INFORMATION_SCHEMA.KEY_COLUMN_USAGE"):
		return keyColumnUsageResult(query)
	case strings.Contains(upper, "INFORMATION_SCHEMA.STATISTICS"):
		return statisticsResult(query)
	case strings.Contains(upper, "INFORMATION_SCHEMA.TABLES"):
		tables, _ := relational.ListTables()
		rows := make([][]any, 0, len(tables))
		for _, table := range tables {
			rows = append(rows, []any{defaultDatabase, table.Name, "BASE TABLE", "TsunamiDB"})
		}
		return &queryResult{
			columns: []column{
				{name: "TABLE_SCHEMA", typ: columnTypeVarString, length: 256},
				{name: "TABLE_NAME", typ: columnTypeVarString, length: 256},
				{name: "TABLE_TYPE", typ: columnTypeVarString, length: 64},
				{name: "ENGINE", typ: columnTypeVarString, length: 64},
			},
			rows: rows,
		}
	case strings.Contains(upper, "INFORMATION_SCHEMA.COLUMNS"):
		return informationSchemaColumns()
	default:
		return emptyResult("value")
	}
}

func informationSchemaColumns() *queryResult {
	tables, _ := relational.ListTables()
	rows := make([][]any, 0)
	for _, table := range tables {
		for i, col := range table.Columns {
			rows = append(rows, []any{
				defaultDatabase,
				table.Name,
				col.Name,
				uint64(i + 1),
				mysqlColumnTypeName(col),
			})
		}
	}
	return &queryResult{
		columns: []column{
			{name: "TABLE_SCHEMA", typ: columnTypeVarString, length: 256},
			{name: "TABLE_NAME", typ: columnTypeVarString, length: 256},
			{name: "COLUMN_NAME", typ: columnTypeVarString, length: 256},
			{name: "ORDINAL_POSITION", typ: columnTypeLongLong, flags: columnFlagUnsigned, length: 20},
			{name: "COLUMN_TYPE", typ: columnTypeVarString, length: 256},
		},
		rows: rows,
	}
}

func keyColumnUsageResult(query string) *queryResult {
	tableFilter := informationSchemaTableFilter(query)
	onlyReferenced := strings.Contains(strings.ToUpper(query), "REFERENCED_TABLE_NAME IS NOT NULL")
	tables, _ := relational.ListTables()
	rows := make([][]any, 0)
	for _, table := range tables {
		if tableFilter != "" && !strings.EqualFold(table.Name, tableFilter) {
			continue
		}
		for ordinal, col := range table.Columns {
			if !col.Indexed && col.RefTable == "" {
				continue
			}
			constraintName := col.Name
			referencedTable := any(nil)
			referencedColumn := any(nil)
			if col.RefTable != "" {
				constraintName = col.Name + "_fk"
				referencedTable = col.RefTable
				referencedColumn = "row_id"
			}
			if onlyReferenced && col.RefTable == "" {
				continue
			}
			rows = append(rows, []any{
				"def",
				defaultDatabase,
				constraintName,
				"def",
				defaultDatabase,
				table.Name,
				col.Name,
				uint64(ordinal + 1),
				uint64(ordinal + 1),
				referencedTable,
				referencedColumn,
			})
		}
	}
	return &queryResult{
		columns: []column{
			{name: "CONSTRAINT_CATALOG", typ: columnTypeVarString, length: 64},
			{name: "CONSTRAINT_SCHEMA", typ: columnTypeVarString, length: 256},
			{name: "CONSTRAINT_NAME", typ: columnTypeVarString, length: 256},
			{name: "TABLE_CATALOG", typ: columnTypeVarString, length: 64},
			{name: "TABLE_SCHEMA", typ: columnTypeVarString, length: 256},
			{name: "TABLE_NAME", typ: columnTypeVarString, length: 256},
			{name: "COLUMN_NAME", typ: columnTypeVarString, length: 256},
			{name: "ORDINAL_POSITION", typ: columnTypeLongLong, flags: columnFlagUnsigned, length: 20},
			{name: "POSITION_IN_UNIQUE_CONSTRAINT", typ: columnTypeLongLong, flags: columnFlagUnsigned, length: 20},
			{name: "REFERENCED_TABLE_NAME", typ: columnTypeVarString, length: 256},
			{name: "REFERENCED_COLUMN_NAME", typ: columnTypeVarString, length: 256},
		},
		rows: rows,
	}
}

func statisticsResult(query string) *queryResult {
	tableFilter := informationSchemaTableFilter(query)
	tables, _ := relational.ListTables()
	rows := make([][]any, 0)
	for _, table := range tables {
		if tableFilter != "" && !strings.EqualFold(table.Name, tableFilter) {
			continue
		}
		rows = append(rows, indexRowsForSchema(table)...)
	}
	return indexResult(rows)
}

func informationSchemaTableFilter(query string) string {
	upper := strings.ToUpper(query)
	for _, key := range []string{"TABLE_NAME=", "TABLE_NAME ="} {
		idx := strings.Index(upper, key)
		if idx < 0 {
			continue
		}
		raw := strings.TrimSpace(query[idx+len(key):])
		if strings.HasPrefix(raw, "'") {
			raw = strings.TrimPrefix(raw, "'")
			if end := strings.Index(raw, "'"); end >= 0 {
				return raw[:end]
			}
		}
		fields := strings.Fields(raw)
		if len(fields) > 0 {
			return strings.Trim(fields[0], "';")
		}
	}
	return ""
}

func showTables(db, query string, full bool) (*queryResult, error) {
	tables, err := relational.ListTables()
	if err != nil {
		return nil, err
	}
	filter := tableLikeFilter(query, reShowTables, reShowFullTables)
	rows := make([][]any, 0, len(tables))
	for _, table := range tables {
		if filter != "" && !strings.Contains(strings.ToLower(table.Name), filter) {
			continue
		}
		if full {
			rows = append(rows, []any{table.Name, "BASE TABLE"})
		} else {
			rows = append(rows, []any{table.Name})
		}
	}

	if full {
		return &queryResult{
			columns: []column{
				{name: "Tables_in_" + db, typ: columnTypeVarString, length: 256},
				{name: "Table_type", typ: columnTypeVarString, length: 64},
			},
			rows: rows,
		}, nil
	}
	return &queryResult{
		columns: []column{{name: "Tables_in_" + db, typ: columnTypeVarString, length: 256}},
		rows:    rows,
	}, nil
}

func tableLikeFilter(query string, regexes ...*regexp.Regexp) string {
	for _, re := range regexes {
		matches := re.FindStringSubmatch(query)
		if len(matches) == 2 && matches[1] != "" {
			return strings.ToLower(strings.ReplaceAll(matches[1], "%", ""))
		}
	}
	return ""
}

func showTableStatus(query string) (*queryResult, error) {
	tables, err := relational.ListTables()
	if err != nil {
		return nil, err
	}
	filter := tableLikeFilter(query, reShowTableStatus)
	rows := make([][]any, 0, len(tables))
	for _, table := range tables {
		if filter != "" && !strings.Contains(strings.ToLower(table.Name), filter) {
			continue
		}
		rows = append(rows, []any{
			table.Name,
			"TsunamiDB",
			uint64(10),
			"Fixed",
			uint64(0),
			table.RowSize,
			uint64(0),
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			"utf8mb4_general_ci",
			nil,
			"",
			"",
		})
	}
	return &queryResult{
		columns: []column{
			{name: "Name", typ: columnTypeVarString, length: 256},
			{name: "Engine", typ: columnTypeVarString, length: 64},
			{name: "Version", typ: columnTypeLongLong, flags: columnFlagUnsigned, length: 20},
			{name: "Row_format", typ: columnTypeVarString, length: 32},
			{name: "Rows", typ: columnTypeLongLong, flags: columnFlagUnsigned, length: 20},
			{name: "Avg_row_length", typ: columnTypeLongLong, flags: columnFlagUnsigned, length: 20},
			{name: "Data_length", typ: columnTypeLongLong, flags: columnFlagUnsigned, length: 20},
			{name: "Max_data_length", typ: columnTypeLongLong, flags: columnFlagUnsigned, length: 20},
			{name: "Index_length", typ: columnTypeLongLong, flags: columnFlagUnsigned, length: 20},
			{name: "Data_free", typ: columnTypeLongLong, flags: columnFlagUnsigned, length: 20},
			{name: "Auto_increment", typ: columnTypeLongLong, flags: columnFlagUnsigned, length: 20},
			{name: "Create_time", typ: columnTypeVarString, length: 64},
			{name: "Update_time", typ: columnTypeVarString, length: 64},
			{name: "Check_time", typ: columnTypeVarString, length: 64},
			{name: "Collation", typ: columnTypeVarString, length: 64},
			{name: "Checksum", typ: columnTypeLongLong, flags: columnFlagUnsigned, length: 20},
			{name: "Create_options", typ: columnTypeVarString, length: 256},
			{name: "Comment", typ: columnTypeVarString, length: 256},
		},
		rows: rows,
	}, nil
}

func showCreateTable(query string) (*queryResult, error) {
	matches := reShowCreateTable.FindStringSubmatch(query)
	if len(matches) != 2 {
		return nil, fmt.Errorf("invalid SHOW CREATE TABLE")
	}
	table := cleanTableName(matches[1])
	schema, err := relational.LoadSchema(table)
	if err != nil {
		return nil, err
	}
	return &queryResult{
		columns: []column{
			{name: "Table", typ: columnTypeVarString, length: 256},
			{name: "Create Table", typ: columnTypeVarString, length: 8192},
		},
		rows: [][]any{{schema.Name, createTableDDL(schema)}},
	}, nil
}

func showColumns(query string) (*queryResult, error) {
	matches := reShowColumns.FindStringSubmatch(query)
	if len(matches) < 3 {
		return nil, fmt.Errorf("invalid SHOW COLUMNS")
	}
	table := cleanTableName(matches[2])
	schema, err := relational.LoadSchema(table)
	if err != nil {
		return nil, err
	}
	filter := ""
	if len(matches) == 4 {
		filter = strings.ToLower(strings.ReplaceAll(matches[3], "%", ""))
	}
	rows := make([][]any, 0, len(schema.Columns))
	for _, col := range schema.Columns {
		if filter != "" && !strings.Contains(strings.ToLower(col.Name), filter) {
			continue
		}
		key := ""
		if col.Indexed {
			key = "MUL"
		}
		rows = append(rows, []any{col.Name, mysqlColumnTypeName(col), "YES", key, nil, ""})
	}
	return &queryResult{
		columns: []column{
			{name: "Field", typ: columnTypeVarString, length: 256},
			{name: "Type", typ: columnTypeVarString, length: 256},
			{name: "Null", typ: columnTypeVarString, length: 8},
			{name: "Key", typ: columnTypeVarString, length: 8},
			{name: "Default", typ: columnTypeVarString, length: 256},
			{name: "Extra", typ: columnTypeVarString, length: 256},
		},
		rows: rows,
	}, nil
}

func showIndexes(query string) (*queryResult, error) {
	matches := reShowIndexes.FindStringSubmatch(query)
	if len(matches) != 2 {
		return nil, fmt.Errorf("invalid SHOW INDEXES")
	}
	table := cleanTableName(matches[1])
	schema, err := relational.LoadSchema(table)
	if err != nil {
		return nil, err
	}
	return indexResult(indexRowsForSchema(*schema)), nil
}

func indexRowsForSchema(schema relational.Schema) [][]any {
	rows := make([][]any, 0)
	for ordinal, col := range schema.Columns {
		if !col.Indexed {
			continue
		}
		rows = append(rows, []any{
			schema.Name,
			uint64(1),
			col.Name,
			uint64(1),
			col.Name,
			"A",
			nil,
			nil,
			nil,
			"YES",
			"BTREE",
			"",
			"",
			"YES",
			nil,
			uint64(ordinal + 1),
		})
	}
	return rows
}

func indexResult(rows [][]any) *queryResult {
	return &queryResult{
		columns: []column{
			{name: "Table", typ: columnTypeVarString, length: 256},
			{name: "Non_unique", typ: columnTypeLongLong, flags: columnFlagUnsigned, length: 1},
			{name: "Key_name", typ: columnTypeVarString, length: 256},
			{name: "Seq_in_index", typ: columnTypeLongLong, flags: columnFlagUnsigned, length: 20},
			{name: "Column_name", typ: columnTypeVarString, length: 256},
			{name: "Collation", typ: columnTypeVarString, length: 8},
			{name: "Cardinality", typ: columnTypeLongLong, flags: columnFlagUnsigned, length: 20},
			{name: "Sub_part", typ: columnTypeLongLong, flags: columnFlagUnsigned, length: 20},
			{name: "Packed", typ: columnTypeVarString, length: 32},
			{name: "Null", typ: columnTypeVarString, length: 8},
			{name: "Index_type", typ: columnTypeVarString, length: 32},
			{name: "Comment", typ: columnTypeVarString, length: 256},
			{name: "Index_comment", typ: columnTypeVarString, length: 256},
			{name: "Visible", typ: columnTypeVarString, length: 8},
			{name: "Expression", typ: columnTypeVarString, length: 256},
			{name: "ORDINAL_POSITION", typ: columnTypeLongLong, flags: columnFlagUnsigned, length: 20},
		},
		rows: rows,
	}
}

func cleanTableName(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.Trim(raw, "`")
	raw = strings.TrimSuffix(raw, ";")
	parts := strings.Split(raw, ".")
	return strings.TrimSpace(parts[len(parts)-1])
}

func createTableDDL(schema *relational.Schema) string {
	lines := make([]string, 0, len(schema.Columns))
	for _, col := range schema.Columns {
		extras := ""
		if col.Indexed {
			extras += " INDEXED"
		}
		if col.TrigramIndexed {
			extras += " TRIGRAM"
		}
		if col.RefTable != "" {
			extras += " REFERENCES " + col.RefTable
		}
		lines = append(lines, fmt.Sprintf("  `%s` %s%s", col.Name, mysqlColumnTypeName(col), extras))
	}
	return fmt.Sprintf("CREATE TABLE `%s` (\n%s\n)", schema.Name, strings.Join(lines, ",\n"))
}

func mysqlColumnTypeName(col relational.Column) string {
	switch col.Type {
	case relational.ColumnTypeUint64, relational.ColumnTypeBlobPtr, relational.ColumnTypeRowRef:
		return "bigint unsigned"
	case relational.ColumnTypeInt64:
		return "bigint"
	case relational.ColumnTypeBool:
		return "tinyint(1)"
	case relational.ColumnTypeFloat64:
		return "double"
	case relational.ColumnTypeString:
		return "varchar(" + strconv.FormatUint(col.Size, 10) + ")"
	default:
		return col.Type
	}
}

func sqlResultToMySQL(query string, result *relational.SQLResult) (*queryResult, error) {
	switch result.Operation {
	case "select":
		return selectSQLResultToMySQL(query, result)
	case "show_tables":
		return showTables(defaultDatabase, "SHOW TABLES", false)
	default:
		insertID := uint64(0)
		if result.RowID != nil {
			insertID = *result.RowID
		}
		return &queryResult{affectedRows: result.RowsAffected, insertID: insertID}, nil
	}
}

func selectSQLResultToMySQL(query string, result *relational.SQLResult) (*queryResult, error) {
	names := selectColumnNames(query, result)
	columns := make([]column, 0, len(names))
	schema, _ := relational.LoadSchema(result.Table)
	for _, name := range names {
		if schema != nil {
			if relCol, ok := schemaColumnByName(schema, name); ok {
				columns = append(columns, columnForRelationalColumn(schema.Name, relCol))
				continue
			}
		}
		col := columnForResultValue(name, firstResultValue(result, name))
		if result.Table != "" {
			col.table = result.Table
			col.orgTable = result.Table
		}
		columns = append(columns, col)
	}

	rows := make([][]any, 0, len(result.Rows))
	for _, row := range result.Rows {
		values := make([]any, 0, len(names))
		for _, name := range names {
			if strings.EqualFold(name, "row_id") {
				values = append(values, row.RowID)
				continue
			}
			values = append(values, row.Values[name])
		}
		rows = append(rows, values)
	}
	return &queryResult{columns: columns, rows: rows}, nil
}

func selectColumnNames(query string, result *relational.SQLResult) []string {
	upper := strings.ToUpper(query)
	from := strings.Index(upper, " FROM ")
	if from > len("SELECT ") {
		projection := strings.TrimSpace(query[len("SELECT "):from])
		if projection == "*" {
			if result.Table != "" {
				if schema, err := relational.LoadSchema(result.Table); err == nil {
					names := make([]string, 0, len(schema.Columns))
					for _, col := range schema.Columns {
						names = append(names, col.Name)
					}
					return names
				}
			}
		} else {
			parts := strings.Split(projection, ",")
			names := make([]string, 0, len(parts))
			for _, part := range parts {
				name := cleanProjectionName(part)
				if name != "" {
					names = append(names, name)
				}
			}
			if len(names) > 0 {
				return names
			}
		}
	}

	names := make([]string, 0)
	if result.Table != "" {
		if schema, err := relational.LoadSchema(result.Table); err == nil {
			for _, col := range schema.Columns {
				names = append(names, col.Name)
			}
			return names
		}
	}
	seen := map[string]struct{}{}
	for _, row := range result.Rows {
		for name := range row.Values {
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func schemaColumnByName(schema *relational.Schema, name string) (relational.Column, bool) {
	for _, col := range schema.Columns {
		if strings.EqualFold(col.Name, name) {
			return col, true
		}
	}
	return relational.Column{}, false
}

func cleanProjectionName(part string) string {
	part = strings.TrimSpace(part)
	if idx := strings.LastIndex(part, "."); idx >= 0 {
		part = part[idx+1:]
	}
	fields := strings.Fields(part)
	if len(fields) >= 3 && strings.EqualFold(fields[len(fields)-2], "AS") {
		return fields[len(fields)-1]
	}
	if len(fields) > 0 {
		return fields[0]
	}
	return ""
}

func firstResultValue(result *relational.SQLResult, name string) any {
	if strings.EqualFold(name, "row_id") {
		return uint64(0)
	}
	for _, row := range result.Rows {
		if value, ok := row.Values[name]; ok {
			return value
		}
	}
	return nil
}

func columnForResultValue(name string, value any) column {
	col := column{name: name, typ: columnTypeVarString, length: 1024}
	switch value.(type) {
	case bool:
		col.typ = columnTypeTiny
		col.length = 1
	case int, int64:
		col.typ = columnTypeLongLong
		col.length = 20
	case uint64:
		col.typ = columnTypeLongLong
		col.flags = columnFlagUnsigned
		col.length = 20
	case float64:
		col.typ = columnTypeDouble
		col.length = 32
		col.decimals = 31
	case nil:
		col.typ = columnTypeNull
	}
	return col
}

func columnForRelationalColumn(table string, relCol relational.Column) column {
	col := column{
		schema:   defaultDatabase,
		table:    table,
		orgTable: table,
		name:     relCol.Name,
		orgName:  relCol.Name,
		length:   uint32(relCol.Size),
	}
	if col.length == 0 {
		col.length = 1024
	}
	switch relCol.Type {
	case relational.ColumnTypeBool:
		col.typ = columnTypeTiny
		col.length = 1
	case relational.ColumnTypeInt64:
		col.typ = columnTypeLongLong
		col.length = 20
	case relational.ColumnTypeUint64, relational.ColumnTypeBlobPtr, relational.ColumnTypeRowRef:
		col.typ = columnTypeLongLong
		col.flags = columnFlagUnsigned
		col.length = 20
	case relational.ColumnTypeFloat64:
		col.typ = columnTypeDouble
		col.length = 32
		col.decimals = 31
	case relational.ColumnTypeString:
		col.typ = columnTypeVarString
	default:
		col.typ = columnTypeVarString
	}
	if relCol.Indexed {
		col.flags |= columnFlagPriKey
	}
	return col
}
