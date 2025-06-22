# Przykładu kodu

pobranie
```
go get github.com/PAW122/TsunamiDB/lib/dbclient@v0.7.8
```

```go
package main

import (
    tsu "github.com/PAW122/TsunamiDB/lib/dbclient"
    "github.com/google/uuid"
)

// inicjalizacja modułów
tsu.InitNetworkManager(3842, nil)
tsu.InitPublicApi(5844)
tsu.InitSubscriptionServer("5845")

id := uuid.New()

// zapisanie danych
err := tsu.Save("test_key", "test_table", []byte("test_data"))
if err != nil {
	fmt.Println(err)
}

// odczytanie danych
data, err := tsu.Read("test_key", "test_table")
if err != nil {
	fmt.Println(err)
}
fmt.Println(string(data))

// usunięcie danych
err = tsu.Free("test_key", "test_table")
if err != nil {
	fmt.Println(err)
}

// save / read using db side 512 bit encryption
err = tsu.SaveEncrypted("test_key", "test_table", "encryption_key", []byte{"dane"})
if err != nil {
	fmt.Println(err)
}

dane, err := tsu.ReadEncrypted("test_key", "test_table", "encryption_key") 
if err != nil {
	fmt.Println(err)
}
fmt.Println(string(dane))
```