//go:build !bentchmark

package logger

// InitLogger inicjalizuje logger i otwiera plik logów
func InitLogger(filePath string, enable bool) error {
	return nil
}

func logWorker() {

}

// Log zapisuje wiadomość do pliku logów
func Log(format string, v ...interface{}) {

}

// MeasureTime - mierzy czas wykonania i zapisuje do statystyk
func MeasureTime(name string) func() {
	return func() {
		// Zakończenie pomiaru czasu
	}
}

// PrintTimingStats - wyświetla średnie czasy wykonania dla wszystkich funkcji
func PrintTimingStats() {

}

// CloseLogger zamyka plik logów
func CloseLogger() {

}
