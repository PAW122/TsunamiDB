package dataManager_v1

import (
	"fmt"
	"os"

	"TsunamiDB/data/defragmentationManager"
)

/**
 * @param data -> binary data to save
 * @param filePath -> path to file
 * @return startPtr -> data start position in file
 * @return endPtr -> data end position in file
 * @return error -> error if occurred
 */
func SaveDataToFile(data []byte, filePath string) (int64, int64, error) {
	// Sprawdzamy, czy są dostępne wolne bloki do nadpisania
	freeBlock, err := defragmentationManager.GetLargestFreeBlock()
	var startPtr int64
	var endPtr int64

	// Jeśli mamy wolny blok, używamy go
	if err == nil {
		fmt.Println("SAVE DEBUG: Using free block at", freeBlock.StartPtr, "to", freeBlock.EndPtr)
		startPtr = freeBlock.StartPtr
		endPtr = startPtr + int64(len(data))

		// Otwieramy plik w trybie zapisu
		file, err := os.OpenFile(filePath, os.O_RDWR, 0644)
		if err != nil {
			return 0, 0, err
		}
		defer file.Close()

		// Przechodzimy do pozycji startowej i nadpisujemy dane
		_, err = file.Seek(startPtr, 0)
		if err != nil {
			return 0, 0, err
		}

		_, err = file.Write(data)
		if err != nil {
			return 0, 0, err
		}

		// Usuwamy blok z listy wolnych bloków
		defragmentationManager.RemoveFreeBlock(freeBlock.FileName)
	} else {
		// Jeśli nie ma wolnych bloków, dopisujemy dane na koniec pliku
		file, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return 0, 0, err
		}
		defer file.Close()

		startPtr, err = file.Seek(0, os.SEEK_END)
		if err != nil {
			return 0, 0, err
		}

		_, err = file.Write(data)
		if err != nil {
			return 0, 0, err
		}

		endPtr = startPtr + int64(len(data))
	}

	return startPtr, endPtr, nil
}
