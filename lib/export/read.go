package export

import (
	"fmt"

	dataManager_v2 "github.com/PAW122/TsunamiDB/data/dataManager/v2"
	fileSystem_v1 "github.com/PAW122/TsunamiDB/data/fileSystem/v1"
	encoder_v1 "github.com/PAW122/TsunamiDB/encoding/v1"
	"github.com/PAW122/TsunamiDB/errors"
	networkmanager "github.com/PAW122/TsunamiDB/servers/network-manager"
	types "github.com/PAW122/TsunamiDB/types"
)

func Read(key, table string) ([]byte, error) {
	nm := networkmanager.GetNetworkManager()
	if nm == nil {
		return nil, fmt.Errorf("error: NetworkManager is not initialized")
	}

	// Try local read
	fs_data, err := fileSystem_v1.GetElementByKey(table, key)
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
	data, err := dataManager_v2.ReadDataFromFileAsync(table, int64(fs_data.StartPtr), int64(fs_data.EndPtr))
	if err != nil {
		return nil, err
	}

	decoded_obj := encoder_v1.Decode(data)
	return []byte(decoded_obj.Data), nil
}
