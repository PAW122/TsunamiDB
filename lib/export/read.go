package export

import (
	"fmt"

	"github.com/PAW122/TsunamiDB/errors"
	types "github.com/PAW122/TsunamiDB/types"
)

func Read(key, table string) ([]byte, error) {
	nm := getNetworkManager()
	if nm == nil {
		return nil, fmt.Errorf("error: NetworkManager is not initialized")
	}

	// Try local read
	fs_data, err := getElementByKey(table, key)
	if err != nil { // if not found locally, send network request
		req := types.NMmessage{
			Task:      "read",
			Args:      []string{table, key},
			ReqSendBy: nm.ServerIP,
		}

		// Send P2P request
		res := nm.SendTaskReq(req)

		// Check for results from other servers
		if res.Finished {
			return res.Content, nil
		} else {
			return nil, errors.ErrNotFound // Używamy niestandardowego błędu
		}
	}

	// If found on local server -> return
	data, err := readDataFromFileAsync(table, int64(fs_data.StartPtr), int64(fs_data.EndPtr))
	if err != nil {
		return nil, err
	}

	decoded_obj := decode(data)
	return []byte(decoded_obj.Data), nil
}
