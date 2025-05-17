package dataManager_v1

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/PAW122/TsunamiDB/data/defragmentationManager"
	debug "github.com/PAW122/TsunamiDB/servers/debug"
)

// getDataFilePath - Tworzy pełną ścieżkę w `/db/data/{filePath}`
func getDataFilePath(filePath string) string {
	basePath := "./db/data"
	return filepath.Join(basePath, filePath)
}

// ensureDirExists - Tworzy katalogi, jeśli ich nie ma
func ensureDirExists(filePath string) error {
	dir := filepath.Dir(filePath)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return os.MkdirAll(dir, 0755)
	}
	return nil
}

/**
 * Zapisuje dane do pliku i obsługuje system defragmentacji
 */
func SaveDataToFile(data []byte, filePath string) (int64, int64, error) {
	defer debug.MeasureTime("save-to-file")()

	var startPtr, endPtr int64
	fullPath := getDataFilePath(filePath)

	// Upewnij się, że katalog istnieje
	if err := ensureDirExists(fullPath); err != nil {
		return 0, 0, fmt.Errorf("błąd tworzenia katalogu: %w", err)
	}

	// Spróbuj pobrać wolny blok pamięci
	// W sekcji "Spróbuj pobrać wolny blok pamięci":
	freeBlock, err := defragmentationManager.GetBlock(int64(len(data)), filePath)
	if err == nil {
		// Użyj znalezionego bloku
		startPtr = freeBlock.StartPtr
		endPtr = startPtr + int64(len(data))

		// 🔹 Otwórz plik, ale jeśli nie istnieje, utwórz go
		file, err := os.OpenFile(fullPath, os.O_WRONLY|os.O_CREATE, 0644)
		if err != nil {
			return 0, 0, fmt.Errorf("błąd otwierania pliku: %w", err)
		}
		defer file.Close()

		// 🔹 Ustaw wskaźnik na początek wolnego bloku
		_, err = file.Seek(startPtr, 0)
		if err != nil {
			return 0, 0, fmt.Errorf("błąd ustawiania wskaźnika pliku: %w", err)
		}

		// 🔹 Zapisz dane
		_, err = file.Write(data)
		if err != nil {
			return 0, 0, fmt.Errorf("błąd zapisu do pliku: %w", err)
		}

		// 🔹 Aktualizuj system defragmentacji
		defragmentationManager.SaveBlockCheck(startPtr, endPtr, filePath)

	} else {
		// 🔹 Jeśli nie znaleziono wolnego bloku, dopisujemy dane na końcu pliku
		file, err := os.OpenFile(fullPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return 0, 0, fmt.Errorf("błąd otwierania pliku do dopisania: %w", err)
		}
		defer file.Close()

		// 🔹 Pobierz offset na końcu pliku
		startPtr, err = file.Seek(0, io.SeekEnd)
		if err != nil {
			return 0, 0, fmt.Errorf("błąd ustawiania wskaźnika pliku: %w", err)
		}

		// 🔹 Zapisz dane
		_, err = file.Write(data)
		if err != nil {
			return 0, 0, fmt.Errorf("błąd zapisu do pliku: %w", err)
		}

		endPtr = startPtr + int64(len(data))
	}

	// 🔹 Debug: Wyświetl ścieżkę zapisu
	// fmt.Printf("✅ Dane zapisane: %s (od %d do %d)\n", fullPath, startPtr, endPtr)
	return startPtr, endPtr, nil
}
