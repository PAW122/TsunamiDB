# auth

1. zabezpieczenie dostępu do db za pomocą apiKey
2. zabezpieczenie za pomocą username:password
3. zabezpieczenie danych - każdy klucz / user może mieć swoj lvl dostępu,
im większa liczba tym miejszy poziom dostępu.
    > dodac opcje zapisania danych z dlagą auth tak aby do danych miał dostęp tylko user z odpowiednimi uprawnieniami np
    save(key, data, <flag 0>)
    read(key, <flag 1>) => return "Data is protected by auth"
    read(key, <flag 0>) => return <data>
    zrobić jakiś max np 0-15 na Int4

* wszystkie hasła hashowane (config)
    + only cache, json jako plik do zapisywania

4. opcja zalockowania tabeli/pliku


```go
package main

import (
	"fmt"
)

// int4 zajmuje 4 bity (wartości od 0 do 15)
type Int4 uint8

// Tworzenie wartości Int4 (maksymalnie 4 bity)
func NewInt4(value uint8) Int4 {
	return Int4(value & 0x0F) // Ograniczamy do 4 bitów (0x0F = 00001111)
}

func main() {
	var a Int4 = NewInt4(7)  // 0111 (7)
	var b Int4 = NewInt4(15) // 1111 (15)
	var c Int4 = NewInt4(20) // 10100 (ale zostanie ucięte do 4 bitów)

	fmt.Println("a =", a) // 7
	fmt.Println("b =", b) // 15
	fmt.Println("c =", c) // 4 (bo 20 = 10100 → 0100)
}
```