package routes

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	sql "TsunamiDB/data/sql"
	debug "TsunamiDB/servers/debug"
	"TsunamiDB/types"
)

func SQL_api(w http.ResponseWriter, r *http.Request) {
	defer debug.MeasureTime("> api [async save]")()

	if r.Method != "POST" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Odczyt ciała żądania
	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "Invalid body")
		return
	}

	// parse body
	var request types.SQL_req

	// Parsowanie JSON
	err = json.Unmarshal([]byte(body), &request)
	if err != nil {
		fmt.Println("Błąd parsowania JSON: %v", err)
		return
	}

	res := sql.Execute_Sql(request)
	if res.Error != nil {
		w.Write([]byte(res.Error.Error()))
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "save")
}
