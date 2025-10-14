package ui

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	dataManager_v2 "github.com/PAW122/TsunamiDB/data/dataManager/v2"
	fileSystem_v1 "github.com/PAW122/TsunamiDB/data/fileSystem/v1"
	encoder_v1 "github.com/PAW122/TsunamiDB/encoding/v1"
	routes "github.com/PAW122/TsunamiDB/servers/public-api/v1/routes"
)

type tableListResponse struct {
	Tables []string `json:"tables"`
}

type entryPreview struct {
	Key             string `json:"key"`
	HasNested       bool   `json:"has_nested"`
	Size            int    `json:"size"`
	Preview         string `json:"preview"`
	PreviewIsBinary bool   `json:"preview_is_binary"`
	DataIsJSON      bool   `json:"data_is_json"`
	StartPtr        int    `json:"start_ptr"`
	EndPtr          int    `json:"end_ptr"`
}

type entriesResponse struct {
	Entries    []entryPreview `json:"entries"`
	NextCursor string         `json:"next_cursor,omitempty"`
}

type entryDetail struct {
	Table      string          `json:"table"`
	Key        string          `json:"key"`
	Data       string          `json:"data"`
	DataIsJSON bool            `json:"data_is_json"`
	IsBinary   bool            `json:"is_binary"`
	HasNested  bool            `json:"has_nested"`
	Size       int             `json:"size"`
	StartPtr   int             `json:"start_ptr"`
	EndPtr     int             `json:"end_ptr"`
	Nested     *nestedSnapshot `json:"nested,omitempty"`
}

type incDescriptor struct {
	Table     string `json:"table"`
	Key       string `json:"key"`
	File      string `json:"file"`
	EntrySize uint64 `json:"entry_size"`
}

type incDescriptorsResponse struct {
	Descriptors []incDescriptor `json:"descriptors"`
}

type incEntry struct {
	ID   uint64 `json:"id"`
	Data string `json:"data"`
}

type incEntriesResponse struct {
	Descriptor incDescriptor `json:"descriptor"`
	Entries    []incEntry    `json:"entries"`
}

func handleTablesList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	tables, err := fileSystem_v1.ListTables()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, tableListResponse{Tables: tables})
}

func handleIncDescriptors(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	descriptors, err := discoverIncDescriptors()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, incDescriptorsResponse{Descriptors: descriptors})
}

func handleTableEntries(w http.ResponseWriter, r *http.Request, table string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	after := r.URL.Query().Get("cursor")
	limit := parseLimit(r.URL.Query().Get("limit"), 100, 200)

	var filter *regexp.Regexp
	if pattern := strings.TrimSpace(r.URL.Query().Get("regex")); pattern != "" {
		rx, err := regexp.Compile(pattern)
		if err != nil {
			http.Error(w, "invalid regex", http.StatusBadRequest)
			return
		}
		filter = rx
	}

	entries, next, err := fileSystem_v1.ListTableEntries(table, after, limit, filter)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	previews := make([]entryPreview, 0, len(entries))
	for _, meta := range entries {
		raw, err := dataManager_v2.ReadDataFromFileAsync(
			meta.FileName,
			int64(meta.StartPtr),
			int64(meta.EndPtr),
		)
		if err != nil {
			continue
		}
		decoded := encoder_v1.Decode(raw)

		previewText, isBinary := summarisePayload(decoded.Data)
		isJSON := isJSONObject(decoded.Data)

		previews = append(previews, entryPreview{
			Key:             meta.Key,
			HasNested:       meta.HasNested,
			Size:            meta.EndPtr - meta.StartPtr,
			Preview:         previewText,
			PreviewIsBinary: isBinary,
			DataIsJSON:      isJSON,
			StartPtr:        meta.StartPtr,
			EndPtr:          meta.EndPtr,
		})
	}

	writeJSON(w, entriesResponse{
		Entries:    previews,
		NextCursor: next,
	})
}

func handleEntryDetail(w http.ResponseWriter, r *http.Request, table, key string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	meta, err := fileSystem_v1.GetElementByKey(table, key)
	if err != nil {
		http.Error(w, "entry not found", http.StatusNotFound)
		return
	}

	raw, err := dataManager_v2.ReadDataFromFileAsync(
		meta.FileName,
		int64(meta.StartPtr),
		int64(meta.EndPtr),
	)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	decoded := encoder_v1.Decode(raw)

	dataText := decoded.Data
	isBinary := isProbablyBinary(dataText)
	isJSON := isJSONObject(dataText)

	detail := entryDetail{
		Table:      table,
		Key:        key,
		Data:       dataText,
		DataIsJSON: isJSON,
		IsBinary:   isBinary,
		HasNested:  meta.HasNested,
		Size:       meta.EndPtr - meta.StartPtr,
		StartPtr:   meta.StartPtr,
		EndPtr:     meta.EndPtr,
	}

	if meta.HasNested {
		snapshot, err := buildNestedSnapshot(table, dataText)
		if err == nil {
			detail.Nested = snapshot
		}
	}

	writeJSON(w, detail)
}

func handleIncEntries(w http.ResponseWriter, r *http.Request, table, key string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	mode := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("mode")))
	if mode == "" {
		mode = "first"
	}
	limit := parseLimit(r.URL.Query().Get("limit"), 100, 500)

	desc, err := loadIncDescriptor(table, key)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var (
		raw []byte
		res incEntriesResponse
	)

	switch mode {
	case "first":
		raw, err = dataManager_v2.ReadIncDataFromFileAsync_FirstEntries(
			desc.File,
			uint64(limit),
			desc.EntrySize,
		)
	case "last":
		raw, err = dataManager_v2.ReadIncDataFromFileAsync_LastEntries(
			desc.File,
			uint64(limit),
			desc.EntrySize,
		)
	default:
		http.Error(w, "unsupported mode", http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	recordSize := int(desc.EntrySize) + 3
	if recordSize <= 3 || len(raw)%recordSize != 0 {
		http.Error(w, "corrupted incremental data", http.StatusInternalServerError)
		return
	}

	entries := make([]incEntry, 0, len(raw)/recordSize)
	for i := 0; i < len(raw)/recordSize; i++ {
		chunk := raw[i*recordSize : (i+1)*recordSize]
		decoded, err := encoder_v1.DecodeIncEntry(desc.EntrySize, chunk)
		if err != nil || decoded.SkipBit {
			continue
		}
		entries = append(entries, incEntry{
			ID:   uint64(i),
			Data: string(decoded.Data),
		})
	}

	res.Descriptor = desc
	res.Entries = entries

	writeJSON(w, res)
}

func loadIncDescriptor(table, key string) (incDescriptor, error) {
	meta, err := fileSystem_v1.GetElementByKey(table, key)
	if err != nil {
		return incDescriptor{}, err
	}

	raw, err := dataManager_v2.ReadDataFromFileAsync(
		meta.FileName,
		int64(meta.StartPtr),
		int64(meta.EndPtr),
	)
	if err != nil {
		return incDescriptor{}, err
	}
	decoded := encoder_v1.Decode(raw)

	info, err := routes.BytesToStructBinary([]byte(decoded.Data))
	if err != nil || info.EntrySize == 0 || strings.TrimSpace(info.TableFileName) == "" {
		return incDescriptor{}, errors.New("entry is not an incremental descriptor")
	}

	return incDescriptor{
		Table:     table,
		Key:       key,
		File:      info.TableFileName,
		EntrySize: info.EntrySize,
	}, nil
}

func discoverIncDescriptors() ([]incDescriptor, error) {
	tables, err := fileSystem_v1.ListTables()
	if err != nil {
		return nil, err
	}

	descriptors := make([]incDescriptor, 0)
	for _, table := range tables {
		entries, _, err := fileSystem_v1.ListTableEntries(table, "", 0, nil)
		if err != nil {
			continue
		}
		for _, meta := range entries {
			desc, err := loadIncDescriptor(table, meta.Key)
			if err != nil {
				continue
			}
			descriptors = append(descriptors, desc)
		}
	}

	sort.Slice(descriptors, func(i, j int) bool {
		if descriptors[i].Table == descriptors[j].Table {
			return descriptors[i].Key < descriptors[j].Key
		}
		return descriptors[i].Table < descriptors[j].Table
	})
	return descriptors, nil
}

func parseLimit(raw string, def, max int) int {
	if raw == "" {
		return def
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return def
	}
	if value > max {
		return max
	}
	return value
}

func summarisePayload(data string) (string, bool) {
	if data == "" {
		return "", false
	}
	if isProbablyBinary(data) {
		encoded := base64.StdEncoding.EncodeToString([]byte(data))
		if len(encoded) > 512 {
			return encoded[:512] + "…", true
		}
		return encoded, true
	}
	if len(data) > 512 {
		return data[:512] + "…", false
	}
	return data, false
}

func isProbablyBinary(data string) bool {
	for _, r := range data {
		if r == 0 {
			return true
		}
		if r < 0x09 {
			return true
		}
	}
	return false
}

func isJSONObject(data string) bool {
	if strings.TrimSpace(data) == "" {
		return false
	}
	var tmp interface{}
	if err := json.Unmarshal([]byte(data), &tmp); err != nil {
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func parseSegments(path, prefix string) ([]string, error) {
	if !strings.HasPrefix(path, prefix) {
		return nil, errors.New("invalid path")
	}
	raw := strings.TrimPrefix(path, prefix)
	raw = strings.Trim(raw, "/")
	if raw == "" {
		return []string{}, nil
	}
	parts := strings.Split(raw, "/")
	decoded := make([]string, 0, len(parts))
	for _, part := range parts {
		val, err := url.PathUnescape(part)
		if err != nil {
			return nil, err
		}
		decoded = append(decoded, val)
	}
	return decoded, nil
}
