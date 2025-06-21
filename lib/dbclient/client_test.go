package TsuClient_test

import (
	"bytes"
	"os"
	"testing"
	"time"

	// fileSystem_v1 "github.com/PAW122/TsunamiDB/data/fileSystem/v1"
	TsuClient "github.com/PAW122/TsunamiDB/lib/dbclient"
	"github.com/stretchr/testify/assert"
)

func TestMain(m *testing.M) {
	TsuClient.InitNetworkManager(5843, []string{})
	TsuClient.InitPublicApi(5844)
	time.Sleep(1500 * time.Millisecond)
	code := m.Run()
	os.Exit(code)
}

func TestClient_SaveReadFree(t *testing.T) {
	table := "test_table"
	key := "client_test_key"
	data := []byte("Hello, TsunamiDB!")

	t.Log("Zapisujemy dane przy użyciu TsuClient.Save")
	err := TsuClient.Save(key, table, data)
	assert.NoError(t, err)
	time.Sleep(500 * time.Millisecond)

	t.Log("Odczytujemy dane przy użyciu TsuClient.Read")
	read, err := TsuClient.Read(key, table)
	assert.NoError(t, err)
	assert.True(t, bytes.Equal(data, read), "odczytane dane nie są zgodne z zapisanymi")

	t.Log("Usuwamy dane przy użyciu TsuClient.Free")
	err = TsuClient.Free(key, table)
	assert.NoError(t, err)
	time.Sleep(500 * time.Millisecond)

	_, err = TsuClient.Read(key, table)
	assert.Error(t, err, "oczekiwano błędu po usunięciu klucza")
}

func TestClient_SaveReadEncrypted(t *testing.T) {
	table := "test_table"
	key := "encrypted_key"
	encKey := "very_secret_key"
	data := []byte("Encrypted data content")

	t.Log("Zapisujemy zaszyfrowane dane przy użyciu TsuClient.SaveEncrypted")
	err := TsuClient.SaveEncrypted(key, table, encKey, data)
	assert.NoError(t, err)
	time.Sleep(500 * time.Millisecond)

	t.Log("Odczytujemy zaszyfrowane dane przy użyciu TsuClient.ReadEncrypted")
	read, err := TsuClient.ReadEncrypted(key, table, encKey)
	assert.NoError(t, err)
	assert.True(t, bytes.Equal(data, read), "odszyfrowane dane nie są zgodne z zapisanymi")

	t.Log("Usuwamy dane")
	err = TsuClient.Free(key, table)
	assert.NoError(t, err)
	time.Sleep(500 * time.Millisecond)

	_, err = TsuClient.ReadEncrypted(key, table, encKey)
	assert.Error(t, err, "oczekiwano błędu po usunięciu klucza")
}

// func TestClient_PersistenceAfterRestart(t *testing.T) {
// 	table := "test_table"
// 	key := "persist_test_key"
// 	data := []byte("Persistent test data")

// 	t.Log("Zapisujemy dane przy użyciu TsuClient.Save")
// 	err := TsuClient.Save(key, table, data)
// 	assert.NoError(t, err)
// 	time.Sleep(1 * time.Second) // daj czas na zapis

// 	t.Log("Symulujemy restart poprzez reset mapLoaded i ponowne wczytanie mapy")
// 	// symulacja restartu: reset mapy i ponowne jej załadowanie
// 	os.Unsetenv("TSU_DB_TEST_MODE")
// 	fileSystem_v1.ResetMapForTesting() // dodaj metodę exportowaną do resetowania mapy i jej ponownego załadowania
// 	time.Sleep(500 * time.Millisecond)

// 	t.Log("Odczytujemy dane po restarcie")
// 	read, err := TsuClient.Read(key, table)
// 	assert.NoError(t, err)
// 	assert.True(t, bytes.Equal(data, read), "dane po restarcie nie są zgodne z zapisanymi")

// 	_ = TsuClient.Free(key, table)
// }
