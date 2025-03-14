package fileSystem_v1

import (
	dataManager_v1 "github.com/PAW122/TsunamiDB/data/dataManager/v1"
)

/*
*
@param key -> key to save
@param data -> pointer to encoded data
*/
func SaveElement(key string, data *string) {

}

/*
*
@param key -> key to read data
@return bool -> true if data was read successfully, false otherwise
@return string -> read data
*/
// ReadElement odczytuje dane na podstawie klucza i zwraca je jako string
func ReadElement(key string) (bool, string, error) {
	// Pobranie wskaźników z mapy
	element, err := GetElementByKey(key)
	if err != nil {
		return false, "", err
	}

	// **DEBUG: Sprawdzamy wskaźniki**
	// fmt.Println("READ ELEMENT DEBUG:", "Start:", element.StartPtr, "End:", element.EndPtr)

	// **Korygujemy StartPtr o 2 bajty, aby pominąć wersję**
	startPtrCorrected := element.StartPtr + 2

	// Odczyt danych z pliku
	data, err := dataManager_v1.ReadDataFromFile("data.bin", int64(startPtrCorrected), int64(element.EndPtr))
	if err != nil {
		return false, "", err
	}

	return true, string(data), nil
}
