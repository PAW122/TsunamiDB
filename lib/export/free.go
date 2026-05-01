package export

import (
	"fmt"
)

func Free(key, table string) error {
	fs_data, err := getElementByKey(table, key)
	if err != nil {
		return fmt.Errorf("error retrieving element from map: %w", err)
	}
	removeElementByKey(table, key)
	markAsFree(key, table, int64(fs_data.StartPtr), int64(fs_data.EndPtr))
	go notifyDeleteAndRemove(key)
	return nil
}
