package routes

import (
	"fmt"
	"io"
	"net/http"

	dataManager_v2 "github.com/PAW122/TsunamiDB/data/dataManager/v2"
	"github.com/PAW122/TsunamiDB/data/defragmentationManager"
	fileSystem_v1 "github.com/PAW122/TsunamiDB/data/fileSystem/v1"
	encoder_v1 "github.com/PAW122/TsunamiDB/encoding/v1"
	debug "github.com/PAW122/TsunamiDB/servers/debug"
	subServer "github.com/PAW122/TsunamiDB/servers/subscriptions"
)

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

	// —1— szybki odczyt body (bufor 1 MiB, można zwiększyć)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Invalid body", http.StatusBadRequest)
		return
	}

	// —2— zwolnij poprzednie dane (jeśli istnieją)
	// -2- kodowanie (funkcja NIE zwraca error)
	encoded, _ := encoder_v1.Encode(body)

	debug.MeasureBlock("save data & map [save_api]", func() {
		startPtr, endPtr, saveErr = dataManager_v2.SaveDataToFileAsync(encoded, file)
	})
	if saveErr != nil {
		fmt.Println(saveErr)
		http.Error(w, "Error saving to file", http.StatusInternalServerError)
		return
	}

	prevMeta, existed, err := fileSystem_v1.SaveElementByKey(file, key, int(startPtr), int(endPtr))
	if err != nil {
		fmt.Println(err)
		http.Error(w, "Error saving metadata", http.StatusInternalServerError)
		return
	}
	if existed {
		if prevMeta.FileName != file || prevMeta.StartPtr != int(startPtr) || prevMeta.EndPtr != int(endPtr) {
			defragmentationManager.MarkAsFree(
				prevMeta.Key, prevMeta.FileName,
				int64(prevMeta.StartPtr), int64(prevMeta.EndPtr),
			)
			fileSystem_v1.RecordDefragFree()
		} else {
			fileSystem_v1.RecordDefragSkip()
		}
	}

	go subServer.NotifySubscribers(key, body)

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("save"))
}
