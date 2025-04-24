package dataManager_v2

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var fileHandles sync.Map // Przechowuje uchwyty do plików

type fileHandle struct {
	file       *os.File
	lastAccess time.Time
}

func init() {
	go func() {
		for {
			time.Sleep(1 * time.Second) // Sprawdzaj co sekundę
			closeIdleHandles()
		}
	}()
}

const handleTimeout = 10 * time.Second // Czas bezczynności po którym uchwyt zostanie zamknięty

func getFileHandle(filePath string) (*os.File, error) {
	fullPath := filepath.Join(basePath, filePath)

	// Sprawdź, czy uchwyt do pliku już istnieje
	if handle, ok := fileHandles.Load(fullPath); ok {
		fh := handle.(*fileHandle)
		fh.lastAccess = time.Now() // Aktualizuj czas ostatniego użycia
		return fh.file, nil
	}

	// Jeśli uchwyt nie istnieje, otwórz plik i zapisz uchwyt
	file, err := os.OpenFile(fullPath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("błąd otwierania pliku: %w", err)
	}

	fh := &fileHandle{
		file:       file,
		lastAccess: time.Now(),
	}
	fileHandles.Store(fullPath, fh)
	return file, nil
}

func closeIdleHandles() {
	fileHandles.Range(func(key, value interface{}) bool {
		fh := value.(*fileHandle)
		if time.Since(fh.lastAccess) > handleTimeout {
			fh.file.Close()
			fileHandles.Delete(key)
		}
		return true
	})
}
