package defragmentationManager

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	debug "github.com/PAW122/TsunamiDB/servers/debug"
)

const freeFileName = "free_blocks.json"

var (
	freeRegistryMu sync.RWMutex
	freeRegistry   = make(map[string]*tableFreeList)
)

type FreeBlock struct {
	FileName string `json:"fileName"`
	StartPtr int64  `json:"startPtr"`
	EndPtr   int64  `json:"endPtr"`
	Size     int64  `json:"size"`
	Tag      string `json:"tag"`
	InUse    bool   `json:"inUse"`
}

type FileMemory struct {
	mu     sync.Mutex
	Blocks []*FreeBlock
}

type tableFreeList struct {
	name     string
	safeName string
	path     string

	mu     sync.Mutex
	loaded bool
	blocks map[string]FreeBlock
}

var nameSanitizer = strings.NewReplacer(
	"/", "_",
	"\\", "_",
	":", "_",
	"*", "_",
	"?", "_",
	"\"", "_",
	"<", "_",
	">", "_",
	"|", "_",
	"..", "__",
)

func sanitizeName(name string) string {
	s := strings.TrimSpace(name)
	if s == "" {
		return "default"
	}
	return nameSanitizer.Replace(s)
}

func getTableFreeList(table string) (*tableFreeList, error) {
	table = strings.TrimSpace(table)
	if table == "" {
		return nil, errors.New("table name cannot be empty")
	}

	freeRegistryMu.RLock()
	fl := freeRegistry[table]
	freeRegistryMu.RUnlock()
	if fl != nil {
		return fl, nil
	}

	safe := sanitizeName(table)
	path := filepath.Join("./db/maps", safe, freeFileName)

	freeRegistryMu.Lock()
	defer freeRegistryMu.Unlock()
	if fl = freeRegistry[table]; fl != nil {
		return fl, nil
	}
	fl = &tableFreeList{
		name:     table,
		safeName: safe,
		path:     path,
		blocks:   make(map[string]FreeBlock),
	}
	freeRegistry[table] = fl
	return fl, nil
}

func (fl *tableFreeList) load() error {
	if fl.loaded {
		return nil
	}

	if fl.blocks == nil {
		fl.blocks = make(map[string]FreeBlock)
	}

	if _, err := os.Stat(fl.path); os.IsNotExist(err) {
		fl.loaded = true
		return nil
	}

	f, err := os.Open(fl.path)
	if err != nil {
		return err
	}
	defer f.Close()

	decoder := json.NewDecoder(f)
	tmp := make(map[string]FreeBlock)
	if err := decoder.Decode(&tmp); err != nil {
		return err
	}
	fl.blocks = tmp
	fl.loaded = true
	return nil
}

func (fl *tableFreeList) save() error {
	if fl.blocks == nil {
		fl.blocks = make(map[string]FreeBlock)
	}
	if err := os.MkdirAll(filepath.Dir(fl.path), 0755); err != nil {
		return err
	}
	f, err := os.Create(fl.path)
	if err != nil {
		return err
	}
	defer f.Close()
	encoder := json.NewEncoder(f)
	return encoder.Encode(fl.blocks)
}

func MarkAsFree(key string, fileName string, startPtr, endPtr int64) error {
	defer debug.MeasureTime("defragmentation [MarkAsFree]")()

	fl, err := getTableFreeList(fileName)
	if err != nil {
		return err
	}

	fl.mu.Lock()
	defer fl.mu.Unlock()

	if err := fl.load(); err != nil {
		return err
	}

	size := endPtr - startPtr
	fl.blocks[key] = FreeBlock{
		FileName: fileName,
		StartPtr: startPtr,
		EndPtr:   endPtr,
		Size:     size,
	}

	return fl.save()
}

func GetBlock(size int64, fileName string) (*FreeBlock, error) {
	defer debug.MeasureTime("defragmentation [GetBlock]")()

	fl, err := getTableFreeList(fileName)
	if err != nil {
		return nil, err
	}

	fl.mu.Lock()
	defer fl.mu.Unlock()

	if err := fl.load(); err != nil {
		return nil, err
	}

	var (
		bestKey   string
		bestBlock FreeBlock
		found     bool
	)

	for key, block := range fl.blocks {
		if block.Size < size {
			continue
		}
		if block.FileName != fileName {
			continue
		}
		if !found || block.Size < bestBlock.Size {
			bestKey = key
			bestBlock = block
			found = true
		}
	}

	if !found {
		return nil, errors.New("no suitable free blocks available for the specified file")
	}

	delete(fl.blocks, bestKey)
	if err := fl.save(); err != nil {
		return nil, err
	}

	return &bestBlock, nil
}

func SaveBlockCheck(fileName string, startPtr, endPtr int64) {
	defer debug.MeasureTime("defragmentation [SaveBlockCheck]")()

	fl, err := getTableFreeList(fileName)
	if err != nil {
		fmt.Println("ERROR: Could not get free list:", err)
		return
	}

	fl.mu.Lock()
	defer fl.mu.Unlock()

	if err := fl.load(); err != nil {
		fmt.Println("ERROR: Could not load free blocks:", err)
		return
	}

	var modified bool

	for key, block := range fl.blocks {
		if block.FileName != fileName {
			continue
		}
		if startPtr >= block.StartPtr && endPtr <= block.EndPtr {
			if startPtr == block.StartPtr && endPtr == block.EndPtr {
				delete(fl.blocks, key)
			} else if startPtr == block.StartPtr {
				fl.blocks[key] = FreeBlock{
					FileName: block.FileName,
					StartPtr: endPtr,
					EndPtr:   block.EndPtr,
					Size:     block.EndPtr - endPtr,
				}
			} else if endPtr == block.EndPtr {
				fl.blocks[key] = FreeBlock{
					FileName: block.FileName,
					StartPtr: block.StartPtr,
					EndPtr:   startPtr,
					Size:     startPtr - block.StartPtr,
				}
			} else {
				fl.blocks[key] = FreeBlock{
					FileName: block.FileName,
					StartPtr: block.StartPtr,
					EndPtr:   startPtr,
					Size:     startPtr - block.StartPtr,
				}
				newKey := fmt.Sprintf("%s_%d", key, endPtr)
				fl.blocks[newKey] = FreeBlock{
					FileName: block.FileName,
					StartPtr: endPtr,
					EndPtr:   block.EndPtr,
					Size:     block.EndPtr - endPtr,
				}
			}
			modified = true
			break
		}
	}

	if modified {
		if err := fl.save(); err != nil {
			fmt.Println("ERROR: Could not save free blocks:", err)
		}
	}
}

func ResetForTests() {
	freeRegistryMu.Lock()
	lists := make([]*tableFreeList, 0, len(freeRegistry))
	for _, fl := range freeRegistry {
		lists = append(lists, fl)
	}
	freeRegistry = make(map[string]*tableFreeList)
	freeRegistryMu.Unlock()

	for _, fl := range lists {
		fl.mu.Lock()
		fl.blocks = make(map[string]FreeBlock)
		fl.loaded = false
		os.Remove(fl.path)
		fl.mu.Unlock()
	}
}
