package routes

import (
	"fmt"
	"net/http"

	dataManager_v1 "github.com/PAW122/TsunamiDB/data/dataManager/v1"
	fileSystem_v1 "github.com/PAW122/TsunamiDB/data/fileSystem/v1"
	encoder_v1 "github.com/PAW122/TsunamiDB/encoding/v1"
	debug "github.com/PAW122/TsunamiDB/servers/debug"
	networkmanager "github.com/PAW122/TsunamiDB/servers/network-manager"
	types "github.com/PAW122/TsunamiDB/types"
)

func ReadEncrypted(w http.ResponseWriter, r *http.Request, c *http.Client) {
	defer debug.MeasureTime("> api [ReadEncrypted]")()
	if r.Method != "GET" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	pathParts := ParseArgs(r.URL.Path, "read_encrypted")
	if len(pathParts) < 4 {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "Invalid URL args")
		return
	}

	file := pathParts[2]
	key := pathParts[3]

	encryption_header := r.Header.Get("encryption_key")
	if encryption_header == "" {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "Missing encryption_key header")
		return
	}

	// Pobranie instancji NetworkManager
	nm := networkmanager.GetNetworkManager()
	if nm == nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "network manager not initialized")
		return
	}
	// Próba pobrania lokalnie
	fs_data, err := fileSystem_v1.GetElementByKey(file, key)
	if err != nil {
		// 🔹 Jeśli nie znaleziono -> wysyłamy zapytanie do innych serwerów
		req := types.NMmessage{
			Task:      "read",
			Args:      []string{file, key},
			ReqSendBy: nm.ServerIP, // Pobranie IP z NetworkManager
		}

		// fmt.Println("read network req by, %s", nm.ServerIP)

		// 🔹 Wysyłamy zapytanie P2P
		res := nm.SendTaskReq(req)

		// 🔹 Sprawdzamy, czy znaleziono wynik na innym serwerze
		if res.Finished {
			w.WriteHeader(http.StatusOK)

			decrypted_content, err := encoder_v1.Decrypt([]byte(res.Content), encryption_header)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprint(w, "Error decryping data")
				return
			}

			w.Write(decrypted_content)
			return
		} else {
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, "Data not found on any server")
			return
		}
	}

	// Jeśli znaleziono lokalnie -> odczytujemy dane
	data, err := dataManager_v1.ReadDataFromFile(file, int64(fs_data.StartPtr), int64(fs_data.EndPtr))
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, "Error reading from file:", err)
		return
	}

	decoded_obj := encoder_v1.Decode(data)

	decrypted_content, err := encoder_v1.Decrypt([]byte(decoded_obj.Data), encryption_header)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "Error decryping data")
		return
	}

	w.Write(decrypted_content)
}
