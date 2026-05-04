package types

type NMmessage struct {
	RequestID string   `json:"requestId,omitempty"`
	Task      string   `json:"task"`
	Args      []string `json:"args"`
	Content   []byte   `json:"content"`
	ReqSendBy string   `json:"reqSendBy"`
	ReqResBy  string   `json:"reqResBy"`
	Finished  bool     `json:"finished"`
}
