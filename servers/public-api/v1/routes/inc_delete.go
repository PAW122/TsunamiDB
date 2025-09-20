package routes

import (
	"fmt"
	"net/http"

	dataManager_v2 "github.com/PAW122/TsunamiDB/data/dataManager/v2"
	defragmentationManager "github.com/PAW122/TsunamiDB/data/defragmentationManager"
	fileSystem_v1 "github.com/PAW122/TsunamiDB/data/fileSystem/v1"
	incindex "github.com/PAW122/TsunamiDB/data/incIndex"
	encoder_v1 "github.com/PAW122/TsunamiDB/encoding/v1"
	debug "github.com/PAW122/TsunamiDB/servers/debug"
	subServer "github.com/PAW122/TsunamiDB/servers/subscriptions"
)

// DeleteIncremental removes the incremental table file and drops its KV metadata entry.
func DeleteIncremental(w http.ResponseWriter, r *http.Request, client *http.Client) {
	defer debug.MeasureTime("> api [DeleteInc]")()

	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	pathParts := ParseArgs(r.URL.Path, "delete_inc")
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
		fmt.Fprint(w, "Key not found")
		return
	}

	data, err := dataManager_v2.ReadDataFromFileAsync(
		file,
		int64(fsData.StartPtr),
		int64(fsData.EndPtr),
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "Cannot read inc table metadata: "+err.Error())
		return
	}

	decoded := encoder_v1.Decode(data)
	incInfo, err := BytesToStructBinary([]byte(decoded.Data))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "Cannot decode inc table metadata: "+err.Error())
		return
	}

	if err := dataManager_v2.DeleteIncTableFile(incInfo.TableFileName); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "Cannot delete inc table file: "+err.Error())
		return
	}

	if err := incindex.DropTable(incInfo.TableFileName); err != nil {
		// index cleanup failure should not block delete, log later if needed
	}

	fileSystem_v1.RemoveElementByKey(file, key)
	defragmentationManager.MarkAsFree(key, file, int64(fsData.StartPtr), int64(fsData.EndPtr))
	go subServer.NotifyDeleteAndRemove(key)

	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "delete_inc")
}
