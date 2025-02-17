package defragmentationManager

import (
	debug "TsunamiDB/servers/debug"
	"fmt"
	"io/fs"
	"os"
)

/*
	loop przez wszystkie pliki /db/data (enkodowane binarki)
	1. sprawdz czy są już free block dla danego pliku które spełniają
	potrzebne rozmiary
		> jak nie to stwórz nowe na końcu pliku

	2. GetBlock() - najpierw wywoła GetAsyncBlock(), jak ten nie ma jeszcze gotowego
	bloku, zwróci nil to wykonuje się normalne GetBlock()

	jak ma async blok to go zwraca




	! GetBlock musi obsługiwać sprawdzanie czy zwraca block dla
	odpowiedneigo pliku
*/

type AsyncFreeBlocks struct {
	FilePath string
	StartPtr int
	EndPtr   int
	InUse    bool
	Async    bool
}

/*
cache - [fileName: AsyncFreeBlock]
loadedFiles - existing files in /data dir
*/
var (
	asyncBlocksTempDir = "./db/temp/async_blocks.json"
	dataFilesPath      = "./db/data"
	cache              = make(map[string]AsyncFreeBlocks)
	loadedFiles        []fs.DirEntry
)

/*
load all already created files to cache
@return succes bool
*/
func LoadFiles(path string) bool {
	debug.Log("[async block manager] [defragManager] LoadFiles")
	files, err := os.ReadDir(dataFilesPath)
	if err != nil {
		debug.Log(fmt.Sprintln("[async block manager] [defragManager] fail: LoadFiles", err))
		return false
	}
	loadedFiles = files
	return true
}

/*
load data from temp to cache
*/
func LoadTempData() {

}

/*
prepare block to async write
(if file dont have any cache blocks or block is used create new block)
*/
func CreateNewAsyncBlock() {
	debug.Log("[async block manager] [defragManager] CreateNewAsyncBlock")
}

/*
save data to temp file to load after db restart
*/
func saveToTemp() {
	debug.Log("[async block manager] [defragManager] saveToTemp")

}
