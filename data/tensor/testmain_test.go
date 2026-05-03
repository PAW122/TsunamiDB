package tensor

import (
	"os"
	"testing"

	dataManager_v2 "github.com/PAW122/TsunamiDB/data/dataManager/v2"
	fileSystem_v1 "github.com/PAW122/TsunamiDB/data/fileSystem/v1"
)

func TestMain(m *testing.M) {
	_ = os.RemoveAll("db")
	code := m.Run()
	dataManager_v2.ShutdownWorkersForTests()
	fileSystem_v1.ShutdownForTests()
	_ = os.RemoveAll("db")
	os.Exit(code)
}
