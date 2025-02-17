package routes

import (
	dataManager_v1 "TsunamiDB/data/dataManager/v1"
	"fmt"
	"net/http"
	"strconv"
)

/*
1. metadane
2. nie dziala odczyt
*/

func StreamMP4(w http.ResponseWriter, r *http.Request) {
	// Pobranie nazwy pliku z URL
	pathParts := ParseArgs(r.URL.Path, "stream")
	if pathParts == nil || len(pathParts) < 2 {
		http.Error(w, "Invalid URL", http.StatusBadRequest)
		return
	}
	file := pathParts[1]

	// Parsowanie nagłówka `Range`
	rangeHeader := r.Header.Get("Range")
	var start int64 = 0
	var chunkSize int64 = 1024 * 512 // Domyślnie 512 KB

	if rangeHeader != "" {
		var startByte, endByte int64
		_, err := fmt.Sscanf(rangeHeader, "bytes=%d-%d", &startByte, &endByte)
		if err == nil {
			start = startByte
			if endByte > start {
				chunkSize = endByte - start + 1
			}
		}
	}

	// **Odczyt danych przez nową funkcję**
	data, err := dataManager_v1.StreamDataFromFile(file, start, chunkSize)
	if err != nil {
		http.Error(w, "Streaming error", http.StatusInternalServerError)
		return
	}

	// Ustawienie nagłówków HTTP
	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, start+int64(len(data))-1, chunkSize))

	// Wysyłanie danych do klienta
	w.WriteHeader(http.StatusPartialContent)
	w.Write(data)
}
