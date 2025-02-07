package networkmanager

import (
	tasks "TsunamiDB/servers/network-manager/tasks"
	types "TsunamiDB/types"
	"encoding/json"
	"log"

	"github.com/gorilla/websocket"
)

// read and execute on message
func handleMsg(peerAddr string, message []byte, nm *NetworkManager, conn *websocket.Conn) {
	var req types.NMmessage
	err := json.Unmarshal(message, &req)
	if err != nil {
		log.Println("📌 Błąd parsowania wiadomości od", peerAddr, ":", err)
		return
	}

	// log.Println("📌 Otrzymano wiadomość od", peerAddr, ":", req)

	// 🔹 Ignorujemy odpowiedzi (Finished: true) - zapobiega pętli!
	if req.Finished {
		// log.Println("📌 Ignoruję wiadomość, bo jest już oznaczona jako Finished")
		return
	}

	// 🔹 Obsługa nowych żądań
	var res types.NMmessage
	switch req.Task {
	case "read":
		res = tasks.Read(req)
	case "save":
		res = tasks.Save(req)
	case "free":
		res = tasks.Free(req)
	default:
		log.Println("📌 Nieznane zadanie:", req.Task)
		return
	}

	// Przypisanie adresu IP serwera, który odpowiada
	res.ReqSendBy = req.ReqSendBy // Nadawca żądania
	res.ReqResBy = nm.ServerIP    // Ten serwer odpowiada
	res.Finished = true           // Oznaczamy jako zakończone

	// Serializacja odpowiedzi do JSON
	responseJSON, err := json.Marshal(res)
	if err != nil {
		log.Println("📌 Błąd serializacji odpowiedzi:", err)
		return
	}

	// Bezpośrednie wysłanie odpowiedzi do klienta
	// log.Println("📌 Wysyłam odpowiedź do", peerAddr)
	err = conn.WriteMessage(websocket.TextMessage, responseJSON)
	if err != nil {
		log.Println("📌 Błąd wysyłania do", peerAddr, ":", err)
	}
}
