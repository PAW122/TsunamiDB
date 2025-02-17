package dataManager_v1

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	debug "TsunamiDB/servers/debug"
)

var basePath = "./db/data"

/**
 * @param filePath -> path to file
 * @param dataStartPtr -> start position in file
 * @param dataEndPtr -> end position in file
 * @return []byte -> read binary data
 * @return error -> error if occurred
 */
func ReadDataFromFile(filePath string, dataStartPtr int64, dataEndPtr int64) ([]byte, error) {
	defer debug.MeasureTime("read-from-file")()

	// Otwórz plik do odczytu
	file, err := os.Open(filepath.Join(basePath, filePath))
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Sprawdzenie rozmiaru pliku
	fileInfo, err := file.Stat()
	if err != nil {
		return nil, err
	}
	fileSize := fileInfo.Size()

	// **DEBUG: Sprawdzamy wskaźniki przed odczytem**
	// fmt.Println("READ DEBUG: FileSize:", fileSize, "StartPtr:", dataStartPtr, "EndPtr:", dataEndPtr)

	// Sprawdzenie poprawności zakresu
	if dataStartPtr < 0 || dataEndPtr <= dataStartPtr || dataEndPtr > fileSize {
		fmt.Println("readData.go:37 READ ERROR: Invalid range detected")
		return nil, errors.New("invalid read range")
	}

	// Ustawienie wskaźnika na początek danych
	_, err = file.Seek(dataStartPtr, 0)
	if err != nil {
		return nil, err
	}

	// Obliczenie długości danych do odczytu
	readLength := dataEndPtr - dataStartPtr
	buffer := make([]byte, readLength)

	// **Użycie `io.ReadFull`, aby wymusić pełny odczyt**
	n, err := io.ReadFull(file, buffer)
	if err != nil && err != io.EOF {
		fmt.Println("READ ERROR: Failed to read full data")
		return nil, err
	}

	// **DEBUG: Sprawdzamy pierwsze bajty, aby upewnić się, że wersja jest poprawna**
	// fmt.Println("READ DATA HEADER:", buffer[:min(5, len(buffer))]) // Pokazujemy max 5 bajtów

	// **DEBUG: Sprawdzamy liczbę odczytanych bajtów**
	// fmt.Println("READ SUCCESS: Read", n, "bytes")

	// Zwracamy dokładnie tyle bajtów, ile odczytano
	return buffer[:n], nil
}

func StreamDataFromFile(filePath string, start int64, chunkSize int64) ([]byte, error) {
	// Otwórz plik
	file, err := os.Open(filepath.Join(basePath, filePath))
	if err != nil {
		return nil, fmt.Errorf("file not found: %w", err)
	}
	defer file.Close()

	// Pobierz informacje o pliku
	fileInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("could not get file info: %w", err)
	}
	fileSize := fileInfo.Size()

	// Walidacja zakresu odczytu
	if start > fileSize {
		return nil, errors.New("invalid range")
	}
	if start+chunkSize > fileSize {
		chunkSize = fileSize - start
	}

	// Przesunięcie do pozycji startowej
	_, err = file.Seek(start, io.SeekStart)
	if err != nil {
		return nil, fmt.Errorf("failed to seek file: %w", err)
	}

	// Odczyt danych
	buffer := make([]byte, chunkSize)
	n, err := io.ReadFull(file, buffer)
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("failed to stream file: %w", err)
	}

	return buffer[:n], nil
}

// Funkcja pomocnicza do pobrania minimum dwóch wartości
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
