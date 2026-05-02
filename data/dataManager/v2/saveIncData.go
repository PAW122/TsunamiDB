package dataManager_v2

import (
	"encoding/binary"
	"errors"
)

// push nowego elementu do table
// w przypadku inc_table fileResponse.data będzie == uint64 id wpisu
func SaveIncDataToFileAsync(data []byte, filePath string, entry_size uint64) (uint64, error) {
	respChan := make(chan fileResponse, 1)
	req := fileRequest{
		op:        "write_inc",
		data:      data,
		entrySize: entry_size,
		timeout:   dataTransferTimeout(int64(len(data))),
		resp:      respChan,
	}
	resp := sendToFileWorker(filePath, req)
	if resp.err != nil {
		return 0, resp.err
	}
	if len(resp.data) < 8 {
		return 0, errors.New("write_inc: missing id in worker response")
	}
	id := binary.LittleEndian.Uint64(resp.data)
	return id, nil
}

func SaveIncDataBatchToFileAsync(batch [][]byte, filePath string, entry_size uint64) ([]uint64, error) {
	respChan := make(chan fileResponse, 1)
	totalBytes := int64(0)
	for _, data := range batch {
		totalBytes += int64(len(data))
	}
	req := fileRequest{
		op:        "write_inc_batch",
		dataBatch: batch,
		entrySize: entry_size,
		timeout:   dataTransferTimeout(totalBytes),
		resp:      respChan,
	}
	resp := sendToFileWorker(filePath, req)
	if resp.err != nil {
		return nil, resp.err
	}
	if len(resp.data)%8 != 0 {
		return nil, errors.New("write_inc_batch: invalid ids in worker response")
	}
	ids := make([]uint64, len(resp.data)/8)
	for i := range ids {
		ids[i] = binary.LittleEndian.Uint64(resp.data[i*8 : (i+1)*8])
	}
	return ids, nil
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
	if resp.err != nil {
		return 0, resp.err
	}
	if len(resp.data) < 8 {
		return 0, errors.New("write_inc_ow: missing id in worker response")
	}
	id := binary.LittleEndian.Uint64(resp.data)
	return id, nil
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
	if resp.err != nil {
		return 0, resp.err
	}
	if len(resp.data) < 8 {
		return 0, errors.New("write_inc_ow: missing id in worker response")
	}
	id := binary.LittleEndian.Uint64(resp.data)
	return id, nil
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
