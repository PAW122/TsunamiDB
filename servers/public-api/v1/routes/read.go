package routes

import (
	"fmt"
	"net/http"

	dataManager_v2 "github.com/PAW122/TsunamiDB/data/dataManager/v2"
	fileSystem_v1 "github.com/PAW122/TsunamiDB/data/fileSystem/v1"
	encoder_v1 "github.com/PAW122/TsunamiDB/encoding/v1"
	debug "github.com/PAW122/TsunamiDB/servers/debug"
	networkmanager "github.com/PAW122/TsunamiDB/servers/network-manager"
	types "github.com/PAW122/TsunamiDB/types"
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

	// Kanał do odbioru wyniku asynchronicznego odczytu:
	readChan := make(chan struct {
		data []byte
		err  error
	}, 1)

	// Uruchamiamy goroutine:
	go func() {
		fsData, err := fileSystem_v1.GetElementByKey(key)
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

		// Odczyt pliku
		data, err := dataManager_v2.ReadDataFromFileAsync(
			file,
			int64(fsData.StartPtr),
			int64(fsData.EndPtr),
		)
		if err != nil {
			readChan <- struct {
				data []byte
				err  error
			}{nil, err}
			return
		}

		// Dekodowanie
		decodedObj := encoder_v1.Decode(data)
		debug.LogExtra("Decoded object:", decodedObj)

		// Zwrócenie wyniku
		readChan <- struct {
			data []byte
			err  error
		}{[]byte(decodedObj.Data), nil}
	}()

	// Blokująco pobieramy wynik z kanału
	res := <-readChan
	if res.err != nil {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, "Error reading from file: ", res.err)
		return
	}

	debug.LogExtra("Data read successfully:", string(res.data))

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(res.data)
}
