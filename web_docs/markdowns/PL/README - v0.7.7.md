```md
# 🇵🇱 Dokumentacja TsunamiDB

**TsunamiDB** to lekka, wysokowydajna baza danych typu *non-SQL*, zaprojektowana z myślą o prostocie, szybkości i elastyczności integracji. Dane są zapisywane według modelu:

```

```
Dane są zapisywane za pomocą schematu: 
key & table = value
```

---

## ⚙️ Tryby pracy

TsunamiDB może działać w dwóch trybach:

### 🌐 1. WebServer (API HTTP)
- W tym trybie TsunamiDB udostępnia REST API umożliwiające zapis danych
- Oraz na osobnym porcie websocket do subskrybowania konkretnych objektów

### 🧩 2. Biblioteka (Go lub DLL)
- pozwala na zintegrowanie bazy danych w projekt i bezpośrednie wywoływanie jej funkcji

#### 📦 Go:

Dodaj do projektu:

```go
import "github.com/PAW122/TsunamiDB"
```

Go Pkg:
```
https://pkg.go.dev/github.com/PAW122/TsunamiDB
```

#### 🪟 DLL (dla C#, Rust i innych:)

Dostępna jest wersja skompilowana jako `TsunamiDB.dll`, którą można pobrać z githuba `https://github.com/PAW122/TsunamiDB/releases`

---

## ⚠️ Zasady działania

1. ✅ **Każdy klucz (`key`) może być zapisany tylko jeden raz**. Ponowny zapis nadpisuje poprzednie dane.
2. ❌ **Nie istnieje klasyczne `DELETE`.**  
   Użyj `free(key)` – oznacza miejsce jako „wolne” (do ponownego nadpisania), ale nie zmniejsza rozmiaru pliku na dysku.
3. ⚠️ **Nie wszystkie wersje DB są kompatybilne**  
   Zmiany w formacie plików mogą powodować brak zgodności między wersjami DB. Przed aktualizacją sprawdź czy wersje są kompatybilne.

---

## 📚 Dostępne moduły

- `save[table, key, value]` - zapisanie danych
- `read[table, key]` - odczytanie danych
- `free[key]` - usunięcie danych
- `readRegex[regex]` - odczytanie listy key na podstawie reguły regex
- `saveEncrypted[table, key, encryption_key, value]` - zapisz danych szyfrowanych przez db
- `readEncrypted[table, key, encryption_key]` - odczytanie danych deszyfrowanych przez db

+ **Lib/dll specyfic**
   - `InitNetworkManager[port{int}, knownPeers]` - uruchomienie managera sieci db
   - `InitPublicApi[port{int}]` - uruchomienie publicznego API
   - `InitSubscriptionServer[port{string}]` - uruchomienie websocketa systemu subskrypcji

---

## 🧠 FAQ

> **Czy TsunamiDB obsługuje typy?**  
> Nie, możesz zapisać wszystko - dowolny typ i rozmiar.
> Dane są przechowywane w takim formacie w jakim je zapiszesz

> **Czy dane są kompresowane?**  
> Nie – dane są zapisywane w customowym formacie, ale nie są kompresowane.

> **Co się stanie kiedy stracę klucz syfrujący dane**
> Dane zostaną utracone - nie ma możliwości ich odszyfrowania bez klucza, klucze nie są automatyczni zapisywane przez db

---
