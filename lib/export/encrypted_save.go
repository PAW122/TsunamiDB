package export

import (
	"fmt"

	dataManager_v2 "github.com/PAW122/TsunamiDB/data/dataManager/v2"
	defragmanager "github.com/PAW122/TsunamiDB/data/defragmentationManager"
	fileSystem_v1 "github.com/PAW122/TsunamiDB/data/fileSystem/v1"
	encoder_v1 "github.com/PAW122/TsunamiDB/encoding/v1"
	subServer "github.com/PAW122/TsunamiDB/servers/subscriptions"
)

func SaveEncrypted(key, table, encryption_key string, data []byte) error {

	encrypted_data, err := encoder_v1.Encrypt(data, encryption_key)
	if err != nil {
		return fmt.Errorf("Error Encryptiong data")
	}

	encoded, _ := encoder_v1.Encode(encrypted_data)

	// save to file
	startPtr, endPtr, err := dataManager_v2.SaveDataToFileAsync(encoded, table)
	if err != nil {
		return fmt.Errorf("error saving to file:", err)
	}

	// save to map
	prevMeta, existed, err := fileSystem_v1.SaveElementByKey(key, table, int(startPtr), int(endPtr))
	if err != nil {
		return fmt.Errorf("error saving to map:", err)
	}
	if existed {
		if prevMeta.FileName != table || prevMeta.StartPtr != int(startPtr) || prevMeta.EndPtr != int(endPtr) {
			defragmanager.MarkAsFree(prevMeta.Key, prevMeta.FileName, int64(prevMeta.StartPtr), int64(prevMeta.EndPtr))
			fileSystem_v1.RecordDefragFree()
		} else {
			fileSystem_v1.RecordDefragSkip()
		}
	}

	go subServer.NotifySubscribers(key, data)
	return nil
}
