package dataManager_v2

func SaveDataToFileAsync(data []byte, filePath string) (int64, int64, error) {
	respChan := make(chan fileResponse, 1)
	req := fileRequest{
		op:      "write",
		data:    data,
		timeout: dataTransferTimeout(int64(len(data))),
		resp:    respChan,
	}
	resp := sendToFileWorker(filePath, req)
	return resp.startPtr, resp.endPtr, resp.err
}

func SaveDataAppendToFileAsync(data []byte, filePath string) (int64, int64, error) {
	respChan := make(chan fileResponse, 1)
	req := fileRequest{
		op:      "write_append",
		data:    data,
		timeout: dataTransferTimeout(int64(len(data))),
		resp:    respChan,
	}
	resp := sendToFileWorker(filePath, req)
	return resp.startPtr, resp.endPtr, resp.err
}
