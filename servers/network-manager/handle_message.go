package networkmanager

import (
	"log"

	tasks "github.com/PAW122/TsunamiDB/servers/network-manager/tasks"
	types "github.com/PAW122/TsunamiDB/types"
)

func executeTask(req types.NMmessage) types.NMmessage {
	switch req.Task {
	case "read":
		return tasks.Read(req)
	case "save":
		return tasks.Save(req)
	case "free":
		return tasks.Free(req)
	default:
		return types.NMmessage{RequestID: req.RequestID, Task: req.Task, ReqSendBy: req.ReqSendBy, Finished: false}
	}
}

func (nm *NetworkManager) handleTaskRequest(peerAddr string, req types.NMmessage) {
	res := executeTask(req)
	res.RequestID = req.RequestID
	res.Task = req.Task
	res.ReqSendBy = req.ReqSendBy
	res.ReqResBy = nm.NodeID
	if res.Finished && len(req.Args) > 0 && (req.Task == "save" || req.Task == "free") {
		nm.NotifyLocalTable(req.Args[0], tableKindKV)
	}

	peer := nm.getPeer(peerAddr)
	if peer == nil {
		return
	}
	if err := nm.sendFrame(peer, protocolFrame{
		Version: 1,
		Type:    frameTypeResponse,
		NodeID:  nm.NodeID,
		Message: res,
	}); err != nil {
		log.Println("send task response error:", err)
	}
}
