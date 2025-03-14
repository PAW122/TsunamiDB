package TsuClient

import (
	export "github.com/PAW122/TsunamiDB/lib/export"
	debug "github.com/PAW122/TsunamiDB/servers/debug"
	networkmanager "github.com/PAW122/TsunamiDB/servers/network-manager"
	public_api_v1 "github.com/PAW122/TsunamiDB/servers/public-api/v1"
)

func Save(key, table string, data []byte) error {
	defer debug.MeasureTime("[lib.dbclient] [save]")()
	return export.Save(key, table, data)
}

func Read(key, table string) ([]byte, error) {
	defer debug.MeasureTime("[lib.dbclient] [read]")()
	return export.Read(key, table)
}

func Free(key, table string) error {
	defer debug.MeasureTime("[lib.dbclient] [free]")()
	return export.Free(key, table)
}

func SaveEncrypted(key, table, encryption_key string, data []byte) error {
	defer debug.MeasureTime("[lib.dbclient] [save-encrypted]")()
	return export.SaveEncrypted(key, table, encryption_key, data)
}

func ReadEncrypted(key, table, encryption_key string) ([]byte, error) {
	defer debug.MeasureTime("[lib.dbclient] [read-encrypted]")()
	return export.ReadEncrypted(key, table, encryption_key)
}

func InitNetworkManager(port int, knownPeers []string) {
	defer debug.Log("[lib.dbclient] [Init-Network-Manager]")
	go networkmanager.StartNetworkManager(port, knownPeers)
}

func InitPublicApi(port int) {
	defer debug.Log("[lib.dbclient] [Init-Public-Api]")
	go public_api_v1.RunPublicApi_v1(port)
}
