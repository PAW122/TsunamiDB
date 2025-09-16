package routes

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/PAW122/TsunamiDB/servers/network-manager"
	"github.com/PAW122/TsunamiDB/servers/public-api/v1/metrics"
	subServer "github.com/PAW122/TsunamiDB/servers/subscriptions"
)

type apiHealth struct {
	UptimeSeconds      float64 `json:"uptime_seconds"`
	TotalRequests      uint64  `json:"total_requests"`
	AverageResponseMS  float64 `json:"average_response_ms"`
	LastRequestISO8601 string  `json:"last_request_at,omitempty"`
}

type healthResponse struct {
	Status        string                `json:"status"`
	Timestamp     string                `json:"timestamp"`
	API           apiHealth             `json:"api"`
	Subscriptions subServer.Stats       `json:"subscriptions"`
	Network       *networkmanager.Stats `json:"network,omitempty"`
}

func Health(w http.ResponseWriter, r *http.Request, _ *http.Client) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	snap := metrics.SnapshotStats()
	api := apiHealth{
		UptimeSeconds:     time.Since(snap.StartedAt).Seconds(),
		TotalRequests:     snap.TotalRequests,
		AverageResponseMS: snap.AverageResponse.Seconds() * 1000,
	}
	if !snap.LastRequestAt.IsZero() {
		api.LastRequestISO8601 = snap.LastRequestAt.UTC().Format(time.RFC3339Nano)
	}

	subStats := subServer.StatsSnapshot()

	var networkStats *networkmanager.Stats
	if nm := networkmanager.GetNetworkManager(); nm != nil {
		stats := nm.Snapshot()
		networkStats = &stats
	}

	resp := healthResponse{
		Status:        "ok",
		Timestamp:     time.Now().UTC().Format(time.RFC3339Nano),
		API:           api,
		Subscriptions: subStats,
		Network:       networkStats,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
