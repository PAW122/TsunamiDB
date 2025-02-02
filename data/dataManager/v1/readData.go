package dataManager_v1

import (
	"os"
)

/**
 * @param filePath -> path to file
 * @param dataStartPtr -> start position in file
 * @param dataEndPtr -> end position in file
 * @return []byte -> read binary data
 * @return error -> error if occurred
 */
func ReadDataFromFile(filePath string, dataStartPtr int, dataEndPtr int) ([]byte, error) {
	// Otwórz plik do odczytu
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Sprawdzenie poprawności zakresu
	if dataStartPtr < 0 || dataEndPtr <= dataStartPtr {
		return nil, os.ErrInvalid
	}

	// Ustawienie wskaźnika na początek danych
	_, err = file.Seek(int64(dataStartPtr), 0)
	if err != nil {
		return nil, err
	}

	// Obliczenie długości danych do odczytu
	readLength := dataEndPtr - dataStartPtr
	buffer := make([]byte, readLength)

	// Odczytanie dokładnie tylu bajtów, ile potrzeba
	n, err := file.Read(buffer)
	if err != nil {
		return nil, err
	}

	// Zwracamy dokładnie tyle bajtów, ile odczytano
	return buffer[:n], nil
}
