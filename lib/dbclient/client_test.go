package TsuClient_test

import (
	"bytes"
	"fmt"
	"os"
	"sort"
	"testing"
	"time"

	dataManager_v2 "github.com/PAW122/TsunamiDB/data/dataManager/v2"
	// fileSystem_v1 "github.com/PAW122/TsunamiDB/data/fileSystem/v1"
	TsuClient "github.com/PAW122/TsunamiDB/lib/dbclient"
	"github.com/stretchr/testify/assert"
)

func TestMain(m *testing.M) {
	originalWD, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	_ = os.RemoveAll("db")
	tmpWD, err := os.MkdirTemp("", "tsunamidb-lib-dbclient-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.Chdir(tmpWD); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	TsuClient.InitNetworkManager(5843, []string{})
	TsuClient.InitPublicApi(5844)
	time.Sleep(1500 * time.Millisecond)
	code := m.Run()
	dataManager_v2.ShutdownWorkersForTests()
	_ = os.Chdir(originalWD)
	_ = os.RemoveAll("db")
	_ = os.RemoveAll(tmpWD)
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

func TestClient_SaveReadInc(t *testing.T) {
	table := "test_table_inc"
	key := "client_inc_key"

	first, err := TsuClient.SaveInc(key, table, []byte("first"), TsuClient.SaveIncOptions{
		MaxEntrySize: 16,
		EntryKey:     "first-key",
	})
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), first.ID)

	second, err := TsuClient.SaveInc(key, table, []byte("second"), TsuClient.SaveIncOptions{
		MaxEntrySize: 16,
		EntryKey:     "second-key",
	})
	assert.NoError(t, err)
	assert.Equal(t, uint64(1), second.ID)

	byKey, err := TsuClient.ReadInc(key, table, TsuClient.ReadIncOptions{
		Type:     TsuClient.ReadIncByKey,
		EntryKey: "second-key",
	})
	assert.NoError(t, err)
	assert.Len(t, byKey, 1)
	assert.Equal(t, uint64(1), byKey[0].ID)
	assert.Equal(t, []byte("second"), byKey[0].Data)

	firstEntries, err := TsuClient.ReadInc(key, table, TsuClient.ReadIncOptions{
		Type:   TsuClient.ReadIncFirstEntries,
		Amount: 2,
	})
	assert.NoError(t, err)
	assert.Len(t, firstEntries, 2)
	assert.Equal(t, []byte("first"), firstEntries[0].Data)
	assert.Equal(t, []byte("second"), firstEntries[1].Data)
}

func TestClient_GetKeysByRegex(t *testing.T) {
	table := "test_table_regex"
	keys := []string{"client_regex_alpha", "client_regex_beta"}

	for _, key := range keys {
		err := TsuClient.Save(key, table, []byte("regex-data"))
		assert.NoError(t, err)
	}

	got, err := TsuClient.GetKeysByRegex(table, `^client_regex_`, 10)
	assert.NoError(t, err)
	sort.Strings(got)
	assert.Equal(t, keys, got)

	for _, key := range keys {
		assert.NoError(t, TsuClient.Free(key, table))
	}
}

func TestClient_RelationalWrappers(t *testing.T) {
	table := "client_rel_products"
	created, err := TsuClient.CreateRelationalTable(TsuClient.RelationalSchema{
		Name: table,
		Columns: []TsuClient.RelationalColumn{
			{Name: "id", Type: TsuClient.RelationalColumnTypeUint64, Indexed: true},
			{Name: "name", Type: TsuClient.RelationalColumnTypeString, Size: 32, TrigramIndexed: true},
			{Name: "price", Type: TsuClient.RelationalColumnTypeUint64},
			{Name: "active", Type: TsuClient.RelationalColumnTypeBool},
		},
	})
	assert.NoError(t, err)
	assert.Equal(t, table, created.Name)

	rowID, err := TsuClient.InsertRelationalRow(table, map[string]any{
		"id":     uint64(1),
		"name":   "widget",
		"price":  uint64(100),
		"active": true,
	})
	assert.NoError(t, err)
	assert.Equal(t, uint64(0), rowID)

	row, err := TsuClient.ReadRelationalRow(table, rowID)
	assert.NoError(t, err)
	assert.Equal(t, "widget", row["name"])

	selected, err := TsuClient.SelectRelationalRows(table, TsuClient.RelationalLike("name", "%wid%"))
	assert.NoError(t, err)
	assert.Len(t, selected, 1)
	assert.Equal(t, uint64(0), selected[0].RowID)

	assert.NoError(t, TsuClient.UpdateRelationalRow(table, rowID, map[string]any{"price": uint64(175)}))
	updated, err := TsuClient.ExecuteRelationalSQL("SELECT row_id, name, price FROM client_rel_products WHERE price = 175")
	assert.NoError(t, err)
	assert.Equal(t, "select", updated.Operation)
	assert.Len(t, updated.Rows, 1)
	assert.Equal(t, uint64(175), updated.Rows[0].Values["price"])

	raw, err := TsuClient.ExecuteRelationalSQLJSON("SHOW TABLES")
	assert.NoError(t, err)
	assert.Contains(t, string(raw), `"operation":"show_tables"`)
	assert.Contains(t, string(raw), table)

	assert.NoError(t, TsuClient.DeleteRelationalRow(table, rowID))
	_, err = TsuClient.ReadRelationalRow(table, rowID)
	assert.Error(t, err)
}

func TestClient_SubscriptionWrappers(t *testing.T) {
	_, err := TsuClient.EnableSubscription(nil)
	assert.Error(t, err)

	authKey, err := TsuClient.EnableSubscription([]string{"client_sub_key"})
	assert.NoError(t, err)
	assert.NotEmpty(t, authKey)

	assert.NoError(t, TsuClient.DisableSubscription("client_sub_key"))
	assert.Error(t, TsuClient.DisableSubscription(""))

	targetAuthKey, err := TsuClient.EnableSubscriptionForTargets([]TsuClient.SubscriptionTarget{{Table: "docs", Key: "client_sub_doc"}})
	assert.NoError(t, err)
	assert.NotEmpty(t, targetAuthKey)
	assert.NoError(t, TsuClient.DisableSubscriptionForTarget("docs", "client_sub_doc"))
	assert.Error(t, TsuClient.DisableSubscriptionForTarget("docs", ""))

	err = TsuClient.InitSubscriptionServer("bad-port")
	assert.Error(t, err)
}

func TestClient_ReadWithRevision(t *testing.T) {
	table := "client_docs"
	key := "read_with_revision"

	assert.NoError(t, TsuClient.Save(key, table, []byte("hello")))
	state, err := TsuClient.SetRevisionPolicy(key, table, TsuClient.RevisionModeCurrent)
	assert.NoError(t, err)
	_, state, err = TsuClient.PatchWithRevision(key, table, state.Rev, []TsuClient.PatchOperation{{Offset: 5, Insert: "!"}})
	assert.NoError(t, err)

	result, err := TsuClient.ReadWithRevision(key, table)
	assert.NoError(t, err)
	assert.Equal(t, []byte("hello!"), result.Data)
	assert.Equal(t, TsuClient.RevisionModeCurrent, result.State.Mode)
	assert.Equal(t, state.Rev, result.State.Rev)
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
