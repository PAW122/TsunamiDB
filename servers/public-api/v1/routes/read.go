package routes

import (
	"encoding/json"
	"fmt"
	dataManager_v2 "github.com/PAW122/TsunamiDB/data/dataManager/v2"
	fileSystem_v1 "github.com/PAW122/TsunamiDB/data/fileSystem/v1"
	encoder_v1 "github.com/PAW122/TsunamiDB/encoding/v1"
	debug "github.com/PAW122/TsunamiDB/servers/debug"
	networkmanager "github.com/PAW122/TsunamiDB/servers/network-manager"
	types "github.com/PAW122/TsunamiDB/types"
	"net/http"
)

func AsyncRead(w http.ResponseWriter, r *http.Request, c *http.Client) {
	defer debug.MeasureTime("> api [async read]")()

	if r.Method != "GET" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	pathParts := ParseArgs(r.URL.Path, "read")
	if len(pathParts) < 4 {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "Invalid URL args")
		return
	}

	file := pathParts[2]
	key := pathParts[3]

	partialPaths, err := parsePathHeader(r.Header.Get("read_nested"))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "Invalid read_nested header")
		return
	}
	onlyPaths, err := parsePathHeader(r.Header.Get("read_only_nested"))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "Invalid read_only_nested header")
		return
	}

	readChan := make(chan struct {
		data []byte
		err  error
	}, 1)

	go func() {
		fsData, err := fileSystem_v1.GetElementByKey(file, key)
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

		decodedObj := encoder_v1.Decode(data)
		debug.LogExtra("Decoded object:", decodedObj)

		responseBytes := []byte(decodedObj.Data)
		needsProcessing := decodedObj.HasNested || len(partialPaths) > 0 || len(onlyPaths) > 0
		if needsProcessing {
			var root interface{}
			if err := json.Unmarshal([]byte(decodedObj.Data), &root); err != nil {
				readChan <- struct {
					data []byte
					err  error
				}{nil, fmt.Errorf("invalid stored json: %w", err)}
				return
			}

			switch {
			case len(onlyPaths) > 0:
				resolved, err := extractNestedOnly(file, root, onlyPaths)
				if err != nil {
					readChan <- struct {
						data []byte
						err  error
					}{nil, err}
					return
				}
				responseBytes, err = json.Marshal(resolved)
				if err != nil {
					readChan <- struct {
						data []byte
						err  error
					}{nil, err}
					return
				}
			case len(partialPaths) > 0:
				resolved, err := resolveNestedPaths(file, root, partialPaths)
				if err != nil {
					readChan <- struct {
						data []byte
						err  error
					}{nil, err}
					return
				}
				responseBytes, err = json.Marshal(resolved)
				if err != nil {
					readChan <- struct {
						data []byte
						err  error
					}{nil, err}
					return
				}
			case decodedObj.HasNested:
				resolved := cloneWithPlaceholders(root)
				marshaled, marshalErr := json.Marshal(resolved)
				if marshalErr != nil {
					readChan <- struct {
						data []byte
						err  error
					}{nil, marshalErr}
					return
				}
				responseBytes = marshaled
			}
		}

		readChan <- struct {
			data []byte
			err  error
		}{responseBytes, nil}
	}()

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
