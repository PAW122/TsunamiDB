package relational

import (
	"fmt"
	"os"
	"strconv"
	"testing"
)

const relationalBenchmarkDefaultRows = 10_000

var (
	relationalBenchmarkRowSink  map[string]any
	relationalBenchmarkRowsSink []ScannedRow
	relationalBenchmarkJoinSink []JoinedRow
)

func BenchmarkRelational(b *testing.B) {
	b.Run("InsertNoIndex", benchmarkRelationalInsertNoIndex)
	b.Run("InsertEqualityAndTrigramIndexes", benchmarkRelationalInsertWithIndexes)
	b.Run("ReadByRowID", benchmarkRelationalReadByRowID)
	b.Run("SelectEqualityScan", benchmarkRelationalSelectEqualityScan)
	b.Run("SelectEqualityIndex", benchmarkRelationalSelectEqualityIndex)
	b.Run("SelectLikeScan", benchmarkRelationalSelectLikeScan)
	b.Run("SelectLikeTrigramIndex", benchmarkRelationalSelectLikeTrigramIndex)
	b.Run("JoinRowRefIndexedPredicate", benchmarkRelationalJoinRowRefIndexedPredicate)
}

func benchmarkRelationalInsertNoIndex(b *testing.B) {
	withRelationalBenchmarkDir(b)

	schema := mustCreateRelationalBenchmarkTable(b, "bench_insert_no_index", false, false)
	b.ReportAllocs()
	b.SetBytes(int64(schema.RowSize))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := InsertRow(schema.Name, relationalBenchmarkValues(i)); err != nil {
			b.Fatalf("InsertRow %d: %v", i, err)
		}
	}
}

func benchmarkRelationalInsertWithIndexes(b *testing.B) {
	withRelationalBenchmarkDir(b)

	schema := mustCreateRelationalBenchmarkTable(b, "bench_insert_indexes", true, true)
	b.ReportAllocs()
	b.SetBytes(int64(schema.RowSize))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := InsertRow(schema.Name, relationalBenchmarkValues(i)); err != nil {
			b.Fatalf("InsertRow %d: %v", i, err)
		}
	}
}

func benchmarkRelationalReadByRowID(b *testing.B) {
	withRelationalBenchmarkDir(b)

	rows := relationalBenchmarkRows()
	schema := mustCreateRelationalBenchmarkTable(b, "bench_read", false, false)
	mustSeedRelationalBenchmarkRows(b, schema.Name, rows)

	b.ReportAllocs()
	b.SetBytes(int64(schema.RowSize))
	b.ReportMetric(float64(rows), "rows/table")
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		rowID := uint64((i * 7919) % rows)
		row, err := ReadRow(schema.Name, rowID)
		if err != nil {
			b.Fatalf("ReadRow %d: %v", rowID, err)
		}
		relationalBenchmarkRowSink = row
	}
}

func benchmarkRelationalSelectEqualityScan(b *testing.B) {
	withRelationalBenchmarkDir(b)

	rows := relationalBenchmarkRows()
	schema := mustCreateRelationalBenchmarkTable(b, "bench_select_eq_scan", false, false)
	mustSeedRelationalBenchmarkRows(b, schema.Name, rows)

	b.ReportAllocs()
	b.SetBytes(int64(rows) * int64(schema.RowSize))
	b.ReportMetric(float64(rows), "rows/table")
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		selected, err := SelectRows(schema.Name, Equal("segment", uint64(i%128)))
		if err != nil {
			b.Fatalf("SelectRows: %v", err)
		}
		relationalBenchmarkRowsSink = selected
	}
}

func benchmarkRelationalSelectEqualityIndex(b *testing.B) {
	withRelationalBenchmarkDir(b)

	rows := relationalBenchmarkRows()
	schema := mustCreateRelationalBenchmarkTable(b, "bench_select_eq_index", false, false)
	mustSeedRelationalBenchmarkRows(b, schema.Name, rows)
	if err := CreateIndex(schema.Name, "segment"); err != nil {
		b.Fatalf("CreateIndex: %v", err)
	}

	b.ReportAllocs()
	b.ReportMetric(float64(rows), "rows/table")
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		selected, err := SelectRows(schema.Name, Equal("segment", uint64(i%128)))
		if err != nil {
			b.Fatalf("SelectRows: %v", err)
		}
		relationalBenchmarkRowsSink = selected
	}
}

func benchmarkRelationalSelectLikeScan(b *testing.B) {
	withRelationalBenchmarkDir(b)

	rows := relationalBenchmarkRows()
	schema := mustCreateRelationalBenchmarkTable(b, "bench_select_like_scan", false, false)
	mustSeedRelationalBenchmarkRows(b, schema.Name, rows)

	b.ReportAllocs()
	b.SetBytes(int64(rows) * int64(schema.RowSize))
	b.ReportMetric(float64(rows), "rows/table")
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		selected, err := SelectRows(schema.Name, Like("note", relationalBenchmarkNeedlePattern(i)))
		if err != nil {
			b.Fatalf("SelectRows: %v", err)
		}
		relationalBenchmarkRowsSink = selected
	}
}

func benchmarkRelationalSelectLikeTrigramIndex(b *testing.B) {
	withRelationalBenchmarkDir(b)

	rows := relationalBenchmarkRows()
	schema := mustCreateRelationalBenchmarkTable(b, "bench_select_like_trigram", false, false)
	mustSeedRelationalBenchmarkRows(b, schema.Name, rows)
	if err := CreateTrigramIndex(schema.Name, "note"); err != nil {
		b.Fatalf("CreateTrigramIndex: %v", err)
	}

	b.ReportAllocs()
	b.ReportMetric(float64(rows), "rows/table")
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		selected, err := SelectRows(schema.Name, Like("note", relationalBenchmarkNeedlePattern(i)))
		if err != nil {
			b.Fatalf("SelectRows: %v", err)
		}
		relationalBenchmarkRowsSink = selected
	}
}

func benchmarkRelationalJoinRowRefIndexedPredicate(b *testing.B) {
	withRelationalBenchmarkDir(b)

	rows := relationalBenchmarkRows()
	mustCreateRelationalBenchmarkUsersAndOrders(b, rows)

	b.ReportAllocs()
	b.ReportMetric(float64(rows), "rows/table")
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		predicate := Equal("user_id", uint64(i%512))
		joined, err := JoinRowRef("bench_orders", "user_id", &predicate)
		if err != nil {
			b.Fatalf("JoinRowRef: %v", err)
		}
		relationalBenchmarkJoinSink = joined
	}
}

func withRelationalBenchmarkDir(b *testing.B) {
	b.Helper()

	wd, err := os.Getwd()
	if err != nil {
		b.Fatalf("getwd: %v", err)
	}
	tmp := b.TempDir()
	if err := os.Chdir(tmp); err != nil {
		b.Fatalf("chdir temp benchmark dir: %v", err)
	}
	b.Cleanup(func() {
		_ = os.Chdir(wd)
	})
}

func relationalBenchmarkRows() int {
	raw := os.Getenv("TSU_REL_BENCH_ROWS")
	if raw == "" {
		return relationalBenchmarkDefaultRows
	}
	rows, err := strconv.Atoi(raw)
	if err != nil || rows <= 0 {
		return relationalBenchmarkDefaultRows
	}
	return rows
}

func mustCreateRelationalBenchmarkTable(b *testing.B, table string, indexed bool, trigramIndexed bool) *Schema {
	b.Helper()

	schema, err := CreateTable(Schema{
		Name: table,
		Columns: []Column{
			{Name: "id", Type: ColumnTypeUint64},
			{Name: "segment", Type: ColumnTypeUint64, Indexed: indexed},
			{Name: "active", Type: ColumnTypeBool},
			{Name: "score", Type: ColumnTypeFloat64},
			{Name: "name", Type: ColumnTypeString, Size: 32},
			{Name: "note", Type: ColumnTypeString, Size: 64, TrigramIndexed: trigramIndexed},
		},
	})
	if err != nil {
		b.Fatalf("CreateTable %s: %v", table, err)
	}
	return schema
}

func mustSeedRelationalBenchmarkRows(b *testing.B, table string, rows int) {
	b.Helper()

	for i := 0; i < rows; i++ {
		if _, err := InsertRow(table, relationalBenchmarkValues(i)); err != nil {
			b.Fatalf("seed InsertRow %d: %v", i, err)
		}
	}
}

func relationalBenchmarkValues(i int) map[string]any {
	return map[string]any{
		"id":      uint64(i),
		"segment": uint64(i % 128),
		"active":  i%3 != 0,
		"score":   float64(i%10_000) / 10,
		"name":    fmt.Sprintf("user_%06d", i),
		"note":    fmt.Sprintf("segment_%03d needle_%03d row_%010d", i%128, i%100, i),
	}
}

func relationalBenchmarkNeedlePattern(i int) string {
	return fmt.Sprintf("%%needle_%03d%%", i%100)
}

func mustCreateRelationalBenchmarkUsersAndOrders(b *testing.B, orderRows int) {
	b.Helper()

	if _, err := CreateTable(Schema{
		Name: "bench_users",
		Columns: []Column{
			{Name: "id", Type: ColumnTypeUint64},
			{Name: "name", Type: ColumnTypeString, Size: 32},
		},
	}); err != nil {
		b.Fatalf("CreateTable users: %v", err)
	}
	for i := 0; i < 512; i++ {
		_, err := InsertRow("bench_users", map[string]any{
			"id":   uint64(i),
			"name": fmt.Sprintf("user_%03d", i),
		})
		if err != nil {
			b.Fatalf("seed user %d: %v", i, err)
		}
	}

	if _, err := CreateTable(Schema{
		Name: "bench_orders",
		Columns: []Column{
			{Name: "id", Type: ColumnTypeUint64},
			{Name: "user_id", Type: ColumnTypeRowRef, RefTable: "bench_users"},
			{Name: "total", Type: ColumnTypeUint64},
		},
	}); err != nil {
		b.Fatalf("CreateTable orders: %v", err)
	}
	for i := 0; i < orderRows; i++ {
		_, err := InsertRow("bench_orders", map[string]any{
			"id":      uint64(i),
			"user_id": uint64(i % 512),
			"total":   uint64(100 + i%900),
		})
		if err != nil {
			b.Fatalf("seed order %d: %v", i, err)
		}
	}
	if err := CreateIndex("bench_orders", "user_id"); err != nil {
		b.Fatalf("CreateIndex orders.user_id: %v", err)
	}
}
