package routes

import (
	"fmt"
	"net/http"

	dataManager_v1 "TsunamiDB/data/dataManager/v1"
	fileSystem_v1 "TsunamiDB/data/fileSystem/v1"
	encoder_v1 "TsunamiDB/encoding/v1"
)

func Read(w http.ResponseWriter, r *http.Request) {

	// /read/<file>/<key>
	// @return data -> res.body

	if r.Method != "GET" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	pathParts := ParseArgs(r.URL.Path, "read")
	if pathParts == nil || len(pathParts) < 2 {
		fmt.Fprint(w, "Invalid url args")
		return
	}

	file := pathParts[2]
	key := pathParts[3]

	// fmt.Println(pathParts)

	fs_data, err := fileSystem_v1.GetElementByKey(key)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, "Error retrieving element from map:", err)
		return
	}

	data, err := dataManager_v1.ReadDataFromFile(file, int64(fs_data.StartPtr), int64(fs_data.EndPtr))
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, "Error reading from file:", err)
		return
	}
	decoded_obj := encoder_v1.Decode(data)

	w.Write([]byte(decoded_obj.Data))
}
