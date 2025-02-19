package fileSystem_v1

import (
	"encoding/gob"
	"errors"
	"os"
	"sync"
	"time"

	debug "TsunamiDB/servers/debug"
)

const mapFilePath = "./db/maps/data_map.gob"

var (
	mutex      sync.RWMutex
	dataMap    = make(map[string]GetElement_output)
	mapLoaded  = false
	updateChan = make(chan struct{}, 1) // Kanał do sygnalizacji zapisu w tle
)

// Struktura przechowująca dane o elemencie

type GetElement_output struct {
	Key      string
	FileName string
	StartPtr int
	EndPtr   int
}

func init() {
	go batchSaveWorker()
}

// Ładowanie mapy z pliku binarnego
func loadMap() error {
	defer debug.MeasureTime("fileSystem [loadMap]")()

	mutex.Lock()
	defer mutex.Unlock()

	if mapLoaded {
		return nil
	}

	file, err := os.Open(mapFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			mapLoaded = true
			return nil
		}
		return err
	}
	defer file.Close()

	decoder := gob.NewDecoder(file)
	if err := decoder.Decode(&dataMap); err != nil {
		return err
	}

	mapLoaded = true
	return nil
}

// Zapisuje mapę do pliku binarnego w tle
func saveMap() {
	select {
	case updateChan <- struct{}{}:
	default:
	}
}

func batchSaveWorker() {
	for {
		<-updateChan
		time.Sleep(1 * time.Second)
		mutex.Lock()
		file, err := os.Create(mapFilePath)
		if err == nil {
			encoder := gob.NewEncoder(file)
			encoder.Encode(dataMap)
			file.Close()
		}
		mutex.Unlock()
	}
}

// Pobiera element z mapy
func GetElementByKey(key string) (*GetElement_output, error) {
	defer debug.MeasureTime("fileSystem [GetElementByKey]")()

	mutex.RLock()
	element, exists := dataMap[key]
	mutex.RUnlock()

	if !exists {
		return nil, errors.New("key not found")
	}
	return &element, nil
}

// Zapisuje nowy element w mapie i wyzwala zapis w tle
func SaveElementByKey(key, fileName string, startPtr, endPtr int) error {
	defer debug.MeasureTime("fileSystem [SaveElementByKey]")()

	mutex.Lock()
	dataMap[key] = GetElement_output{
		Key:      key,
		FileName: fileName,
		StartPtr: startPtr,
		EndPtr:   endPtr,
	}
	mutex.Unlock()

	saveMap()
	return nil
}

// Usuwa element z mapy i wyzwala zapis w tle
func RemoveElementByKey(key string) error {
	defer debug.MeasureTime("fileSystem [RemoveElementByKey]")()

	mutex.Lock()
	delete(dataMap, key)
	mutex.Unlock()

	saveMap()
	return nil
}
