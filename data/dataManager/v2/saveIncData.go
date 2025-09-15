package dataManager_v2

import "encoding/binary"

// push nowego elementu do table
// w przypadku inc_table fileResponse.data będzie == uint64 id wpisu
func SaveIncDataToFileAsync(data []byte, filePath string, entry_size uint64) (uint64, error) {
	respChan := make(chan fileResponse, 1)
	req := fileRequest{
		op:        "write_inc",
		data:      data,
		entrySize: entry_size,
		resp:      respChan,
	}
	resp := sendToFileWorker(filePath, req)
	id := binary.LittleEndian.Uint64(resp.data)
	return id, resp.err
}

// allows you to enter a new element anywhere in inc_table as long as it is not a new id
func SaveIncDataToFileAsync_Put(data []byte, filePath string, entry_size uint64, pref_id uint64, count_from string) (uint64, error) {
	respChan := make(chan fileResponse, 1)
	req := fileRequest{
		op:         "write_inc_ow", // overwrite if exists
		data:       data,
		entrySize:  entry_size,
		inc_id:     pref_id, // custom id
		read_type:  0,       // 0 = append
		count_from: count_from,
		resp:       respChan,
	}
	resp := sendToFileWorker(filePath, req)
	id := binary.LittleEndian.Uint64(resp.data)
	return id, resp.err
}

// overwriting an existing inc_table entry with a given id
func SaveIncDataToFileAsync_OverWrite(data []byte, filePath string, entry_size uint64, pref_id uint64, count_from string) (uint64, error) {
	respChan := make(chan fileResponse, 1)
	req := fileRequest{
		op:         "write_inc_ow", // overwrite if exists
		data:       data,
		entrySize:  entry_size,
		inc_id:     pref_id, // custom id
		read_type:  1,       // 1 = overwrite existing
		count_from: count_from,
		resp:       respChan,
	}
	resp := sendToFileWorker(filePath, req)
	id := binary.LittleEndian.Uint64(resp.data)
	return id, resp.err
}

// DeleteIncTableFile removes the file backing an incremental table via the file worker.
func DeleteIncTableFile(filePath string) error {
	respChan := make(chan fileResponse, 1)
	req := fileRequest{
		op:   "delete_inc",
		resp: respChan,
	}
	resp := sendToFileWorker(filePath, req)
	return resp.err
}
