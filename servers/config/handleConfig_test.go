package config

import (
	"encoding/json"
	"testing"
)

func TestConfigJSONShapeAndLoadConfigNoop(t *testing.T) {
	raw := []byte(`{"tsu_network_config":{"servers":[{"ip":"127.0.0.1","type":"sync-node"}]}}`)
	var cfg Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	if len(cfg.Tsu_network_config.Servers) != 1 {
		t.Fatalf("unexpected servers: %+v", cfg)
	}
	if cfg.Tsu_network_config.Servers[0].Ip != "127.0.0.1" || cfg.Tsu_network_config.Servers[0].Type != "sync-node" {
		t.Fatalf("unexpected server config: %+v", cfg.Tsu_network_config.Servers[0])
	}
	LoadConfig("unused")
}
