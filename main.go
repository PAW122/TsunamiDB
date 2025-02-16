package main

import (
	dataManager_v1 "TsunamiDB/data/dataManager/v1"
	defragmentationManager "TsunamiDB/data/defragmentationManager"
	fileSystem_v1 "TsunamiDB/data/fileSystem/v1"
	encoder_v1 "TsunamiDB/encoding/v1"
	core "TsunamiDB/servers/core"
	"fmt"
)

func main() {
	// test()
	core.RunCore()
}

func test2() {
	// ************ save data to file 1 ************
	encoded, _ := encoder_v1.Encode([]byte("Hello, World"))
	startPtr, endPtr, err := dataManager_v1.SaveDataToFile(encoded, "data.bin")
	if err != nil {
		fmt.Println("Error saving to file:", err)
		return
	}
	err = fileSystem_v1.SaveElementByKey("test5", "data.bin", int(startPtr), int(endPtr))
	if err != nil {
		fmt.Println("Error saving to map:", err)
		return
	}

	// ************ read data from file 1 ************
	fs_data, err := fileSystem_v1.GetElementByKey("test5")
	if err != nil {
		fmt.Println("Error retrieving element from map:", err)
		return
	}
	// ************ defragmentation - key: test6 ************
	fs_data, err = fileSystem_v1.GetElementByKey("test5")
	if err != nil {
		fmt.Println("Error retrieving element from map:", err)
		return
	}
	fileSystem_v1.RemoveElementByKey("test5")
	defragmentationManager.MarkAsFree("test5", "data.bin", int64(fs_data.StartPtr), int64(fs_data.EndPtr))

}

func test() {
	fmt.Println("test - TsunamiDB")

	// ************ save data to file 1 ************
	encoded, _ := encoder_v1.Encode([]byte("Hello, World"))
	startPtr, endPtr, err := dataManager_v1.SaveDataToFile(encoded, "data.bin")
	if err != nil {
		fmt.Println("Error saving to file:", err)
		return
	}
	err = fileSystem_v1.SaveElementByKey("test5", "data.bin", int(startPtr), int(endPtr))
	if err != nil {
		fmt.Println("Error saving to map:", err)
		return
	}

	// ************ save data to file 2 ************
	encoded, _ = encoder_v1.Encode([]byte("Hello"))
	startPtr, endPtr, err = dataManager_v1.SaveDataToFile(encoded, "data.bin")
	if err != nil {
		fmt.Println("Error saving to file:", err)
		return
	}
	fileSystem_v1.SaveElementByKey("test6", "data.bin", int(startPtr), int(endPtr))

	// ************ read data from file 1 ************
	fs_data, err := fileSystem_v1.GetElementByKey("test5")
	if err != nil {
		fmt.Println("Error retrieving element from map:", err)
		return
	}
	// 🔹 Odczytanie danych z pliku według wskaźników z mapy
	data, err := dataManager_v1.ReadDataFromFile("data.bin", int64(fs_data.StartPtr), int64(fs_data.EndPtr))
	if err != nil {
		fmt.Println("Error reading from file:", err)
		return
	}
	decoded_obj := encoder_v1.Decode(data)
	fmt.Println("Decoded res:", decoded_obj.Data)

	// ************ read data from file 2 ************
	fs_data, err = fileSystem_v1.GetElementByKey("test6")
	if err != nil {
		fmt.Println("Error retrieving element from map:", err)
		return
	}
	data, err = dataManager_v1.ReadDataFromFile("data.bin", int64(fs_data.StartPtr), int64(fs_data.EndPtr))
	if err != nil {
		fmt.Println("Error reading from file:", err)
		return
	}
	decoded_obj = encoder_v1.Decode(data)
	fmt.Println("Decoded res:", decoded_obj.Data)

	// ************ defragmentation - key: test6 ************
	fs_data, err = fileSystem_v1.GetElementByKey("test6")
	if err != nil {
		fmt.Println("Error retrieving element from map:", err)
		return
	}
	fileSystem_v1.RemoveElementByKey("test6")
	defragmentationManager.MarkAsFree("test6", "data.bin", int64(fs_data.StartPtr), int64(fs_data.EndPtr))

	// ************ try to read from test6 ************
	// save prts for later tests
	old_startPtr := int64(fs_data.StartPtr)
	old_endPtr := int64(fs_data.EndPtr)
	data, err = dataManager_v1.ReadDataFromFile("data.bin", int64(fs_data.StartPtr), int64(fs_data.EndPtr))
	if err != nil {
		fmt.Println("Error reading from file:", err)
		return
	}
	decoded_obj = encoder_v1.Decode(data)
	fmt.Println("Decoded (deleted) res:", decoded_obj.Data)

	// ************ read data from map after deleted ************
	// correctly returns error bc key is deleted
	/*
		fs_data, err = fileSystem_v1.GetElementByKey("test6")
		if err != nil {
			fmt.Println("Error retrieving element from map:", err)
			return
		}
		data, err = dataManager_v1.ReadDataFromFile("data.bin", int64(fs_data.StartPtr), int64(fs_data.EndPtr))
		if err != nil {
			fmt.Println("Error reading from file:", err)
			return
		}
		decoded_obj = encoder_v1.Decode(data)
		fmt.Println("Decoded (deleted) res:", decoded_obj.Data)
	*/

	// ************ write data to defragmented space ************
	encoded, _ = encoder_v1.Encode([]byte("Helo"))
	startPtr, endPtr, err = dataManager_v1.SaveDataToFile(encoded, "data.bin")
	if err != nil {
		fmt.Println("Error saving to file:", err)
		return
	}
	fileSystem_v1.SaveElementByKey("test6", "data.bin", int(startPtr), int(endPtr))

	// ************ read data from defragmented space using old ptrs************
	data, err = dataManager_v1.ReadDataFromFile("data.bin", int64(old_startPtr), int64(old_endPtr))
	if err != nil {
		fmt.Println("Error reading from file:", err)
		return
	}
	decoded_obj = encoder_v1.Decode(data)
	fmt.Println("Decoded (old ptrs) res:", decoded_obj.Data)

	// ************ read data from defragmented space using new ptrs ************
	data, err = dataManager_v1.ReadDataFromFile("data.bin", int64(startPtr), int64(endPtr))
	if err != nil {
		fmt.Println("Error reading from file:", err)
		return
	}
	decoded_obj = encoder_v1.Decode(data)
	fmt.Println("Decoded (overwriten ptrs) res:", decoded_obj.Data)
}
