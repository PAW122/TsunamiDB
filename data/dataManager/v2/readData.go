package dataManager_v2

import "errors"

func ReadDataFromFileAsync(filePath string, dataStartPtr int64, dataEndPtr int64) ([]byte, error) {
	respChan := make(chan fileResponse, 1)
	req := fileRequest{
		op:       "read",
		startPtr: dataStartPtr,
		endPtr:   dataEndPtr,
		resp:     respChan,
	}
	resp := sendToFileWorker(filePath, req)
	if resp.err != nil {
		return nil, resp.err
	}
	if int64(len(resp.data)) != dataEndPtr-dataStartPtr {
		return nil, errors.New("incomplete read")
	}
	return resp.data, nil
}
