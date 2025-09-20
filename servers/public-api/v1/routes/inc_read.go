package routes

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	dataManager_v2 "github.com/PAW122/TsunamiDB/data/dataManager/v2"
	fileSystem_v1 "github.com/PAW122/TsunamiDB/data/fileSystem/v1"
	incindex "github.com/PAW122/TsunamiDB/data/incIndex"
	encoder_v1 "github.com/PAW122/TsunamiDB/encoding/v1"
	encoding_v1 "github.com/PAW122/TsunamiDB/encoding/v1"
)

/*

GET /read_inc/{file}/{key}
Params:
- read_type: "by_id" | "last_entries" | "first_entries" | "by_key" (default: by_id)
	- by_id: {id}
	- last_entries: {amount_to_read}
	- first_entries: {amount_to_read}
	- by_key: {entry_key}

Response:
- 200 OK + JsonList:{decoded entries}
*/

func ReadIncremental(w http.ResponseWriter, r *http.Request, c *http.Client) {
	var read_id uint64
	var read_type_int uint8 // 0 = by id, 1 = last N entries, 2 = first N entries
	var amount_to_read uint64
	var requestedKey string

	if r.Method != "GET" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	pathParts := ParseArgs(r.URL.Path, "read_inc")
	if pathParts == nil || len(pathParts) < 2 {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "Invalid URL args")
		return
	}

	file := pathParts[2]
	key := pathParts[3]

	read_type := r.Header.Get("read_type")
	if read_type == "" {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "Missing read_type header")
		return
	}

	switch read_type {
	case "by_id":
		raw_id := r.Header.Get("id")
		if raw_id == "" {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, "Missing id header")
			return
		}

		id, err := strconv.ParseUint(raw_id, 10, 64)
		if err != nil {
			http.Error(w, "Invalid header value", http.StatusBadRequest)
			return
		}

		read_id = id
		read_type_int = 0
	case "last_entries", "first_entries":
		raw_amount := r.Header.Get("amount_to_read")
		if raw_amount == "" {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, "Missing amount_to_read header")
			return
		}
		amount, err := strconv.ParseUint(raw_amount, 10, 64)
		if err != nil {
			http.Error(w, "Invalid header value", http.StatusBadRequest)
			return
		}
		amount_to_read = amount

		if read_type == "first_entries" {
			read_type_int = 2
		} else {
			read_type_int = 1
		}
	case "by_key":
		requestedKey = r.Header.Get("entry_key")
		if requestedKey == "" {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, "Missing entry_key header")
			return
		}
		read_type_int = 3
	default:
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "Unsupported read_type: %s", read_type)
		return
	}

	fsData, err := fileSystem_v1.GetElementByKey(file, key)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "Error: "+err.Error())
		return
	}

	data, err := dataManager_v2.ReadDataFromFileAsync(
		file,
		int64(fsData.StartPtr),
		int64(fsData.EndPtr),
	)

	decodedObj := encoder_v1.Decode(data)

	raw_table_data, err := BytesToStructBinary([]byte(decodedObj.Data))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "Cannot deserialize inc table data: "+err.Error())
		return
	}

	if read_type_int == 3 {
		pos, ok, err := incindex.Lookup(raw_table_data.TableFileName, requestedKey)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, "Index lookup error: "+err.Error())
			return
		}
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, "Entry not found")
			return
		}
		read_id = pos
		read_type_int = 0
	}

	// req odczytania danych z inc_table
	if read_type_int == 0 {
		raw, err := dataManager_v2.ReadIncDataFromFileAsync_ById(raw_table_data.TableFileName, read_id, raw_table_data.EntrySize)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, "Entry not found")
			return
		}
		if len(raw) == 0 {
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, "Entry not found")
			return
		}
		entry, err := encoder_v1.DecodeIncEntry(raw_table_data.EntrySize, raw)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, "Error decoding entry: "+err.Error())
			return
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"data":"%s"}`, entry.Data)
		return
		// raw ma długość (entrySize+3). Zdekodujesz przez DecodeIncEntry(entrySize, raw).
	} else if read_type_int == 2 {
		raw, err := dataManager_v2.ReadIncDataFromFileAsync_FirstEntries(
			raw_table_data.TableFileName,
			amount_to_read,
			raw_table_data.EntrySize,
		)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, "Error reading entries: "+err.Error())
			return
		}

		recordSize := int(raw_table_data.EntrySize) + 3
		if recordSize <= 0 {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, "Invalid entry size")
			return
		}
		if len(raw)%recordSize != 0 {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "Corrupted read: data len=%d not divisible by recordSize=%d", len(raw), recordSize)
			return
		}

		// Struktura odpowiedzi (ID + dane; []byte zserializuje się do base64 w JSON)
		type IncEntryJSON struct {
			ID   uint64 `json:"id"`
			Data string `json:"data"`
			// jeśli chcesz debug: Skip/Next:
			// Skip bool   `json:"skip"`
			// Next uint64 `json:"next,omitempty"`
		}

		entries := make([]IncEntryJSON, 0, len(raw)/recordSize)

		// Pierwsze N rekordów → ich ID to 0..N-1
		for i := 0; i < len(raw)/recordSize; i++ {
			chunk := raw[i*recordSize : (i+1)*recordSize]

			dec, err := encoding_v1.DecodeIncEntry(raw_table_data.EntrySize, chunk)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprint(w, "DecodeIncEntry error: "+err.Error())
				return
			}
			if dec.SkipBit {
				// zgodnie z założeniem: pomijamy wpisy oznaczone skip
				continue
			}

			entries = append(entries, IncEntryJSON{
				ID:   uint64(i),
				Data: string(dec.Data),
				// Skip: dec.SkipBit,
				// Next: dec.NextEntryPointer,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(entries); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, "JSON encode error: "+err.Error())
			return
		}
	} else {
		raw, err := dataManager_v2.ReadIncDataFromFileAsync_LastEntries(
			raw_table_data.TableFileName,
			amount_to_read,
			raw_table_data.EntrySize,
		)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, "Error reading entries: "+err.Error())
			return
		}

		recordSize := int(raw_table_data.EntrySize) + 3
		if recordSize <= 0 {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, "Invalid entry size")
			return
		}
		if len(raw)%recordSize != 0 {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "Corrupted read: data len=%d not divisible by recordSize=%d", len(raw), recordSize)
			return
		}

		type IncEntryJSON struct {
			ID   uint64 `json:"id"`   // 0 = najnowszy w tej odpowiedzi
			Data string `json:"data"` // []byte → base64 w JSON
			// Skip bool   `json:"skip"`
			// Next uint64 `json:"next,omitempty"`
		}

		count := len(raw) / recordSize
		entries := make([]IncEntryJSON, 0, count)

		// Worker zwraca newest→oldest, więc i=0 to najnowszy rekord w buforze.
		for i := 0; i < count; i++ {
			chunk := raw[i*recordSize : (i+1)*recordSize]

			dec, err := encoding_v1.DecodeIncEntry(raw_table_data.EntrySize, chunk)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprint(w, "DecodeIncEntry error: "+err.Error())
				return
			}
			if dec.SkipBit {
				// Pomijamy wpisy oznaczone jako skip
				continue
			}

			entries = append(entries, IncEntryJSON{
				ID:   uint64(i), // lokalny indeks: 0 = najnowszy
				Data: string(dec.Data),
				// Skip: dec.SkipBit,
				// Next: dec.NextEntryPointer,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(entries); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, "JSON encode error: "+err.Error())
			return
		}
	}
	// 200 OK + JSON list

}
