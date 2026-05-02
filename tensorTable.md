# Tensor Table Design

Ten dokument opisuje zalozenia dla warstwy `tensor table` w TsunamiDB.

## Cel

`tensor table` ma sluzyc do tworzenia statystycznych danych diagnostycznych na
podstawie duzych zestawow parametrow testowych oraz wynikow zewnetrznych testow.

Docelowy przypadek uzycia:

- test zlozonego sprzetu zapisuje duza liczbe parametrow,
- test moze zakonczyc sie statusem `pass`, `fail` albo np. `unknown`,
- po nieudanym tescie wymieniany jest jeden albo wiele komponentow,
- test jest powtarzany,
- baza uczy sie, ktore parametry wejscia najczesciej prowadza do konkretnych
  wynikow diagnostycznych,
- po podaniu nowych parametrow baza zwraca najbardziej prawdopodobne wyniki,
  np. komponenty do wymiany, razem z informacja, ktore parametry mialy najwiekszy
  wplyw na decyzje.

## Najwazniejsze Zalozenie

`tensor table` nie powinien fizycznie zapisywac jednego wielkiego tensora na dysku.

Lepszy model to logiczna tabela diagnostyczna, ktora przechowuje:

- schemat danych,
- probki/obserwacje,
- referencje do duzych blobow,
- wyciagniete cechy,
- etykiety wynikowe,
- agregaty statystyczne,
- indeksy podobienstwa.

W praktyce jest to bardziej `multi-label diagnostic table` niz klasyczny tensor
numeryczny znany z bibliotek ML.

## Dane Wejsciowe

Kazda tabela tensorowa definiuje zestaw inputow. Kazdy input musi miec staly typ.

Przyklady typow:

```text
uint64
int64
float64
bool
string
enum/string
timeseries
blob_ref
```

Typ danej musi byc stabilny. Jezeli pole `voltage_avg` jest `float64`, to kazda
probka musi podawac je jako liczbe. Nie powinno byc mieszania `"12"` jako stringa
z `12` jako liczba.

## Wyniki

Jedna obserwacja moze miec wiele wynikow naraz.

Przyklad:

```json
{
  "results": [
    {"key": "component", "value": "power_supply"},
    {"key": "component", "value": "fan"},
    {"key": "failure_type", "value": "thermal"}
  ]
}
```

Oznacza to, ze jedna probka moze wzmacniac wiele etykiet wynikowych.

## Statusy

Nalezy rozdzielic status samego testu od statusu wiedzy diagnostycznej.

Przyklad:

```json
{
  "test_status": "fail",
  "learning_status": "positive",
  "results": [
    {"key": "component", "value": "power_supply"}
  ]
}
```

Proponowana semantyka:

- `test_status: pass` - test zakonczyl sie powodzeniem,
- `test_status: fail` - test zakonczyl sie bledem,
- `learning_status: positive` - pozytywny sygnal uczacy; wyniki przypisane do
  probki sa wiarygodna etykieta,
- `learning_status: negative` - negatywny sygnal uczacy; wyniki przypisane do
  probki raczej nie wyjasniaja obserwacji,
- `learning_status: unknown` - probka moze byc pominieta przy uczeniu.

Sam fakt `test_status: fail` oraz jakakolwiek etykieta wyniku nie musi oznaczac,
ze etykieta byla przyczyna. Mocny sygnal powstaje dopiero wtedy, gdy zewnetrzny
proces oznaczy probke jako `learning_status: positive`.

## Duze Dane

Pojedynczy test moze zawierac bardzo duzo danych, np. okolo 80 MB czystych danych.
Moga to byc zarowno gotowe cechy, jak i opracowane przebiegi czasowe.

Nie nalezy porownywac pelnych 80 MB przy kazdym zapytaniu `tensor-result`.

Zamiast tego:

- surowe dane powinny byc zapisane jako blob/KV,
- tabela tensorowa powinna trzymac referencje do bloba,
- do predykcji powinny byc uzywane wyciagniete cechy i skroty przebiegow.

Przyklad pola z przebiegiem czasowym:

```json
{
  "fan_rpm_curve": {
    "type": "series_ref",
    "ref": "blob://tests/000123/fan_rpm",
    "features": {
      "min": 1200,
      "max": 4800,
      "avg": 3900,
      "stddev": 180,
      "spikes": 4,
      "trend": "unstable"
    }
  }
}
```

## Cechy Dla Przebiegow Czasowych

Dla przebiegow czasowych warto zapisywac:

- `min`,
- `max`,
- `avg`,
- `stddev`,
- percentyle,
- liczbe pikow,
- trend,
- czas powyzej progu,
- czas ponizej progu,
- liczbe przekroczen progu,
- opcjonalny downsample/sketch, np. 256-1024 punktow zamiast calej serii.

Pierwsza wersja powinna dzialac na cechach i statystykach, a nie na pelnych
surowych seriach.

## Przykladowy Schemat

```json
{
  "name": "hardware_diagnostics",
  "ignore_statuses": ["unknown"],
  "inputs": [
    {"name": "temperature_max", "type": "float64"},
    {"name": "voltage_avg", "type": "float64"},
    {"name": "error_code", "type": "string"},
    {
      "name": "fan_rpm_curve",
      "type": "timeseries",
      "store_raw": true,
      "features": ["min", "max", "avg", "stddev", "spikes", "trend"]
    }
  ],
  "results": [
    {"key": "component", "type": "string", "multi": true},
    {"key": "failure_type", "type": "string", "multi": true}
  ]
}
```

## Przykladowy Input

```json
{
  "sample_id": "test_2026_000123",
  "test_status": "fail",
  "learning_status": "positive",
  "input": {
    "temperature_max": 91.2,
    "voltage_avg": 11.8,
    "error_code": "E17",
    "fan_rpm_curve": {
      "type": "series_ref",
      "ref": "blob://tests/000123/fan_rpm",
      "features": {
        "min": 1200,
        "max": 4800,
        "avg": 3900,
        "stddev": 180,
        "spikes": 4,
        "trend": "unstable"
      }
    }
  },
  "results": [
    {"key": "component", "value": "power_supply"},
    {"key": "component", "value": "fan"},
    {"key": "failure_type", "value": "thermal"}
  ]
}
```

## API Sketch

Proponowane endpointy:

```text
POST /tensor/schema/{table}
GET  /tensor/schema/{table}

POST /tensor/{table}/input
POST /tensor/{table}/result
```

Opcjonalne aliasy zgodne z pierwotnym opisem:

```text
POST /tensor-input/{table}
POST /tensor-result/{table}
```

## tensor-input

`tensor-input` powinien:

1. zwalidowac nazwe tabeli,
2. wczytac schemat,
3. zwalidowac typy inputow,
4. zwalidowac wyniki,
5. zapisac probke append-only,
6. zapisac referencje do duzych danych, jezeli wystepuja,
7. zaktualizowac agregaty statystyczne tylko wtedy, gdy status probki powinien
   brac udzial w uczeniu.

Probki z `learning_status: unknown` moga byc zapisane jako historia, ale powinny byc
pomijane przy aktualizacji modelu.

## tensor-result

`tensor-result` powinien:

1. zwalidowac input,
2. porownac input z agregatami statystycznymi i/lub indeksami podobienstwa,
3. policzyc ranking wynikow,
4. zwrocic top N najbardziej prawdopodobnych wynikow,
5. zwrocic informacje, ktore parametry najbardziej wplynely na decyzje.

Przykladowa odpowiedz:

```json
{
  "results": [
    {
      "key": "component",
      "value": "power_supply",
      "probability": 0.74,
      "score": 18.2,
      "samples": 412,
      "influences": [
        {
          "input": "voltage_avg",
          "impact": 0.31,
          "reason": "low value matched positive learning cases"
        },
        {
          "input": "error_code",
          "impact": 0.24,
          "reason": "E17 strongly correlated"
        },
        {
          "input": "temperature_max",
          "impact": 0.12,
          "reason": "within common range"
        }
      ]
    }
  ]
}
```

## Proponowany Uklad Plikow

```text
db/tensor/<table>.schema
db/tensor/<table>.samples
db/tensor/<table>.features
db/tensor/<table>.labels
db/tensor/<table>.stats
db/tensor/<table>.series_index
```

Znaczenie:

- `.schema` - definicja inputow, wynikow i statusow,
- `.samples` - historia probek append-only,
- `.features` - znormalizowane cechy uzywane do predykcji,
- `.labels` - etykiety wynikowe,
- `.stats` - agregaty statystyczne per wynik,
- `.series_index` - opcjonalny indeks/sketch dla przebiegow czasowych.

## Proponowany Storage

Dla tego use case najlepszy jest model hybrydowy:

- `incremental lists` jako glowny append-only log historii,
- KV jako magazyn schematow, manifestow, duzych blobow, agregatow i snapshotow
  modelu,
- relacyjna baza opcjonalnie pozniej do katalogow, filtrowania i narzedzi
  administracyjnych.

Najwazniejsza zasada:

```text
historia w incList, aktualny stan w KV
```

### Dlaczego incList + KV

Dane tensorowe sa naturalnie append-only. Kazdy test tworzy nowa obserwacje.
Historia testow raczej nie powinna byc czesto edytowana. Jezeli pojawi sie nowa
wiedza, np. wynik naprawy, mozna dopisac kolejny wpis lub wpis aktualizujacy.

`incremental lists` dobrze pasuja do:

- sekwencyjnego dopisywania probek,
- przechowywania historii,
- pozniejszego rebuildowania modelu,
- ograniczania presji na RAM przy bardzo duzej liczbie obserwacji.

KV dobrze pasuje do:

- szybkiego odczytu schematu,
- odczytu manifestu konkretnej probki po `sample_id`,
- przechowywania duzych blobow albo ich chunkow,
- przechowywania aktualnych agregatow,
- przechowywania snapshotu modelu uzywanego przez `tensor-result`.

### Ograniczenie KV

Obecna baza KV ma istotny koszt RAM: okolo `1 GB RAM / 1M wpisow`.

Z tego powodu nie nalezy zapisywac kazdego parametru lub cechy jako osobnego
klucza KV.

Zly model:

```text
sample:123:temperature_max
sample:123:voltage_avg
sample:123:error_code
sample:123:fan_rpm:min
sample:123:fan_rpm:max
```

Lepszy model:

```text
sample:123:manifest -> jeden dokument JSON/binarny
```

KV powinno przechowywac grubsze dokumenty, manifesty i snapshoty, a nie miliony
malych kluczy.

### Proponowany Podzial

```text
schema     -> KV
samples    -> incList
raw data   -> KV/blob refs, najlepiej chunkowane przy duzych payloadach
features   -> incList + opcjonalny KV manifest per sample
stats      -> KV snapshot
model      -> KV snapshot
rebuild    -> skan incList od poczatku
```

Przykladowe logiczne klucze/listy:

```text
tensor:<table>:schema       -> KV, schemat tabeli
tensor:<table>:stats        -> KV, aktualne agregaty
tensor:<table>:model        -> KV, snapshot modelu
tensor:<table>:samples      -> incList, historia obserwacji
tensor:<table>:features     -> incList, znormalizowane cechy
tensor:<table>:labels       -> incList, etykiety wynikowe
blob:<sample_id>:raw        -> KV/blob, duze dane zrodlowe albo manifest
blob:<sample_id>:series:*   -> KV/blob, segmenty przebiegow czasowych
```

### Duze Bloby

Pojedyncze dane testowe moga miec okolo 80 MB, dlatego probka w incList nie
powinna zawierac pelnego payloadu.

Probka w incList powinna zawierac manifest:

```json
{
  "sample_id": "test_000123",
  "schema_version": 1,
  "test_status": "fail",
  "learning_status": "positive",
  "features_ref": "kv://tensor/hardware/features/test_000123",
  "raw_ref": "kv://blob/tests/test_000123/raw",
  "results": [
    {"key": "component", "value": "power_supply"}
  ]
}
```

Pelne dane powinny byc zapisane jako blob albo zestaw chunkow:

```text
blob:test_000123:chunk:0000
blob:test_000123:chunk:0001
blob:test_000123:chunk:0002
blob:test_000123:manifest
```

### Rola Relacyjnej Bazy

Relacyjna baza nie powinna byc podstawowym storage dla `tensor table` w MVP.

Powody:

- jest nowo zaimplementowana i jeszcze nie tak dobrze przetestowana jak KV,
- fixed-row model pasuje do przewidywalnych tabel, ale dane tensorowe beda czesto
  zmienne i wielowymiarowe,
- przebiegi czasowe oraz cechy moga miec nieregularna strukture,
- glowny zapis tensorowy jest append-only, wiec lepiej pasuje do incList.

Relacyjna baza moze byc przydatna pozniej do:

- katalogu probek,
- tabeli statusow,
- dashboardu,
- filtrowania po `sample_id`, dacie, statusie, typie urzadzenia,
- narzedzi administracyjnych.

Nie powinna jednak blokowac pierwszej wersji `tensor table`.

## Algorytm MVP

Pierwsza wersja powinna byc prostym, explainable modelem statystycznym:

- liczby sa porownywane przez odleglosc od sredniej/zakresu dla danego wyniku,
- stringi/enum/bool sa porownywane przez zgodnosc i czestosc wystepowania,
- kazdy wynik dostaje score,
- score jest normalizowany do prawdopodobienstwa wzgledem innych wynikow,
- dla kazdego wyniku zapisywany jest wplyw poszczegolnych inputow.

To nie bedzie pelny silnik ML, ale bedzie uzyteczne, szybkie i latwe do
debugowania w diagnostyce sprzetu.

## Ograniczenia MVP

MVP nie powinno od razu:

- trenowac ciezkich modeli ML,
- porownywac pelnych surowych przebiegow czasowych przy kazdym zapytaniu,
- implementowac pelnego systemu feature engineering,
- wymagac ladowania wszystkich probek do RAM.

Najpierw nalezy zbudowac stabilna warstwe:

- schemat,
- walidacja,
- zapis probek,
- agregaty,
- ranking,
- explainability.

Zaawansowane podobienstwo przebiegow czasowych mozna dodac pozniej jako osobny
indeks.
