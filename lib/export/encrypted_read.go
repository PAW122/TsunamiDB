package export

import (
	"fmt"

	dataManager_v1 "github.com/PAW122/TsunamiDB/data/dataManager/v1"
	fileSystem_v1 "github.com/PAW122/TsunamiDB/data/fileSystem/v1"
	encoder_v1 "github.com/PAW122/TsunamiDB/encoding/v1"
	networkmanager "github.com/PAW122/TsunamiDB/servers/network-manager"
	types "github.com/PAW122/TsunamiDB/types"
)

func ReadEncrypted(key, table, encryption_key string) ([]byte, error) {

	nm := networkmanager.GetNetworkManager()
	if nm == nil {
		return nil, fmt.Errorf("error: NetworkManager is not initialized")
	}

	// Próba pobrania lokalnie
	fs_data, err := fileSystem_v1.GetElementByKey(key)
	if err != nil {
		// 🔹 Jeśli nie znaleziono -> wysyłamy zapytanie do innych serwerów
		req := types.NMmessage{
			Task:      "read",
			Args:      []string{table, key},
			ReqSendBy: nm.ServerIP,
		}

		// 🔹 Wysyłamy zapytanie P2P
		res := nm.SendTaskReq(req)

		// 🔹 Sprawdzamy, czy znaleziono wynik na innym serwerze
		if res.Finished {
			decrypted_content, err := encoder_v1.Decrypt([]byte(res.Content), encryption_key)
			if err != nil {
				return nil, fmt.Errorf("error decryping data")
			}
			return decrypted_content, nil
		} else {
			return nil, fmt.Errorf("data not found on any server")
		}
	}

	// Jeśli znaleziono lokalnie -> odczytujemy dane
	data, err := dataManager_v1.ReadDataFromFile(table, int64(fs_data.StartPtr), int64(fs_data.EndPtr))
	if err != nil {
		return nil, fmt.Errorf("error reading from file: ")
	}

	decoded_obj := encoder_v1.Decode(data)

	decrypted_content, err := encoder_v1.Decrypt([]byte(decoded_obj.Data), encryption_key)
	if err != nil {
		return nil, fmt.Errorf("error decryping data")
	}

	return decrypted_content, nil

}
