# /data
zapisywanie danych

fileSystem:
zarządzanie strukturami plików
cała logika sprawdzania i tworzenia nowych plików,
client_req -> fs --<file>--> DataManager --<data>--> encoding --<json>--> Client_res 

DataManager:
zappis/oczyt/modyfikacja pojedyńczych plików

# req examples
json data:
```json
{
    "key": "string",
    "data_type": "data ",
    "action": "write/add",
    "data": "JSON"
}
```

```json
{
    "key": "string",
    "action": "read/delete"
}
```

```json
{
    "key": "string",
    "data_type": "file",
    "data": "file_data"
}
```

> mapa powinna być po odczytaniu w formie hashmapy
# struktury:
+ /db/data-map
    pliki mapujące pliki danych np
    key: <pointer-to-file><data-start-pointer><data-end-pointer>
    > dla danego klucza informacje gdzie dokładnie w jakim pliku są dane

    dla key.<value>.<value2> 
    dla każdego key oprócz ostatniego (ostatni key może być np id)
    dodawać kolejną sub-mapę plików

    dla plików typu raw zrobić mapę tak samo jak dla key.<value>.<value2>


+ /db/blob
    pliki danych custom-bin

+ /db/raw
    pliki statyczne - dokumenty / zdjęcia itp