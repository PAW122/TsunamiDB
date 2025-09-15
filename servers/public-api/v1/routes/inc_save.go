package routes

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

	dataManager_v2 "github.com/PAW122/TsunamiDB/data/dataManager/v2"
	defragmentationManager "github.com/PAW122/TsunamiDB/data/defragmentationManager"
	fileSystem_v1 "github.com/PAW122/TsunamiDB/data/fileSystem/v1"
	encoder_v1 "github.com/PAW122/TsunamiDB/encoding/v1"
	debug "github.com/PAW122/TsunamiDB/servers/debug"
	subServer "github.com/PAW122/TsunamiDB/servers/subscriptions"
	types "github.com/PAW122/TsunamiDB/types"
)

// [uint64 EntrySize][uint32 nameLen][nameLen bytes of TableFileName]
func StructToBytesBinary(s types.IncTableEntryData) ([]byte, error) {
	var buf bytes.Buffer

	// 1) stałe pole
	if err := binary.Write(&buf, binary.LittleEndian, s.EntrySize); err != nil {
		return nil, err
	}

	// 2) string jako length-prefixed
	nameBytes := []byte(s.TableFileName)
	if len(nameBytes) > int(^uint32(0)) {
		return nil, errors.New("TableFileName too long")
	}
	nameLen := uint32(len(nameBytes))
	if err := binary.Write(&buf, binary.LittleEndian, nameLen); err != nil {
		return nil, err
	}
	if _, err := buf.Write(nameBytes); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func BytesToStructBinary(b []byte) (types.IncTableEntryData, error) {
	var out types.IncTableEntryData
	reader := bytes.NewReader(b)

	// 1) EntrySize
	if err := binary.Read(reader, binary.LittleEndian, &out.EntrySize); err != nil {
		return out, err
	}

	// 2) długość nazwy + sama nazwa
	var nameLen uint32
	if err := binary.Read(reader, binary.LittleEndian, &nameLen); err != nil {
		return out, err
	}
	if nameLen > uint32(reader.Len()) {
		return out, errors.New("corrupted payload: nameLen exceeds buffer")
	}
	nameBytes := make([]byte, nameLen)
	if _, err := reader.Read(nameBytes); err != nil {
		return out, err
	}
	out.TableFileName = string(nameBytes)

	return out, nil
}

/*
POST /save_inc/<table>/<key>
body = []bytes r.Body
headers:

	max_entry_size = <uint64>
	*id = <uint64> (id służy do nadpisania wpisu o danym id; jeżeli nie istnieje, zwróci błąd)
	*mode = append | overwrite
	*count_from = top | bottom [default = top]
		> switching to "bottom" allows you to use for example id 1 instead of some high number

response:

	200 OK
		//id które zoastało przypisane do tego wpisu
		body: {
			"id": "<unique_id>"
			}
	400 Bad Request
	405 Method Not Allowed
	500 Internal Server Error

todo:
  - walidacja czy table istnieje
  - jeżeli tabela już istnieje, walidacja czy rozmiar z headerów się zgadza
*/
func SaveIncremental(w http.ResponseWriter, r *http.Request, client *http.Client) {
	var startPtr, endPtr int64
	var saveErr error
	var user_custom_id bool = false

	var inc_table_exists bool = false
	var inc_table_data types.IncTableEntryData
	var warningMsg string

	defer debug.MeasureTime("> api [SaveInc]")()

	if r.Method != "POST" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	pathParts := ParseArgs(r.URL.Path, "save_inc")
	if len(pathParts) < 2 {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Print(w, "Invalid url args")
		return
	}

	file := pathParts[2]
	key := pathParts[3]

	size_header := r.Header.Get("max_entry_size")
	var (
		headerProvided     bool
		requestedEntrySize uint64
		entry_size         uint64
	)
	if size_header != "" {
		headerProvided = true
		size, err := strconv.ParseUint(size_header, 10, 64)
		if err != nil {
			http.Error(w, "Invalid header value", http.StatusBadRequest)
			return
		}
		reqSize := size
		requestedEntrySize = reqSize
		entry_size = reqSize
	}

	id_header := r.Header.Get("id")
	var entry_id uint64
	if id_header != "" {
		parsedID, err := strconv.ParseUint(id_header, 10, 64)
		if err == nil {
			user_custom_id = true
			entry_id = parsedID
		}
	}

	mode_header := r.Header.Get("mode")
	if mode_header != "append" && mode_header != "overwrite" {
		mode_header = "append" // default
	}

	count_from_header := r.Header.Get("count_from")
	if count_from_header != "top" && count_from_header != "bottom" {
		count_from_header = "top"
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "Invalid body")
		return
	}

	// czy table istnieje?
	fsData, err := fileSystem_v1.GetElementByKey(key)
	if err != nil {
		// nie ma tabeli, stworzy nową
		// 1. zapisać dane wpisu w KV
		new_inc_table_data := types.IncTableEntryData{
			EntrySize:     entry_size,
			TableFileName: fmt.Sprintf("inc_table_%s.tbl", key),
		}
		inc_table_data = new_inc_table_data
		inc_table_exists = true
		entry_size = inc_table_data.EntrySize

		// struktura do byte
		byte_body, err := StructToBytesBinary(new_inc_table_data)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, "Cannot serialize inc table data: "+err.Error())
			return
		}
		encoded, _ := encoder_v1.Encode(byte_body)
		startPtr, endPtr, saveErr = dataManager_v2.SaveDataToFileAsync(encoded, file)

		if saveErr != nil {
			fmt.Println(saveErr)
			http.Error(w, "Error saving to file", http.StatusInternalServerError)
			return
		}

		prevMeta, existed, err := fileSystem_v1.SaveElementByKey(key, file, int(startPtr), int(endPtr))
		if err != nil {
			fmt.Println(err)
			http.Error(w, "Error saving metadata", http.StatusInternalServerError)
			return
		}
		if existed {
			if prevMeta.FileName != file || prevMeta.StartPtr != int(startPtr) || prevMeta.EndPtr != int(endPtr) {
				defragmentationManager.MarkAsFree(prevMeta.Key, prevMeta.FileName, int64(prevMeta.StartPtr), int64(prevMeta.EndPtr))
				fileSystem_v1.RecordDefragFree()
			} else {
				fileSystem_v1.RecordDefragSkip()
			}
		}

		// dade zapisane, nie robie subServer.NotifySubscribers
		// bo nie ma żadnego powodu żeby user dostawał te dane
	}

	// trzeba wyciągnąć dane
	if inc_table_exists == false {
		data, err := dataManager_v2.ReadDataFromFileAsync(
			file,
			int64(fsData.StartPtr),
			int64(fsData.EndPtr),
		)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, "Cannot read inc table data: "+err.Error())
			return
		}

		decodedObj := encoder_v1.Decode(data)

		raw_table_data, err := BytesToStructBinary([]byte(decodedObj.Data))
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, "Cannot deserialize inc table data: "+err.Error())
			return
		}

		inc_table_data = raw_table_data
		entry_size = inc_table_data.EntrySize
		if headerProvided && requestedEntrySize != inc_table_data.EntrySize {
			warningMsg = fmt.Sprintf("max_entry_size header (%d) does not match existing table (%d); header ignored", requestedEntrySize, inc_table_data.EntrySize)
		}
	}

	// mamy już dane o inc_table
	// req o zapisanie danych w inc_table_data
	// jeżeli istnieje to czy rozmiar się zgadza?

	if len(body) > int(entry_size) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "Body size exceeds entry size")
		return
	}

	// req o zapisanie danych w inc_table
	encoded_inc_body := encoder_v1.EncodeIncEntry(entry_size, body)

	if user_custom_id == false {
		id, err := dataManager_v2.SaveIncDataToFileAsync(encoded_inc_body, inc_table_data.TableFileName, entry_size)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, "Error saving inc entry: "+err.Error())
			return
		}

		go subServer.NotifyIncTableSubscribers(key, "add", id, body)

		// zwrócenie id
		respondWithIncID(w, id, warningMsg)

	} else {

		// mode 1
		if mode_header == "overwrite" {
			id, err := dataManager_v2.SaveIncDataToFileAsync_OverWrite(encoded_inc_body, inc_table_data.TableFileName, entry_size, entry_id, count_from_header)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprint(w, "Error saving inc entry: "+err.Error())
				return
			}

			go subServer.NotifyIncTableSubscribers(key, "overwrite", id, body)

			// zwrócenie id
			respondWithIncID(w, id, warningMsg)

		} else { // append mode [0]
			id, err := dataManager_v2.SaveIncDataToFileAsync_Put(encoded_inc_body, inc_table_data.TableFileName, entry_size, entry_id, count_from_header)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprint(w, "Error saving inc entry: "+err.Error())
				return
			}

			go subServer.NotifyIncTableSubscribers(key, "insert", id, body)

			// zwrócenie id
			respondWithIncID(w, id, warningMsg)

		}
	}

}

func respondWithIncID(w http.ResponseWriter, id uint64, warning string) {
	w.Header().Set("Content-Type", "application/json")
	resp := map[string]string{
		"id": fmt.Sprintf("%d", id),
	}
	if warning != "" {
		resp["warning"] = warning
	}
	_ = json.NewEncoder(w).Encode(resp)
}
