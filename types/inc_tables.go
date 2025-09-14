package types

// dane inc-table wewnątrz wpisu w KV store
type IncTableEntryData struct {
	EntrySize     uint64 `json:"entry_size"` // maksymalny rozmiar wpisu
	TableFileName string `json:"table_file"` // plik z tabelą przyrostową
}

type IncTableBody struct {
	Data             []byte
	EntrySize        uint64
	SkipBit          bool
	NextEntryPointer uint64
}
