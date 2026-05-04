package mysqlcompat

import (
	"os"
	"testing"

	"github.com/PAW122/TsunamiDB/data/relational"
)

func withTempWorkingDir(t *testing.T) {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir temp: %v", err)
	}
	t.Cleanup(func() {
		relational.ResetForTests()
		_ = os.Chdir(wd)
	})
}

func TestExecuteCompatQuerySupportsHeidiMetadata(t *testing.T) {
	withTempWorkingDir(t)

	if _, err := relational.ExecuteSQL("CREATE TABLE products (id uint64 INDEXED, name string(16), active bool)"); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := relational.ExecuteSQL("CREATE TABLE test (id uint64 INDEXED, name string(16))"); err != nil {
		t.Fatalf("create test table: %v", err)
	}

	tests := []struct {
		query         string
		wantResultset bool
	}{
		{query: "SELECT VERSION()", wantResultset: true},
		{query: "SELECT @@version_comment LIMIT 1", wantResultset: true},
		{query: "SELECT VERSION() AS version, @@version_comment AS version_comment, @@lower_case_table_names AS lctn", wantResultset: true},
		{query: "SELECT DATABASE() AS db, USER() AS user_name, 1 AS ok, 'abc' AS label, NULL AS missing", wantResultset: true},
		{query: "SELECT @@session.tx_isolation AS tx_isolation", wantResultset: true},
		{query: "SELECT @@global.max_allowed_packet AS max_packet", wantResultset: true},
		{query: "SELECT IFNULL(@@session.transaction_isolation, @@session.tx_isolation) AS tx_isolation", wantResultset: true},
		{query: "SELECT COALESCE(@@session.transaction_isolation, @@session.tx_isolation) AS tx_isolation FROM DUAL", wantResultset: true},
		{query: "/* HeidiSQL init */ SELECT VERSION()", wantResultset: true},
		{query: "/*!40101 SET NAMES utf8mb4 */", wantResultset: false},
		{query: "/*!40101 SET character_set_client = utf8 */;", wantResultset: false},
		{query: "-- HeidiSQL init\nSELECT DATABASE()", wantResultset: true},
		{query: "SHOW DATABASES", wantResultset: true},
		{query: "SHOW VARIABLES LIKE 'character_set_client'", wantResultset: true},
		{query: "SHOW FULL TABLES FROM `tsunamidb`", wantResultset: true},
		{query: "SHOW TABLE STATUS FROM `tsunamidb` LIKE 'products'", wantResultset: true},
		{query: "SHOW CREATE TABLE `products`", wantResultset: true},
		{query: "SHOW COLUMNS FROM `products`", wantResultset: true},
		{query: "SHOW PROCEDURE STATUS WHERE `Db`='tsunamidb'", wantResultset: true},
		{query: "SHOW FUNCTION STATUS WHERE `Db`='tsunamidb'", wantResultset: true},
		{query: "SHOW TRIGGERS FROM `tsunamidb`", wantResultset: true},
		{query: "SHOW EVENTS FROM `tsunamidb`", wantResultset: true},
		{query: "SELECT TABLE_NAME FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA='tsunamidb'", wantResultset: true},
		{query: "SELECT *, EVENT_SCHEMA AS `Db`, EVENT_NAME AS `Name` FROM information_schema.`EVENTS` WHERE `EVENT_SCHEMA`='tsunamidb'", wantResultset: true},
		{query: "SELECT * FROM information_schema.`TRIGGERS` WHERE `TRIGGER_SCHEMA`='tsunamidb'", wantResultset: true},
		{query: "SELECT * FROM information_schema.`ROUTINES` WHERE `ROUTINE_SCHEMA`='tsunamidb'", wantResultset: true},
		{query: "SELECT * FROM information_schema.REFERENTIAL_CONSTRAINTS WHERE CONSTRAINT_SCHEMA='tsunamidb' AND TABLE_NAME='test' AND REFERENCED_TABLE_NAME IS NOT NULL", wantResultset: true},
		{query: "SELECT * FROM information_schema.KEY_COLUMN_USAGE WHERE TABLE_SCHEMA='tsunamidb' AND TABLE_NAME='test' AND REFERENCED_TABLE_NAME IS NOT NULL", wantResultset: true},
		{query: "SHOW CREATE TABLE `tsunamidb`.`test`", wantResultset: true},
		{query: "SHOW INDEXES FROM `test` FROM `tsunamidb`", wantResultset: true},
		{query: "ALTER TABLE `test` CHANGE COLUMN `id` `id` INT(11) NOT NULL FIRST", wantResultset: false},
	}

	for _, test := range tests {
		t.Run(test.query, func(t *testing.T) {
			result, err := executeCompatQuery(defaultDatabase, test.query)
			if err != nil {
				t.Fatalf("executeCompatQuery: %v", err)
			}
			if (result.columns != nil) != test.wantResultset {
				t.Fatalf("resultset = %t, want %t", result.columns != nil, test.wantResultset)
			}
		})
	}
}

func TestExecuteCompatQueryRunsRelationalSQLWithMySQLQuoting(t *testing.T) {
	withTempWorkingDir(t)

	if _, err := executeCompatQuery(defaultDatabase, "CREATE TABLE `products` (`id` BIGINT UNSIGNED PRIMARY KEY, `name` VARCHAR(16), `active` TINYINT(1)) ENGINE=InnoDB"); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := executeCompatQuery(defaultDatabase, "INSERT INTO `products` (`id`, `name`, `active`) VALUES (1, 'widget', true)"); err != nil {
		t.Fatalf("insert: %v", err)
	}

	result, err := executeCompatQuery(defaultDatabase, "SELECT `row_id`, `name`, `active` FROM `tsunamidb`.`products` LIMIT 1000")
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if len(result.rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(result.rows))
	}
	if got := result.rows[0][1]; got != "widget" {
		t.Fatalf("name = %#v, want widget", got)
	}
}
