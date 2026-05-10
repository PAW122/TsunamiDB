package TsuClient

import (
	fileSystem_v1 "github.com/PAW122/TsunamiDB/data/fileSystem/v1"
	export "github.com/PAW122/TsunamiDB/lib/export"
	debug "github.com/PAW122/TsunamiDB/servers/debug"
	networkmanager "github.com/PAW122/TsunamiDB/servers/network-manager"
	public_api_v1 "github.com/PAW122/TsunamiDB/servers/public-api/v1"
	subServer "github.com/PAW122/TsunamiDB/servers/subscriptions"
)

type SaveIncMode = export.SaveIncMode

const (
	SaveIncModeAppend    = export.SaveIncModeAppend
	SaveIncModeOverwrite = export.SaveIncModeOverwrite
)

type IncCountFrom = export.IncCountFrom

const (
	IncCountFromTop    = export.IncCountFromTop
	IncCountFromBottom = export.IncCountFromBottom
)

type ReadIncType = export.ReadIncType

const (
	ReadIncByID         = export.ReadIncByID
	ReadIncByKey        = export.ReadIncByKey
	ReadIncFirstEntries = export.ReadIncFirstEntries
	ReadIncLastEntries  = export.ReadIncLastEntries
)

type SaveIncOptions = export.SaveIncOptions
type SaveIncResult = export.SaveIncResult
type ReadIncOptions = export.ReadIncOptions
type IncEntry = export.IncEntry
type PatchOperation = export.PatchOperation

func Save(key, table string, data []byte) error {
	defer debug.MeasureTime("[lib.dbclient] [save]")()
	return export.Save(key, table, data)
}

func Read(key, table string) ([]byte, error) {
	defer debug.MeasureTime("[lib.dbclient] [read]")()
	return export.Read(key, table)
}

func Patch(key, table string, ops []PatchOperation) ([]byte, error) {
	defer debug.MeasureTime("[lib.dbclient] [patch]")()
	return export.Patch(key, table, ops)
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

func SaveInc(key, table string, data []byte, options SaveIncOptions) (SaveIncResult, error) {
	defer debug.MeasureTime("[lib.dbclient] [save-inc]")()
	return export.SaveInc(key, table, data, options)
}

func ReadInc(key, table string, options ReadIncOptions) ([]IncEntry, error) {
	defer debug.MeasureTime("[lib.dbclient] [read-inc]")()
	return export.ReadInc(key, table, options)
}

func InitNetworkManager(port int, knownPeers []string) {
	defer debug.Log("[lib.dbclient] [Init-Network-Manager]")
	go networkmanager.StartNetworkManager(port, knownPeers)
}

func InitPublicApi(port int) {
	defer debug.Log("[lib.dbclient] [Init-Public-Api]")
	go public_api_v1.RunPublicApi_v1(port)
}

func GetKeysByRegex(table, regex string, max int) ([]string, error) {
	defer debug.MeasureTime("[lib.dbclient] [get keys by regex]")()
	return fileSystem_v1.GetKeysByRegex(table, regex, max)
}

// Sub System
func InitSubscriptionServer(port string) error {
	return subServer.StartWSServer(port)
}

// TODO test
func EnableSubscription(keys []string) (string, error) {
	defer debug.MeasureTime("[lib.dbclient] [EnableSubscription]")()
	return subServer.EnableSubscriptionInternal(keys)
}

func DisableSubscription(key string) error {
	defer debug.MeasureTime("[lib.dbclient] [DisableSubscription]")()
	_, error := subServer.DisableSubscriptionInternal(key)
	return error
}
