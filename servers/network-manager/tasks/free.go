package tasks

import (
	defragmentationManager "TsunamiDB/data/defragmentationManager"
	fileSystem_v1 "TsunamiDB/data/fileSystem/v1"
	types "TsunamiDB/types"
)

func Free(req types.NMmessage) types.NMmessage {
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
		// fmt.Fprintln(w, "Error retrieving element from map:", err)
		return types.NMmessage{
			Finished: false,
		}
	}
	fileSystem_v1.RemoveElementByKey(key)
	defragmentationManager.MarkAsFree(key, file, int64(fs_data.StartPtr), int64(fs_data.EndPtr))

	req.Finished = true
	return req

}
