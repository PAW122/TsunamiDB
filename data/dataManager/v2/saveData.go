package dataManager_v2

func SaveDataToFileAsync(data []byte, filePath string) (int64, int64, error) {
	respChan := make(chan fileResponse, 1)
	req := fileRequest{
		op:   "write",
		data: data,
		resp: respChan,
	}
	resp := sendToFileWorker(filePath, req)
	return resp.startPtr, resp.endPtr, resp.err
}
