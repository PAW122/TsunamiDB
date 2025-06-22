package routes

import (
	"fmt"
	"io"
	"net/http"

	dataManager_v1 "github.com/PAW122/TsunamiDB/data/dataManager/v1"
	"github.com/PAW122/TsunamiDB/data/defragmentationManager"
	fileSystem_v1 "github.com/PAW122/TsunamiDB/data/fileSystem/v1"
	encoder_v1 "github.com/PAW122/TsunamiDB/encoding/v1"
	debug "github.com/PAW122/TsunamiDB/servers/debug"
	subServer "github.com/PAW122/TsunamiDB/servers/subscriptions"
)

/*
	/save_ecnrypted
	1. wymaga elementu encruption_key w headers
	2. dodac w encoder funkcje encoded_and_encrypt


	3. dodac read_decrypt
*/

func SaveEncrypted(w http.ResponseWriter, r *http.Request, c *http.Client) {
	defer debug.MeasureTime("> api [SaveEncrypted]")()
	// /save/<file>/<key>
	// body = []bytes r.Body

	if r.Method != "POST" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	pathParts := ParseArgs(r.URL.Path, "save_encrypted")
	if pathParts == nil || len(pathParts) < 2 {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Print(w, "Invalid url args")
		return
	}

	file := pathParts[2]
	key := pathParts[3]

	encryption_header := r.Header.Get("encryption_key")
	if encryption_header == "" {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "Missing encryption_key header")
		return
	}

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

	encrypted_data, err := encoder_v1.Encrypt(body, encryption_header)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "Error Encryptiong data")
		return
	}

	encoded, _ := encoder_v1.Encode(encrypted_data)
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

	// sends "plain text data" (not encrypted)
	go subServer.NotifySubscribers(key, body)

	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "save")
	return
}
