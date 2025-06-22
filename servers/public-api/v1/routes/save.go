package routes

import (
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
	debug.MeasureBlock("CheckExistingKey [save_api]", func() {
		if meta, err := fileSystem_v1.GetElementByKey(key); err == nil {
			defragmentationManager.MarkAsFree(
				meta.Key, meta.FileName,
				int64(meta.StartPtr), int64(meta.EndPtr),
			)
		}
	})

	// —3— kodowanie (funkcja NIE zwraca error)
	encoded, _ := encoder_v1.Encode(body)

	debug.MeasureBlock("save data & map [save_api]", func() {
		// —4— zapis do pliku
		startPtr, endPtr, err := dataManager_v2.SaveDataToFileAsync(encoded, file)
		if err != nil {
			http.Error(w, "Error saving to file", http.StatusInternalServerError)
			return
		}

		// —5— zapis metadanych
		if err := fileSystem_v1.SaveElementByKey(key, file, int(startPtr), int(endPtr)); err != nil {
			http.Error(w, "Error saving metadata", http.StatusInternalServerError)
			return
		}
	})

	go subServer.NotifySubscribers(key, body)

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("save"))
}
