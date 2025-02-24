package routes

import (
	"fmt"
	"net/http"

	dataManager_v2 "TsunamiDB/data/dataManager/v2"
	fileSystem_v1 "TsunamiDB/data/fileSystem/v1"
	encoder_v1 "TsunamiDB/encoding/v1"
	debug "TsunamiDB/servers/debug"
	networkmanager "TsunamiDB/servers/network-manager"
	types "TsunamiDB/types"
)

func AsyncRead(w http.ResponseWriter, r *http.Request) {
	defer debug.MeasureTime("> api [async read]")()

	if r.Method != "GET" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	pathParts := ParseArgs(r.URL.Path, "read")
	if pathParts == nil || len(pathParts) < 2 {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "Invalid URL args")
		return
	}

	file := pathParts[2]
	key := pathParts[3]

	readChan := make(chan struct {
		data []byte
		err  error
	}, 1)

	readWG.Add(1)
	go func() {
		defer readWG.Done()

		fs_data, err := fileSystem_v1.GetElementByKey(key)
		if err != nil {
			nm := networkmanager.GetNetworkManager()
			if nm == nil {
				readChan <- struct {
					data []byte
					err  error
				}{nil, fmt.Errorf("network manager not initialized")}
				return
			}

			req := types.NMmessage{
				Task:      "read",
				Args:      []string{file, key},
				ReqSendBy: nm.ServerIP,
			}
			res := nm.SendTaskReq(req)
			if res.Finished {
				readChan <- struct {
					data []byte
					err  error
				}{res.Content, nil}
			} else {
				readChan <- struct {
					data []byte
					err  error
				}{nil, fmt.Errorf("data not found on any server")}
			}
			return
		}

		data, err := dataManager_v2.ReadDataFromFileAsync(file, int64(fs_data.StartPtr), int64(fs_data.EndPtr))
		if err != nil {
			readChan <- struct {
				data []byte
				err  error
			}{nil, err}
			return
		}

		decodedObj := encoder_v1.Decode(data)
		readChan <- struct {
			data []byte
			err  error
		}{[]byte(decodedObj.Data), nil}
	}()

	readWG.Wait()
	close(readChan)

	res := <-readChan
	if res.err != nil {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, "Error reading from file: ", res.err)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write(res.data)
}
