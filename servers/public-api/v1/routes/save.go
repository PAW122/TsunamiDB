package routes

import (
	"fmt"
	"io"
	"net/http"

	dataManager_v1 "TsunamiDB/data/dataManager/v1"
	fileSystem_v1 "TsunamiDB/data/fileSystem/v1"
	encoder_v1 "TsunamiDB/encoding/v1"
)

func Save(w http.ResponseWriter, r *http.Request) {

	// /save/<file>/<key>
	// body = []bytes r.Body

	if r.Method != "POST" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	pathParts := ParseArgs(r.URL.Path, "save")
	if pathParts == nil || len(pathParts) < 2 {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Print(w, "Invalid url args")
		return
	}

	file := pathParts[2]
	key := pathParts[3]

	// fmt.Println(pathParts)
	// fmt.Println(file)

	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "Invalid body")
		return
	}

	encoded, _ := encoder_v1.Encode(body)
	startPtr, endPtr, err := dataManager_v1.SaveDataToFile(encoded, file)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "Error saving to file:", err)
		return
	}
	err = fileSystem_v1.SaveElementByKey(key, file, int(startPtr), int(endPtr))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "Error saving to map:", err)
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "save")
	return
}
