package TsuClient

import (
	export "TsunamiDB/lib/export"
	debug "TsunamiDB/servers/debug"
	networkmanager "TsunamiDB/servers/network-manager"
	public_api_v1 "TsunamiDB/servers/public-api/v1"
)

/*
	przygotować funkcje wyższego poziomu
	np save() tak aby nie trzeba było wykoywać wszystkich calli ręcznie.

	init(port, data dir)

	save()
	read()
	free()
	save_encrypt()
	read_encrypt()

	startTsuNetwork(port, []knownPeers) // uruchomienie
	getConectedServers() // zwraca liste serwerów z sieci Tsu
	sendToServer() // pozwala na wysłanie req do serwera
	// jakiś on.IncomingMsg()



*/

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
