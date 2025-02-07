package routes

import (
	"fmt"
	"net/http"

	dataManager_v1 "TsunamiDB/data/dataManager/v1"
	fileSystem_v1 "TsunamiDB/data/fileSystem/v1"
	encoder_v1 "TsunamiDB/encoding/v1"
	networkmanager "TsunamiDB/servers/network-manager"
	types "TsunamiDB/types"
)

func Read(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	pathParts := ParseArgs(r.URL.Path, "read")
	if pathParts == nil || len(pathParts) < 2 {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "Invalid URL args")
		return
	}

	file := pathParts[2]
	key := pathParts[3]

	// Pobranie instancji NetworkManager
	nm := networkmanager.GetNetworkManager()
	if nm == nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "Błąd: NetworkManager nie został poprawnie zainicjalizowany")
		return
	}

	// Próba pobrania lokalnie
	fs_data, err := fileSystem_v1.GetElementByKey(key)
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
			w.Write(res.Content) // Zwracamy dane z innego serwera
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

	w.Write([]byte(decoded_obj.Data))
}
