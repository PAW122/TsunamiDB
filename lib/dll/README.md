# TsunamiDB DLL wrapper

This package exposes `github.com/PAW122/TsunamiDB/lib/dbclient` through cgo exports so it can be compiled as a native shared library.

## Build

Windows:

```powershell
go build -buildvcs=false -buildmode=c-shared -o .dist\tsunamidb.dll .\lib\dll
```

Linux:

```bash
go build -buildvcs=false -buildmode=c-shared -o .dist/libtsunamidb.so ./lib/dll

# or
bash ./lib/dll/build.sh
```

```powershell
# Linux .so from Windows, using WSL
.\lib\dll\build.bat linux
```

The build produces both the shared library and a generated C header file next to the output file.

For Linux builds, run the command on Linux or WSL with a working C compiler, because `-buildmode=c-shared` requires cgo.

## Memory

Buffers returned by `Read`, `ReadEncrypted`, `ReadInc`, and `EnableSubscription` must be released with `FreeBuf`.

Arrays returned by `GetKeysByRegex` must be released with `FreeKeysArray`.

## Exports

- `Save(key, table, data, length)`
- `Read(key, table, out, outLen)`
- `FreeBuf(ptr)`
- `Free(key, table)`
- `SaveEncrypted(key, table, encryptionKey, data, length)`
- `ReadEncrypted(key, table, encryptionKey, out, outLen)`
- `SaveInc(key, table, data, length, maxEntrySize, id, hasID, mode, countFrom, entryKey, outID)`
- `ReadInc(key, table, readType, id, entryKey, amount, out, outLen)`
- `RelationalSQL(query, out, outLen)`
- `InitNetworkManager(port, peers, count)`
- `InitPublicApi(port)`
- `InitSubscriptionServer(port)`
- `EnableSubscription(keys, count, authKey)`
- `DisableSubscription(key)`
- `GetKeysByRegex(table, regex, max, result, count)`
- `FreeKeysArray(array, count)`

## Incremental Tables

`SaveInc` appends by default. Pass `hasID=1` with `mode="append"` to insert at `id`, or `mode="overwrite"` to replace an existing entry. `countFrom` accepts `"top"` or `"bottom"`; empty strings use Go-lib defaults.

`ReadInc` returns a JSON array of `{ "id": number, "data": string }` entries. `readType` accepts `"by_id"`, `"by_key"`, `"first_entries"`, and `"last_entries"`.

## Relational SQL

`RelationalSQL` executes one relational SQL statement and returns a JSON `SQLResult` buffer.
Release the returned buffer with `FreeBuf`.

Example:

```c
char* out = NULL;
int outLen = 0;
int rc = RelationalSQL(
    "SELECT row_id, name FROM products WHERE id = 1",
    &out,
    &outLen
);
if (rc == 0) {
    // out points to outLen JSON bytes.
    FreeBuf(out);
}
```
