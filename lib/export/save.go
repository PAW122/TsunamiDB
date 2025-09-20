package export

import (
	"fmt"

	dataManager_v2 "github.com/PAW122/TsunamiDB/data/dataManager/v2"
	defragManager "github.com/PAW122/TsunamiDB/data/defragmentationManager"
	fileSystem_v1 "github.com/PAW122/TsunamiDB/data/fileSystem/v1"
	encoder_v1 "github.com/PAW122/TsunamiDB/encoding/v1"
	subServer "github.com/PAW122/TsunamiDB/servers/subscriptions"
)

func Save(key, table string, data []byte) error {

	if key == "" || table == "" {
		return fmt.Errorf("Invalid key or table value")
	}

	encoded, _ := encoder_v1.Encode(data)
	startPtr, endPtr, err := dataManager_v2.SaveDataToFileAsync(encoded, table)
	if err != nil {
		return err
	}
	prevMeta, existed, err := fileSystem_v1.SaveElementByKey(table, key, int(startPtr), int(endPtr))
	if err != nil {
		return err
	}
	if existed {
		if prevMeta.FileName != table || prevMeta.StartPtr != int(startPtr) || prevMeta.EndPtr != int(endPtr) {
			defragManager.MarkAsFree(prevMeta.Key, prevMeta.FileName, int64(prevMeta.StartPtr), int64(prevMeta.EndPtr))
			fileSystem_v1.RecordDefragFree()
		} else {
			fileSystem_v1.RecordDefragSkip()
		}
	}
	go subServer.NotifySubscribers(key, data)
	return nil
}
