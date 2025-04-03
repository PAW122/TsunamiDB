# TsunamiDB

![Version](https://img.shields.io/badge/version-0.7.1-brightgreen.svg)

fast, simple non sql key-value db

install
```
go get github.com/PAW122/TsunamiDB/lib/dbclient@v0.7.1
```

+ execute:
    go build -tags debug

    - when starting 1'st server - ```./TsunamiDB.exe 5845```
        > ./TsunamiDB <port for node's comunication>
    - when starting secound server - ```./TsunamiDB-linux 5845 192.168.55.110:5845```
        > ./TsunamiDB-linux <same port> <ip and port of other server>


# bentchmarks data:
* Avg save time: 215µs
* Avg read time: 43µs
```
save obj:
{
    key: "key<id>"
    data: "data-<id>"
}

read obj:
get("key<id>") res -> "data-<id>"
```
>    benchmarks results are an average values ​​from data field records with sizes ranging from 10_000 to 100_000 entries.

>    all tests are performed on local hardware (personal PC), data may not be accurate

see test code [https://github.com/PAW122/TsunamiDB/blob/main/tests/test1.go]