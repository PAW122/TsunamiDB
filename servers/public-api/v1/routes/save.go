package routes

import (
	"fmt"
	"io"
	"net/http"

	dataManager_v1 "TsunamiDB/data/dataManager/v1"
	"TsunamiDB/data/defragmentationManager"
	fileSystem_v1 "TsunamiDB/data/fileSystem/v1"
	encoder_v1 "TsunamiDB/encoding/v1"
	debug "TsunamiDB/servers/debug"
)

func Save(w http.ResponseWriter, r *http.Request) {
	defer debug.MeasureTime("> api [save]")()
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

	/*
		this call dont allow to simuntoniusly exist 2 identical keys values.
		eaven if keys are in difrent tables / files

		if key is in file2 and save is executed to file1 with identical key
		key in file2 will by free'd / deleted
	*/
	// free previous data for same key value if exist
	prevMetaData, err := fileSystem_v1.GetElementByKey(key)
	if err == nil {
		defragmentationManager.MarkAsFree(prevMetaData.Key, prevMetaData.FileName, int64(prevMetaData.StartPtr), int64(prevMetaData.EndPtr))
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "Invalid body")
		return
	}

	encoded, _ := encoder_v1.Encode(body)
	// save to file
	startPtr, endPtr, err := dataManager_v1.SaveDataToFile(encoded, file)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "Error saving to file:", err)
		return
	}

	// save to map
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
