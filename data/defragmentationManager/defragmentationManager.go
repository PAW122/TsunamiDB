package defragmentationManager

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	debug "github.com/PAW122/TsunamiDB/servers/debug"
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
	Tag      string `json:"tag"` // sync / async
	InUse    bool   // Czy blok jest aktualnie używany
}

// FileMemory - reprezentacja pamięci w pliku
type FileMemory struct {
	mu     sync.Mutex
	Blocks []*FreeBlock
}

// **🔹 Ładowanie wolnych bloków z pliku JSON**
func loadFreeBlocks() error {
	defer debug.MeasureTime("defragmentation [loadFreeBlocks]")()

	if defrag_mapLoaded {
		return nil
	}

	os.MkdirAll(filepath.Dir(freeSpaceFilePath), os.ModePerm)

	file, err := os.Open(freeSpaceFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			defrag_mapLoaded = true
			return nil
		}
		return err
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	err = decoder.Decode(&freeBlocks)
	if err != nil {
		return err
	}

	defrag_mapLoaded = true
	return nil
}

// **🔹 Zapis listy wolnych bloków**
func saveFreeBlocks() error {
	defer debug.MeasureTime("defragmentation [saveFreeBlocks]")()

	file, err := os.Create(freeSpaceFilePath)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	return encoder.Encode(freeBlocks)
	/*
		TODO rework

		trzeba zamienić jsona na enkoder,
		tak to zrobić aby można było asynchrnoiczne blokować dane pointery
		i edytować dane bez potrzeby wczytywania i zapisywania całego pliku
	*/
}

// **🔹 Dodaje nowy wolny blok**
func MarkAsFree(key string, fileName string, startPtr, endPtr int64) error {
	defer debug.MeasureTime("defragmentation [MarkAsFree]")()

	defrag_mutex.Lock()
	defer defrag_mutex.Unlock()

	err := loadFreeBlocks()
	if err != nil {
		return err
	}

	size := endPtr - startPtr
	// fmt.Println("DEFRAG DEBUG: Marking block as free:", key, "Size:", size)

	freeBlocks[key] = FreeBlock{
		FileName: fileName,
		StartPtr: startPtr,
		EndPtr:   endPtr,
		Size:     size,
	}

	return saveFreeBlocks()
}

// **🔹 Pobiera najmniejszy wolny blok, który pasuje do podanego rozmiaru i nazwy pliku**
func GetBlock(size int64, fileName string) (*FreeBlock, error) {
	defer debug.MeasureTime("defragmentation [GetBlock]")()

	defrag_mutex.Lock()
	defer defrag_mutex.Unlock()

	err := loadFreeBlocks()
	if err != nil {
		return nil, err
	}

	var bestFitBlock *FreeBlock
	for _, block := range freeBlocks {
		// Sprawdzamy tylko bloki pasujące do nazwy pliku i rozmiaru
		if block.Size >= size && block.FileName == fileName {
			if bestFitBlock == nil || block.Size < bestFitBlock.Size {
				bestFitBlock = &block
			}
		}
	}

	if bestFitBlock == nil {
		return nil, errors.New("no suitable free blocks available for the specified file")
	}

	// Usunięcie bloku po przydzieleniu
	delete(freeBlocks, bestFitBlock.FileName)
	saveFreeBlocks()

	return bestFitBlock, nil
}

// **🔹 Sprawdza i aktualizuje wolne bloki po zajęciu miejsca**
func SaveBlockCheck(startPtr, endPtr int64) {
	defer debug.MeasureTime("defragmentation [SaveBlockCheck]")()

	defrag_mutex.Lock()
	defer defrag_mutex.Unlock()

	err := loadFreeBlocks()
	if err != nil {
		fmt.Println("ERROR: Could not load free blocks:", err)
		return
	}

	for key, block := range freeBlocks {
		// **Sprawdzenie, czy nowy zapis pokrywa się z wolnym blokiem**
		if startPtr >= block.StartPtr && endPtr <= block.EndPtr {
			// fmt.Println("DEFRAG DEBUG: Save overlaps with free block", key)

			// **Cały blok został zajęty - usuwamy go**
			if startPtr == block.StartPtr && endPtr == block.EndPtr {
				// fmt.Println("DEFRAG: Entire block occupied, removing", key)
				delete(freeBlocks, key)
			} else if startPtr == block.StartPtr {
				// **Początek bloku jest zajęty - przesuwamy start**
				// fmt.Println("DEFRAG: Adjusting start of block", key)
				freeBlocks[key] = FreeBlock{
					FileName: block.FileName,
					StartPtr: endPtr,
					EndPtr:   block.EndPtr,
					Size:     block.EndPtr - endPtr,
				}
			} else if endPtr == block.EndPtr {
				// **Koniec bloku jest zajęty - przesuwamy koniec**
				// fmt.Println("DEFRAG: Adjusting end of block", key)
				freeBlocks[key] = FreeBlock{
					FileName: block.FileName,
					StartPtr: block.StartPtr,
					EndPtr:   startPtr,
					Size:     startPtr - block.StartPtr,
				}
			} else {
				// **Blok został podzielony na dwa mniejsze**
				// fmt.Println("DEFRAG: Splitting block", key)
				freeBlocks[key] = FreeBlock{
					FileName: block.FileName,
					StartPtr: block.StartPtr,
					EndPtr:   startPtr,
					Size:     startPtr - block.StartPtr,
				}
				newKey := fmt.Sprintf("%s_%d", key, endPtr)
				freeBlocks[newKey] = FreeBlock{
					FileName: block.FileName,
					StartPtr: endPtr,
					EndPtr:   block.EndPtr,
					Size:     block.EndPtr - endPtr,
				}
			}

			// **Zapisujemy zaktualizowane wolne bloki**
			saveFreeBlocks()
			return
		}
	}
}

// ResetForTests clears internal state and removes on-disk metadata.
func ResetForTests() {
	defrag_mutex.Lock()
	defer defrag_mutex.Unlock()
	freeBlocks = make(map[string]FreeBlock)
	defrag_mapLoaded = false
	_ = os.Remove(freeSpaceFilePath)
}
