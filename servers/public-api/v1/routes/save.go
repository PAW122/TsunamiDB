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
	logger "github.com/PAW122/TsunamiDB/servers/logger"
)

func AsyncSave(w http.ResponseWriter, r *http.Request) {
	defer debug.MeasureTime("> api [async save]")()
	defer logger.MeasureTime("[Api] [AsyncSave]")()

	if r.Method != "POST" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	pathParts := ParseArgs(r.URL.Path, "save")
	if pathParts == nil || len(pathParts) < 2 {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "Invalid url args")
		return
	}

	file := pathParts[2]
	key := pathParts[3]

	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "Invalid body")
		return
	}

	// Kanał do przekazania ewentualnego błędu zapisu
	saveChan := make(chan error, 1)

	// Uruchamiamy goroutine, by zachować asynchroniczność wewnątrz zapisu
	go func() {
		// Usuwamy poprzednie dane, jeśli istnieją
		prevMetaData, err := fileSystem_v1.GetElementByKey(key)
		if err == nil {
			debug.LogExtra("Freeing previous data")
			debug.LogExtra(prevMetaData.Key, prevMetaData.FileName,
				int64(prevMetaData.StartPtr), int64(prevMetaData.EndPtr))
			defragmentationManager.MarkAsFree(
				prevMetaData.Key,
				prevMetaData.FileName,
				int64(prevMetaData.StartPtr),
				int64(prevMetaData.EndPtr),
			)
		}

		// Kodowanie i asynchroniczny zapis
		encoded, _ := encoder_v1.Encode(body)
		startPtr, endPtr, err := dataManager_v2.SaveDataToFileAsync(encoded, file)
		if err != nil {
			saveChan <- err
			return
		}

		debug.LogExtra("startPtr, endPtr:", startPtr, endPtr)

		// Zapamiętanie w mapowaniu kluczy
		err = fileSystem_v1.SaveElementByKey(key, file, int(startPtr), int(endPtr))
		saveChan <- err
	}()

	// Czekamy na wynik z kanału
	if err := <-saveChan; err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "Error saving to file: ", err)
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "save")
}
