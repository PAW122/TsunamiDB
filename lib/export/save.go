package export

import (
	"fmt"
)

func Save(key, table string, data []byte) error {

	if key == "" || table == "" {
		return fmt.Errorf("Invalid key or table value")
	}

	encoded, _ := encode(data, false)
	startPtr, endPtr, err := saveDataToFileAsync(encoded, table)
	if err != nil {
		return err
	}
	prevMeta, existed, err := saveElementByKey(table, key, int(startPtr), int(endPtr), false)
	if err != nil {
		return err
	}
	if existed {
		if prevMeta.FileName != table || prevMeta.StartPtr != int(startPtr) || prevMeta.EndPtr != int(endPtr) {
			markAsFree(prevMeta.Key, prevMeta.FileName, int64(prevMeta.StartPtr), int64(prevMeta.EndPtr))
			recordDefragFree()
		} else {
			recordDefragSkip()
		}
	}
	go notifySubscribers(key, data)
	return nil
}
