package networkmanager

import (
	"strings"
	"time"

	types "github.com/PAW122/TsunamiDB/types"
)

// SendTaskReq wysyła żądanie do peerów posiadających tabelę i czeka na pierwszą udaną odpowiedź.
func (nm *NetworkManager) SendTaskReq(req types.NMmessage) types.NMmessage {
	if nm == nil {
		return types.NMmessage{Finished: false}
	}
	nm.initState()

	targets := nm.selectPeersForRequest(req)
	if len(targets) == 0 {
		return types.NMmessage{Finished: false}
	}

	if req.RequestID == "" {
		req.RequestID = nm.nextRequestID()
	}
	if req.ReqSendBy == "" {
		req.ReqSendBy = nm.NodeID
	}

	responseChannel := make(chan types.NMmessage, len(targets))
	nm.Lock()
	nm.responseChannels[req.RequestID] = &pendingResponse{ch: responseChannel}
	nm.Unlock()
	defer func() {
		nm.Lock()
		delete(nm.responseChannels, req.RequestID)
		nm.Unlock()
	}()

	frame := protocolFrame{
		Version: 1,
		Type:    frameTypeRequest,
		NodeID:  nm.NodeID,
		Message: req,
	}
	sent := 0
	for _, peer := range targets {
		if err := nm.sendFrame(peer, frame); err == nil {
			sent++
		}
	}
	if sent == 0 {
		return types.NMmessage{Finished: false}
	}

	deadline := time.NewTimer(defaultRequestTimeout)
	defer deadline.Stop()

	for pending := sent; pending > 0; pending-- {
		select {
		case res := <-responseChannel:
			if res.Finished {
				return res
			}
		case <-deadline.C:
			return types.NMmessage{Finished: false}
		}
	}

	return types.NMmessage{Finished: false}
}

func (nm *NetworkManager) selectPeersForRequest(req types.NMmessage) []*Peer {
	allPeers := nm.snapshotPeersExcept("")
	if len(req.Args) == 0 {
		return allPeers
	}

	table := strings.TrimSpace(req.Args[0])
	if table == "" {
		return allPeers
	}

	targets := make([]*Peer, 0, len(allPeers))
	nm.RLock()
	for _, peer := range allPeers {
		if _, ok := peer.tables[table]; ok {
			targets = append(targets, peer)
		}
	}
	nm.RUnlock()
	if len(targets) > 0 {
		return targets
	}
	return allPeers
}

// HandleResponse obsługuje odpowiedź i przekazuje ją do kanału oczekującego requestu.
func (nm *NetworkManager) HandleResponse(response types.NMmessage) {
	if nm == nil || response.RequestID == "" {
		return
	}

	nm.RLock()
	pending := nm.responseChannels[response.RequestID]
	nm.RUnlock()

	if pending == nil {
		return
	}

	select {
	case pending.ch <- response:
	default:
	}
}
