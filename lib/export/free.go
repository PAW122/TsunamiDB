package export

import (
	defragmentationManager "TsunamiDB/data/defragmentationManager"
	fileSystem_v1 "TsunamiDB/data/fileSystem/v1"
	"fmt"
)

func Free(key, table string) error {
	fs_data, err := fileSystem_v1.GetElementByKey(key)
	if err != nil {
		return fmt.Errorf("Error retrieving element from map:", err)
	}
	fileSystem_v1.RemoveElementByKey(key)
	defragmentationManager.MarkAsFree(key, table, int64(fs_data.StartPtr), int64(fs_data.EndPtr))
	return nil
}
