package fileSystem_v1

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

const (
	mapFilePath    = "./db/maps/data_map.json"
	cacheSizeLimit = 10000 // Maksymalna liczba elementów w cache
)

var (
	mutex      sync.Mutex
	dataMap    = make(map[string]GetElement_output)
	cacheData  = make(map[string]GetElement_output) // Cache
	cacheOrder []string                             // Kolejność kluczy dla LRU
	mapLoaded  = false
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

// Pobiera element z mapy (z cache lub pliku)
func GetElementByKey(key string) (*GetElement_output, error) {
	mutex.Lock()
	defer mutex.Unlock()

	// Sprawdź cache
	if element, exists := cacheData[key]; exists {
		moveToFront(key) // Przesuwamy w kolejności LRU
		return &element, nil
	}

	// Załaduj mapę, jeśli nie jest w pamięci
	err := loadMap()
	if err != nil {
		return nil, err
	}

	element, exists := dataMap[key]
	if !exists {
		return nil, errors.New("key not found")
	}

	// Dodaj do cache
	addToCache(key, element)

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
	element := GetElement_output{
		Key:      key,
		FileName: fileName,
		StartPtr: startPtr,
		EndPtr:   endPtr,
	}
	dataMap[key] = element

	// Aktualizacja cache
	addToCache(key, element)

	// Zapis mapy do pliku
	return saveMap()
}

// Usuwa element z mapy i cache
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

	// Usuwamy element z mapy i cache
	delete(dataMap, key)
	delete(cacheData, key)
	removeFromCacheOrder(key)

	// Zapisujemy zaktualizowaną mapę do pliku
	return saveMap()
}

// ======== MECHANIZM CACHE (LRU) ========

// Dodaje element do cache
func addToCache(key string, element GetElement_output) {
	// Jeśli cache przekroczył limit, usuń najstarszy element (LRU)
	if len(cacheData) >= cacheSizeLimit {
		oldestKey := cacheOrder[0]
		delete(cacheData, oldestKey)
		cacheOrder = cacheOrder[1:]
	}

	// Dodaj nowy element do cache
	cacheData[key] = element
	cacheOrder = append(cacheOrder, key)
}

// Przesuwa element na początek (najczęściej używany)
func moveToFront(key string) {
	// Znajdź indeks elementu
	for i, v := range cacheOrder {
		if v == key {
			// Usuń z tej pozycji
			cacheOrder = append(cacheOrder[:i], cacheOrder[i+1:]...)
			break
		}
	}
	// Dodaj na koniec (najświeższy)
	cacheOrder = append(cacheOrder, key)
}

// Usuwa element z cache order (gdy usuwamy z mapy)
func removeFromCacheOrder(key string) {
	for i, v := range cacheOrder {
		if v == key {
			cacheOrder = append(cacheOrder[:i], cacheOrder[i+1:]...)
			break
		}
	}
}
