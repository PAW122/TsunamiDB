package fileSystem_v1

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

const mapFilePath = "./db/maps/data_map.json"

var (
	mutex     sync.Mutex
	dataMap   = make(map[string]GetElement_output)
	mapLoaded = false
)

// Struktura przechowująca dane o elemencie
type GetElement_output struct {
	Key      string `json:"key"`
	FileName string `json:"fileName"`
	StartPtr int    `json:"startPtr"`
	EndPtr   int    `json:"endPtr"`
}

// Funkcja ładuje mapę z pliku JSON
func loadMap() error {
	if mapLoaded {
		return nil
	}

	// Upewnij się, że katalog istnieje
	os.MkdirAll(filepath.Dir(mapFilePath), os.ModePerm)

	// Sprawdzenie czy plik istnieje
	file, err := os.Open(mapFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			mapLoaded = true
			return nil
		}
		return err
	}
	defer file.Close()

	// Dekodowanie JSON
	decoder := json.NewDecoder(file)
	err = decoder.Decode(&dataMap)
	if err != nil {
		return err
	}

	mapLoaded = true
	return nil
}

// Zapisuje mapę do pliku JSON
func saveMap() error {

	file, err := os.Create(mapFilePath)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	return encoder.Encode(dataMap)
}

// Pobiera element z mapy na podstawie klucza
func GetElementByKey(key string) (*GetElement_output, error) {
	mutex.Lock()
	defer mutex.Unlock()

	// Załaduj mapę, jeśli nie jest w pamięci
	err := loadMap()
	if err != nil {
		return nil, err
	}

	element, exists := dataMap[key]
	if !exists {
		return nil, errors.New("key not found")
	}
	return &element, nil
}

// Zapisuje nowy element w mapie
func SaveElementByKey(key, fileName string, startPtr, endPtr int) error {
	mutex.Lock()
	defer mutex.Unlock()
	// Załaduj mapę, jeśli nie jest w pamięci
	err := loadMap()
	if err != nil {
		return err
	}

	// Aktualizacja mapy
	dataMap[key] = GetElement_output{
		Key:      key,
		FileName: fileName,
		StartPtr: startPtr,
		EndPtr:   endPtr,
	}

	// Zapis mapy do pliku
	return saveMap()
}

// Usuwa element z mapy na podstawie klucza
func RemoveElementByKey(key string) error {
	mutex.Lock()
	defer mutex.Unlock()

	// Załaduj mapę, jeśli nie jest w pamięci
	err := loadMap()
	if err != nil {
		return err
	}

	// Sprawdź, czy element istnieje
	if _, exists := dataMap[key]; !exists {
		return errors.New("element not found")
	}

	// Usuwamy element z mapy
	delete(dataMap, key)

	// Zapisujemy zaktualizowaną mapę do pliku
	return saveMap()
}
