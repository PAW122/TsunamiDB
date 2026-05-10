package export

import (
	"fmt"

	"github.com/PAW122/TsunamiDB/data/valuepatch"
	networkmanager "github.com/PAW122/TsunamiDB/servers/network-manager"
)

type PatchOperation = valuepatch.Operation

func Patch(key, table string, ops []valuepatch.Operation) ([]byte, error) {
	if key == "" || table == "" {
		return nil, fmt.Errorf("Invalid key or table value")
	}

	unlock := valuepatch.LockKey(table, key)
	defer unlock()

	fsData, err := getElementByKey(table, key)
	if err != nil {
		return nil, fmt.Errorf("error retrieving element from map: %w", err)
	}

	data, err := readDataFromFileAsync(table, int64(fsData.StartPtr), int64(fsData.EndPtr))
	if err != nil {
		return nil, err
	}

	decoded := decode(data)
	updated, err := valuepatch.Apply([]byte(decoded.Data), ops)
	if err != nil {
		return nil, err
	}

	encoded, meta := encode(updated, decoded.HasNested)
	startPtr, endPtr, err := saveDataToFileAsync(encoded, table)
	if err != nil {
		return nil, err
	}

	prevMeta, existed, err := saveElementByKey(table, key, int(startPtr), int(endPtr), meta.HasNested)
	if err != nil {
		_ = markAsFree(key, table, startPtr, endPtr)
		return nil, err
	}
	if existed {
		if prevMeta.FileName != table || prevMeta.StartPtr != int(startPtr) || prevMeta.EndPtr != int(endPtr) {
			_ = markAsFree(prevMeta.Key, prevMeta.FileName, int64(prevMeta.StartPtr), int64(prevMeta.EndPtr))
			recordDefragFree()
		} else {
			recordDefragSkip()
		}
	}

	networkmanager.NotifyKVTable(table)
	go notifyPatchSubscribers(key, ops)
	return updated, nil
}
