package networkmanager

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"sort"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	types "github.com/PAW122/TsunamiDB/types"
)

// Peer to pojedynczy serwer w sieci P2P
type Peer struct {
	Conn       *websocket.Conn
	Address    string
	LastActive time.Time
}

// NetworkManager obsługuje połączenia P2P
type NetworkManager struct {
	sync.Mutex
	peers            map[string]*Peer
	port             int
	ServerIP         string
	upgrader         websocket.Upgrader
	responseChannels map[string]chan types.NMmessage
}

type Stats struct {
	ServerIP         string   `json:"server_ip"`
	Port             int      `json:"port"`
	ConnectedPeers   int      `json:"connected_peers"`
	PeerAddresses    []string `json:"peer_addresses"`
	PendingResponses int      `json:"pending_responses"`
}

var nmInstance *NetworkManager

// getLocalIP pobiera lokalny adres IP serwera
func getLocalIP() (string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", err
	}

	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
			if ipNet.IP.To4() != nil {
				return ipNet.IP.String(), nil
			}
		}
	}
	return "", nil
}

func GetOutboundIP() net.IP {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)

	return localAddr.IP
}

func StartNetworkManager(port int, knownPeers []string) {
	// Pobranie lokalnego IP
	// localIP, err := getLocalIP()
	// if err != nil {
	// 	log.Println("📌 Nie udało się pobrać IP, zapytam sieć.")
	// 	localIP = ""
	// }
	localIP := GetOutboundIP().String()

	nmInstance = &NetworkManager{
		peers:            make(map[string]*Peer),
		responseChannels: make(map[string]chan types.NMmessage),
		port:             port,
		ServerIP:         localIP,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}

	// Start serwera WebSocket
	go nmInstance.startServer()

	// Połącz się do znanych serwerów
	for _, peerAddr := range knownPeers {
		go nmInstance.connectToPeer(peerAddr)
	}

	// 🔹 Jeśli nie mamy IP, zapytajmy o nie sieć
	if nmInstance.ServerIP == "" {
		log.Println("📌 Brak lokalnego IP, wysyłam zapytanie do sieci")
		req := types.NMmessage{
			Task: "get_my_ip",
		}
		reqJSON, err := json.Marshal(req)
		if err != nil {
			log.Println("📌 Błąd serializacji get_my_ip:", err)
		} else {
			nmInstance.BroadcastMessage("", reqJSON) // Wysyłamy do wszystkich peerów
		}
	}

	// Uruchom heartbeat checker
	go nmInstance.heartbeatChecker()
}

func GetNetworkManager() *NetworkManager {
	if nmInstance == nil {
		log.Println("Błąd: NetworkManager nie został poprawnie zainicjalizowany")
	}
	return nmInstance
}

func SetInstanceForTests(nm *NetworkManager) {
	nmInstance = nm
}

// startServer uruchamia lokalny serwer WebSocket
func (nm *NetworkManager) startServer() {
	http.HandleFunc("/ws", nm.handleConnection)
	addr := fmt.Sprintf(":%d", nm.port)
	log.Println("Serwer WebSocket działa na", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}

// handleConnection obsługuje nowe połączenia WebSocket
func (nm *NetworkManager) handleConnection(w http.ResponseWriter, r *http.Request) {
	conn, err := nm.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Błąd połączenia:", err)
		return
	}

	peerAddr := conn.RemoteAddr().String()
	log.Println("Nowe połączenie:", peerAddr)

	/*
		assign connect server Ip to new conected server in the network
	*/
	// Pobieramy IP z połączenia WebSocket
	peerIP, _, _ := net.SplitHostPort(conn.RemoteAddr().String())

	// Wysyłamy do klienta jego własny adres IP, aby używał go do komunikacji
	msg := types.NMmessage{
		Task:      "set_ip",
		Args:      []string{peerIP},
		ReqSendBy: peerIP, // To, co widzi serwer
	}

	msgJSON, _ := json.Marshal(msg)
	conn.WriteMessage(websocket.TextMessage, msgJSON) // Wysyłamy poprawny IP do klienta

	// log.Println("Przekazano IP klientowi:", peerIP)

	nm.Lock()
	nm.peers[peerAddr] = &Peer{Conn: conn, Address: peerAddr, LastActive: time.Now()}
	nm.Unlock()

	go nm.listenForMessages(peerAddr, conn)
}

// connectToPeer łączy się do znanego serwera
func (nm *NetworkManager) connectToPeer(peerAddr string) {
	u := url.URL{Scheme: "ws", Host: peerAddr, Path: "/ws"}
	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		log.Println("📌 Nie można połączyć z:", peerAddr, ":", err)
		return
	}

	log.Println("📌 Połączono z:", peerAddr)

	nm.Lock()
	nm.peers[peerAddr] = &Peer{Conn: conn, Address: peerAddr, LastActive: time.Now()}
	nm.Unlock()

	log.Println("📌 Aktualna lista peerów po połączeniu:", nm.listPeers())

	go nm.listenForMessages(peerAddr, conn)
}

// uzywac conn do odp
func (nm *NetworkManager) listenForMessages(peerAddr string, conn *websocket.Conn) {
	defer func() {
		nm.Lock()
		delete(nm.peers, peerAddr)
		nm.Unlock()
		conn.Close()
	}()

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			log.Println("📌 Rozłączono:", peerAddr, err)
			return
		}

		nm.Lock()
		nm.peers[peerAddr].LastActive = time.Now()
		nm.Unlock()

		// Dekodowanie wiadomości
		var response types.NMmessage
		err = json.Unmarshal(message, &response)
		if err != nil {
			// log.Println("📌 Read unparsed:", string(message))
			continue
		}

		// 🔹 Jeśli to odpowiedź (Finished: true), przekazujemy do HandleResponse()
		if response.Finished {
			// log.Println("📌 Otrzymano odpowiedź od", peerAddr, "dla zadania", response.Task)
			go nm.HandleResponse(response) // Przekazanie odpowiedzi do SendTaskReq()
			continue
		}

		// 🔹 Obsługujemy nowe żądania
		go handleMsg(peerAddr, message, nm, conn)
	}
}

// Snapshot returns a thread-safe view of the network manager state
func (nm *NetworkManager) Snapshot() Stats {
	if nm == nil {
		return Stats{}
	}

	nm.Lock()
	defer nm.Unlock()

	peers := make([]string, 0, len(nm.peers))
	for addr := range nm.peers {
		peers = append(peers, addr)
	}
	sort.Strings(peers)

	return Stats{
		ServerIP:         nm.ServerIP,
		Port:             nm.port,
		ConnectedPeers:   len(peers),
		PeerAddresses:    peers,
		PendingResponses: len(nm.responseChannels),
	}
}

// BroadcastMessage relays messages to all connected peers
func (nm *NetworkManager) BroadcastMessage(sender string, message []byte) {
	nm.Lock()
	defer nm.Unlock()

	for peerAddr, peer := range nm.peers {
		if peerAddr != sender { // Nie wysyłamy do nadawcy
			err := peer.Conn.WriteMessage(websocket.TextMessage, message)
			if err != nil {
				log.Println("📌 Błąd wysyłania do", peerAddr, err)
				peer.Conn.Close()
				delete(nm.peers, peerAddr)
			}
		}
	}
}

// sendToPeer wysyła wiadomość do konkretnego serwera w sieci P2P
func (nm *NetworkManager) sendToPeer(peerAddr string, message []byte) {
	nm.Lock()
	defer nm.Unlock()

	peer, exists := nm.peers[peerAddr]
	if !exists {
		log.Println("📌 Błąd: serwer", peerAddr, "nie jest połączony. Aktualne peery:", nm.listPeers())
		return
	}

	err := peer.Conn.WriteMessage(1, message)
	if err != nil {
		log.Println("📌 Błąd wysyłania do", peerAddr, ":", err)
		peer.Conn.Close()
		delete(nm.peers, peerAddr)
	}
}

// Funkcja do debugowania listy peerów
func (nm *NetworkManager) listPeers() []string {
	var peerList []string
	for peerAddr := range nm.peers {
		peerList = append(peerList, peerAddr)
	}
	return peerList
}

// heartbeatChecker sprawdza, które serwery są aktywne
func (nm *NetworkManager) heartbeatChecker() {
	for {
		time.Sleep(5 * time.Second)

		nm.Lock()
		for peerAddr, peer := range nm.peers {
			if time.Since(peer.LastActive) > 11*time.Second {
				log.Println("Usunięto nieaktywnego peera:", peerAddr)
				peer.Conn.Close()
				delete(nm.peers, peerAddr)
			} else {
				err := peer.Conn.WriteMessage(websocket.TextMessage, []byte("heartbeat"))
				if err != nil {
					log.Println("Błąd wysyłania heartbeat do", peerAddr, err)
					peer.Conn.Close()
					delete(nm.peers, peerAddr)
				}
			}
		}
		nm.Unlock()
	}
}
