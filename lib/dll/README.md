# TsunamiDB DLL wrapper

This package exposes `github.com/PAW122/TsunamiDB/lib/dbclient` through cgo exports so it can be compiled as a native shared library.

## Build

Windows:

```powershell
go build -buildmode=c-shared -o .dist\tsunamidb.dll .\lib\dll
```

Linux:

```bash
go build -buildmode=c-shared -o .dist/libtsunamidb.so ./lib/dll
```

The build produces both the shared library and a generated C header file next to the output file.

## Memory

Buffers returned by `Read`, `ReadEncrypted`, and `EnableSubscription` must be released with `FreeBuf`.

Arrays returned by `GetKeysByRegex` must be released with `FreeKeysArray`.

## Exports

- `Save(key, table, data, length)`
- `Read(key, table, out, outLen)`
- `FreeBuf(ptr)`
- `Free(key, table)`
- `SaveEncrypted(key, table, encryptionKey, data, length)`
- `ReadEncrypted(key, table, encryptionKey, out, outLen)`
- `InitNetworkManager(port, peers, count)`
- `InitPublicApi(port)`
- `InitSubscriptionServer(port)`
- `EnableSubscription(keys, count, authKey)`
- `DisableSubscription(key)`
- `GetKeysByRegex(table, regex, max, result, count)`
- `FreeKeysArray(array, count)`
