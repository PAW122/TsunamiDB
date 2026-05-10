package export

import (
	dataManager_v1 "github.com/PAW122/TsunamiDB/data/dataManager/v1"
	dataManager_v2 "github.com/PAW122/TsunamiDB/data/dataManager/v2"
	defragManager "github.com/PAW122/TsunamiDB/data/defragmentationManager"
	fileSystem_v1 "github.com/PAW122/TsunamiDB/data/fileSystem/v1"
	"github.com/PAW122/TsunamiDB/data/revision"
	encoder_v1 "github.com/PAW122/TsunamiDB/encoding/v1"
	networkmanager "github.com/PAW122/TsunamiDB/servers/network-manager"
	subServer "github.com/PAW122/TsunamiDB/servers/subscriptions"
)

var (
	encode                                  = encoder_v1.Encode
	decode                                  = encoder_v1.Decode
	encodeIncEntry                          = encoder_v1.EncodeIncEntry
	decodeIncEntry                          = encoder_v1.DecodeIncEntry
	encrypt                                 = encoder_v1.Encrypt
	decrypt                                 = encoder_v1.Decrypt
	saveDataToFileAsync                     = dataManager_v2.SaveDataToFileAsync
	readDataFromFileAsync                   = dataManager_v2.ReadDataFromFileAsync
	readDataFromFile                        = dataManager_v1.ReadDataFromFile
	saveIncData                             = dataManager_v2.SaveIncDataToFileAsync
	saveIncDataPut                          = dataManager_v2.SaveIncDataToFileAsync_Put
	saveIncDataOverwrite                    = dataManager_v2.SaveIncDataToFileAsync_OverWrite
	readIncDataByID                         = dataManager_v2.ReadIncDataFromFileAsync_ById
	readIncDataLast                         = dataManager_v2.ReadIncDataFromFileAsync_LastEntries
	readIncDataFirst                        = dataManager_v2.ReadIncDataFromFileAsync_FirstEntries
	getIncRecordCount                       = dataManager_v2.GetIncRecordCount
	saveElementByKey                        = fileSystem_v1.SaveElementByKey
	getElementByKey                         = fileSystem_v1.GetElementByKey
	removeElementByKey                      = fileSystem_v1.RemoveElementByKey
	recordDefragFree                        = fileSystem_v1.RecordDefragFree
	recordDefragSkip                        = fileSystem_v1.RecordDefragSkip
	markAsFree                              = defragManager.MarkAsFree
	getNetworkManager                       = networkmanager.GetNetworkManager
	notifySubscribers                       = subServer.NotifySubscribers
	notifySubscribersWithRevision           = subServer.NotifySubscribersWithRevision
	notifyTableSubscribers                  = subServer.NotifyTableSubscribers
	notifyTableSubscribersWithRevision      = subServer.NotifyTableSubscribersWithRevision
	notifyPatchSubscribers                  = subServer.NotifyPatchSubscribers
	notifyPatchSubscribersWithRevision      = subServer.NotifyPatchSubscribersWithRevision
	notifyTablePatchSubscribers             = subServer.NotifyTablePatchSubscribers
	notifyTablePatchSubscribersWithRevision = subServer.NotifyTablePatchSubscribersWithRevision
	notifyDeleteAndRemove                   = subServer.NotifyDeleteAndRemove
	notifyTableDeleteAndRemove              = subServer.NotifyTableDeleteAndRemove
	advanceFullWriteRevision                = revision.AdvanceFullWrite
	checkPatchRevision                      = revision.CheckPatch
	advancePatchRevision                    = revision.AdvancePatch
	setRevisionPolicy                       = revision.SetPolicy
	getRevisionState                        = revision.GetState
	getRevisionHistory                      = revision.History
)
