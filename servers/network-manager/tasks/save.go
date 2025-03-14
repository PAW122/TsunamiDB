package tasks

import (
	dataManager_v1 "github.com/PAW122/TsunamiDB/data/dataManager/v1"
	fileSystem_v1 "github.com/PAW122/TsunamiDB/data/fileSystem/v1"
	encoder_v1 "github.com/PAW122/TsunamiDB/encoding/v1"
	types "github.com/PAW122/TsunamiDB/types"
)

func Save(req types.NMmessage) types.NMmessage {

	file := req.Args[0]
	key := req.Args[1]
	if len(file) < 1 || len(key) < 1 {
		return types.NMmessage{
			Finished: false,
		}
	}

	encoded, _ := encoder_v1.Encode(req.Content)
	startPtr, endPtr, err := dataManager_v1.SaveDataToFile(encoded, file)
	if err != nil {
		// w.WriteHeader(http.StatusInternalServerError)
		// fmt.Fprint(w, "Error saving to file:", err)
		return types.NMmessage{
			Finished: false,
		}
	}
	err = fileSystem_v1.SaveElementByKey(key, file, int(startPtr), int(endPtr))
	if err != nil {
		// w.WriteHeader(http.StatusInternalServerError)
		// fmt.Fprint(w, "Error saving to map:", err)
		return types.NMmessage{
			Finished: false,
		}
	}

	//saved
	req.Finished = true
	req.Content = nil // no need to process extra data
	return req
}
