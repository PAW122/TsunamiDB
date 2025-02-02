package dataManager_v1

import (
	"os"
)

/*
*
@param data -> binary data to save
@param filePath -> path to file
@return bool -> true if data was saved successfully, false otherwise
@return error -> error if occurred
*/
func SaveDataToFile(data []byte, filePath string) (bool, error) {
	err := os.WriteFile(filePath, data, 0644)
	if err != nil {
		return false, err
	}
	return true, nil
}
