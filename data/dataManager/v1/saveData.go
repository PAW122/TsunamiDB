package dataManager_v1

import (
	"io"
	"os"
)

/**
 * @param data -> binary data to save
 * @param filePath -> path to file
 * @return startPtr -> data start position in file
 * @return endPtr -> data end position in file
 * @return error -> error if occurred
 */
func SaveDataToFile(data []byte, filePath string) (int64, int64, error) {
	// Otwórz plik w trybie dodawania (nie nadpisuje pliku)
	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return 0, 0, err
	}
	defer file.Close()

	// Pobierz aktualną wielkość pliku (gdzie zaczniemy zapis)
	startPtr, err := file.Seek(0, io.SeekEnd)
	if err != nil {
		return 0, 0, err
	}

	// Zapis danych do pliku
	n, err := file.Write(data)
	if err != nil {
		return 0, 0, err
	}

	// **Sprawdzamy, czy cała długość danych została poprawnie zapisana**
	if n != len(data) {
		return 0, 0, io.ErrShortWrite
	}

	// Pozycja końcowa danych w pliku
	endPtr := startPtr + int64(n)

	// **DEBUG: Sprawdzamy wartości pointerów**
	// println("SAVE DATA DEBUG: Start:", startPtr, "End:", endPtr, "Bytes Written:", n)

	return startPtr, endPtr, nil
}
