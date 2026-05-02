package dataManager_v2

func ReadIncDataFromFileAsync_ById(filePath string, id uint64, entrySize uint64) ([]byte, error) {
	respChan := make(chan fileResponse, 1)
	req := fileRequest{
		op:        "read_inc",
		entrySize: entrySize,
		inc_id:    id,
		read_type: 0,
		timeout:   dataTransferTimeout(int64(entrySize) + 3),
		resp:      respChan,
	}
	resp := sendToFileWorker(filePath, req)
	if resp.err != nil {
		return nil, resp.err
	}

	return resp.data, nil
}

func ReadIncDataFromFileAsync_LastEntries(filePath string, amount_to_read uint64, entrySize uint64) ([]byte, error) {
	respChan := make(chan fileResponse, 1)
	req := fileRequest{
		op:        "read_inc",
		entrySize: entrySize,
		inc_id:    amount_to_read, // id używane jako ilość wpisów do odczytania
		read_type: 1,
		timeout:   dataTransferTimeout(int64(amount_to_read * (entrySize + 3))),
		resp:      respChan,
	}
	resp := sendToFileWorker(filePath, req)
	if resp.err != nil {
		return nil, resp.err
	}

	return resp.data, nil
}

func ReadIncDataFromFileAsync_FirstEntries(filePath string, amount_to_read uint64, entrySize uint64) ([]byte, error) {
	respChan := make(chan fileResponse, 1)
	req := fileRequest{
		op:        "read_inc",
		entrySize: entrySize,
		inc_id:    amount_to_read, // id używane jako ilość wpisów do odczytania
		read_type: 2,
		timeout:   dataTransferTimeout(int64(amount_to_read * (entrySize + 3))),
		resp:      respChan,
	}
	resp := sendToFileWorker(filePath, req)
	if resp.err != nil {
		return nil, resp.err
	}

	return resp.data, nil
}

func ReadIncDataFromFileAsync_Range(filePath string, startID uint64, amount_to_read uint64, entrySize uint64) ([]byte, error) {
	respChan := make(chan fileResponse, 1)
	req := fileRequest{
		op:        "read_inc",
		entrySize: entrySize,
		inc_id:    startID,
		amount:    amount_to_read,
		read_type: 3,
		timeout:   dataTransferTimeout(int64(amount_to_read * (entrySize + 3))),
		resp:      respChan,
	}
	resp := sendToFileWorker(filePath, req)
	if resp.err != nil {
		return nil, resp.err
	}

	return resp.data, nil
}
