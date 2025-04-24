package main

import (

	// dataManager_v2 "github.com/PAW122/TsunamiDB/data/dataManager/v2"

	// tests "github.com/PAW122/TsunamiDB/tests"

	"fmt"
	"net/http"
	"time"

	core "github.com/PAW122/TsunamiDB/servers/core"
	dump "github.com/PAW122/TsunamiDB/servers/debug/pprof"
)

func main() {
	go func() {
		fmt.Println("Starting pprof server on http://localhost:6060")
		if err := http.ListenAndServe("localhost:6060", nil); err != nil {
			fmt.Printf("Error starting pprof server: %v\n", err)
		}
	}()

	dump.StartAutomaticDump(1*time.Minute, "./pprof_dumps")

	// test()
	// tests.TestAsyncSaveAndRead(10000)
	// return
	core.RunCore()
}
