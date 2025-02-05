package defragmentationManager

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

const freeSpaceFilePath = "./db/maps/free_blocks.json"

var (
	defrag_mutex     sync.Mutex
	freeBlocks       = make(map[string]FreeBlock)
	defrag_mapLoaded = false
)

// Struktura przechowująca wolne bloki
type FreeBlock struct {
	FileName string `json:"fileName"`
	StartPtr int64  `json:"startPtr"`
	EndPtr   int64  `json:"endPtr"`
	Size     int64  `json:"size"`
}

// **🔹 Funkcja ładuje wolne bloki z pliku JSON**
func loadFreeBlocks() error {
	if defrag_mapLoaded {
		return nil
	}

	// Upewniamy się, że katalog istnieje
	os.MkdirAll(filepath.Dir(freeSpaceFilePath), os.ModePerm)

	// Sprawdzenie czy plik istnieje
	file, err := os.Open(freeSpaceFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			defrag_mapLoaded = true
			return nil
		}
		return err
	}
	defer file.Close()

	// Dekodowanie JSON
	decoder := json.NewDecoder(file)
	err = decoder.Decode(&freeBlocks)
	if err != nil {
		return err
	}

	defrag_mapLoaded = true
	return nil
}

// **🔹 Funkcja zapisuje wolne bloki do pliku**
func saveFreeBlocks() error {

	file, err := os.Create(freeSpaceFilePath)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	return encoder.Encode(freeBlocks)
}

// **🔹 Dodaje nowy wolny blok do listy**
func MarkAsFree(key string, fileName string, startPtr, endPtr int64) error {
	defrag_mutex.Lock()
	defer defrag_mutex.Unlock()

	// Załaduj listę wolnych bloków, jeśli jeszcze nie jest w pamięci
	err := loadFreeBlocks()
	if err != nil {
		return err
	}

	// Obliczamy rozmiar wolnego bloku
	size := endPtr - startPtr

	// **DEBUG: Sprawdzamy dodawane bloki**
	fmt.Println("DEFRAG DEBUG: Marking block as free:", key, "Size:", size)

	// Aktualizacja listy wolnych bloków
	freeBlocks[key] = FreeBlock{
		FileName: fileName,
		StartPtr: startPtr,
		EndPtr:   endPtr,
		Size:     size,
	}

	// Zapis listy do pliku JSON
	return saveFreeBlocks()
}

// **🔹 Pobiera największy dostępny wolny blok**
func GetLargestFreeBlock() (*FreeBlock, error) {
	defrag_mutex.Lock()
	defer defrag_mutex.Unlock()

	err := loadFreeBlocks()
	if err != nil {
		return nil, err
	}

	var largestBlock *FreeBlock
	for _, block := range freeBlocks {
		if largestBlock == nil || block.Size > largestBlock.Size {
			largestBlock = &block
		}
	}

	if largestBlock == nil {
		return nil, errors.New("no free blocks available")
	}

	return largestBlock, nil
}

// **🔹 Usuwa blok po jego wykorzystaniu**
func RemoveFreeBlock(key string) error {
	defrag_mutex.Lock()
	defer defrag_mutex.Unlock()

	err := loadFreeBlocks()
	if err != nil {
		return err
	}

	if _, exists := freeBlocks[key]; !exists {
		return errors.New("block not found")
	}

	delete(freeBlocks, key)

	defrag_mutex.Lock()
	defer defrag_mutex.Unlock()
	return saveFreeBlocks()
}
