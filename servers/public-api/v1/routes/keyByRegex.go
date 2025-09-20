package routes

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	fileSystem_v1 "github.com/PAW122/TsunamiDB/data/fileSystem/v1"
	debug "github.com/PAW122/TsunamiDB/servers/debug"
)

func GetKeysByRegex(w http.ResponseWriter, r *http.Request, c *http.Client) {
	defer debug.MeasureTime("> api [async key by regex]")()

	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	pathParts := ParseArgs(r.URL.Path, "key_by_regex")
	if len(pathParts) < 3 || pathParts[2] == "" {
		http.Error(w, "Missing table segment", http.StatusBadRequest)
		return
	}
	table := pathParts[2]

	regex := r.URL.Query().Get("regex")
	if regex == "" {
		http.Error(w, "Missing 'regex' parameter", http.StatusBadRequest)
		return
	}

	maxStr := r.URL.Query().Get("max")
	max := 0
	if maxStr != "" {
		var err error
		max, err = strconv.Atoi(maxStr)
		if err != nil {
			http.Error(w, "Invalid 'max' parameter", http.StatusBadRequest)
			return
		}
	}

	keys, err := fileSystem_v1.GetKeysByRegex(table, regex, max)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error from GetKeysByRegex: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(keys)
}
