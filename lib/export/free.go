package export

import (
	"fmt"

	networkmanager "github.com/PAW122/TsunamiDB/servers/network-manager"
)

func Free(key, table string) error {
	fs_data, err := getElementByKey(table, key)
	if err != nil {
		return fmt.Errorf("error retrieving element from map: %w", err)
	}
	removeElementByKey(table, key)
	markAsFree(key, table, int64(fs_data.StartPtr), int64(fs_data.EndPtr))
	networkmanager.NotifyKVTable(table)
	go notifyDeleteAndRemove(key)
	return nil
}
