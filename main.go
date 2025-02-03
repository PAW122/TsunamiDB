package main

import (
	dataManager_v1 "TsunamiDB/data/dataManager/v1"
	fileSystem_v1 "TsunamiDB/data/fileSystem/v1"
	encoder_v1 "TsunamiDB/encoding/v1"
	"fmt"
)

func main() {
	fmt.Println("TsunamiDB")

	// 🔹 Testowanie /data & /encoding
	// todo: połączyć do core

	// 🔹 Enkodowanie, zapis do pliku & mapy
	encoded, _ := encoder_v1.Encode("Hello, World")
	startPtr, endPtr, err := dataManager_v1.SaveDataToFile(encoded, "data.bin")
	if err != nil {
		fmt.Println("Error saving to file:", err)
		return
	}
	fileSystem_v1.SaveElementByKey("test5", "data.bin", int(startPtr), int(endPtr))

	encoded, _ = encoder_v1.Encode("Hello")
	startPtr, endPtr, err = dataManager_v1.SaveDataToFile(encoded, "data.bin")
	if err != nil {
		fmt.Println("Error saving to file:", err)
		return
	}
	fileSystem_v1.SaveElementByKey("test6", "data.bin", int(startPtr), int(endPtr))

	// 🔹 Pobranie wskaźników z mapy
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

	// 🔹 Ponowne dekodowanie odczytanych danych
	decoded_obj := encoder_v1.Decode(data)
	fmt.Println("Decoded res:", decoded_obj.Data)

	//2
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

	// 🔹 Ponowne dekodowanie odczytanych danych
	decoded_obj = encoder_v1.Decode(data)
	fmt.Println("Decoded res:", decoded_obj.Data)
}
