package dataManager_v1

import (
	"io"
	"os"

	"TsunamiDB/data/defragmentationManager"
)

/**
 * @param data -> binary data to save
 * @param filePath -> path to file
 * @return startPtr -> start position in file
 * @return endPtr -> end position in file
 * @return error -> error if occurred
 */
func SaveDataToFile(data []byte, filePath string) (int64, int64, error) {
	freeBlock, err := defragmentationManager.GetBlock(int64(len(data)))
	var startPtr int64
	var endPtr int64

	if err == nil {
		// fmt.Println("SAVE DEBUG: Using free block at", freeBlock.StartPtr, "to", freeBlock.EndPtr)
		startPtr = freeBlock.StartPtr
		endPtr = startPtr + int64(len(data))

		file, err := os.OpenFile(filePath, os.O_RDWR, 0644)
		if err != nil {
			return 0, 0, err
		}
		defer file.Close()

		_, err = file.Seek(startPtr, 0)
		if err != nil {
			return 0, 0, err
		}

		_, err = file.Write(data)
		if err != nil {
			return 0, 0, err
		}

		defragmentationManager.SaveBlockCheck(startPtr, endPtr)
	} else {
		file, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return 0, 0, err
		}
		defer file.Close()

		startPtr, err = file.Seek(0, io.SeekEnd)
		if err != nil {
			return 0, 0, err
		}

		_, err = file.Write(data)
		if err != nil {
			return 0, 0, err
		}

		endPtr = startPtr + int64(len(data))
	}

	return startPtr, endPtr, nil
}
