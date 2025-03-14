package routes

import (
	"fmt"
	"net/http"

	defragmentationManager "github.com/PAW122/TsunamiDB/data/defragmentationManager"
	fileSystem_v1 "github.com/PAW122/TsunamiDB/data/fileSystem/v1"
	debug "github.com/PAW122/TsunamiDB/servers/debug"
)

func Free(w http.ResponseWriter, r *http.Request) {
	defer debug.MeasureTime("> api [free]")()
	// /free/<file>/<key>

	if r.Method != "GET" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	pathParts := ParseArgs(r.URL.Path, "free")
	if pathParts == nil || len(pathParts) < 2 {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Print(w, "Invalid url args")
		return
	}

	file := pathParts[2]
	key := pathParts[3]

	fs_data, err := fileSystem_v1.GetElementByKey(key)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintln(w, "Error retrieving element from map:", err)
		return
	}
	fileSystem_v1.RemoveElementByKey(key)
	defragmentationManager.MarkAsFree(key, file, int64(fs_data.StartPtr), int64(fs_data.EndPtr))

	fmt.Fprint(w, "free")
}
