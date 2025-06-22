package export

import (
	"fmt"

	defragmentationManager "github.com/PAW122/TsunamiDB/data/defragmentationManager"
	fileSystem_v1 "github.com/PAW122/TsunamiDB/data/fileSystem/v1"
	subServer "github.com/PAW122/TsunamiDB/servers/subscriptions"
)

func Free(key, table string) error {
	fs_data, err := fileSystem_v1.GetElementByKey(key)
	if err != nil {
		return fmt.Errorf("error retrieving element from map:", err)
	}
	fileSystem_v1.RemoveElementByKey(key)
	defragmentationManager.MarkAsFree(key, table, int64(fs_data.StartPtr), int64(fs_data.EndPtr))
	go subServer.NotifyDeleteAndRemove(key)
	return nil
}
