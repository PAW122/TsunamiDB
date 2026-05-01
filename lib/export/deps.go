package export

import (
	dataManager_v1 "github.com/PAW122/TsunamiDB/data/dataManager/v1"
	dataManager_v2 "github.com/PAW122/TsunamiDB/data/dataManager/v2"
	defragManager "github.com/PAW122/TsunamiDB/data/defragmentationManager"
	fileSystem_v1 "github.com/PAW122/TsunamiDB/data/fileSystem/v1"
	encoder_v1 "github.com/PAW122/TsunamiDB/encoding/v1"
	networkmanager "github.com/PAW122/TsunamiDB/servers/network-manager"
	subServer "github.com/PAW122/TsunamiDB/servers/subscriptions"
)

var (
	encode                = encoder_v1.Encode
	decode                = encoder_v1.Decode
	encrypt               = encoder_v1.Encrypt
	decrypt               = encoder_v1.Decrypt
	saveDataToFileAsync   = dataManager_v2.SaveDataToFileAsync
	readDataFromFileAsync = dataManager_v2.ReadDataFromFileAsync
	readDataFromFile      = dataManager_v1.ReadDataFromFile
	saveElementByKey      = fileSystem_v1.SaveElementByKey
	getElementByKey       = fileSystem_v1.GetElementByKey
	removeElementByKey    = fileSystem_v1.RemoveElementByKey
	recordDefragFree      = fileSystem_v1.RecordDefragFree
	recordDefragSkip      = fileSystem_v1.RecordDefragSkip
	markAsFree            = defragManager.MarkAsFree
	getNetworkManager     = networkmanager.GetNetworkManager
	notifySubscribers     = subServer.NotifySubscribers
	notifyDeleteAndRemove = subServer.NotifyDeleteAndRemove
)
