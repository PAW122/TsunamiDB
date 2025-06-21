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

	if r.Method != "GET" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	// Pobierz regex z parametru query "regex"
	regex := r.URL.Query().Get("regex")
	if regex == "" {
		http.Error(w, "Missing 'regex' parameter", http.StatusBadRequest)
		return
	}

	// Pobierz max z parametru query "max", domyślnie 0 (wszystkie)
	maxStr := r.URL.Query().Get("max")
	max := 0 // Domyślnie: pobierz wszystkie

	if maxStr != "" {
		var err error
		max, err = strconv.Atoi(maxStr)
		if err != nil {
			http.Error(w, "Invalid 'max' parameter", http.StatusBadRequest)
			return
		}
	}

	// Wywołaj funkcję GetKeysByRegex z fileSystem_v1
	keys, err := fileSystem_v1.GetKeysByRegex(regex, max)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error from GetKeysByRegex: %v", err), http.StatusInternalServerError)
		return
	}

	// Ustaw Content-Type na application/json
	w.Header().Set("Content-Type", "application/json")

	// Zwróć wynik jako JSON
	json.NewEncoder(w).Encode(keys)
	keys = nil
}
