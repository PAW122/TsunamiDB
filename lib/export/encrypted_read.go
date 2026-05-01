package export

import (
	"fmt"

	types "github.com/PAW122/TsunamiDB/types"
)

func ReadEncrypted(key, table, encryption_key string) ([]byte, error) {

	nm := getNetworkManager()
	if nm == nil {
		return nil, fmt.Errorf("error: NetworkManager is not initialized")
	}

	// Próba pobrania lokalnie
	fs_data, err := getElementByKey(table, key)
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
			decrypted_content, err := decrypt([]byte(res.Content), encryption_key)
			if err != nil {
				return nil, fmt.Errorf("error decryping data")
			}
			return decrypted_content, nil
		} else {
			return nil, fmt.Errorf("data not found on any server")
		}
	}

	// Jeśli znaleziono lokalnie -> odczytujemy dane
	data, err := readDataFromFile(table, int64(fs_data.StartPtr), int64(fs_data.EndPtr))
	if err != nil {
		return nil, fmt.Errorf("error reading from file: ")
	}

	decoded_obj := decode(data)

	decrypted_content, err := decrypt([]byte(decoded_obj.Data), encryption_key)
	if err != nil {
		return nil, fmt.Errorf("error decryping data")
	}

	return decrypted_content, nil

}
