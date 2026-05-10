package routes

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	dataManager_v2 "github.com/PAW122/TsunamiDB/data/dataManager/v2"
	"github.com/PAW122/TsunamiDB/data/defragmentationManager"
	fileSystem_v1 "github.com/PAW122/TsunamiDB/data/fileSystem/v1"
	"github.com/PAW122/TsunamiDB/data/revision"
	"github.com/PAW122/TsunamiDB/data/valuepatch"
	encoder_v1 "github.com/PAW122/TsunamiDB/encoding/v1"
	debug "github.com/PAW122/TsunamiDB/servers/debug"
	networkmanager "github.com/PAW122/TsunamiDB/servers/network-manager"
	subServer "github.com/PAW122/TsunamiDB/servers/subscriptions"
)

type patchRequest struct {
	BaseRev *uint64                `json:"base_rev,omitempty"`
	Ops     []valuepatch.Operation `json:"ops"`
}

type patchResponse struct {
	Status       string        `json:"status"`
	Key          string        `json:"key"`
	Size         int           `json:"size"`
	RevisionMode revision.Mode `json:"revision_mode,omitempty"`
	BaseRev      *uint64       `json:"base_rev,omitempty"`
	Rev          *uint64       `json:"rev,omitempty"`
}

func PatchValue(w http.ResponseWriter, r *http.Request, _ *http.Client) {
	defer debug.MeasureTime("> api [patch]")()

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	pathParts := ParseArgs(r.URL.Path, "patch")
	if len(pathParts) < 4 {
		http.Error(w, "Invalid url args", http.StatusBadRequest)
		return
	}
	file := pathParts[2]
	key := pathParts[3]

	var req patchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid patch body", http.StatusBadRequest)
		return
	}

	unlock := valuepatch.LockKey(file, key)
	defer unlock()

	updated, revState, record, hasRevision, err := patchStoredValue(file, key, req.BaseRev, req.Ops)
	if err != nil {
		writePatchError(w, err)
		return
	}

	networkmanager.NotifyKVTable(file)
	if hasRevision && record != nil {
		go subServer.NotifyTablePatchSubscribersWithRevision(file, key, req.Ops, record.BaseRev, record.Rev)
	} else {
		go subServer.NotifyTablePatchSubscribers(file, key, req.Ops)
	}

	resp := patchResponse{
		Status: "patched",
		Key:    key,
		Size:   len(updated),
	}
	if hasRevision && record != nil {
		resp.RevisionMode = revState.Mode
		resp.BaseRev = &record.BaseRev
		resp.Rev = &record.Rev
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func patchStoredValue(file, key string, baseRev *uint64, ops []valuepatch.Operation) ([]byte, revision.State, *revision.PatchRecord, bool, error) {
	_, hasRevision, err := revision.CheckPatch(file, key, baseRev)
	if err != nil {
		return nil, revision.State{}, nil, hasRevision, err
	}

	fsData, err := fileSystem_v1.GetElementByKey(file, key)
	if err != nil {
		return nil, revision.State{}, nil, hasRevision, fmt.Errorf("patch read metadata: %w", err)
	}

	data, err := dataManager_v2.ReadDataFromFileAsync(file, int64(fsData.StartPtr), int64(fsData.EndPtr))
	if err != nil {
		return nil, revision.State{}, nil, hasRevision, fmt.Errorf("patch read data: %w", err)
	}

	decoded := encoder_v1.Decode(data)
	updated, err := valuepatch.Apply([]byte(decoded.Data), ops)
	if err != nil {
		return nil, revision.State{}, nil, hasRevision, err
	}

	encoded, meta := encoder_v1.Encode(updated, decoded.HasNested)
	startPtr, endPtr, err := dataManager_v2.SaveDataToFileAsync(encoded, file)
	if err != nil {
		return nil, revision.State{}, nil, hasRevision, fmt.Errorf("patch save data: %w", err)
	}

	prevMeta, existed, err := fileSystem_v1.SaveElementByKey(file, key, int(startPtr), int(endPtr), meta.HasNested)
	if err != nil {
		_ = defragmentationManager.MarkAsFree(key, file, startPtr, endPtr)
		return nil, revision.State{}, nil, hasRevision, fmt.Errorf("patch save metadata: %w", err)
	}
	if existed {
		if prevMeta.FileName != file || prevMeta.StartPtr != int(startPtr) || prevMeta.EndPtr != int(endPtr) {
			_ = defragmentationManager.MarkAsFree(prevMeta.Key, prevMeta.FileName, int64(prevMeta.StartPtr), int64(prevMeta.EndPtr))
			fileSystem_v1.RecordDefragFree()
		} else {
			fileSystem_v1.RecordDefragSkip()
		}
	}

	revState, record, hasRevision, err := revision.AdvancePatch(file, key, baseRev, ops)
	if err != nil {
		return nil, revision.State{}, nil, hasRevision, err
	}

	return updated, revState, record, hasRevision, nil
}

func writePatchError(w http.ResponseWriter, err error) {
	if conflict := (*revision.ConflictError)(nil); errors.As(err, &conflict) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error":       "revision_conflict",
			"base_rev":    conflict.BaseRev,
			"current_rev": conflict.CurrentRev,
		})
		return
	}
	if errors.Is(err, revision.ErrMissingBaseRev) {
		http.Error(w, "missing base_rev", http.StatusBadRequest)
		return
	}
	http.Error(w, err.Error(), patchHTTPStatus(err))
}

func patchHTTPStatus(err error) int {
	switch {
	case errors.Is(err, valuepatch.ErrNoOps),
		errors.Is(err, valuepatch.ErrInvalidOffset),
		errors.Is(err, valuepatch.ErrInvalidDelete),
		errors.Is(err, valuepatch.ErrInvalidInsert):
		return http.StatusBadRequest
	case errors.Is(err, revision.ErrMissingBaseRev):
		return http.StatusBadRequest
	case errors.Is(err, revision.ErrRevisionConflict):
		return http.StatusConflict
	case strings.Contains(err.Error(), "patch read metadata"):
		return http.StatusNotFound
	default:
		return http.StatusInternalServerError
	}
}
