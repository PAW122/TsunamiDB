package export

import (
	dataManager_v1 "TsunamiDB/data/dataManager/v1"
	defragManager "TsunamiDB/data/defragmentationManager"
	fileSystem_v1 "TsunamiDB/data/fileSystem/v1"
	encoder_v1 "TsunamiDB/encoding/v1"
	"fmt"
)

func Save(key, table string, data []byte) error {

	if key == "" || table == "" {
		return fmt.Errorf("Invalid key or table value")
	}

	// free previous data for same key value if exist
	prevMetaData, err := fileSystem_v1.GetElementByKey(key)
	if err == nil {
		defragManager.MarkAsFree(prevMetaData.Key, prevMetaData.FileName, int64(prevMetaData.StartPtr), int64(prevMetaData.EndPtr))
	}
	encoded, _ := encoder_v1.Encode(data)
	startPtr, endPtr, err := dataManager_v1.SaveDataToFile(encoded, table)
	if err != nil {
		return err
	}
	err = fileSystem_v1.SaveElementByKey(key, table, int(startPtr), int(endPtr))
	if err != nil {
		return err
	}
	return nil
}
