package main

import (
	encoder_v1 "TsunamiDB/encoding/v1"
	// types "TsunamiDB/types"
	dataManager_v1 "TsunamiDB/data/dataManager/v1"
	fileSystem_v1 "TsunamiDB/data/fileSystem/v1"
	"fmt"
)

func main() {
	fmt.Println("TsunamiDB")

	// Enkodowanie
	encoded, res_data := encoder_v1.Encode("Hello, World!")
	fmt.Println("Encoded:", encoded)
	// encoder_v1.Encode("test2")

	// Zapis do pliku
	fileSystem_v1.SaveElementByKey("test", "data.bin", res_data.StartPointer, res_data.EndPointer)
	dataManager_v1.SaveDataToFile(encoded, "data.bin")

	// Dekodowanie z zakodowanych danych
	decoded := encoder_v1.Decode(encoded)
	fmt.Println("Decoded:", decoded)

	// Odczytanie danych z pliku według pointerów
	data, err := dataManager_v1.ReadDataFromFile("data.bin", decoded.StartPointer, decoded.EndPointer)
	if err != nil {
		fmt.Println("Error reading from file:", err)
		return
	}

	fmt.Println("Read from file:", data)

	// Ponowne dekodowanie odczytanych danych
	decoded_string := encoder_v1.DecodeRawData(data)
	fmt.Println("Decoded again:", decoded_string)

	fmt.Println("Map Test")

	fs_data, err := fileSystem_v1.GetElementByKey("test")
	if err != nil {
		fmt.Print(err)
		return
	}

	fmt.Println("FS Data:", fs_data)
	data2, err := dataManager_v1.ReadDataFromFile("data.bin", fs_data.StartPtr, fs_data.EndPtr)
	if err != nil {
		fmt.Println("Error reading from file:", err)
		return
	}

	fmt.Println("Read from file:", data2)

	// Ponowne dekodowanie odczytanych danych
	decoded_string2 := encoder_v1.DecodeRawData(data2)
	fmt.Println("Decoded res:", decoded_string2)
}
