package pprof_dumper

import (
	"fmt"
	"os"
	"os/exec"
	"runtime/pprof"
	"time"
)

// ensureDir - tworzy katalog, jeśli nie istnieje
func ensureDir(dir string) error {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return os.MkdirAll(dir, 0755)
	}
	return nil
}

// DumpCPUProfile - zapisuje profil CPU do pliku
func DumpCPUProfile(filePath string) error {
	f, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("błąd tworzenia pliku CPU profile: %v", err)
	}
	defer f.Close()

	// Zbieranie profilu CPU przez 30 sekund
	if err := pprof.StartCPUProfile(f); err != nil {
		return fmt.Errorf("błąd uruchamiania CPU profile: %v", err)
	}
	time.Sleep(30 * time.Second) // Czas zbierania danych
	pprof.StopCPUProfile()

	// Konwersja na czytelny format
	return ConvertToReadableFormat(filePath, "cpu")
}

// DumpHeapProfile - zapisuje profil pamięci (heap) do pliku
func DumpHeapProfile(filePath string) error {
	f, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("błąd tworzenia pliku heap profile: %v", err)
	}
	defer f.Close()

	// Zapis profilu pamięci
	if err := pprof.WriteHeapProfile(f); err != nil {
		return fmt.Errorf("błąd zapisu heap profile: %v", err)
	}

	// Konwersja na czytelny format
	return ConvertToReadableFormat(filePath, "heap")
}

// DumpGoroutineProfile - zapisuje profil gorutyn do pliku
func DumpGoroutineProfile(filePath string) error {
	f, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("błąd tworzenia pliku goroutine profile: %v", err)
	}
	defer f.Close()

	// Zapis profilu gorutyn
	profile := pprof.Lookup("goroutine")
	if profile == nil {
		return fmt.Errorf("profil gorutyn nie istnieje")
	}
	if err := profile.WriteTo(f, 0); err != nil {
		return fmt.Errorf("błąd zapisu goroutine profile: %v", err)
	}

	// Konwersja na czytelny format
	return ConvertToReadableFormat(filePath, "goroutine")
}

// DumpBlockProfile - zapisuje profil blokad do pliku
func DumpBlockProfile(filePath string) error {
	f, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("błąd tworzenia pliku block profile: %v", err)
	}
	defer f.Close()

	// Zapis profilu blokad
	profile := pprof.Lookup("block")
	if profile == nil {
		return fmt.Errorf("profil blokad nie istnieje")
	}
	if err := profile.WriteTo(f, 0); err != nil {
		return fmt.Errorf("błąd zapisu block profile: %v", err)
	}

	// Konwersja na czytelny format
	return ConvertToReadableFormat(filePath, "block")
}

// ConvertToReadableFormat - konwertuje plik dumpu na czytelny format (tekstowy i graficzny)
func ConvertToReadableFormat(filePath string, profileType string) error {
	// Konwersja na format tekstowy
	textFilePath := filePath + ".txt"
	cmdText := exec.Command("go", "tool", "pprof", "-text", filePath)
	textFile, err := os.Create(textFilePath)
	if err != nil {
		return fmt.Errorf("błąd tworzenia pliku tekstowego: %v", err)
	}
	defer textFile.Close()
	cmdText.Stdout = textFile
	if err := cmdText.Run(); err != nil {
		return fmt.Errorf("błąd konwersji na tekst: %v", err)
	}

	// Konwersja na format graficzny (SVG)
	svgFilePath := filePath + ".svg"
	cmdSVG := exec.Command("go", "tool", "pprof", "-svg", filePath)
	svgFile, err := os.Create(svgFilePath)
	if err != nil {
		return fmt.Errorf("błąd tworzenia pliku SVG: %v", err)
	}
	defer svgFile.Close()
	cmdSVG.Stdout = svgFile
	if err := cmdSVG.Run(); err != nil {
		return fmt.Errorf("błąd konwersji na SVG: %v", err)
	}

	fmt.Printf("Profil %s został zapisany jako tekst (%s) i SVG (%s)\n", profileType, textFilePath, svgFilePath)
	return nil
}

// StartAutomaticDump - uruchamia automatyczne dumpowanie danych co określony czas
func StartAutomaticDump(interval time.Duration, outputDir string) {
	// Upewnij się, że katalog istnieje
	if err := ensureDir(outputDir); err != nil {
		fmt.Printf("Błąd tworzenia katalogu dumpów: %v\n", err)
		return
	}

	go func() {
		for {
			timestamp := time.Now().Format("2006-01-02_15-04-05")

			// Dump CPU profile
			cpuFile := fmt.Sprintf("%s/cpu_profile_%s.prof", outputDir, timestamp)
			if err := DumpCPUProfile(cpuFile); err != nil {
				fmt.Printf("Błąd dumpowania CPU profile: %v\n", err)
			}

			// Dump heap profile
			heapFile := fmt.Sprintf("%s/heap_profile_%s.prof", outputDir, timestamp)
			if err := DumpHeapProfile(heapFile); err != nil {
				fmt.Printf("Błąd dumpowania heap profile: %v\n", err)
			}

			// Dump goroutine profile
			goroutineFile := fmt.Sprintf("%s/goroutine_profile_%s.prof", outputDir, timestamp)
			if err := DumpGoroutineProfile(goroutineFile); err != nil {
				fmt.Printf("Błąd dumpowania goroutine profile: %v\n", err)
			}

			// Dump block profile
			blockFile := fmt.Sprintf("%s/block_profile_%s.prof", outputDir, timestamp)
			if err := DumpBlockProfile(blockFile); err != nil {
				fmt.Printf("Błąd dumpowania block profile: %v\n", err)
			}

			// Poczekaj na kolejną iterację
			time.Sleep(interval)
		}
	}()
}
