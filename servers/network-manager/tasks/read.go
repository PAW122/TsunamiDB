package tasks

import (
	dataManager_v1 "TsunamiDB/data/dataManager/v1"
	fileSystem_v1 "TsunamiDB/data/fileSystem/v1"
	encoder_v1 "TsunamiDB/encoding/v1"
	types "TsunamiDB/types"
)

func Read(req types.NMmessage) types.NMmessage {

	file := req.Args[0]
	key := req.Args[1]
	if len(file) < 1 || len(key) < 1 {
		return types.NMmessage{
			Finished: false,
		}
	}

	fs_data, err := fileSystem_v1.GetElementByKey(key)
	if err != nil {
		// w.WriteHeader(http.StatusNotFound)
		// fmt.Fprint(w, "Error retrieving element from map:", err)
		return types.NMmessage{
			Finished: false,
		}
	}

	data, err := dataManager_v1.ReadDataFromFile(file, int64(fs_data.StartPtr), int64(fs_data.EndPtr))
	if err != nil {
		// w.WriteHeader(http.StatusNotFound)
		// fmt.Fprint(w, "Error reading from file:", err)
		return types.NMmessage{
			Finished: false,
		}
	}
	decoded_obj := encoder_v1.Decode(data)

	req.Content = []byte(decoded_obj.Data)
	req.Finished = true

	return req
}
