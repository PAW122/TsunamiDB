package routes

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"

	dataManager_v2 "github.com/PAW122/TsunamiDB/data/dataManager/v2"
	fileSystem_v1 "github.com/PAW122/TsunamiDB/data/fileSystem/v1"
	"github.com/PAW122/TsunamiDB/data/revision"
	"github.com/PAW122/TsunamiDB/data/valuepatch"
	encoder_v1 "github.com/PAW122/TsunamiDB/encoding/v1"
	debug "github.com/PAW122/TsunamiDB/servers/debug"
)

type readWithRevisionResponse struct {
	Table          string        `json:"table"`
	Key            string        `json:"key"`
	DataBase64     string        `json:"data_b64"`
	Size           int           `json:"size"`
	RevisionMode   revision.Mode `json:"revision_mode"`
	Rev            uint64        `json:"rev"`
	HistoryFromRev uint64        `json:"history_from_rev,omitempty"`
}

func ReadWithRevision(w http.ResponseWriter, r *http.Request, _ *http.Client) {
	defer debug.MeasureTime("> api [read_with_revision]")()

	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	pathParts := ParseArgs(r.URL.Path, "read_with_revision")
	if len(pathParts) < 4 {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "Invalid URL args")
		return
	}

	table := pathParts[2]
	key := pathParts[3]

	unlock := valuepatch.LockKey(table, key)
	defer unlock()

	data, state, err := readValueAndRevision(table, key)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, "Error reading from file: ", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(readWithRevisionResponse{
		Table:          table,
		Key:            key,
		DataBase64:     base64.StdEncoding.EncodeToString(data),
		Size:           len(data),
		RevisionMode:   state.Mode,
		Rev:            state.Rev,
		HistoryFromRev: state.HistoryFromRev,
	})
}

func readValueAndRevision(table, key string) ([]byte, revision.State, error) {
	fsData, err := fileSystem_v1.GetElementByKey(table, key)
	if err != nil {
		return nil, revision.State{}, err
	}

	data, err := dataManager_v2.ReadDataFromFileAsync(table, int64(fsData.StartPtr), int64(fsData.EndPtr))
	if err != nil {
		return nil, revision.State{}, err
	}

	state, err := revision.GetState(table, key)
	if err != nil {
		return nil, revision.State{}, err
	}

	decoded := encoder_v1.Decode(data)
	return []byte(decoded.Data), state, nil
}
