package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const dllConsumerC = `
#include <windows.h>
#include <stdio.h>
#include <string.h>

typedef int  (*SaveFn)(char*, char*, char*, int);
typedef int  (*ReadFn)(char*, char*, char**, int*);
typedef void (*FreeBufFn)(char*);
typedef int  (*FreeFn)(char*, char*);
typedef int  (*SaveEncryptedFn)(char*, char*, char*, char*, int);
typedef int  (*ReadEncryptedFn)(char*, char*, char*, char**, int*);
typedef int  (*RelationalSQLFn)(char*, char**, int*);
typedef void (*InitNetworkManagerFn)(int, char**, int);
typedef void (*InitPublicApiFn)(int);
typedef int  (*InitSubscriptionServerFn)(char*);
typedef int  (*EnableSubscriptionFn)(char**, int, char**);
typedef int  (*DisableSubscriptionFn)(char*);
typedef int  (*GetKeysByRegexFn)(char*, char*, int, char***, int*);
typedef void (*FreeKeysArrayFn)(char**, int);

static InitSubscriptionServerFn g_init_subscription_server = NULL;

static DWORD WINAPI subscription_thread(LPVOID arg) {
	(void)arg;
	int rc = g_init_subscription_server("0");
	fprintf(stderr, "InitSubscriptionServer returned unexpectedly: %d\n", rc);
	return (DWORD)(rc == 0 ? 0 : 1);
}

#define CHECK(cond, msg) do { \
	if (!(cond)) { \
		fprintf(stderr, "check failed: %s at line %d\n", msg, __LINE__); \
		return 1; \
	} \
} while (0)

#define LOAD_FN(type, name) \
	type name = (type)GetProcAddress(dll, #name); \
	CHECK(name != NULL, "missing export " #name)

static int read_exact(ReadFn read_fn, FreeBufFn free_buf, char* key, char* table, const char* want, int want_len) {
	char* out = NULL;
	int out_len = -1;
	if (read_fn(key, table, &out, &out_len) != 0) {
		fprintf(stderr, "read failed for key: %s\n", key);
		return 1;
	}
	if (out_len != want_len) {
		fprintf(stderr, "read length mismatch for %s: got %d want %d\n", key, out_len, want_len);
		free_buf(out);
		return 1;
	}
	if (memcmp(out, want, want_len) != 0) {
		fprintf(stderr, "read data mismatch for key: %s\n", key);
		free_buf(out);
		return 1;
	}
	free_buf(out);
	return 0;
}

static int read_encrypted_exact(ReadEncryptedFn read_fn, FreeBufFn free_buf, char* key, char* table, char* enc_key, const char* want, int want_len) {
	char* out = NULL;
	int out_len = -1;
	if (read_fn(key, table, enc_key, &out, &out_len) != 0) {
		fprintf(stderr, "encrypted read failed for key: %s\n", key);
		return 1;
	}
	if (out_len != want_len) {
		fprintf(stderr, "encrypted read length mismatch for %s: got %d want %d\n", key, out_len, want_len);
		free_buf(out);
		return 1;
	}
	if (memcmp(out, want, want_len) != 0) {
		fprintf(stderr, "encrypted read data mismatch for key: %s\n", key);
		free_buf(out);
		return 1;
	}
	free_buf(out);
	return 0;
}

static int contains_key(char** keys, int count, const char* want) {
	for (int i = 0; i < count; i++) {
		if (strcmp(keys[i], want) == 0) {
			return 1;
		}
	}
	return 0;
}

int main(int argc, char** argv) {
	CHECK(argc == 2, "usage: consumer.exe <dll-path>");

	HMODULE dll = LoadLibraryA(argv[1]);
	if (dll == NULL) {
		fprintf(stderr, "LoadLibraryA failed: %lu\n", GetLastError());
		return 1;
	}

	LOAD_FN(SaveFn, Save);
	LOAD_FN(ReadFn, Read);
	LOAD_FN(FreeBufFn, FreeBuf);
	LOAD_FN(FreeFn, Free);
	LOAD_FN(SaveEncryptedFn, SaveEncrypted);
	LOAD_FN(ReadEncryptedFn, ReadEncrypted);
	LOAD_FN(RelationalSQLFn, RelationalSQL);
	LOAD_FN(InitNetworkManagerFn, InitNetworkManager);
	LOAD_FN(InitPublicApiFn, InitPublicApi);
	LOAD_FN(InitSubscriptionServerFn, InitSubscriptionServer);
	LOAD_FN(EnableSubscriptionFn, EnableSubscription);
	LOAD_FN(DisableSubscriptionFn, DisableSubscription);
	LOAD_FN(GetKeysByRegexFn, GetKeysByRegex);
	LOAD_FN(FreeKeysArrayFn, FreeKeysArray);

	char* table = "dll_integration_table";
	char plain[] = {'p', 'l', 'a', 'i', 'n', '\0', 'd', 'a', 't', 'a'};
	char encrypted[] = "encrypted-payload";
	char persistent[] = "persistent-payload";
	char* enc_key = "dll-secret";

	InitNetworkManager(0, NULL, 0);
	InitPublicApi(0);
	g_init_subscription_server = InitSubscriptionServer;
	HANDLE sub_thread = CreateThread(NULL, 0, subscription_thread, NULL, 0, NULL);
	CHECK(sub_thread != NULL, "InitSubscriptionServer thread");
	Sleep(200);
	DWORD sub_code = 0;
	CHECK(GetExitCodeThread(sub_thread, &sub_code), "GetExitCodeThread");
	CHECK(sub_code == STILL_ACTIVE, "InitSubscriptionServer should keep serving");
	CloseHandle(sub_thread);

	CHECK(Save("dll_plain_key", table, plain, (int)sizeof(plain)) == 0, "Save plain");
	CHECK(read_exact(Read, FreeBuf, "dll_plain_key", table, plain, (int)sizeof(plain)) == 0, "Read plain data");

	CHECK(SaveEncrypted("dll_encrypted_key", table, enc_key, encrypted, (int)strlen(encrypted)) == 0, "SaveEncrypted");
	CHECK(read_encrypted_exact(ReadEncrypted, FreeBuf, "dll_encrypted_key", table, enc_key, encrypted, (int)strlen(encrypted)) == 0, "ReadEncrypted data");

	char* sql_out = NULL;
	int sql_len = -1;
	CHECK(RelationalSQL("CREATE TABLE dll_rel_products (id uint64 PRIMARY KEY, name string(32) INDEXED TRIGRAM, price uint64, active bool)", &sql_out, &sql_len) == 0, "RelationalSQL create table");
	CHECK(sql_out != NULL && sql_len > 0, "RelationalSQL create returns JSON");
	CHECK(strstr(sql_out, "\"operation\":\"create_table\"") != NULL, "RelationalSQL create operation");
	FreeBuf(sql_out);

	sql_out = NULL;
	sql_len = -1;
	CHECK(RelationalSQL("INSERT INTO dll_rel_products (id, name, price, active) VALUES (1, 'widget', 100, true)", &sql_out, &sql_len) == 0, "RelationalSQL insert");
	CHECK(sql_out != NULL && strstr(sql_out, "\"row_id\":0") != NULL, "RelationalSQL insert row ID");
	FreeBuf(sql_out);

	sql_out = NULL;
	sql_len = -1;
	CHECK(RelationalSQL("SELECT row_id, name, price FROM dll_rel_products WHERE name LIKE '%wid%'", &sql_out, &sql_len) == 0, "RelationalSQL select");
	CHECK(sql_out != NULL && strstr(sql_out, "\"name\":\"widget\"") != NULL, "RelationalSQL select name");
	CHECK(strstr(sql_out, "\"price\":100") != NULL, "RelationalSQL select price");
	FreeBuf(sql_out);

	sql_out = (char*)1;
	sql_len = -1;
	CHECK(RelationalSQL("DROP TABLE dll_rel_products", &sql_out, &sql_len) == -1, "RelationalSQL rejects unsupported SQL");
	CHECK(sql_out == NULL && sql_len == 0, "RelationalSQL clears output on failure");

	CHECK(Save("dll_persist_key", table, persistent, (int)strlen(persistent)) == 0, "Save persistent");
	CHECK(read_exact(Read, FreeBuf, "dll_persist_key", table, persistent, (int)strlen(persistent)) == 0, "Read persistent data");

	char** keys = NULL;
	int count = -1;
	CHECK(GetKeysByRegex(table, "^dll_", 20, &keys, &count) == 0, "GetKeysByRegex before delete");
	CHECK(count >= 3, "regex should return saved keys");
	CHECK(contains_key(keys, count, "dll_plain_key"), "regex contains plain key");
	CHECK(contains_key(keys, count, "dll_encrypted_key"), "regex contains encrypted key");
	CHECK(contains_key(keys, count, "dll_persist_key"), "regex contains persistent key");
	FreeKeysArray(keys, count);

	char* sub_keys[] = {"dll_sub_key", "dll_plain_key"};
	char* auth_key = NULL;
	CHECK(EnableSubscription(sub_keys, 2, &auth_key) == 0, "EnableSubscription");
	CHECK(auth_key != NULL && strlen(auth_key) > 0, "EnableSubscription auth key");
	FreeBuf(auth_key);
	CHECK(EnableSubscription(NULL, 0, &auth_key) == -1, "EnableSubscription rejects empty keys");
	CHECK(DisableSubscription("dll_sub_key") == 0, "DisableSubscription");

	CHECK(Free("dll_plain_key", table) == 0, "Free plain key");
	char* deleted_out = (char*)1;
	int deleted_len = -1;
	CHECK(Read("dll_plain_key", table, &deleted_out, &deleted_len) == -1, "Read deleted plain key fails");
	CHECK(deleted_out == NULL && deleted_len == 0, "Read clears output on failure");

	CHECK(Free("dll_encrypted_key", table) == 0, "Free encrypted key");
	CHECK(ReadEncrypted("dll_encrypted_key", table, enc_key, &deleted_out, &deleted_len) == -1, "ReadEncrypted deleted key fails");

	keys = NULL;
	count = -1;
	CHECK(GetKeysByRegex(table, "^dll_", 20, &keys, &count) == 0, "GetKeysByRegex after delete");
	CHECK(contains_key(keys, count, "dll_persist_key"), "regex still contains persistent key");
	CHECK(!contains_key(keys, count, "dll_plain_key"), "regex excludes deleted plain key");
	CHECK(!contains_key(keys, count, "dll_encrypted_key"), "regex excludes deleted encrypted key");
	FreeKeysArray(keys, count);

	FreeBuf(NULL);
	FreeKeysArray(NULL, 0);
	Sleep(300);

	printf("dll integration smoke passed\n");
	return 0;
}
`

func TestDLLBuildImportAndABI(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("DLL import smoke test uses Windows LoadLibrary/GetProcAddress")
	}

	packageDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	tmpDir := t.TempDir()
	dllPath := filepath.Join(tmpDir, "tsunamidb.dll")
	build := exec.Command("go", "build", "-buildmode=c-shared", "-o", dllPath, ".")
	build.Dir = packageDir
	build.Env = append(os.Environ(), "CGO_ENABLED=1", "GOOS=windows", "GOARCH=amd64")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build DLL failed: %v\n%s", err, out)
	}

	ccBytes, err := exec.Command("go", "env", "CC").Output()
	if err != nil {
		t.Fatalf("go env CC failed: %v", err)
	}
	cc := strings.TrimSpace(string(ccBytes))
	if cc == "" {
		t.Fatal("go env CC returned an empty compiler")
	}

	consumerPath := filepath.Join(tmpDir, "dll_consumer.c")
	if err := os.WriteFile(consumerPath, []byte(dllConsumerC), 0644); err != nil {
		t.Fatal(err)
	}

	exePath := filepath.Join(tmpDir, "dll_consumer.exe")
	compile := exec.Command(cc, "-Wall", "-Wextra", "-o", exePath, consumerPath)
	if out, err := compile.CombinedOutput(); err != nil {
		t.Fatalf("compile DLL consumer failed: %v\n%s", err, out)
	}

	dbWorkDir := filepath.Join(tmpDir, "db-work")
	if err := os.MkdirAll(dbWorkDir, 0755); err != nil {
		t.Fatal(err)
	}

	run := exec.Command(exePath, dllPath)
	run.Dir = dbWorkDir
	if out, err := run.CombinedOutput(); err != nil {
		t.Fatalf("DLL consumer failed: %v\n%s", err, out)
	} else {
		t.Logf("%s", out)
	}

	assertDatabaseFiles(t, dbWorkDir)
}

func assertDatabaseFiles(t *testing.T, dbWorkDir string) {
	t.Helper()

	tableFile := filepath.Join(dbWorkDir, "db", "data", "dll_integration_table")
	info, err := os.Stat(tableFile)
	if err != nil {
		t.Fatalf("expected table data file %s: %v", tableFile, err)
	}
	if info.Size() == 0 {
		t.Fatalf("expected table data file %s to contain data", tableFile)
	}

	walPath := filepath.Join(dbWorkDir, "db", "maps", "dll_integration_table", "index.wal")
	wal, err := os.ReadFile(walPath)
	if err != nil {
		t.Fatalf("expected table WAL %s: %v", walPath, err)
	}

	walText := string(wal)
	if !strings.Contains(walText, "S|dll_persist_key|dll_integration_table|") {
		t.Fatalf("expected WAL to contain persistent key save, got:\n%s", walText)
	}
	if !strings.Contains(walText, "D|dll_plain_key") {
		t.Fatalf("expected WAL to contain plain key delete, got:\n%s", walText)
	}

	relSchema := filepath.Join(dbWorkDir, "db", "rel", "dll_rel_products.schema")
	if _, err := os.Stat(relSchema); err != nil {
		t.Fatalf("expected relational schema file %s: %v", relSchema, err)
	}
	relRows := filepath.Join(dbWorkDir, "db", "rel", "dll_rel_products.rows")
	if info, err := os.Stat(relRows); err != nil {
		t.Fatalf("expected relational rows file %s: %v", relRows, err)
	} else if info.Size() == 0 {
		t.Fatalf("expected relational rows file %s to contain data", relRows)
	}
}
