package mysqlcompat

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

func TestMySQLClientCompatibilityFlow(t *testing.T) {
	withTempWorkingDir(t)

	db := openMySQLClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("PingContext: %v", err)
	}

	execMySQL(t, ctx, db, "USE `tsunamidb`")
	execMySQL(t, ctx, db, "SET NAMES utf8mb4")
	execMySQL(t, ctx, db, "/*!40101 SET character_set_client = utf8mb4 */")

	var version, database string
	if err := db.QueryRowContext(ctx, "SELECT VERSION() AS version, DATABASE() AS db").Scan(&version, &database); err != nil {
		t.Fatalf("session SELECT scan: %v", err)
	}
	if !strings.Contains(version, "TsunamiDB") || database != defaultDatabase {
		t.Fatalf("session values version=%q database=%q", version, database)
	}

	execMySQL(t, ctx, db, `
		CREATE TABLE `+"`client_flow`"+` (
			`+"`id`"+` BIGINT UNSIGNED PRIMARY KEY,
			`+"`name`"+` VARCHAR(32),
			`+"`active`"+` TINYINT(1),
			`+"`score`"+` DOUBLE
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
	`)

	inserts := []struct {
		id     int
		name   string
		active string
		score  string
	}{
		{id: 1, name: "alpha", active: "true", score: "1.5"},
		{id: 2, name: "beta", active: "false", score: "2.75"},
		{id: 3, name: "alphabet", active: "true", score: "3.25"},
	}
	for _, row := range inserts {
		result := execMySQL(t, ctx, db,
			fmt.Sprintf("INSERT INTO `client_flow` (`id`, `name`, `active`, `score`) VALUES (%d, '%s', %s, %s)", row.id, row.name, row.active, row.score),
		)
		if affected := mustRowsAffected(t, result); affected != 1 {
			t.Fatalf("insert affected rows = %d, want 1", affected)
		}
	}

	execMySQL(t, ctx, db, "ALTER TABLE `client_flow` CHANGE COLUMN `id` `id` INT(11) NOT NULL FIRST")

	result := execMySQL(t, ctx, db, "UPDATE `client_flow` SET `score` = 4.5, `name` = 'gamma' WHERE `id` = 2")
	if affected := mustRowsAffected(t, result); affected != 1 {
		t.Fatalf("update affected rows = %d, want 1", affected)
	}

	var rowID uint64
	var name string
	var score float64
	if err := db.QueryRowContext(ctx, "SELECT `row_id`, `name`, `score` FROM `client_flow` WHERE `id` = 2").Scan(&rowID, &name, &score); err != nil {
		t.Fatalf("select updated row: %v", err)
	}
	if name != "gamma" || score != 4.5 {
		t.Fatalf("updated row row_id=%d name=%q score=%f", rowID, name, score)
	}

	allRows := queryMySQL(t, ctx, db, "SELECT * FROM `client_flow` LIMIT 10")
	allColumns, err := allRows.Columns()
	if err != nil {
		t.Fatalf("SELECT * columns: %v", err)
	}
	if got := strings.Join(allColumns, ","); got != "id,name,active,score" {
		t.Fatalf("SELECT * columns = %q, want id,name,active,score", got)
	}
	columnTypes, err := allRows.ColumnTypes()
	if err != nil {
		t.Fatalf("SELECT * column types: %v", err)
	}
	if len(columnTypes) != 4 {
		t.Fatalf("SELECT * column type count = %d, want 4", len(columnTypes))
	}

	rows := queryMySQL(t, ctx, db, "SELECT `row_id`, `name` FROM `client_flow` WHERE `name` LIKE '%alpha%' ORDER BY `name` ASC")
	var names []string
	for rows.Next() {
		var id uint64
		var n string
		if err := rows.Scan(&id, &n); err != nil {
			t.Fatalf("scan LIKE row: %v", err)
		}
		names = append(names, n)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("LIKE rows: %v", err)
	}
	if strings.Join(names, ",") != "alpha,alphabet" {
		t.Fatalf("LIKE names = %v", names)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO `client_flow` (`id`, `name`, `active`, `score`) VALUES (4, 'delta', true, 8.5)"); err != nil {
		_ = tx.Rollback()
		t.Fatalf("tx insert: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("tx commit: %v", err)
	}

	result = execMySQL(t, ctx, db, "DELETE FROM `client_flow` WHERE `id` = 1")
	if affected := mustRowsAffected(t, result); affected != 1 {
		t.Fatalf("delete affected rows = %d, want 1", affected)
	}

	assertHasRows(t, ctx, db, "SHOW DATABASES")
	assertHasRows(t, ctx, db, "SHOW FULL TABLES FROM `tsunamidb`")
	assertHasRows(t, ctx, db, "SHOW CREATE TABLE `tsunamidb`.`client_flow`")
	assertHasRows(t, ctx, db, "SHOW COLUMNS FROM `client_flow`")
	assertHasRows(t, ctx, db, "SHOW INDEXES FROM `client_flow` FROM `tsunamidb`")
	assertQueryOK(t, ctx, db, "SHOW PROCEDURE STATUS WHERE `Db`='tsunamidb'")
	assertQueryOK(t, ctx, db, "SHOW TRIGGERS FROM `tsunamidb`")
	assertQueryOK(t, ctx, db, "SELECT * FROM information_schema.KEY_COLUMN_USAGE WHERE TABLE_SCHEMA='tsunamidb' AND TABLE_NAME='client_flow'")
	assertQueryOK(t, ctx, db, "SELECT * FROM information_schema.REFERENTIAL_CONSTRAINTS WHERE CONSTRAINT_SCHEMA='tsunamidb' AND TABLE_NAME='client_flow'")
	assertQueryOK(t, ctx, db, "SELECT *, EVENT_SCHEMA AS `Db`, EVENT_NAME AS `Name` FROM information_schema.`EVENTS` WHERE `EVENT_SCHEMA`='tsunamidb'")
}

func openMySQLClient(t *testing.T) *sql.DB {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		err := Serve(listener)
		if errors.Is(err, net.ErrClosed) {
			err = nil
		}
		done <- err
	}()

	dsn := fmt.Sprintf(
		"root:password@tcp(%s)/%s?timeout=2s&readTimeout=2s&writeTimeout=2s&allowNativePasswords=true",
		listener.Addr().String(),
		defaultDatabase,
	)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		_ = listener.Close()
		t.Fatalf("sql.Open: %v", err)
	}

	t.Cleanup(func() {
		_ = db.Close()
		_ = listener.Close()
		if err := <-done; err != nil {
			t.Fatalf("mysql compat server: %v", err)
		}
	})
	return db
}

func execMySQL(t *testing.T, ctx context.Context, db execer, query string) sql.Result {
	t.Helper()
	result, err := db.ExecContext(ctx, query)
	if err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
	return result
}

type execer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func queryMySQL(t *testing.T, ctx context.Context, db *sql.DB, query string) *sql.Rows {
	t.Helper()
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		t.Fatalf("query %q: %v", query, err)
	}
	t.Cleanup(func() { _ = rows.Close() })
	return rows
}

func assertQueryOK(t *testing.T, ctx context.Context, db *sql.DB, query string) {
	t.Helper()
	rows := queryMySQL(t, ctx, db, query)
	if _, err := rows.Columns(); err != nil {
		t.Fatalf("columns for %q: %v", query, err)
	}
}

func assertHasRows(t *testing.T, ctx context.Context, db *sql.DB, query string) {
	t.Helper()
	rows := queryMySQL(t, ctx, db, query)
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			t.Fatalf("rows for %q: %v", query, err)
		}
		t.Fatalf("query %q returned no rows", query)
	}
}

func mustRowsAffected(t *testing.T, result sql.Result) int64 {
	t.Helper()
	affected, err := result.RowsAffected()
	if err != nil {
		t.Fatalf("RowsAffected: %v", err)
	}
	return affected
}
