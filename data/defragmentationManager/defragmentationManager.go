package defragmentationManager

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

var (
	freeListMu sync.Mutex
)

// Struktura wolnego bloku
type FreeBlock struct {
	FileName string `json:"fileName"`
	StartPtr int64  `json:"startPtr"`
	EndPtr   int64  `json:"endPtr"`
	Size     int64  `json:"size"`
}

// --- Helper: wyznacz plik free-listy dla danego fileName (max 4 shardy) ---
func freeBlocksFile(fileName string) string {
	hash := 0
	for _, c := range fileName {
		hash += int(c)
	}
	shard := hash % 4
	// Plik dla każdej z 4 grup
	return fmt.Sprintf("./db/maps/free_blocks_%d.json", shard)
}

// --- Ładowanie free-listy dla danego fileName/sharda ---
func loadFreeBlocks(fileName string) (map[string]FreeBlock, error) {
	path := freeBlocksFile(fileName)
	m := make(map[string]FreeBlock)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return m, nil // pusta lista
		}
		return nil, err
	}
	defer f.Close()
	if err := json.NewDecoder(f).Decode(&m); err != nil {
		return nil, err
	}
	return m, nil
}

// --- Zapis free-listy dla danego fileName/sharda ---
func saveFreeBlocks(fileName string, m map[string]FreeBlock) error {
	path := freeBlocksFile(fileName)
	os.MkdirAll(filepath.Dir(path), os.ModePerm)
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(m)
}

// --- Dodanie bloku jako wolnego ---
func MarkAsFree(key string, fileName string, startPtr, endPtr int64) error {
	freeListMu.Lock()
	defer freeListMu.Unlock()
	m, err := loadFreeBlocks(fileName)
	if err != nil {
		return err
	}
	size := endPtr - startPtr
	m[key] = FreeBlock{
		FileName: fileName,
		StartPtr: startPtr,
		EndPtr:   endPtr,
		Size:     size,
	}
	return saveFreeBlocks(fileName, m)
}

// --- Pobranie najlepiej pasującego bloku (best-fit) ---
func GetBlock(size int64, fileName string) (*FreeBlock, error) {
	freeListMu.Lock()
	defer freeListMu.Unlock()
	m, err := loadFreeBlocks(fileName)
	if err != nil {
		return nil, err
	}
	var bestKey string
	var bestBlock *FreeBlock
	for key, block := range m {
		if block.Size >= size && block.FileName == fileName {
			if bestBlock == nil || block.Size < bestBlock.Size {
				b := block // kopia (żeby wskaźnik był prawidłowy)
				bestBlock = &b
				bestKey = key
			}
		}
	}
	if bestBlock == nil {
		return nil, errors.New("no suitable free blocks available for the specified file")
	}
	// Po pobraniu usuwamy blok
	delete(m, bestKey)
	saveFreeBlocks(fileName, m)
	return bestBlock, nil
}

// --- Aktualizacja wolnych bloków po zapisie (fragmentacja, podział) ---
func SaveBlockCheck(startPtr, endPtr int64, fileName string) {
	freeListMu.Lock()
	defer freeListMu.Unlock()
	m, err := loadFreeBlocks(fileName)
	if err != nil {
		fmt.Println("ERROR: Could not load free blocks:", err)
		return
	}
	for key, block := range m {
		if startPtr >= block.StartPtr && endPtr <= block.EndPtr && block.FileName == fileName {
			// Cały blok zajęty
			if startPtr == block.StartPtr && endPtr == block.EndPtr {
				delete(m, key)
			} else if startPtr == block.StartPtr {
				// Początek bloku zajęty — przesuwamy start
				m[key] = FreeBlock{
					FileName: block.FileName,
					StartPtr: endPtr,
					EndPtr:   block.EndPtr,
					Size:     block.EndPtr - endPtr,
				}
			} else if endPtr == block.EndPtr {
				// Koniec bloku zajęty — przesuwamy koniec
				m[key] = FreeBlock{
					FileName: block.FileName,
					StartPtr: block.StartPtr,
					EndPtr:   startPtr,
					Size:     startPtr - block.StartPtr,
				}
			} else {
				// Podział na dwa bloki
				m[key] = FreeBlock{
					FileName: block.FileName,
					StartPtr: block.StartPtr,
					EndPtr:   startPtr,
					Size:     startPtr - block.StartPtr,
				}
				newKey := fmt.Sprintf("%s_%d", key, endPtr)
				m[newKey] = FreeBlock{
					FileName: block.FileName,
					StartPtr: endPtr,
					EndPtr:   block.EndPtr,
					Size:     block.EndPtr - endPtr,
				}
			}
			saveFreeBlocks(fileName, m)
			return
		}
	}
}
