# TsunamiDB

![Version](https://img.shields.io/badge/version-0.8.0-brightgreen.svg)

fast, simple non sql key-value db

install
```
go get github.com/PAW122/TsunamiDB/lib/dbclient@v0.8.0
``` 

+ execute:
    go build -tags debug

    - when starting 1'st server - ```./TsunamiDB.exe 5845```
        > ./TsunamiDB <port for node's comunication> [-ui <admin ui port>]
    - example with browser UI - ```./TsunamiDB.exe 5845 -ui 8080```
    - when starting secound server - ```./TsunamiDB-linux 5845 127.0.0.1:5845```
        > ./TsunamiDB-linux <same port> <ip and port of other server>

+ performance:
+ I5-10400f
+ on my local pc Iam geting consistant 45K writes/s
+ and ~100K reads/s

### changeLog for v0.8.0
+ added subscription system
+ improved saving data speed
+ better stability in long run
  > tested stable with sizes around 12GB
  > maps system rework

# commands:
```bash
-tags debug
go test -v ./lib/dbclient

go test -run . -v ./data/dataManager/v2
go test -run . -v ./servers/public-api/v1/routes
```

## Admin UI

The optional admin panel lives under `http://localhost:<ui port>` when you start the server with `-ui <port>`.

- Dashboard with live API metrics, subscription counters, and network status.
- Built-in request builder that covers every public endpoint (save, read, SQL, incremental tables, regex search, etc.).
- Subscription helper to request auth keys, connect to the WebSocket feed (default `5845`), and watch real-time updates.

New flags work with the legacy positional arguments, for example:
```
./TsunamiDB.exe -ui 8080 5845 peer-a:5845 peer-b:5845
```
The first non-flag value is still treated as the core port, anything else is considered a known peer address.
