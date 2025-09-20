package tests

import (
	"fmt"
	"sync"
	"time"

	dataManager_v2 "github.com/PAW122/TsunamiDB/data/dataManager/v2"
	fileSystem_v1 "github.com/PAW122/TsunamiDB/data/fileSystem/v1"
	encoder_v1 "github.com/PAW122/TsunamiDB/encoding/v1"
	debug "github.com/PAW122/TsunamiDB/servers/debug"
)

func TestAsyncSaveAndRead(amount int) {
	start := time.Now()
	var wg sync.WaitGroup

	saveStart := time.Now()
	saveTimes := make([]time.Duration, 0, amount)
	readTimes := make([]time.Duration, 0, amount)

	// Kanały do przekazywania wyników
	saveChan := make(chan string, amount)
	readChan := make(chan struct {
		key  string
		data string
		err  error
	}, amount)

	// Funkcja asynchronicznego zapisu
	asyncSave := func(key string, data string) {
		defer wg.Done()
		localStart := time.Now()
		encoded, _ := encoder_v1.Encode([]byte(data))
		startPtr, endPtr, err := dataManager_v2.SaveDataToFileAsync(encoded, "data.bin")
		if err != nil {
			fmt.Printf("❌ Error saving %s: %v\n", key, err)
			return
		}
		_, _, err = fileSystem_v1.SaveElementByKey("data.bin", key, int(startPtr), int(endPtr))
		if err != nil {
			fmt.Printf("❌ Error mapping key %s: %v\n", key, err)
			return
		}
		saveChan <- key
		saveTimes = append(saveTimes, time.Since(localStart))
	}

	// Funkcja asynchronicznego odczytu
	asyncRead := func(key string) {
		defer wg.Done()
		localStart := time.Now()
		elem, err := fileSystem_v1.GetElementByKey("data.bin", key)
		if err != nil {
			readChan <- struct {
				key  string
				data string
				err  error
			}{key, "", err}
			return
		}
		rawData, err := dataManager_v2.ReadDataFromFileAsync(elem.FileName, int64(elem.StartPtr), int64(elem.EndPtr))
		if err != nil {
			readChan <- struct {
				key  string
				data string
				err  error
			}{key, "", err}
			return
		}
		decoded := encoder_v1.Decode(rawData)
		readChan <- struct {
			key  string
			data string
			err  error
		}{key, decoded.Data, nil}
		readTimes = append(readTimes, time.Since(localStart))
	}

	// Uruchomienie zapisów
	for i := 0; i < amount; i++ {
		wg.Add(1)
		go asyncSave(fmt.Sprintf("key%d", i), fmt.Sprintf("data-%d", i))
	}

	// Czekanie na zakończenie zapisów
	wg.Wait()
	close(saveChan)
	totalSaveTime := time.Since(saveStart)
	avgSaveTime := totalSaveTime / time.Duration(amount)
	fmt.Printf("Total save time: %v, Avg save time: %v\n", totalSaveTime, avgSaveTime)

	readStart := time.Now()

	// Uruchomienie odczytów dla zakończonych zapisów
	for key := range saveChan {
		wg.Add(1)
		go asyncRead(key)
	}

	// Czekanie na zakończenie odczytów
	wg.Wait()
	close(readChan)
	totalReadTime := time.Since(readStart)
	avgReadTime := totalReadTime / time.Duration(amount)
	fmt.Printf("Total read time: %v, Avg read time: %v\n", totalReadTime, avgReadTime)

	// Podsumowanie wyników
	for res := range readChan {
		if res.err == nil {
			//fmt.Printf("✅ Read %s -> Data: %s\n", res.key, res.data)
		} else {
			fmt.Printf("❌ Failed to read %s: %v\n", res.key, res.err)
		}
	}

	fmt.Printf("Total execution time: %v\n", time.Since(start))
	debug.PrintTimingStats()
}
