package routes

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/PAW122/TsunamiDB/data/revision"
	"github.com/PAW122/TsunamiDB/data/valuepatch"
	debug "github.com/PAW122/TsunamiDB/servers/debug"
)

type revisionPolicyRequest struct {
	Mode revision.Mode `json:"mode"`
}

type revisionHistoryResponse struct {
	Key          string                 `json:"key"`
	Mode         revision.Mode          `json:"mode"`
	CurrentRev   uint64                 `json:"current_rev"`
	FromRev      uint64                 `json:"from_rev"`
	HistoryStart uint64                 `json:"history_from_rev"`
	Patches      []revision.PatchRecord `json:"patches"`
}

func Revision(w http.ResponseWriter, r *http.Request, _ *http.Client) {
	defer debug.MeasureTime("> api [revision]")()

	pathParts := ParseArgs(r.URL.Path, "revision")
	if len(pathParts) < 4 {
		http.Error(w, "Invalid url args", http.StatusBadRequest)
		return
	}
	table := pathParts[2]
	key := pathParts[3]

	if len(pathParts) >= 5 && pathParts[4] == "patches" {
		handleRevisionHistory(w, r, table, key)
		return
	}

	switch r.Method {
	case http.MethodGet:
		state, err := revision.GetState(table, key)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeRevisionJSON(w, state)
	case http.MethodPost:
		var req revisionPolicyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid revision policy body", http.StatusBadRequest)
			return
		}

		unlock := valuepatch.LockKey(table, key)
		defer unlock()

		state, err := revision.SetPolicy(table, key, req.Mode)
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, revision.ErrInvalidMode) {
				status = http.StatusBadRequest
			}
			http.Error(w, err.Error(), status)
			return
		}
		writeRevisionJSON(w, state)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func handleRevisionHistory(w http.ResponseWriter, r *http.Request, table, key string) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	fromRev, err := parseUintQuery(r, "from_rev", 0)
	if err != nil {
		http.Error(w, "invalid from_rev", http.StatusBadRequest)
		return
	}
	toRev, err := parseUintQuery(r, "to_rev", 0)
	if err != nil {
		http.Error(w, "invalid to_rev", http.StatusBadRequest)
		return
	}

	records, state, err := revision.History(table, key, fromRev, toRev)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, revision.ErrHistoryUnavailable) {
			status = http.StatusConflict
		}
		http.Error(w, err.Error(), status)
		return
	}

	writeRevisionJSON(w, revisionHistoryResponse{
		Key:          key,
		Mode:         state.Mode,
		CurrentRev:   state.Rev,
		FromRev:      fromRev,
		HistoryStart: state.HistoryFromRev,
		Patches:      records,
	})
}

func parseUintQuery(r *http.Request, name string, fallback uint64) (uint64, error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return fallback, nil
	}
	return strconv.ParseUint(raw, 10, 64)
}

func writeRevisionJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}
