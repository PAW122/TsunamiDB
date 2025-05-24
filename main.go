package main

import (

	// dataManager_v2 "github.com/PAW122/TsunamiDB/data/dataManager/v2"

	tests "github.com/PAW122/TsunamiDB/tests"
	// core "github.com/PAW122/TsunamiDB/servers/core"
)

func main() {
	// test()
	tests.TestAsyncSaveAndRead(10000)
	// return
	// core.RunCore()
}
