package relational

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"slices"
	"testing"
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
		_ = os.Chdir(wd)
	})
}

func TestCreateTableCalculatesOffsetsAndWritesFiles(t *testing.T) {
	withTempWorkingDir(t)

	got, err := CreateTable(Schema{
		Name: "users",
		Columns: []Column{
			{Name: "id", Type: ColumnTypeUint64, Indexed: true},
			{Name: "name", Type: ColumnTypeString, Size: 32, Indexed: true},
			{Name: "email", Type: "string[48]", Indexed: true},
		},
	})
	if err != nil {
		t.Fatalf("CreateTable: %v", err)
	}

	if got.RowHeaderSize != RowHeaderSize {
		t.Fatalf("RowHeaderSize = %d, want %d", got.RowHeaderSize, RowHeaderSize)
	}
	if got.RowSize != 96 {
		t.Fatalf("RowSize = %d, want 96", got.RowSize)
	}

	wantOffsets := []uint64{0, 8, 40}
	wantSizes := []uint64{8, 32, 48}
	for i := range got.Columns {
		if got.Columns[i].Offset != wantOffsets[i] {
			t.Fatalf("column %d offset = %d, want %d", i, got.Columns[i].Offset, wantOffsets[i])
		}
		if got.Columns[i].Size != wantSizes[i] {
			t.Fatalf("column %d size = %d, want %d", i, got.Columns[i].Size, wantSizes[i])
		}
	}
	if got.Columns[2].Type != ColumnTypeString {
		t.Fatalf("string[N] type should be normalized to %q, got %q", ColumnTypeString, got.Columns[2].Type)
	}

	for _, path := range []string{
		filepath.Join("db", "rel", "users.schema"),
		filepath.Join("db", "rel", "users.rows"),
		filepath.Join("db", "rel", "users.free"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected %s to exist: %v", path, err)
		}
	}

	raw, err := os.ReadFile(filepath.Join("db", "rel", "users.schema"))
	if err != nil {
		t.Fatalf("read schema file: %v", err)
	}
	var persisted Schema
	if err := json.Unmarshal(raw, &persisted); err != nil {
		t.Fatalf("unmarshal schema: %v", err)
	}
	if persisted.RowSize != got.RowSize || len(persisted.Columns) != len(got.Columns) {
		t.Fatalf("persisted schema = %+v, want calculated schema %+v", persisted, got)
	}
}

func TestCreateTableRejectsDuplicateTable(t *testing.T) {
	withTempWorkingDir(t)

	schema := Schema{
		Name:    "users",
		Columns: []Column{{Name: "id", Type: ColumnTypeUint64}},
	}
	if _, err := CreateTable(schema); err != nil {
		t.Fatalf("first CreateTable: %v", err)
	}
	if _, err := CreateTable(schema); !errors.Is(err, ErrTableExists) {
		t.Fatalf("second CreateTable error = %v, want ErrTableExists", err)
	}
}

func TestCalculateSchemaValidation(t *testing.T) {
	tests := []struct {
		name   string
		schema Schema
	}{
		{
			name:   "invalid table name",
			schema: Schema{Name: "../bad", Columns: []Column{{Name: "id", Type: ColumnTypeUint64}}},
		},
		{
			name:   "missing columns",
			schema: Schema{Name: "users"},
		},
		{
			name: "duplicate column",
			schema: Schema{
				Name: "users",
				Columns: []Column{
					{Name: "id", Type: ColumnTypeUint64},
					{Name: "id", Type: ColumnTypeUint64},
				},
			},
		},
		{
			name:   "string without size",
			schema: Schema{Name: "users", Columns: []Column{{Name: "name", Type: ColumnTypeString}}},
		},
		{
			name:   "fixed size mismatch",
			schema: Schema{Name: "users", Columns: []Column{{Name: "id", Type: ColumnTypeUint64, Size: 4}}},
		},
		{
			name:   "unsupported type",
			schema: Schema{Name: "users", Columns: []Column{{Name: "value", Type: "json"}}},
		},
		{
			name:   "ref table on non row ref",
			schema: Schema{Name: "users", Columns: []Column{{Name: "id", Type: ColumnTypeUint64, RefTable: "groups"}}},
		},
		{
			name:   "invalid ref table name",
			schema: Schema{Name: "users", Columns: []Column{{Name: "group_id", Type: ColumnTypeRowRef, RefTable: "../groups"}}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := CalculateSchema(tt.schema); !errors.Is(err, ErrInvalidSchema) {
				t.Fatalf("CalculateSchema error = %v, want ErrInvalidSchema", err)
			}
		})
	}
}

func TestInsertRowEncodesFixedRowsAndAppends(t *testing.T) {
	withTempWorkingDir(t)

	schema, err := CreateTable(Schema{
		Name: "events",
		Columns: []Column{
			{Name: "id", Type: ColumnTypeUint64},
			{Name: "delta", Type: ColumnTypeInt64},
			{Name: "enabled", Type: ColumnTypeBool},
			{Name: "score", Type: ColumnTypeFloat64},
			{Name: "name", Type: ColumnTypeString, Size: 8},
			{Name: "blob", Type: ColumnTypeBlobPtr},
			{Name: "owner", Type: ColumnTypeRowRef},
		},
	})
	if err != nil {
		t.Fatalf("CreateTable: %v", err)
	}

	firstID, err := InsertRow("events", map[string]any{
		"id":      uint64(7),
		"delta":   int64(-2),
		"enabled": true,
		"score":   12.5,
		"name":    "alice",
		"blob":    uint64(99),
		"owner":   uint64(3),
	})
	if err != nil {
		t.Fatalf("InsertRow first: %v", err)
	}
	if firstID != 0 {
		t.Fatalf("first row id = %d, want 0", firstID)
	}

	secondID, err := InsertRow("events", map[string]any{
		"id":   uint64(8),
		"name": "bob",
	})
	if err != nil {
		t.Fatalf("InsertRow second: %v", err)
	}
	if secondID != 1 {
		t.Fatalf("second row id = %d, want 1", secondID)
	}

	raw, err := os.ReadFile(filepath.Join("db", "rel", "events.rows"))
	if err != nil {
		t.Fatalf("read rows: %v", err)
	}
	if got, want := uint64(len(raw)), schema.RowSize*2; got != want {
		t.Fatalf("rows file size = %d, want %d", got, want)
	}

	row := raw[:schema.RowSize]
	if row[0] != RowFlagActive {
		t.Fatalf("row flag = %d, want %d", row[0], RowFlagActive)
	}
	if got := binary.LittleEndian.Uint32(row[1:5]); got != RowVersion {
		t.Fatalf("row version = %d, want %d", got, RowVersion)
	}

	mustUint64At(t, row, schema, "id", 7)
	mustInt64At(t, row, schema, "delta", -2)
	mustByteAt(t, row, schema, "enabled", 1)
	mustFloat64At(t, row, schema, "score", 12.5)
	mustStringAt(t, row, schema, "name", "alice")
	mustUint64At(t, row, schema, "blob", 99)
	mustUint64At(t, row, schema, "owner", 3)

	second := raw[schema.RowSize:]
	mustUint64At(t, second, schema, "id", 8)
	mustInt64At(t, second, schema, "delta", 0)
	mustByteAt(t, second, schema, "enabled", 0)
	mustFloat64At(t, second, schema, "score", 0)
	mustStringAt(t, second, schema, "name", "bob")
	mustUint64At(t, second, schema, "blob", 0)
	mustUint64At(t, second, schema, "owner", 0)
}

func TestReadRowReadsByIDAndDecodesValues(t *testing.T) {
	withTempWorkingDir(t)

	if _, err := CreateTable(Schema{
		Name: "events",
		Columns: []Column{
			{Name: "id", Type: ColumnTypeUint64},
			{Name: "delta", Type: ColumnTypeInt64},
			{Name: "enabled", Type: ColumnTypeBool},
			{Name: "score", Type: ColumnTypeFloat64},
			{Name: "name", Type: ColumnTypeString, Size: 8},
			{Name: "blob", Type: ColumnTypeBlobPtr},
			{Name: "owner", Type: ColumnTypeRowRef},
		},
	}); err != nil {
		t.Fatalf("CreateTable: %v", err)
	}

	if _, err := InsertRow("events", map[string]any{
		"id":      uint64(7),
		"delta":   int64(-2),
		"enabled": true,
		"score":   12.5,
		"name":    "alice",
		"blob":    uint64(99),
		"owner":   uint64(3),
	}); err != nil {
		t.Fatalf("InsertRow first: %v", err)
	}
	if _, err := InsertRow("events", map[string]any{
		"id":   uint64(8),
		"name": "bob",
	}); err != nil {
		t.Fatalf("InsertRow second: %v", err)
	}

	first, err := ReadRow("events", 0)
	if err != nil {
		t.Fatalf("ReadRow first: %v", err)
	}
	if first["id"] != uint64(7) {
		t.Fatalf("id = %#v, want uint64(7)", first["id"])
	}
	if first["delta"] != int64(-2) {
		t.Fatalf("delta = %#v, want int64(-2)", first["delta"])
	}
	if first["enabled"] != true {
		t.Fatalf("enabled = %#v, want true", first["enabled"])
	}
	if first["score"] != 12.5 {
		t.Fatalf("score = %#v, want 12.5", first["score"])
	}
	if first["name"] != "alice" {
		t.Fatalf("name = %#v, want alice", first["name"])
	}
	if first["blob"] != uint64(99) {
		t.Fatalf("blob = %#v, want uint64(99)", first["blob"])
	}
	if first["owner"] != uint64(3) {
		t.Fatalf("owner = %#v, want uint64(3)", first["owner"])
	}

	second, err := ReadRow("events", 1)
	if err != nil {
		t.Fatalf("ReadRow second: %v", err)
	}
	if second["id"] != uint64(8) {
		t.Fatalf("second id = %#v, want uint64(8)", second["id"])
	}
	if second["delta"] != int64(0) {
		t.Fatalf("second delta = %#v, want int64(0)", second["delta"])
	}
	if second["enabled"] != false {
		t.Fatalf("second enabled = %#v, want false", second["enabled"])
	}
	if second["score"] != float64(0) {
		t.Fatalf("second score = %#v, want 0", second["score"])
	}
	if second["name"] != "bob" {
		t.Fatalf("second name = %#v, want bob", second["name"])
	}
	if second["blob"] != uint64(0) {
		t.Fatalf("second blob = %#v, want uint64(0)", second["blob"])
	}
	if second["owner"] != uint64(0) {
		t.Fatalf("second owner = %#v, want uint64(0)", second["owner"])
	}
}

func TestRelationalCRUDScenarioValidatesResults(t *testing.T) {
	withTempWorkingDir(t)

	if _, err := CreateTable(Schema{
		Name: "products",
		Columns: []Column{
			{Name: "id", Type: ColumnTypeUint64, Indexed: true},
			{Name: "name", Type: ColumnTypeString, Size: 16, Indexed: true, TrigramIndexed: true},
			{Name: "price", Type: ColumnTypeUint64},
			{Name: "active", Type: ColumnTypeBool},
		},
	}); err != nil {
		t.Fatalf("CreateTable: %v", err)
	}

	widgetID, err := InsertRow("products", map[string]any{
		"id":     uint64(1),
		"name":   "widget",
		"price":  uint64(100),
		"active": true,
	})
	if err != nil {
		t.Fatalf("InsertRow widget: %v", err)
	}
	gadgetID, err := InsertRow("products", map[string]any{
		"id":     uint64(2),
		"name":   "gadget",
		"price":  uint64(250),
		"active": false,
	})
	if err != nil {
		t.Fatalf("InsertRow gadget: %v", err)
	}
	proID, err := InsertRow("products", map[string]any{
		"id":     uint64(3),
		"name":   "widgetpro",
		"price":  uint64(300),
		"active": true,
	})
	if err != nil {
		t.Fatalf("InsertRow widgetpro: %v", err)
	}
	if widgetID != 0 || gadgetID != 1 || proID != 2 {
		t.Fatalf("row IDs = %d, %d, %d; want 0, 1, 2", widgetID, gadgetID, proID)
	}

	read, err := ReadRow("products", widgetID)
	if err != nil {
		t.Fatalf("ReadRow widget: %v", err)
	}
	if read["name"] != "widget" || read["price"] != uint64(100) || read["active"] != true {
		t.Fatalf("read widget = %+v, want original widget values", read)
	}

	if err := UpdateRow("products", widgetID, map[string]any{
		"name":  "bluewidget",
		"price": uint64(175),
	}); err != nil {
		t.Fatalf("UpdateRow widget: %v", err)
	}
	updated, err := ReadRow("products", widgetID)
	if err != nil {
		t.Fatalf("ReadRow updated widget: %v", err)
	}
	if updated["id"] != uint64(1) || updated["name"] != "bluewidget" || updated["price"] != uint64(175) || updated["active"] != true {
		t.Fatalf("updated widget = %+v, want edited name/price with other fields preserved", updated)
	}

	oldName, err := SelectRows("products", Equal("name", "widget"))
	if err != nil {
		t.Fatalf("SelectRows old name: %v", err)
	}
	if len(oldName) != 0 {
		t.Fatalf("old name selected = %+v, want no rows after update", oldName)
	}
	newName, err := SelectRows("products", Equal("name", "bluewidget"))
	if err != nil {
		t.Fatalf("SelectRows new name: %v", err)
	}
	if len(newName) != 1 || newName[0].RowID != widgetID {
		t.Fatalf("new name selected = %+v, want updated row ID %d", newName, widgetID)
	}
	liked, err := SelectRows("products", Like("name", "%widget%"))
	if err != nil {
		t.Fatalf("SelectRows like widget: %v", err)
	}
	if len(liked) != 2 || liked[0].RowID != widgetID || liked[1].RowID != proID {
		t.Fatalf("like selected = %+v, want updated widget and widgetpro", liked)
	}

	if err := DeleteRow("products", gadgetID); err != nil {
		t.Fatalf("DeleteRow gadget: %v", err)
	}
	if _, err := ReadRow("products", gadgetID); !errors.Is(err, ErrRowNotFound) {
		t.Fatalf("ReadRow deleted error = %v, want ErrRowNotFound", err)
	}
	deletedName, err := SelectRows("products", Equal("name", "gadget"))
	if err != nil {
		t.Fatalf("SelectRows deleted name: %v", err)
	}
	if len(deletedName) != 0 {
		t.Fatalf("deleted name selected = %+v, want no rows", deletedName)
	}
	scanned, err := ScanRows("products", nil)
	if err != nil {
		t.Fatalf("ScanRows after delete: %v", err)
	}
	if len(scanned) != 2 || scanned[0].RowID != widgetID || scanned[1].RowID != proID {
		t.Fatalf("scanned after delete = %+v, want rows %d and %d", scanned, widgetID, proID)
	}

	freeRaw, err := os.ReadFile(filepath.Join("db", "rel", "products.free"))
	if err != nil {
		t.Fatalf("read free file: %v", err)
	}
	if string(freeRaw) != "1\n" {
		t.Fatalf("free file = %q, want %q", string(freeRaw), "1\n")
	}

	result, err := CompactTable("products")
	if err != nil {
		t.Fatalf("CompactTable: %v", err)
	}
	if result.RowsBefore != 3 || result.RowsAfter != 2 || result.Removed != 1 {
		t.Fatalf("compaction result = %+v, want 3 before, 2 after, 1 removed", result)
	}
	if result.RowIDMap[widgetID] != 0 || result.RowIDMap[proID] != 1 {
		t.Fatalf("row ID map = %v, want %d->0 and %d->1", result.RowIDMap, widgetID, proID)
	}
	if _, ok := result.RowIDMap[gadgetID]; ok {
		t.Fatalf("deleted row ID %d should not be mapped: %v", gadgetID, result.RowIDMap)
	}

	compacted, err := ReadRow("products", 1)
	if err != nil {
		t.Fatalf("ReadRow compacted widgetpro: %v", err)
	}
	if compacted["id"] != uint64(3) || compacted["name"] != "widgetpro" {
		t.Fatalf("compacted row 1 = %+v, want widgetpro", compacted)
	}
	if _, err := ReadRow("products", 2); !errors.Is(err, ErrRowNotFound) {
		t.Fatalf("ReadRow old tail error = %v, want ErrRowNotFound", err)
	}

	selectedAfterCompact, err := SelectRows("products", Equal("name", "widgetpro"))
	if err != nil {
		t.Fatalf("SelectRows after compact: %v", err)
	}
	if len(selectedAfterCompact) != 1 || selectedAfterCompact[0].RowID != 1 {
		t.Fatalf("selected after compact = %+v, want widgetpro at row ID 1", selectedAfterCompact)
	}
}

func TestScanSequentiallyFiltersRows(t *testing.T) {
	withTempWorkingDir(t)

	schema, err := CreateTable(Schema{
		Name: "users",
		Columns: []Column{
			{Name: "id", Type: ColumnTypeUint64},
			{Name: "name", Type: ColumnTypeString, Size: 8},
			{Name: "active", Type: ColumnTypeBool},
		},
	})
	if err != nil {
		t.Fatalf("CreateTable: %v", err)
	}

	for _, values := range []map[string]any{
		{"id": uint64(1), "name": "alice", "active": true},
		{"id": uint64(2), "name": "bob", "active": false},
		{"id": uint64(3), "name": "alex", "active": true},
	} {
		if _, err := InsertRow("users", values); err != nil {
			t.Fatalf("InsertRow: %v", err)
		}
	}

	all, err := Scan("users", nil)
	if err != nil {
		t.Fatalf("Scan all: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("Scan all returned %d rows, want 3", len(all))
	}

	filtered, err := Scan("users", func(row map[string]any) bool {
		return row["active"] == true
	})
	if err != nil {
		t.Fatalf("Scan filtered: %v", err)
	}
	if len(filtered) != 2 {
		t.Fatalf("Scan filtered returned %d rows, want 2", len(filtered))
	}
	if filtered[0]["name"] != "alice" || filtered[1]["name"] != "alex" {
		t.Fatalf("filtered names = %q, %q; want alice, alex", filtered[0]["name"], filtered[1]["name"])
	}

	rowsPath := filepath.Join("db", "rel", "users.rows")
	file, err := os.OpenFile(rowsPath, os.O_RDWR, 0o644)
	if err != nil {
		t.Fatalf("open rows: %v", err)
	}
	if _, err := file.WriteAt([]byte{0}, int64(schema.RowSize)); err != nil {
		_ = file.Close()
		t.Fatalf("mark row inactive: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close rows: %v", err)
	}

	scanned, err := ScanRows("users", nil)
	if err != nil {
		t.Fatalf("ScanRows: %v", err)
	}
	if len(scanned) != 2 {
		t.Fatalf("ScanRows returned %d rows, want 2", len(scanned))
	}
	if scanned[0].RowID != 0 || scanned[1].RowID != 2 {
		t.Fatalf("ScanRows row IDs = %d, %d; want 0, 2", scanned[0].RowID, scanned[1].RowID)
	}
}

func TestCreateIndexBuildsEqualityIndexAndMarksSchema(t *testing.T) {
	withTempWorkingDir(t)

	schema, err := CreateTable(Schema{
		Name: "users",
		Columns: []Column{
			{Name: "id", Type: ColumnTypeUint64},
			{Name: "name", Type: ColumnTypeString, Size: 8},
			{Name: "active", Type: ColumnTypeBool},
		},
	})
	if err != nil {
		t.Fatalf("CreateTable: %v", err)
	}

	for _, values := range []map[string]any{
		{"id": uint64(1), "name": "alice", "active": true},
		{"id": uint64(2), "name": "bob", "active": true},
		{"id": uint64(3), "name": "alice", "active": false},
	} {
		if _, err := InsertRow("users", values); err != nil {
			t.Fatalf("InsertRow: %v", err)
		}
	}

	rowsPath := filepath.Join("db", "rel", "users.rows")
	file, err := os.OpenFile(rowsPath, os.O_RDWR, 0o644)
	if err != nil {
		t.Fatalf("open rows: %v", err)
	}
	if _, err := file.WriteAt([]byte{0}, int64(schema.RowSize)); err != nil {
		_ = file.Close()
		t.Fatalf("mark row inactive: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close rows: %v", err)
	}

	if err := CreateIndex("users", "name"); err != nil {
		t.Fatalf("CreateIndex: %v", err)
	}

	loaded, err := LoadSchema("users")
	if err != nil {
		t.Fatalf("LoadSchema: %v", err)
	}
	if !mustColumn(t, loaded, "name").Indexed {
		t.Fatal("name column Indexed = false, want true")
	}

	index := mustReadEqualityIndex(t, "users", "name")
	nameColumn := mustColumn(t, loaded, "name")
	aliceKey, err := indexKey(nameColumn, "alice")
	if err != nil {
		t.Fatalf("indexKey alice: %v", err)
	}
	bobKey, err := indexKey(nameColumn, "bob")
	if err != nil {
		t.Fatalf("indexKey bob: %v", err)
	}
	if got, want := index.Values[aliceKey], []uint64{0, 2}; !slices.Equal(got, want) {
		t.Fatalf("alice row IDs = %v, want %v", got, want)
	}
	if _, ok := index.Values[bobKey]; ok {
		t.Fatalf("bob key should be absent after row 1 was marked inactive")
	}
}

func TestInsertRowUpdatesEqualityIndex(t *testing.T) {
	withTempWorkingDir(t)

	if _, err := CreateTable(Schema{
		Name: "users",
		Columns: []Column{
			{Name: "id", Type: ColumnTypeUint64},
			{Name: "name", Type: ColumnTypeString, Size: 8},
		},
	}); err != nil {
		t.Fatalf("CreateTable: %v", err)
	}

	if _, err := InsertRow("users", map[string]any{"id": uint64(1), "name": "alice"}); err != nil {
		t.Fatalf("InsertRow first: %v", err)
	}
	if err := CreateIndex("users", "name"); err != nil {
		t.Fatalf("CreateIndex: %v", err)
	}
	if _, err := InsertRow("users", map[string]any{"id": uint64(2), "name": "alice"}); err != nil {
		t.Fatalf("InsertRow second: %v", err)
	}
	if _, err := InsertRow("users", map[string]any{"id": uint64(3), "name": "bob"}); err != nil {
		t.Fatalf("InsertRow third: %v", err)
	}

	loaded, err := LoadSchema("users")
	if err != nil {
		t.Fatalf("LoadSchema: %v", err)
	}
	nameColumn := mustColumn(t, loaded, "name")
	aliceKey, err := indexKey(nameColumn, "alice")
	if err != nil {
		t.Fatalf("indexKey alice: %v", err)
	}
	bobKey, err := indexKey(nameColumn, "bob")
	if err != nil {
		t.Fatalf("indexKey bob: %v", err)
	}

	index := mustReadEqualityIndex(t, "users", "name")
	if got, want := index.Values[aliceKey], []uint64{0, 1}; !slices.Equal(got, want) {
		t.Fatalf("alice row IDs = %v, want %v", got, want)
	}
	if got, want := index.Values[bobKey], []uint64{2}; !slices.Equal(got, want) {
		t.Fatalf("bob row IDs = %v, want %v", got, want)
	}
}

func TestCreateTrigramIndexBuildsIndexAndMarksSchema(t *testing.T) {
	withTempWorkingDir(t)

	if _, err := CreateTable(Schema{
		Name: "users",
		Columns: []Column{
			{Name: "id", Type: ColumnTypeUint64},
			{Name: "name", Type: ColumnTypeString, Size: 16},
		},
	}); err != nil {
		t.Fatalf("CreateTable: %v", err)
	}

	for _, values := range []map[string]any{
		{"id": uint64(1), "name": "alexander"},
		{"id": uint64(2), "name": "lexicon"},
		{"id": uint64(3), "name": "bob"},
	} {
		if _, err := InsertRow("users", values); err != nil {
			t.Fatalf("InsertRow: %v", err)
		}
	}

	if err := CreateTrigramIndex("users", "name"); err != nil {
		t.Fatalf("CreateTrigramIndex: %v", err)
	}

	loaded, err := LoadSchema("users")
	if err != nil {
		t.Fatalf("LoadSchema: %v", err)
	}
	if !mustColumn(t, loaded, "name").TrigramIndexed {
		t.Fatal("name column TrigramIndexed = false, want true")
	}

	index := mustReadTrigramIndex(t, "users", "name")
	if got, want := index.Values["lex"], []uint64{0, 1}; !slices.Equal(got, want) {
		t.Fatalf("lex row IDs = %v, want %v", got, want)
	}
	if got, want := index.Values["ale"], []uint64{0}; !slices.Equal(got, want) {
		t.Fatalf("ale row IDs = %v, want %v", got, want)
	}
}

func TestInsertRowUpdatesTrigramIndex(t *testing.T) {
	withTempWorkingDir(t)

	if _, err := CreateTable(Schema{
		Name: "users",
		Columns: []Column{
			{Name: "id", Type: ColumnTypeUint64},
			{Name: "name", Type: ColumnTypeString, Size: 16},
		},
	}); err != nil {
		t.Fatalf("CreateTable: %v", err)
	}

	if _, err := InsertRow("users", map[string]any{"id": uint64(1), "name": "alexander"}); err != nil {
		t.Fatalf("InsertRow first: %v", err)
	}
	if err := CreateTrigramIndex("users", "name"); err != nil {
		t.Fatalf("CreateTrigramIndex: %v", err)
	}
	if _, err := InsertRow("users", map[string]any{"id": uint64(2), "name": "plexus"}); err != nil {
		t.Fatalf("InsertRow second: %v", err)
	}

	index := mustReadTrigramIndex(t, "users", "name")
	if got, want := index.Values["lex"], []uint64{0, 1}; !slices.Equal(got, want) {
		t.Fatalf("lex row IDs = %v, want %v", got, want)
	}
}

func TestRebuildIndexesRecreatesConfiguredIndexes(t *testing.T) {
	withTempWorkingDir(t)

	if _, err := CreateTable(Schema{
		Name: "users",
		Columns: []Column{
			{Name: "id", Type: ColumnTypeUint64, Indexed: true},
			{Name: "name", Type: ColumnTypeString, Size: 16, Indexed: true, TrigramIndexed: true},
		},
	}); err != nil {
		t.Fatalf("CreateTable: %v", err)
	}

	for _, values := range []map[string]any{
		{"id": uint64(1), "name": "alexander"},
		{"id": uint64(2), "name": "lexicon"},
		{"id": uint64(3), "name": "bob"},
	} {
		if _, err := InsertRow("users", values); err != nil {
			t.Fatalf("InsertRow: %v", err)
		}
	}

	if err := os.WriteFile(indexPath("users", "name"), []byte(`{"broken":true}`), 0o644); err != nil {
		t.Fatalf("corrupt equality index: %v", err)
	}
	if err := os.WriteFile(trigramIndexPath("users", "name"), []byte(`{"broken":true}`), 0o644); err != nil {
		t.Fatalf("corrupt trigram index: %v", err)
	}

	if err := RebuildIndexes("users"); err != nil {
		t.Fatalf("RebuildIndexes: %v", err)
	}

	loaded, err := LoadSchema("users")
	if err != nil {
		t.Fatalf("LoadSchema: %v", err)
	}
	nameColumn := mustColumn(t, loaded, "name")
	alexanderKey, err := indexKey(nameColumn, "alexander")
	if err != nil {
		t.Fatalf("indexKey alexander: %v", err)
	}

	equality := mustReadEqualityIndex(t, "users", "name")
	if got, want := equality.Values[alexanderKey], []uint64{0}; !slices.Equal(got, want) {
		t.Fatalf("alexander row IDs = %v, want %v", got, want)
	}

	trigram := mustReadTrigramIndex(t, "users", "name")
	if got, want := trigram.Values["lex"], []uint64{0, 1}; !slices.Equal(got, want) {
		t.Fatalf("lex row IDs = %v, want %v", got, want)
	}
}

func TestRebuildSingleIndexesMarkSchema(t *testing.T) {
	withTempWorkingDir(t)

	if _, err := CreateTable(Schema{
		Name: "users",
		Columns: []Column{
			{Name: "id", Type: ColumnTypeUint64},
			{Name: "name", Type: ColumnTypeString, Size: 16},
		},
	}); err != nil {
		t.Fatalf("CreateTable: %v", err)
	}
	if _, err := InsertRow("users", map[string]any{"id": uint64(1), "name": "alexander"}); err != nil {
		t.Fatalf("InsertRow: %v", err)
	}

	if err := RebuildEqualityIndex("users", "id"); err != nil {
		t.Fatalf("RebuildEqualityIndex: %v", err)
	}
	if err := RebuildTrigramIndex("users", "name"); err != nil {
		t.Fatalf("RebuildTrigramIndex: %v", err)
	}

	loaded, err := LoadSchema("users")
	if err != nil {
		t.Fatalf("LoadSchema: %v", err)
	}
	if !mustColumn(t, loaded, "id").Indexed {
		t.Fatal("id column Indexed = false, want true")
	}
	if !mustColumn(t, loaded, "name").TrigramIndexed {
		t.Fatal("name column TrigramIndexed = false, want true")
	}
}

func TestCompactTableRemovesInactiveRowsClearsFreeAndRebuildsIndexes(t *testing.T) {
	withTempWorkingDir(t)

	schema, err := CreateTable(Schema{
		Name: "users",
		Columns: []Column{
			{Name: "id", Type: ColumnTypeUint64, Indexed: true},
			{Name: "name", Type: ColumnTypeString, Size: 16, Indexed: true, TrigramIndexed: true},
		},
	})
	if err != nil {
		t.Fatalf("CreateTable: %v", err)
	}

	for _, values := range []map[string]any{
		{"id": uint64(1), "name": "alice"},
		{"id": uint64(2), "name": "bob"},
		{"id": uint64(3), "name": "alex"},
	} {
		if _, err := InsertRow("users", values); err != nil {
			t.Fatalf("InsertRow: %v", err)
		}
	}

	markRowInactive(t, "users", schema.RowSize, 1)
	if err := os.WriteFile(filepath.Join("db", "rel", "users.free"), []byte("stale"), 0o644); err != nil {
		t.Fatalf("write free file: %v", err)
	}

	result, err := CompactTable("users")
	if err != nil {
		t.Fatalf("CompactTable: %v", err)
	}
	if result.RowsBefore != 3 || result.RowsAfter != 2 || result.Removed != 1 {
		t.Fatalf("compaction result = %+v, want 3 before, 2 after, 1 removed", result)
	}
	if result.RowIDMap[0] != 0 || result.RowIDMap[2] != 1 {
		t.Fatalf("row ID map = %v, want 0->0 and 2->1", result.RowIDMap)
	}
	if _, ok := result.RowIDMap[1]; ok {
		t.Fatalf("deleted row ID 1 should not be in row ID map: %v", result.RowIDMap)
	}

	rowsInfo, err := os.Stat(filepath.Join("db", "rel", "users.rows"))
	if err != nil {
		t.Fatalf("stat rows: %v", err)
	}
	if got, want := rowsInfo.Size(), int64(schema.RowSize*2); got != want {
		t.Fatalf("rows file size = %d, want %d", got, want)
	}
	freeInfo, err := os.Stat(filepath.Join("db", "rel", "users.free"))
	if err != nil {
		t.Fatalf("stat free: %v", err)
	}
	if freeInfo.Size() != 0 {
		t.Fatalf("free file size = %d, want 0", freeInfo.Size())
	}

	row, err := ReadRow("users", 1)
	if err != nil {
		t.Fatalf("ReadRow compacted row: %v", err)
	}
	if row["id"] != uint64(3) || row["name"] != "alex" {
		t.Fatalf("compacted row 1 = %+v, want alex row", row)
	}
	if _, err := ReadRow("users", 2); !errors.Is(err, ErrRowNotFound) {
		t.Fatalf("ReadRow old tail error = %v, want ErrRowNotFound", err)
	}

	selected, err := SelectRows("users", Equal("name", "alex"))
	if err != nil {
		t.Fatalf("SelectRows equality: %v", err)
	}
	if len(selected) != 1 || selected[0].RowID != 1 {
		t.Fatalf("equality selected = %+v, want row ID 1", selected)
	}

	liked, err := SelectRows("users", Like("name", "%lex%"))
	if err != nil {
		t.Fatalf("SelectRows like: %v", err)
	}
	if len(liked) != 1 || liked[0].RowID != 1 {
		t.Fatalf("like selected = %+v, want row ID 1", liked)
	}
}

func TestSelectUsesTrigramIndexForLikeWhenAvailable(t *testing.T) {
	withTempWorkingDir(t)

	schema, err := CreateTable(Schema{
		Name: "users",
		Columns: []Column{
			{Name: "id", Type: ColumnTypeUint64},
			{Name: "name", Type: ColumnTypeString, Size: 16},
		},
	})
	if err != nil {
		t.Fatalf("CreateTable: %v", err)
	}

	for _, values := range []map[string]any{
		{"id": uint64(1), "name": "alexander"},
		{"id": uint64(2), "name": "lexicon"},
		{"id": uint64(3), "name": "bob"},
	} {
		if _, err := InsertRow("users", values); err != nil {
			t.Fatalf("InsertRow: %v", err)
		}
	}
	if err := CreateTrigramIndex("users", "name"); err != nil {
		t.Fatalf("CreateTrigramIndex: %v", err)
	}

	rowsPath := filepath.Join("db", "rel", "users.rows")
	file, err := os.OpenFile(rowsPath, os.O_RDWR, 0o644)
	if err != nil {
		t.Fatalf("open rows: %v", err)
	}
	if _, err := file.WriteAt([]byte{99}, int64(schema.RowSize*2)+1); err != nil {
		_ = file.Close()
		t.Fatalf("corrupt non-candidate row version: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close rows: %v", err)
	}

	selected, err := SelectRows("users", Like("name", "%lex%"))
	if err != nil {
		t.Fatalf("SelectRows: %v", err)
	}
	if len(selected) != 2 {
		t.Fatalf("SelectRows returned %d rows, want 2", len(selected))
	}
	if selected[0].RowID != 0 || selected[1].RowID != 1 {
		t.Fatalf("SelectRows row IDs = %d, %d; want 0, 1", selected[0].RowID, selected[1].RowID)
	}
}

func TestSelectUsesEqualityIndexWhenAvailable(t *testing.T) {
	withTempWorkingDir(t)

	schema, err := CreateTable(Schema{
		Name: "users",
		Columns: []Column{
			{Name: "id", Type: ColumnTypeUint64},
			{Name: "name", Type: ColumnTypeString, Size: 8},
			{Name: "active", Type: ColumnTypeBool},
		},
	})
	if err != nil {
		t.Fatalf("CreateTable: %v", err)
	}

	for _, values := range []map[string]any{
		{"id": uint64(1), "name": "alice", "active": true},
		{"id": uint64(2), "name": "bob", "active": true},
		{"id": uint64(3), "name": "alice", "active": false},
	} {
		if _, err := InsertRow("users", values); err != nil {
			t.Fatalf("InsertRow: %v", err)
		}
	}
	if err := CreateIndex("users", "name"); err != nil {
		t.Fatalf("CreateIndex: %v", err)
	}

	rowsPath := filepath.Join("db", "rel", "users.rows")
	file, err := os.OpenFile(rowsPath, os.O_RDWR, 0o644)
	if err != nil {
		t.Fatalf("open rows: %v", err)
	}
	if _, err := file.WriteAt([]byte{99}, int64(schema.RowSize)+1); err != nil {
		_ = file.Close()
		t.Fatalf("corrupt non-candidate row version: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close rows: %v", err)
	}

	selected, err := SelectRows("users", Equal("name", "alice"))
	if err != nil {
		t.Fatalf("SelectRows: %v", err)
	}
	if len(selected) != 2 {
		t.Fatalf("SelectRows returned %d rows, want 2", len(selected))
	}
	if selected[0].RowID != 0 || selected[1].RowID != 2 {
		t.Fatalf("SelectRows row IDs = %d, %d; want 0, 2", selected[0].RowID, selected[1].RowID)
	}
	if selected[0].Values["name"] != "alice" || selected[1].Values["name"] != "alice" {
		t.Fatalf("SelectRows names = %q, %q; want alice, alice", selected[0].Values["name"], selected[1].Values["name"])
	}
}

func TestSelectFallsBackToScanWithoutIndex(t *testing.T) {
	withTempWorkingDir(t)

	if _, err := CreateTable(Schema{
		Name: "users",
		Columns: []Column{
			{Name: "id", Type: ColumnTypeUint64},
			{Name: "name", Type: ColumnTypeString, Size: 8},
			{Name: "active", Type: ColumnTypeBool},
		},
	}); err != nil {
		t.Fatalf("CreateTable: %v", err)
	}

	for _, values := range []map[string]any{
		{"id": uint64(1), "name": "alice", "active": true},
		{"id": uint64(2), "name": "bob", "active": false},
		{"id": uint64(3), "name": "alex", "active": true},
	} {
		if _, err := InsertRow("users", values); err != nil {
			t.Fatalf("InsertRow: %v", err)
		}
	}

	selected, err := Select("users", Equal("active", true))
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if len(selected) != 2 {
		t.Fatalf("Select returned %d rows, want 2", len(selected))
	}
	if selected[0]["name"] != "alice" || selected[1]["name"] != "alex" {
		t.Fatalf("selected names = %q, %q; want alice, alex", selected[0]["name"], selected[1]["name"])
	}
}

func TestRowRefReadsReferencedRows(t *testing.T) {
	withTempWorkingDir(t)

	if _, err := CreateTable(Schema{
		Name: "users",
		Columns: []Column{
			{Name: "id", Type: ColumnTypeUint64},
			{Name: "name", Type: ColumnTypeString, Size: 8},
		},
	}); err != nil {
		t.Fatalf("CreateTable users: %v", err)
	}

	aliceID, err := InsertRow("users", map[string]any{"id": uint64(10), "name": "alice"})
	if err != nil {
		t.Fatalf("InsertRow alice: %v", err)
	}
	bobID, err := InsertRow("users", map[string]any{"id": uint64(11), "name": "bob"})
	if err != nil {
		t.Fatalf("InsertRow bob: %v", err)
	}

	if _, err := CreateTable(Schema{
		Name: "orders",
		Columns: []Column{
			{Name: "id", Type: ColumnTypeUint64},
			{Name: "user_id", Type: ColumnTypeRowRef, RefTable: "users", Indexed: true},
			{Name: "total", Type: ColumnTypeUint64},
		},
	}); err != nil {
		t.Fatalf("CreateTable orders: %v", err)
	}

	if _, err := InsertRow("orders", map[string]any{"id": uint64(100), "user_id": aliceID, "total": uint64(25)}); err != nil {
		t.Fatalf("InsertRow first order: %v", err)
	}
	if _, err := InsertRow("orders", map[string]any{"id": uint64(101), "user_id": bobID, "total": uint64(50)}); err != nil {
		t.Fatalf("InsertRow second order: %v", err)
	}

	referenced, err := ReadRowRef("orders", 1, "user_id")
	if err != nil {
		t.Fatalf("ReadRowRef: %v", err)
	}
	if referenced.Table != "users" || referenced.RowID != bobID {
		t.Fatalf("referenced row = %s/%d, want users/%d", referenced.Table, referenced.RowID, bobID)
	}
	if referenced.Values["name"] != "bob" {
		t.Fatalf("referenced name = %#v, want bob", referenced.Values["name"])
	}

	joined, err := JoinRowRef("orders", "user_id", nil)
	if err != nil {
		t.Fatalf("JoinRowRef all: %v", err)
	}
	if len(joined) != 2 {
		t.Fatalf("JoinRowRef returned %d rows, want 2", len(joined))
	}
	if joined[0].Referenced.Values["name"] != "alice" || joined[1].Referenced.Values["name"] != "bob" {
		t.Fatalf("joined names = %q, %q; want alice, bob", joined[0].Referenced.Values["name"], joined[1].Referenced.Values["name"])
	}

	predicate := Equal("user_id", bobID)
	filtered, err := JoinRowRef("orders", "user_id", &predicate)
	if err != nil {
		t.Fatalf("JoinRowRef filtered: %v", err)
	}
	if len(filtered) != 1 {
		t.Fatalf("JoinRowRef filtered returned %d rows, want 1", len(filtered))
	}
	if filtered[0].Values["id"] != uint64(101) || filtered[0].Referenced.Values["name"] != "bob" {
		t.Fatalf("filtered join = %+v", filtered[0])
	}
}

func TestRowRefValidation(t *testing.T) {
	withTempWorkingDir(t)

	if _, err := CreateTable(Schema{
		Name:    "users",
		Columns: []Column{{Name: "id", Type: ColumnTypeUint64}},
	}); err != nil {
		t.Fatalf("CreateTable users: %v", err)
	}
	if _, err := InsertRow("users", map[string]any{"id": uint64(1)}); err != nil {
		t.Fatalf("InsertRow user: %v", err)
	}

	if _, err := CreateTable(Schema{
		Name: "orders",
		Columns: []Column{
			{Name: "id", Type: ColumnTypeUint64},
			{Name: "user_id", Type: ColumnTypeRowRef},
			{Name: "plain_id", Type: ColumnTypeUint64},
			{Name: "missing_user_id", Type: ColumnTypeRowRef, RefTable: "users"},
		},
	}); err != nil {
		t.Fatalf("CreateTable orders: %v", err)
	}
	if _, err := InsertRow("orders", map[string]any{
		"id":              uint64(1),
		"user_id":         uint64(0),
		"plain_id":        uint64(0),
		"missing_user_id": uint64(9),
	}); err != nil {
		t.Fatalf("InsertRow order: %v", err)
	}

	if _, err := ReadRowRef("orders", 0, "missing"); !errors.Is(err, ErrInvalidSchema) {
		t.Fatalf("ReadRowRef missing column error = %v, want ErrInvalidSchema", err)
	}
	if _, err := ReadRowRef("orders", 0, "plain_id"); !errors.Is(err, ErrInvalidSchema) {
		t.Fatalf("ReadRowRef non row_ref error = %v, want ErrInvalidSchema", err)
	}
	if _, err := ReadRowRef("orders", 0, "user_id"); !errors.Is(err, ErrInvalidSchema) {
		t.Fatalf("ReadRowRef missing ref_table error = %v, want ErrInvalidSchema", err)
	}
	if _, err := ReadRowRef("orders", 0, "missing_user_id"); !errors.Is(err, ErrRowNotFound) {
		t.Fatalf("ReadRowRef missing target error = %v, want ErrRowNotFound", err)
	}
}

func TestReadRowValidation(t *testing.T) {
	withTempWorkingDir(t)

	if _, err := ReadRow("missing", 0); !errors.Is(err, ErrTableNotFound) {
		t.Fatalf("ReadRow missing table error = %v, want ErrTableNotFound", err)
	}

	if _, err := CreateTable(Schema{
		Name:    "users",
		Columns: []Column{{Name: "id", Type: ColumnTypeUint64}},
	}); err != nil {
		t.Fatalf("CreateTable: %v", err)
	}
	if _, err := InsertRow("users", map[string]any{"id": uint64(1)}); err != nil {
		t.Fatalf("InsertRow: %v", err)
	}
	if _, err := ReadRow("users", 1); !errors.Is(err, ErrRowNotFound) {
		t.Fatalf("ReadRow missing row error = %v, want ErrRowNotFound", err)
	}

	if err := os.WriteFile(filepath.Join("db", "rel", "users.rows"), []byte{1}, 0o644); err != nil {
		t.Fatalf("corrupt rows file: %v", err)
	}
	if _, err := ReadRow("users", 0); !errors.Is(err, ErrCorruptRows) {
		t.Fatalf("ReadRow corrupt rows error = %v, want ErrCorruptRows", err)
	}
}

func TestScanValidation(t *testing.T) {
	withTempWorkingDir(t)

	if _, err := Scan("missing", nil); !errors.Is(err, ErrTableNotFound) {
		t.Fatalf("Scan missing table error = %v, want ErrTableNotFound", err)
	}

	if _, err := CreateTable(Schema{
		Name:    "users",
		Columns: []Column{{Name: "id", Type: ColumnTypeUint64}},
	}); err != nil {
		t.Fatalf("CreateTable: %v", err)
	}
	if err := os.WriteFile(filepath.Join("db", "rel", "users.rows"), []byte{1}, 0o644); err != nil {
		t.Fatalf("corrupt rows file: %v", err)
	}
	if _, err := Scan("users", nil); !errors.Is(err, ErrCorruptRows) {
		t.Fatalf("Scan corrupt rows error = %v, want ErrCorruptRows", err)
	}
}

func TestSelectValidation(t *testing.T) {
	withTempWorkingDir(t)

	if _, err := Select("missing", Equal("id", uint64(1))); !errors.Is(err, ErrTableNotFound) {
		t.Fatalf("Select missing table error = %v, want ErrTableNotFound", err)
	}

	if _, err := CreateTable(Schema{
		Name:    "users",
		Columns: []Column{{Name: "id", Type: ColumnTypeUint64}},
	}); err != nil {
		t.Fatalf("CreateTable: %v", err)
	}
	if _, err := Select("users", Equal("missing", uint64(1))); !errors.Is(err, ErrInvalidSchema) {
		t.Fatalf("Select missing column error = %v, want ErrInvalidSchema", err)
	}
	if _, err := Select("users", Predicate{Column: "id", Op: "like", Value: uint64(1)}); !errors.Is(err, ErrInvalidPredicate) {
		t.Fatalf("Select unsupported op error = %v, want ErrInvalidPredicate", err)
	}
	if _, err := Select("users", Equal("id", "bad")); !errors.Is(err, ErrInvalidPredicate) {
		t.Fatalf("Select bad value error = %v, want ErrInvalidPredicate", err)
	}
}

func TestCreateIndexValidation(t *testing.T) {
	withTempWorkingDir(t)

	if err := CreateIndex("missing", "id"); !errors.Is(err, ErrTableNotFound) {
		t.Fatalf("CreateIndex missing table error = %v, want ErrTableNotFound", err)
	}

	if _, err := CreateTable(Schema{
		Name:    "users",
		Columns: []Column{{Name: "id", Type: ColumnTypeUint64}},
	}); err != nil {
		t.Fatalf("CreateTable: %v", err)
	}
	if err := CreateIndex("users", "missing"); !errors.Is(err, ErrInvalidSchema) {
		t.Fatalf("CreateIndex missing column error = %v, want ErrInvalidSchema", err)
	}
}

func TestInsertRowValidation(t *testing.T) {
	withTempWorkingDir(t)

	if _, err := InsertRow("missing", map[string]any{"id": uint64(1)}); !errors.Is(err, ErrTableNotFound) {
		t.Fatalf("InsertRow missing table error = %v, want ErrTableNotFound", err)
	}

	if _, err := CreateTable(Schema{
		Name: "users",
		Columns: []Column{
			{Name: "id", Type: ColumnTypeUint64},
			{Name: "name", Type: ColumnTypeString, Size: 4},
			{Name: "active", Type: ColumnTypeBool},
		},
	}); err != nil {
		t.Fatalf("CreateTable: %v", err)
	}

	tests := []struct {
		name   string
		values map[string]any
	}{
		{name: "unknown column", values: map[string]any{"id": uint64(1), "age": 10}},
		{name: "negative uint", values: map[string]any{"id": -1}},
		{name: "long string", values: map[string]any{"id": uint64(1), "name": "alice"}},
		{name: "bool type mismatch", values: map[string]any{"id": uint64(1), "active": 1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := InsertRow("users", tt.values); !errors.Is(err, ErrInvalidRow) {
				t.Fatalf("InsertRow error = %v, want ErrInvalidRow", err)
			}
		})
	}
}

func mustColumn(t *testing.T, schema *Schema, name string) Column {
	t.Helper()
	for _, column := range schema.Columns {
		if column.Name == name {
			return column
		}
	}
	t.Fatalf("column %q not found", name)
	return Column{}
}

func mustReadEqualityIndex(t *testing.T, table, column string) equalityIndex {
	t.Helper()
	raw, err := os.ReadFile(indexPath(table, column))
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	var index equalityIndex
	if err := json.Unmarshal(raw, &index); err != nil {
		t.Fatalf("unmarshal index: %v", err)
	}
	return index
}

func mustReadTrigramIndex(t *testing.T, table, column string) trigramIndex {
	t.Helper()
	raw, err := os.ReadFile(trigramIndexPath(table, column))
	if err != nil {
		t.Fatalf("read trigram index: %v", err)
	}
	var index trigramIndex
	if err := json.Unmarshal(raw, &index); err != nil {
		t.Fatalf("unmarshal trigram index: %v", err)
	}
	return index
}

func markRowInactive(t *testing.T, table string, rowSize uint64, rowID uint64) {
	t.Helper()
	rowsPath := filepath.Join("db", "rel", table+".rows")
	file, err := os.OpenFile(rowsPath, os.O_RDWR, 0o644)
	if err != nil {
		t.Fatalf("open rows: %v", err)
	}
	defer file.Close()

	if _, err := file.WriteAt([]byte{0}, int64(rowSize*rowID)); err != nil {
		t.Fatalf("mark row inactive: %v", err)
	}
}

func mustColumnRange(t *testing.T, row []byte, schema *Schema, name string) []byte {
	t.Helper()
	column := mustColumn(t, schema, name)
	start := schema.RowHeaderSize + column.Offset
	end := start + column.Size
	if end > uint64(len(row)) {
		t.Fatalf("column %q range %d:%d exceeds row length %d", name, start, end, len(row))
	}
	return row[start:end]
}

func mustUint64At(t *testing.T, row []byte, schema *Schema, name string, want uint64) {
	t.Helper()
	got := binary.LittleEndian.Uint64(mustColumnRange(t, row, schema, name))
	if got != want {
		t.Fatalf("%s = %d, want %d", name, got, want)
	}
}

func mustInt64At(t *testing.T, row []byte, schema *Schema, name string, want int64) {
	t.Helper()
	got := int64(binary.LittleEndian.Uint64(mustColumnRange(t, row, schema, name)))
	if got != want {
		t.Fatalf("%s = %d, want %d", name, got, want)
	}
}

func mustByteAt(t *testing.T, row []byte, schema *Schema, name string, want byte) {
	t.Helper()
	got := mustColumnRange(t, row, schema, name)[0]
	if got != want {
		t.Fatalf("%s = %d, want %d", name, got, want)
	}
}

func mustFloat64At(t *testing.T, row []byte, schema *Schema, name string, want float64) {
	t.Helper()
	got := math.Float64frombits(binary.LittleEndian.Uint64(mustColumnRange(t, row, schema, name)))
	if got != want {
		t.Fatalf("%s = %f, want %f", name, got, want)
	}
}

func mustStringAt(t *testing.T, row []byte, schema *Schema, name string, want string) {
	t.Helper()
	got := mustColumnRange(t, row, schema, name)
	if string(got[:len(want)]) != want {
		t.Fatalf("%s prefix = %q, want %q", name, string(got[:len(want)]), want)
	}
	for i, b := range got[len(want):] {
		if b != 0 {
			t.Fatalf("%s padding byte %d = %d, want 0", name, i, b)
		}
	}
}
