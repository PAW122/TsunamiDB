package relational

import (
	"errors"
	"strings"
	"testing"
)

func TestExecuteSQLCRUD(t *testing.T) {
	withTempWorkingDir(t)

	created, err := ExecuteSQL("CREATE TABLE products (id uint64 INDEXED, name string(16) INDEXED TRIGRAM, price uint64, active bool)")
	if err != nil {
		t.Fatalf("CREATE TABLE: %v", err)
	}
	if created.Operation != "create_table" || created.Table != "products" || created.Schema == nil {
		t.Fatalf("create result = %+v", created)
	}

	inserted, err := ExecuteSQL("INSERT INTO products (id, name, price, active) VALUES (1, 'widget', 100, true)")
	if err != nil {
		t.Fatalf("INSERT first: %v", err)
	}
	if inserted.RowID == nil || *inserted.RowID != 0 || inserted.RowsAffected != 1 {
		t.Fatalf("insert result = %+v, want row_id 0 and one affected row", inserted)
	}
	if _, err := ExecuteSQL("INSERT INTO products (id, name, price, active) VALUES (2, 'gadget', 250, false)"); err != nil {
		t.Fatalf("INSERT second: %v", err)
	}

	selected, err := ExecuteSQL("SELECT row_id, name, price FROM products WHERE name LIKE '%wid%'")
	if err != nil {
		t.Fatalf("SELECT LIKE: %v", err)
	}
	if len(selected.Rows) != 1 || selected.Rows[0].RowID != 0 {
		t.Fatalf("selected rows = %+v, want row 0", selected.Rows)
	}
	if selected.Rows[0].Values["name"] != "widget" || selected.Rows[0].Values["price"] != uint64(100) {
		t.Fatalf("selected values = %+v, want projected widget values", selected.Rows[0].Values)
	}

	updated, err := ExecuteSQL("UPDATE products SET price = 175, name = 'bluewidget' WHERE row_id = 0")
	if err != nil {
		t.Fatalf("UPDATE: %v", err)
	}
	if updated.RowsAffected != 1 {
		t.Fatalf("updated rows = %d, want 1", updated.RowsAffected)
	}

	afterUpdate, err := ExecuteSQL("SELECT * FROM products WHERE price = 175")
	if err != nil {
		t.Fatalf("SELECT equality: %v", err)
	}
	if len(afterUpdate.Rows) != 1 || afterUpdate.Rows[0].Values["name"] != "bluewidget" {
		t.Fatalf("after update rows = %+v", afterUpdate.Rows)
	}

	deleted, err := ExecuteSQL("DELETE FROM products WHERE name = 'gadget'")
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}
	if deleted.RowsAffected != 1 {
		t.Fatalf("deleted rows = %d, want 1", deleted.RowsAffected)
	}

	remaining, err := ExecuteSQL("SELECT * FROM products")
	if err != nil {
		t.Fatalf("SELECT all: %v", err)
	}
	if len(remaining.Rows) != 1 || remaining.Rows[0].Values["name"] != "bluewidget" {
		t.Fatalf("remaining rows = %+v", remaining.Rows)
	}
}

func TestExecuteSQLCreateIndexes(t *testing.T) {
	withTempWorkingDir(t)

	if _, err := ExecuteSQL("CREATE TABLE users (id uint64, name string[16])"); err != nil {
		t.Fatalf("CREATE TABLE: %v", err)
	}
	if _, err := ExecuteSQL("INSERT INTO users (id, name) VALUES (1, 'alexander')"); err != nil {
		t.Fatalf("INSERT: %v", err)
	}
	if _, err := ExecuteSQL("CREATE INDEX users_id_idx ON users (id)"); err != nil {
		t.Fatalf("CREATE INDEX: %v", err)
	}
	if _, err := ExecuteSQL("CREATE TRIGRAM INDEX ON users (name)"); err != nil {
		t.Fatalf("CREATE TRIGRAM INDEX: %v", err)
	}

	schema, err := LoadSchema("users")
	if err != nil {
		t.Fatalf("LoadSchema: %v", err)
	}
	if !mustColumn(t, schema, "id").Indexed {
		t.Fatal("id column Indexed = false, want true")
	}
	if !mustColumn(t, schema, "name").TrigramIndexed {
		t.Fatal("name column TrigramIndexed = false, want true")
	}
}

func TestExecuteSQLOrderByNumbers(t *testing.T) {
	withTempWorkingDir(t)

	if _, err := ExecuteSQL("CREATE TABLE products (id uint64, name string(16), price uint64, delta int64, score float64)"); err != nil {
		t.Fatalf("CREATE TABLE: %v", err)
	}
	for _, query := range []string{
		"INSERT INTO products (id, name, price, delta, score) VALUES (1, 'cheap', 20, -5, 9.5)",
		"INSERT INTO products (id, name, price, delta, score) VALUES (2, 'expensive', 300, 4, 1.25)",
		"INSERT INTO products (id, name, price, delta, score) VALUES (3, 'middle', 100, 0, 3.75)",
	} {
		if _, err := ExecuteSQL(query); err != nil {
			t.Fatalf("INSERT: %v", err)
		}
	}

	ascending, err := ExecuteSQL("SELECT name, price FROM products ORDER BY price ASC")
	if err != nil {
		t.Fatalf("SELECT ORDER BY ASC: %v", err)
	}
	if got := orderedNames(ascending); got != "cheap,middle,expensive" {
		t.Fatalf("ascending names = %s", got)
	}

	descending, err := ExecuteSQL("SELECT name, price FROM products ORDER BY price DESC")
	if err != nil {
		t.Fatalf("SELECT ORDER BY DESC: %v", err)
	}
	if got := orderedNames(descending); got != "expensive,middle,cheap" {
		t.Fatalf("descending names = %s", got)
	}

	floatAscending, err := ExecuteSQL("SELECT name, score FROM products OREDER BY score")
	if err != nil {
		t.Fatalf("SELECT OREDER BY score: %v", err)
	}
	if got := orderedNames(floatAscending); got != "expensive,middle,cheap" {
		t.Fatalf("float ascending names = %s", got)
	}
}

func TestExecuteSQLOrderByDates(t *testing.T) {
	withTempWorkingDir(t)

	if _, err := ExecuteSQL("CREATE TABLE events (id uint64, name string(16), created_at string(32))"); err != nil {
		t.Fatalf("CREATE TABLE: %v", err)
	}
	for _, query := range []string{
		"INSERT INTO events (id, name, created_at) VALUES (1, 'spring', '2026-05-02')",
		"INSERT INTO events (id, name, created_at) VALUES (2, 'winter', '2025-12-24')",
		"INSERT INTO events (id, name, created_at) VALUES (3, 'summer', '2026-06-10T12:30:00Z')",
	} {
		if _, err := ExecuteSQL(query); err != nil {
			t.Fatalf("INSERT: %v", err)
		}
	}

	ascending, err := ExecuteSQL("SELECT name FROM events ORDER BY created_at ASC")
	if err != nil {
		t.Fatalf("SELECT ORDER BY date ASC: %v", err)
	}
	if got := orderedNames(ascending); got != "winter,spring,summer" {
		t.Fatalf("date ascending names = %s", got)
	}

	descending, err := ExecuteSQL("SELECT name FROM events ORDER BY created_at DESC")
	if err != nil {
		t.Fatalf("SELECT ORDER BY date DESC: %v", err)
	}
	if got := orderedNames(descending); got != "summer,spring,winter" {
		t.Fatalf("date descending names = %s", got)
	}
}

func TestExecuteSQLShowTables(t *testing.T) {
	withTempWorkingDir(t)

	if _, err := ExecuteSQL("CREATE TABLE products (id uint64, name string(16) INDEXED TRIGRAM)"); err != nil {
		t.Fatalf("CREATE TABLE products: %v", err)
	}
	if _, err := ExecuteSQL("CREATE TABLE users (id uint64, email string(32) INDEXED)"); err != nil {
		t.Fatalf("CREATE TABLE users: %v", err)
	}

	result, err := ExecuteSQL("SHOW TABLES")
	if err != nil {
		t.Fatalf("SHOW TABLES: %v", err)
	}
	if result.Operation != "show_tables" || result.RowsAffected != 2 || len(result.Rows) != 2 {
		t.Fatalf("SHOW TABLES result = %+v, want two rows", result)
	}
	if result.Rows[0].Values["table"] != "products" || result.Rows[1].Values["table"] != "users" {
		t.Fatalf("SHOW TABLES rows = %+v, want sorted products/users", result.Rows)
	}
	if result.Rows[0].Values["columns"] != uint64(2) || result.Rows[0].Values["trigram_indexes"] != uint64(1) {
		t.Fatalf("products metadata = %+v, want columns/trigram counts", result.Rows[0].Values)
	}
}

func TestExecuteSQLValidation(t *testing.T) {
	withTempWorkingDir(t)

	if _, err := ExecuteSQL(""); !errors.Is(err, ErrInvalidSQL) {
		t.Fatalf("empty SQL error = %v, want ErrInvalidSQL", err)
	}
	if _, err := ExecuteSQL("DROP TABLE users"); !errors.Is(err, ErrInvalidSQL) {
		t.Fatalf("unsupported SQL error = %v, want ErrInvalidSQL", err)
	}
	if _, err := ExecuteSQL("CREATE TABLE users (name string)"); !errors.Is(err, ErrInvalidSchema) {
		t.Fatalf("invalid schema error = %v, want ErrInvalidSchema", err)
	}
}

func orderedNames(result *SQLResult) string {
	names := make([]string, 0, len(result.Rows))
	for _, row := range result.Rows {
		names = append(names, row.Values["name"].(string))
	}
	return strings.Join(names, ",")
}
