package routes

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/PAW122/TsunamiDB/data/relational"
)

type relationalInsertRequest struct {
	Values map[string]any `json:"values"`
}

type relationalUpdateRequest struct {
	Values map[string]any `json:"values"`
}

type relationalSQLRequest struct {
	Query string `json:"query"`
}

type relationalStatusResponse struct {
	Status string `json:"status"`
}

type relationalRowIDResponse struct {
	RowID uint64 `json:"row_id"`
}

func RelationalSchema(w http.ResponseWriter, r *http.Request, _ *http.Client) {
	relCORS(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	table, ok := relSchemaTable(r.URL.Path)
	if !ok {
		http.Error(w, "invalid URL args", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		schema, err := relational.LoadSchema(table)
		if err != nil {
			relError(w, err)
			return
		}
		writeRelJSON(w, http.StatusOK, schema)
	case http.MethodPost:
		var schema relational.Schema
		if err := decodeRelJSON(r, &schema); err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}
		if schema.Name == "" {
			schema.Name = table
		} else if schema.Name != table {
			http.Error(w, "schema name does not match URL table", http.StatusBadRequest)
			return
		}

		created, err := relational.CreateTable(schema)
		if err != nil {
			relError(w, err)
			return
		}
		writeRelJSON(w, http.StatusCreated, created)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func Relational(w http.ResponseWriter, r *http.Request, _ *http.Client) {
	relCORS(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	parts := relPathParts(r.URL.Path)
	if len(parts) < 2 {
		http.Error(w, "invalid URL args", http.StatusBadRequest)
		return
	}

	table := parts[0]
	action := parts[1]
	switch action {
	case "insert":
		relInsert(w, r, table)
	case "row":
		relRow(w, r, table, parts)
	case "select":
		relSelect(w, r, table)
	case "index":
		relIndex(w, r, table, parts, false)
	case "trigram-index":
		relIndex(w, r, table, parts, true)
	case "compact":
		relCompact(w, r, table)
	default:
		http.Error(w, "unknown relational endpoint", http.StatusNotFound)
	}
}

func RelationalSQL(w http.ResponseWriter, r *http.Request, _ *http.Client) {
	relCORS(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil || len(strings.TrimSpace(string(body))) == 0 {
		http.Error(w, "invalid SQL body", http.StatusBadRequest)
		return
	}

	query := string(body)
	var req relationalSQLRequest
	if err := json.Unmarshal(body, &req); err == nil && strings.TrimSpace(req.Query) != "" {
		query = req.Query
	}

	result, err := relational.ExecuteSQL(query)
	if err != nil {
		relError(w, err)
		return
	}
	writeRelJSON(w, http.StatusOK, result)
}

func relInsert(w http.ResponseWriter, r *http.Request, table string) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req relationalInsertRequest
	if err := decodeRelJSON(r, &req); err != nil || req.Values == nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	rowID, err := relational.InsertRow(table, req.Values)
	if err != nil {
		relError(w, err)
		return
	}
	writeRelJSON(w, http.StatusCreated, relationalRowIDResponse{RowID: rowID})
}

func relRow(w http.ResponseWriter, r *http.Request, table string, parts []string) {
	if len(parts) != 3 {
		http.Error(w, "invalid URL args", http.StatusBadRequest)
		return
	}

	rowID, err := strconv.ParseUint(parts[2], 10, 64)
	if err != nil {
		http.Error(w, "invalid row ID", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		row, err := relational.ReadRow(table, rowID)
		if err != nil {
			relError(w, err)
			return
		}
		writeRelJSON(w, http.StatusOK, row)
	case http.MethodPatch, http.MethodPost:
		var req relationalUpdateRequest
		if err := decodeRelJSON(r, &req); err != nil || req.Values == nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}
		if err := relational.UpdateRow(table, rowID, req.Values); err != nil {
			relError(w, err)
			return
		}
		writeRelJSON(w, http.StatusOK, relationalStatusResponse{Status: "ok"})
	case http.MethodDelete:
		if err := relational.DeleteRow(table, rowID); err != nil {
			relError(w, err)
			return
		}
		writeRelJSON(w, http.StatusOK, relationalStatusResponse{Status: "ok"})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func relSelect(w http.ResponseWriter, r *http.Request, table string) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var predicate relational.Predicate
	if err := decodeRelJSON(r, &predicate); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	rows, err := relational.SelectRows(table, predicate)
	if err != nil {
		relError(w, err)
		return
	}
	writeRelJSON(w, http.StatusOK, rows)
}

func relIndex(w http.ResponseWriter, r *http.Request, table string, parts []string, trigram bool) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if len(parts) != 3 {
		http.Error(w, "invalid URL args", http.StatusBadRequest)
		return
	}

	var err error
	if trigram {
		err = relational.CreateTrigramIndex(table, parts[2])
	} else {
		err = relational.CreateIndex(table, parts[2])
	}
	if err != nil {
		relError(w, err)
		return
	}
	writeRelJSON(w, http.StatusOK, relationalStatusResponse{Status: "ok"})
}

func relCompact(w http.ResponseWriter, r *http.Request, table string) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	result, err := relational.CompactTable(table)
	if err != nil {
		relError(w, err)
		return
	}
	writeRelJSON(w, http.StatusOK, result)
}

func relSchemaTable(path string) (string, bool) {
	parts := strings.Split(strings.Trim(strings.TrimPrefix(path, "/rel/schema/"), "/"), "/")
	if len(parts) != 1 || parts[0] == "" {
		return "", false
	}
	return parts[0], true
}

func relPathParts(path string) []string {
	trimmed := strings.Trim(strings.TrimPrefix(path, "/rel/"), "/")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "/")
}

func decodeRelJSON(r *http.Request, dst any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.UseNumber()
	return decoder.Decode(dst)
}

func writeRelJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func relCORS(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
}

func relError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, relational.ErrTableNotFound), errors.Is(err, relational.ErrRowNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, relational.ErrTableExists):
		http.Error(w, err.Error(), http.StatusConflict)
	case errors.Is(err, relational.ErrInvalidSchema), errors.Is(err, relational.ErrInvalidRow), errors.Is(err, relational.ErrInvalidPredicate), errors.Is(err, relational.ErrInvalidSQL):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, relational.ErrCorruptRows):
		http.Error(w, err.Error(), http.StatusInternalServerError)
	default:
		http.Error(w, fmt.Sprintf("relational error: %v", err), http.StatusInternalServerError)
	}
}
