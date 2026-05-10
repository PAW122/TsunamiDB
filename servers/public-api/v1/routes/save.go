package routes

import (
	"fmt"
	"io"
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

type savedNestedEntry struct {
	PointerID string
	StartPtr  int
	EndPtr    int
}

func AsyncSave(w http.ResponseWriter, r *http.Request, c *http.Client) {
	var startPtr, endPtr int64
	var saveErr error

	defer debug.MeasureTime("> api [async save]")()

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	pathParts := ParseArgs(r.URL.Path, "save")
	if len(pathParts) < 4 {
		http.Error(w, "Invalid url args", http.StatusBadRequest)
		return
	}
	file := pathParts[2]
	key := pathParts[3]

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Invalid body", http.StatusBadRequest)
		return
	}

	unlock := valuepatch.LockKey(file, key)
	defer unlock()

	mode := strings.TrimSpace(r.Header.Get("mode"))
	nestedMode := strings.EqualFold(mode, "save_nested_json")

	var (
		nestedEntries []pendingNestedEntry
		savedNested   []savedNestedEntry
	)

	encodedBody := body
	encodedBytes, meta := encoder_v1.Encode(body, false)

	if nestedMode {
		normalizedBody, entries, err := processNestedPayload(body)
		if err != nil {
			http.Error(w, "Invalid nested payload", http.StatusBadRequest)
			return
		}
		encodedBody = normalizedBody
		nestedEntries = entries
		hasNested := len(entries) > 0
		encodedBytes, meta = encoder_v1.Encode(normalizedBody, hasNested)
	}

	if len(nestedEntries) > 0 {
		savedNested, err = persistNestedEntries(file, nestedEntries)
		if err != nil {
			fmt.Println("persist nested error:", err)
			http.Error(w, "Error saving nested data", http.StatusInternalServerError)
			return
		}
	}

	debug.MeasureBlock("save data & map [save_api]", func() {
		startPtr, endPtr, saveErr = dataManager_v2.SaveDataToFileAsync(encodedBytes, file)
	})
	if saveErr != nil {
		if len(savedNested) > 0 {
			rollbackNestedEntries(file, savedNested)
		}
		fmt.Println(saveErr)
		http.Error(w, "Error saving to file", http.StatusInternalServerError)
		return
	}

	prevMeta, existed, err := fileSystem_v1.SaveElementByKey(file, key, int(startPtr), int(endPtr), meta.HasNested)
	if err != nil {
		if len(savedNested) > 0 {
			rollbackNestedEntries(file, savedNested)
		}
		_ = defragmentationManager.MarkAsFree(key, file, startPtr, endPtr)
		fmt.Println(err)
		http.Error(w, "Error saving metadata", http.StatusInternalServerError)
		return
	}

	if existed {
		if prevMeta.FileName != file || prevMeta.StartPtr != int(startPtr) || prevMeta.EndPtr != int(endPtr) {
			_ = defragmentationManager.MarkAsFree(
				prevMeta.Key, prevMeta.FileName,
				int64(prevMeta.StartPtr), int64(prevMeta.EndPtr),
			)
			fileSystem_v1.RecordDefragFree()
		} else {
			fileSystem_v1.RecordDefragSkip()
		}
		if prevMeta.HasNested {
			cleanupRecordNested(file, prevMeta)
		}
	}

	networkmanager.NotifyKVTable(file)
	revState, hasRevision, err := revision.AdvanceFullWrite(file, key)
	if err != nil {
		http.Error(w, "Error saving revision metadata", http.StatusInternalServerError)
		return
	}
	if hasRevision {
		go subServer.NotifySubscribersWithRevision(key, encodedBody, revState.Rev)
	} else {
		go subServer.NotifySubscribers(key, encodedBody)
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("save"))
}

func persistNestedEntries(table string, entries []pendingNestedEntry) ([]savedNestedEntry, error) {
	nestedFile := nestedDataFile(table)
	saved := make([]savedNestedEntry, 0, len(entries))

	for _, entry := range entries {
		encoded, meta := encoder_v1.Encode(entry.JSONValue, entry.HasNested)
		startPtr, endPtr, err := dataManager_v2.SaveDataToFileAsync(encoded, nestedFile)
		if err != nil {
			rollbackNestedEntries(table, saved)
			return nil, err
		}

		prevMeta, existed, err := fileSystem_v1.SaveElementByKey(nestedFile, entry.PointerID, int(startPtr), int(endPtr), meta.HasNested)
		if err != nil {
			_ = defragmentationManager.MarkAsFree(entry.PointerID, nestedFile, startPtr, endPtr)
			rollbackNestedEntries(table, saved)
			return nil, err
		}
		if existed {
			if prevMeta.FileName != nestedFile || prevMeta.StartPtr != int(startPtr) || prevMeta.EndPtr != int(endPtr) {
				_ = defragmentationManager.MarkAsFree(prevMeta.Key, prevMeta.FileName, int64(prevMeta.StartPtr), int64(prevMeta.EndPtr))
				fileSystem_v1.RecordDefragFree()
			} else {
				fileSystem_v1.RecordDefragSkip()
			}
		}

		saved = append(saved, savedNestedEntry{
			PointerID: entry.PointerID,
			StartPtr:  int(startPtr),
			EndPtr:    int(endPtr),
		})
	}

	return saved, nil
}

func rollbackNestedEntries(table string, entries []savedNestedEntry) {
	if len(entries) == 0 {
		return
	}
	nestedFile := nestedDataFile(table)
	for _, entry := range entries {
		_ = fileSystem_v1.RemoveElementByKey(nestedFile, entry.PointerID)
		_ = defragmentationManager.MarkAsFree(entry.PointerID, nestedFile, int64(entry.StartPtr), int64(entry.EndPtr))
	}
}
