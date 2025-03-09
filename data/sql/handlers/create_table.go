package sql_handlers

import (
	"TsunamiDB/types"
	"encoding/json"
	"fmt"
	"os"
)

var Sql_table_map string = "./db/sql_map"

// TODO: create dir & file

// CreateTable zapisuje definicję tabeli jako JSON do pliku
func CreateTable(query types.SQL_req) error {
	tableName := query.TableName
	columns := query.Columns

	query.Query = ""

	// Walidacja nazwy tabeli
	if len(tableName) < 1 {
		return fmt.Errorf("invalid table name")
	}

	// Walidacja kolumn
	if len(columns) < 1 {
		return fmt.Errorf("columns cannot be empty")
	}

	// Tworzenie folderu na pliki tabeli (jeśli nie istnieje)
	err := os.MkdirAll(Sql_table_map, os.ModePerm)
	if err != nil {
		return fmt.Errorf("failed to create directory: %v", err)
	}

	// Ścieżka do pliku JSON tabeli
	filePath := fmt.Sprintf("%s/%s.json", Sql_table_map, tableName)

	// Serializacja do JSON
	tableData, err := json.MarshalIndent(query, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize table definition to JSON: %v", err)
	}

	// Zapis do pliku JSON
	err = os.WriteFile(filePath, tableData, 0644)
	if err != nil {
		return fmt.Errorf("failed to write table definition to file: %v", err)
	}

	fmt.Printf("Table %s created successfully and saved to %s\n", tableName, filePath)
	return nil
}
