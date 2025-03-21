package networkmanager

import (
	"encoding/json"
	"log"
	"time"

	types "github.com/PAW122/TsunamiDB/types"

	"github.com/gorilla/websocket"
)

// SendTaskReq wysyła żądanie do wszystkich serwerów i czeka na pierwszą odpowiedź
func (nm *NetworkManager) SendTaskReq(req types.NMmessage) types.NMmessage {
	responseChannel := make(chan types.NMmessage, 1)

	if nm.peers == nil {
		return types.NMmessage{Finished: false}
	}

	// Tworzymy zapytanie `get_my_ip`
	if nm.ServerIP == "" {
		reqIP := types.NMmessage{
			Task: "get_my_ip",
		}
		reqJSON, err := json.Marshal(reqIP)
		if err != nil {
			// log.Println("📌 Błąd serializacji get_my_ip:", err)
		} else {
			nm.BroadcastMessage("", reqJSON)
		}
		time.Sleep(2 * time.Second)
	}
	if nm.ServerIP == "" {
		// log.Println("📌 Nadal brak IP, anuluję żądanie.")
		return types.NMmessage{Finished: false}
	}

	req.ReqSendBy = nm.ServerIP // 🔹 Poprawne IP serwera

	// Tworzymy klucz dla odpowiedzi
	reqKey := req.ReqSendBy + "_" + req.Task

	// Rejestracja kanału odpowiedzi
	nm.Lock()
	nm.responseChannels[reqKey] = responseChannel
	nm.Unlock()

	// Serializacja JSON
	reqJSON, err := json.Marshal(req)
	if err != nil {
		log.Println("📌 Błąd serializacji żądania:", err)
		return types.NMmessage{Finished: false}
	}

	// Wysłanie żądania do wszystkich peerów
	nm.Lock()
	for _, peer := range nm.peers {
		go peer.Conn.WriteMessage(websocket.TextMessage, reqJSON)
	}
	nm.Unlock()

	// Czekamy na odpowiedź lub timeout 5s
	select {
	case res := <-responseChannel:
		// log.Println("📌 Otrzymano odpowiedź:", res)

		/*
			todo
			tutaj trzeba cachować conn - has <key
			key: conn
		*/

		return res
	case <-time.After(5 * time.Second):
		// log.Println("📌 Timeout: brak odpowiedzi od serwerów")
		nm.Lock()
		delete(nm.responseChannels, reqKey)
		nm.Unlock()
		return types.NMmessage{Finished: false}
	}
}

// HandleResponse obsługuje odpowiedź z handleMsg() i przekazuje ją do kanału
func (nm *NetworkManager) HandleResponse(response types.NMmessage) {
	reqKey := response.ReqSendBy + "_" + response.Task

	nm.Lock()
	responseChannel, exists := nm.responseChannels[reqKey]
	nm.Unlock()

	if exists {
		// log.Println("📌 Przekazuję odpowiedź dla", reqKey, "od", response.ReqResBy)
		select {
		case responseChannel <- response:
			// log.Println("📌 Odpowiedź dostarczona do kanału:", reqKey)
		default:
			// log.Println("📌 Kanał odpowiedzi dla", reqKey, "jest pełny.")
		}
	} else {
		// log.Println("📌 Brak kanału odpowiedzi dla:", reqKey, "Odpowiedź zostanie zignorowana.")
	}
}
