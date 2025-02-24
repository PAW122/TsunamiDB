# TsunamiDB
fast, simple non sql key-value db

+ execute:
    go build -tags debug

    - when starting 1'st server - ```./TsunamiDB.exe 5845```
        > ./TsunamiDB <port for node's comunication>
    - when starting secound server - ```./TsunamiDB-linux 5845 192.168.55.110:5845```
        > ./TsunamiDB-linux <same port> <ip and port of other server>
