package export

import (
	dataManager_v2 "TsunamiDB/data/dataManager/v2"
	fileSystem_v1 "TsunamiDB/data/fileSystem/v1"
	encoder_v1 "TsunamiDB/encoding/v1"
	networkmanager "TsunamiDB/servers/network-manager"
	types "TsunamiDB/types"
	"fmt"
)

func Read(key, table string) ([]byte, error) {

	nm := networkmanager.GetNetworkManager()
	if nm == nil {
		return nil, fmt.Errorf("Error: NetworkManager is not initialized")
	}

	// Try local read
	fs_data, err := fileSystem_v1.GetElementByKey(key)
	if err != nil { // if not found on local send network req
		req := types.NMmessage{
			Task:      "read",
			Args:      []string{table, key},
			ReqSendBy: nm.ServerIP,
		}

		// send P2P req
		res := nm.SendTaskReq(req)

		// check for results from other servers
		if res.Finished {
			return res.Content, nil
		} else {
			return nil, fmt.Errorf("Data not found on any server")
		}
	}

	// if found on local server -> return
	data, err := dataManager_v2.ReadDataFromFileAsync(table, int64(fs_data.StartPtr), int64(fs_data.EndPtr))
	if err != nil {
		return nil, err
	}

	decoded_obj := encoder_v1.Decode(data)
	return []byte(decoded_obj.Data), nil

}
