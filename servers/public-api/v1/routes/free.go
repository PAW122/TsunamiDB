package routes

import (
	"fmt"
	"net/http"

	defragmentationManager "github.com/PAW122/TsunamiDB/data/defragmentationManager"
	fileSystem_v1 "github.com/PAW122/TsunamiDB/data/fileSystem/v1"
	debug "github.com/PAW122/TsunamiDB/servers/debug"
	networkmanager "github.com/PAW122/TsunamiDB/servers/network-manager"
	subServer "github.com/PAW122/TsunamiDB/servers/subscriptions"
)

func Free(w http.ResponseWriter, r *http.Request, c *http.Client) {
	defer debug.MeasureTime("> api [free]")()
	// /free/<file>/<key>

	if r.Method != "GET" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	pathParts := ParseArgs(r.URL.Path, "free")
	if len(pathParts) < 4 {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "Invalid url args")
		return
	}

	file := pathParts[2]
	key := pathParts[3]

	fsData, err := fileSystem_v1.GetElementByKey(file, key)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintln(w, "Error retrieving element from map:", err)
		return
	}

	if fsData.HasNested {
		cleanupRecordNested(file, *fsData)
	}

	fileSystem_v1.RemoveElementByKey(file, key)
	defragmentationManager.MarkAsFree(key, file, int64(fsData.StartPtr), int64(fsData.EndPtr))

	networkmanager.NotifyKVTable(file)
	go subServer.NotifyTableDeleteAndRemove(file, key)
	fmt.Fprint(w, "free")
}
