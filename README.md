# TsunamiDB

![Version](https://img.shields.io/badge/version-v0.7.8-brightgreen.svg)

fast, simple non sql key-value db

install
```
go get github.com/PAW122/TsunamiDB/lib/dbclient@v0.7.8
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

# Go lib examples
instal
```
go get github.com/PAW122/TsunamiDB/lib/dbclient@v0.7.8
```

```go

/**
*important in tsu DB you cant use same Key value twaice eaven if data is saved to other table.
old key will be free'd when overwriter

when reusing ex user_id recomanded way is:
key := "account_<user_id>"
table := "accounts"
data := <any>
tsu.Save(key, table, data)
**/

import (
    tsu "github.com/PAW122/TsunamiDB/lib/dbclient"
    "github.com/google/uuid"
)

// init NetworkManager on start (required, arg[0] = any free port)
tsu.InitNetworkManager(3842, nil)

id := uuid.New()

// save example data
err := tsu.Save("test_key", "test_table", []byte("test_data"))
if err != nil {
	fmt.Println(err)
}

// read example data
data, err := tsu.Read("test_key", "test_table")
if err != nil {
	fmt.Println(err)
}
fmt.Println(string(data))


// OTHER FUNCTIONS:

// free / delete key
tsu.Free(key, table string) error

// save / read using db side 512 bit encryption
tsu.SaveEncrypted(key, table, encryption_key string, data []byte) error
tsu.ReadEncrypted(key, table, encryption_key string) ([]byte, error)

// init public api for db http requests
tsu.InitPublicApi(port int)
```
