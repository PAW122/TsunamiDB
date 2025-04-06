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
)

func AsyncSave(w http.ResponseWriter, r *http.Request) {
	defer debug.MeasureTime("> api [async save]")()

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

	// Odczyt ciała żądania
	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "Invalid body")
		return
	}

	// Kanał do przekazania wyniku zapisu
	saveChan := make(chan error, 1)

	saveWG.Add(1)
	go func() {
		defer saveWG.Done()
		// Usunięcie poprzednich danych
		prevMetaData, err := fileSystem_v1.GetElementByKey(key)
		if err == nil {
			debug.LogExtra("Freeing previous data")
			debug.LogExtra(prevMetaData.Key, prevMetaData.FileName, int64(prevMetaData.StartPtr), int64(prevMetaData.EndPtr))
			defragmentationManager.MarkAsFree(prevMetaData.Key, prevMetaData.FileName, int64(prevMetaData.StartPtr), int64(prevMetaData.EndPtr))
		}

		// Kodowanie i zapis asynchroniczny
		encoded, _ := encoder_v1.Encode(body)
		startPtr, endPtr, err := dataManager_v2.SaveDataToFileAsync(encoded, file)
		if err != nil {
			saveChan <- err
			return
		}

		debug.LogExtra(startPtr, endPtr)
		debug.LogExtra(int(startPtr), int(endPtr))

		// Mapowanie klucza
		err = fileSystem_v1.SaveElementByKey(key, file, int(startPtr), int(endPtr))
		saveChan <- err
	}()

	saveWG.Wait()
	close(saveChan)

	if err := <-saveChan; err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "Error saving to file: ", err)
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "save")
}
