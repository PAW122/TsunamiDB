package networkmanager

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	fileSystem_v1 "github.com/PAW122/TsunamiDB/data/fileSystem/v1"
	relational "github.com/PAW122/TsunamiDB/data/relational"
	encoder_v1 "github.com/PAW122/TsunamiDB/encoding/v1"
	types "github.com/PAW122/TsunamiDB/types"
	"github.com/gorilla/websocket"
)

const (
	frameTypeHello    = "hello"
	frameTypeCatalog  = "catalog"
	frameTypeRequest  = "request"
	frameTypeResponse = "response"
	frameTypePing     = "ping"
	frameTypePong     = "pong"

	defaultRequestTimeout = 5 * time.Second
	heartbeatInterval     = 5 * time.Second
	peerTimeout           = 15 * time.Second

	tableKindKV         = "kv"
	tableKindRelational = "relational"
)

type TableInfo struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
}

type protocolFrame struct {
	Version       int             `json:"version"`
	Type          string          `json:"type"`
	NodeID        string          `json:"node_id,omitempty"`
	AdvertiseAddr string          `json:"advertise_addr,omitempty"`
	Tables        []TableInfo     `json:"tables,omitempty"`
	Message       types.NMmessage `json:"message,omitempty"`
}

type pendingResponse struct {
	ch chan types.NMmessage
}

// Peer to pojedynczy serwer w sieci P2P.
type Peer struct {
	Conn       *websocket.Conn
	Address    string
	Advertise  string
	NodeID     string
	LastActive time.Time

	sendMu sync.Mutex
	tables map[string]TableInfo
}

func (p *Peer) snapshotTables() map[string]TableInfo {
	out := make(map[string]TableInfo, len(p.tables))
	for name, info := range p.tables {
		out[name] = info
	}
	return out
}

// NetworkManager obsługuje połączenia P2P.
type NetworkManager struct {
	sync.RWMutex

	peers            map[string]*Peer
	port             int
	ServerIP         string
	NodeID           string
	sharedSecret     string
	upgrader         websocket.Upgrader
	responseChannels map[string]*pendingResponse
	localTables      map[string]TableInfo
	requestSeq       uint64
}

type Stats struct {
	ServerIP         string              `json:"server_ip"`
	NodeID           string              `json:"node_id"`
	Port             int                 `json:"port"`
	ConnectedPeers   int                 `json:"connected_peers"`
	PeerAddresses    []string            `json:"peer_addresses"`
	PendingResponses int                 `json:"pending_responses"`
	LocalTables      []TableInfo         `json:"local_tables,omitempty"`
	RemoteTables     map[string][]string `json:"remote_tables,omitempty"`
	SecureTransport  bool                `json:"secure_transport"`
}

var nmInstance *NetworkManager

func getLocalIP() (string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", err
	}

	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() && ipNet.IP.To4() != nil {
			return ipNet.IP.String(), nil
		}
	}
	return "", nil
}

func GetOutboundIP() net.IP {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return nil
	}
	defer conn.Close()

	localAddr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok {
		return nil
	}
	return localAddr.IP
}

func StartNetworkManager(port int, knownPeers []string) {
	serverIP := resolveAdvertiseAddress(port)
	nmInstance = &NetworkManager{
		peers:            make(map[string]*Peer),
		responseChannels: make(map[string]*pendingResponse),
		port:             port,
		ServerIP:         serverIP,
		NodeID:           serverIP,
		sharedSecret:     strings.TrimSpace(os.Getenv("TSUNAMI_NETWORK_SECRET")),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
		localTables: make(map[string]TableInfo),
	}
	nmInstance.refreshLocalCatalog()

	if port > 0 {
		go nmInstance.startServer()
	}
	for _, peerAddr := range knownPeers {
		go nmInstance.connectToPeer(peerAddr)
	}
	go nmInstance.heartbeatChecker()
}

func GetNetworkManager() *NetworkManager {
	if nmInstance == nil {
		log.Println("network manager is not initialized")
	}
	return nmInstance
}

func SetInstanceForTests(nm *NetworkManager) {
	nmInstance = nm
}

func resolveAdvertiseAddress(port int) string {
	if advertised := strings.TrimSpace(os.Getenv("TSUNAMI_NETWORK_ADVERTISE_ADDR")); advertised != "" {
		return advertised
	}
	if ip := GetOutboundIP(); ip != nil && port > 0 {
		return net.JoinHostPort(ip.String(), fmt.Sprintf("%d", port))
	}
	if ip, err := getLocalIP(); err == nil && ip != "" && port > 0 {
		return net.JoinHostPort(ip, fmt.Sprintf("%d", port))
	}
	if port > 0 {
		return fmt.Sprintf("127.0.0.1:%d", port)
	}
	return "127.0.0.1"
}

func (nm *NetworkManager) initState() {
	if nm == nil {
		return
	}
	nm.Lock()
	defer nm.Unlock()
	nm.initStateLocked()
}

func (nm *NetworkManager) initStateLocked() {
	if nm.peers == nil {
		nm.peers = make(map[string]*Peer)
	}
	if nm.responseChannels == nil {
		nm.responseChannels = make(map[string]*pendingResponse)
	}
	if nm.localTables == nil {
		nm.localTables = make(map[string]TableInfo)
	}
	if nm.NodeID == "" {
		nm.NodeID = nm.ServerIP
	}
	if nm.upgrader.CheckOrigin == nil {
		nm.upgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	}
}

func (nm *NetworkManager) startServer() {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", nm.handleConnection)
	addr := fmt.Sprintf(":%d", nm.port)
	log.Println("network manager websocket listening on", addr)
	log.Println(http.ListenAndServe(addr, mux))
}

func (nm *NetworkManager) handleConnection(w http.ResponseWriter, r *http.Request) {
	nm.initState()
	conn, err := nm.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("websocket upgrade error:", err)
		return
	}

	peerAddr := conn.RemoteAddr().String()
	peer := &Peer{
		Conn:       conn,
		Address:    peerAddr,
		LastActive: time.Now(),
		tables:     make(map[string]TableInfo),
	}

	nm.Lock()
	nm.peers[peerAddr] = peer
	nm.Unlock()

	if err := nm.sendHello(peer); err != nil {
		log.Println("send hello error:", err)
	}
	go nm.listenForMessages(peerAddr, conn)
}

func (nm *NetworkManager) connectToPeer(peerAddr string) {
	nm.initState()
	u := url.URL{Scheme: "ws", Host: peerAddr, Path: "/ws"}
	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		log.Println("connect to peer failed:", peerAddr, err)
		return
	}

	peer := &Peer{
		Conn:       conn,
		Address:    peerAddr,
		LastActive: time.Now(),
		tables:     make(map[string]TableInfo),
	}

	nm.Lock()
	nm.peers[peerAddr] = peer
	nm.Unlock()

	if err := nm.sendHello(peer); err != nil {
		log.Println("send hello error:", err)
	}
	go nm.listenForMessages(peerAddr, conn)
}

func (nm *NetworkManager) listenForMessages(peerAddr string, conn *websocket.Conn) {
	defer func() {
		nm.Lock()
		delete(nm.peers, peerAddr)
		nm.Unlock()
		_ = conn.Close()
	}()

	for {
		_, payload, err := conn.ReadMessage()
		if err != nil {
			log.Println("peer disconnected:", peerAddr, err)
			return
		}

		frame, err := nm.decodeFrame(payload)
		if err != nil {
			log.Println("decode network frame error from", peerAddr, ":", err)
			return
		}

		nm.touchPeer(peerAddr)

		switch frame.Type {
		case frameTypeHello:
			nm.updatePeerMetadata(peerAddr, frame.NodeID, frame.AdvertiseAddr, frame.Tables)
			if peer := nm.getPeer(peerAddr); peer != nil {
				if err := nm.sendCatalog(peer); err != nil {
					log.Println("send catalog error:", err)
				}
			}
		case frameTypeCatalog:
			nm.updatePeerMetadata(peerAddr, frame.NodeID, frame.AdvertiseAddr, frame.Tables)
		case frameTypePing:
			if peer := nm.getPeer(peerAddr); peer != nil {
				_ = nm.sendFrame(peer, protocolFrame{Version: 1, Type: frameTypePong, NodeID: nm.NodeID})
			}
		case frameTypePong:
		case frameTypeResponse:
			nm.HandleResponse(frame.Message)
		case frameTypeRequest:
			go nm.handleTaskRequest(peerAddr, frame.Message)
		default:
			log.Println("unknown frame type:", frame.Type)
		}
	}
}

func (nm *NetworkManager) Snapshot() Stats {
	if nm == nil {
		return Stats{}
	}
	nm.initState()

	nm.RLock()
	defer nm.RUnlock()

	peers := make([]string, 0, len(nm.peers))
	remoteTables := make(map[string][]string)
	for addr, peer := range nm.peers {
		peers = append(peers, addr)
		for table := range peer.tables {
			owner := addr
			if peer.Advertise != "" {
				owner = peer.Advertise
			}
			remoteTables[table] = append(remoteTables[table], owner)
		}
	}
	sort.Strings(peers)

	for table := range remoteTables {
		sort.Strings(remoteTables[table])
	}

	localTables := make([]TableInfo, 0, len(nm.localTables))
	for _, info := range nm.localTables {
		localTables = append(localTables, info)
	}
	sort.Slice(localTables, func(i, j int) bool {
		if localTables[i].Kind == localTables[j].Kind {
			return localTables[i].Name < localTables[j].Name
		}
		return localTables[i].Kind < localTables[j].Kind
	})

	return Stats{
		ServerIP:         nm.ServerIP,
		NodeID:           nm.NodeID,
		Port:             nm.port,
		ConnectedPeers:   len(peers),
		PeerAddresses:    peers,
		PendingResponses: len(nm.responseChannels),
		LocalTables:      localTables,
		RemoteTables:     remoteTables,
		SecureTransport:  nm.sharedSecret != "",
	}
}

func (nm *NetworkManager) BroadcastMessage(sender string, message []byte) {
	nm.initState()

	var req types.NMmessage
	if err := json.Unmarshal(message, &req); err != nil {
		log.Println("broadcast payload is not NMmessage:", err)
		return
	}
	if req.RequestID == "" {
		req.RequestID = nm.nextRequestID()
	}

	targets := nm.snapshotPeersExcept(sender)
	for _, peer := range targets {
		_ = nm.sendFrame(peer, protocolFrame{
			Version: 1,
			Type:    frameTypeRequest,
			NodeID:  nm.NodeID,
			Message: req,
		})
	}
}

func (nm *NetworkManager) sendToPeer(peerAddr string, message []byte) {
	nm.initState()

	var req types.NMmessage
	if err := json.Unmarshal(message, &req); err != nil {
		log.Println("direct payload is not NMmessage:", err)
		return
	}
	if req.RequestID == "" {
		req.RequestID = nm.nextRequestID()
	}

	peer := nm.getPeer(peerAddr)
	if peer == nil {
		log.Println("peer not connected:", peerAddr)
		return
	}
	_ = nm.sendFrame(peer, protocolFrame{
		Version: 1,
		Type:    frameTypeRequest,
		NodeID:  nm.NodeID,
		Message: req,
	})
}

func (nm *NetworkManager) listPeers() []string {
	nm.initState()
	nm.RLock()
	defer nm.RUnlock()

	peerList := make([]string, 0, len(nm.peers))
	for peerAddr := range nm.peers {
		peerList = append(peerList, peerAddr)
	}
	sort.Strings(peerList)
	return peerList
}

func (nm *NetworkManager) heartbeatChecker() {
	for {
		time.Sleep(heartbeatInterval)
		nm.initState()

		for _, peer := range nm.snapshotPeersExcept("") {
			if time.Since(peer.LastActive) > peerTimeout {
				log.Println("removing inactive peer:", peer.Address)
				nm.removePeer(peer.Address)
				continue
			}
			if err := nm.sendFrame(peer, protocolFrame{Version: 1, Type: frameTypePing, NodeID: nm.NodeID}); err != nil {
				log.Println("heartbeat error:", err)
				nm.removePeer(peer.Address)
			}
		}
	}
}

func (nm *NetworkManager) nextRequestID() string {
	seq := atomic.AddUint64(&nm.requestSeq, 1)
	return fmt.Sprintf("%s-%d", nm.NodeID, seq)
}

func (nm *NetworkManager) touchPeer(peerAddr string) {
	nm.Lock()
	defer nm.Unlock()
	nm.initStateLocked()
	if peer := nm.peers[peerAddr]; peer != nil {
		peer.LastActive = time.Now()
	}
}

func (nm *NetworkManager) getPeer(peerAddr string) *Peer {
	nm.RLock()
	defer nm.RUnlock()
	if nm.peers == nil {
		return nil
	}
	return nm.peers[peerAddr]
}

func (nm *NetworkManager) removePeer(peerAddr string) {
	nm.Lock()
	peer := nm.peers[peerAddr]
	delete(nm.peers, peerAddr)
	nm.Unlock()
	if peer != nil {
		_ = peer.Conn.Close()
	}
}

func (nm *NetworkManager) snapshotPeersExcept(except string) []*Peer {
	nm.RLock()
	defer nm.RUnlock()
	peers := make([]*Peer, 0, len(nm.peers))
	for addr, peer := range nm.peers {
		if except != "" && addr == except {
			continue
		}
		peers = append(peers, peer)
	}
	return peers
}

func (nm *NetworkManager) sendHello(peer *Peer) error {
	return nm.sendFrame(peer, protocolFrame{
		Version:       1,
		Type:          frameTypeHello,
		NodeID:        nm.NodeID,
		AdvertiseAddr: nm.ServerIP,
		Tables:        nm.snapshotLocalTables(),
	})
}

func (nm *NetworkManager) sendCatalog(peer *Peer) error {
	return nm.sendFrame(peer, protocolFrame{
		Version:       1,
		Type:          frameTypeCatalog,
		NodeID:        nm.NodeID,
		AdvertiseAddr: nm.ServerIP,
		Tables:        nm.snapshotLocalTables(),
	})
}

func (nm *NetworkManager) sendFrame(peer *Peer, frame protocolFrame) error {
	payload, err := nm.encodeFrame(frame)
	if err != nil {
		return err
	}

	peer.sendMu.Lock()
	defer peer.sendMu.Unlock()
	return peer.Conn.WriteMessage(websocket.BinaryMessage, payload)
}

func (nm *NetworkManager) encodeFrame(frame protocolFrame) ([]byte, error) {
	raw, err := json.Marshal(frame)
	if err != nil {
		return nil, err
	}
	if nm.sharedSecret == "" {
		return raw, nil
	}
	return encoder_v1.Encrypt(raw, nm.sharedSecret)
}

func (nm *NetworkManager) decodeFrame(payload []byte) (protocolFrame, error) {
	raw := payload
	if nm.sharedSecret != "" {
		decoded, err := encoder_v1.Decrypt(payload, nm.sharedSecret)
		if err != nil {
			return protocolFrame{}, err
		}
		raw = decoded
	}

	var frame protocolFrame
	if err := json.Unmarshal(raw, &frame); err != nil {
		return protocolFrame{}, err
	}
	if frame.Version == 0 {
		frame.Version = 1
	}
	return frame, nil
}

func (nm *NetworkManager) refreshLocalCatalog() {
	nm.initState()

	kvTables, err := fileSystem_v1.ListTables()
	if err != nil {
		log.Println("list kv tables:", err)
	}
	relTables, err := relational.ListTables()
	if err != nil {
		log.Println("list relational tables:", err)
	}

	next := make(map[string]TableInfo, len(kvTables)+len(relTables))
	for _, table := range kvTables {
		next[table] = TableInfo{Name: table, Kind: tableKindKV}
	}
	for _, schema := range relTables {
		next[schema.Name] = TableInfo{Name: schema.Name, Kind: tableKindRelational}
	}

	nm.Lock()
	nm.localTables = next
	nm.Unlock()
}

func (nm *NetworkManager) snapshotLocalTables() []TableInfo {
	nm.RLock()
	defer nm.RUnlock()

	tables := make([]TableInfo, 0, len(nm.localTables))
	for _, info := range nm.localTables {
		tables = append(tables, info)
	}
	sort.Slice(tables, func(i, j int) bool {
		if tables[i].Kind == tables[j].Kind {
			return tables[i].Name < tables[j].Name
		}
		return tables[i].Kind < tables[j].Kind
	})
	return tables
}

func (nm *NetworkManager) updatePeerMetadata(peerAddr, nodeID, advertiseAddr string, tables []TableInfo) {
	nm.Lock()
	defer nm.Unlock()
	nm.initStateLocked()

	peer := nm.peers[peerAddr]
	if peer == nil {
		return
	}
	if nodeID != "" {
		peer.NodeID = nodeID
	}
	if advertiseAddr != "" {
		peer.Advertise = advertiseAddr
	}
	peer.LastActive = time.Now()
	peer.tables = make(map[string]TableInfo, len(tables))
	for _, info := range tables {
		peer.tables[info.Name] = info
	}
}

func (nm *NetworkManager) NotifyLocalTable(table, kind string) {
	if nm == nil || strings.TrimSpace(table) == "" {
		return
	}
	nm.initState()

	if kind == "" {
		kind = tableKindKV
	}

	nm.Lock()
	nm.localTables[table] = TableInfo{Name: table, Kind: kind}
	nm.Unlock()

	frame := protocolFrame{
		Version:       1,
		Type:          frameTypeCatalog,
		NodeID:        nm.NodeID,
		AdvertiseAddr: nm.ServerIP,
		Tables:        nm.snapshotLocalTables(),
	}
	for _, peer := range nm.snapshotPeersExcept("") {
		if err := nm.sendFrame(peer, frame); err != nil {
			log.Println("catalog broadcast error:", err)
		}
	}
}

func NotifyKVTable(table string) {
	if nmInstance != nil {
		nmInstance.NotifyLocalTable(table, tableKindKV)
	}
}
