# todo list:

? wbudowany system auth
[] system auth
    > opcjonalny do włączania automatyczny systema autoryzacji obsługujący table google-auth
    > załączenie poprzez flage "-modules google-auth"
    > dla dll załączane poprzez InitGoogleAuth
    > dla http wystawiało by dodatkowe endpointy
        > /modules/google-auth
            > register return (succes, auth_token)
            > login return (auth_token/nil, err)
            > is_auth(token) return (succes)
            
    > dane:
    > google_id, email, nickname, data_rejestracji, avatar_url, auth_token

[] save-safe
    > header który można przekazać do funkcji save()
    > header: safe: true
    po zapisaniu danych zostanie wykonane odczytanie i porównanie
    aby upewnić się, że dane są na 100% poprawne
    (unikanie utraty danych w przypadku problemu z dyskiem)

[] live-stream
    > funkcja pomocnicza do systemu streamowania video/danych.
    1. zapisanie dużego pliku do db
    2. jakiś rodzaj połączenia usera z DB, można zrobić coś podobnego jak subskrybcje (UDP)
    3. zależnie od configu wygenerowanego key będzie ustalany
        > częstotliwość przesyłania np 1/s
        > rozmiar przesyłanych danych np 10MB
        > opcja pause, resume

[] read-bytes
    > funkcjae od odczytywania konkretnych wyrywków danych
    np: save(key, table, 1mb_data)
    getSize(key) return int64
    readBytes(key, table, startFrom: 0, endOn: 10_000)


[] redukacja użyca ramu:
    1. zamianić string na jakiś customowy []byte 
    > słaba efektywność podczas read & write na raz
    > możliwe że read() blokuje cały plik czy coś
    > posprawdzać co tam się dzieje
        > rozebrać read() na miejsze debug logi 

[] webowa prezentacja działania systemu subskrypcji np chat 
[] jakiś db ludzi od DB, żeby dali swoją opinie o zasadach działania db

!! zmienić kod tak aby można byłu używać 2 identyczych key w 2 różnych tabelach, w free() wymagać zarówno key jak i table
> albo w map zamiast używać key i table
to użyć key: "<table>@<key>"
i w razie potrzeby parsować key do pierwszego @ w stringu,
zakazać używania @ w nazwie tabeli 

7. te same funkcji co obecnie są używane, ale "cache"
czyli przechowywane tylko w ram

5. add "record_not_found" res in network manager
    > now db need to wait 5s (timeout) after asking
    for not existing data for no reason, eaven when all db's
    already processed req.
     
1. "official" go-lang lib for /lib/dbclient
2. implement "sync-node" mode for server's
3. add go-lang & js lib for http/client lib
4. add auto-tests for local lib

==================

## inc table

teoretyczne założenia:
---
lista inc (inkrementacyjna):

(oddzielny plik)
podczas tworzenia nowej:
Key: <server_id>_messages_inc_table
entry: <max_entry_size>

podczas
/api/inc_table/push
body: {data}
res: {id, status}

db sama przydzieli id wiadomości.
i na podstawie id będzie można uzyskać łątwy dostęp do wiadomości (bo będie dało się policzyć offset)

to coś też nie potrzebywało by mapy - w KV był by tylko odnośnik do tego w jakim pliku inc_btale są dane
i można by robić duże wpisy

+ w KV musiał by być zapisany schema tak aby pamiętać rozmiary po pierwszym stworzeniu

albo nawet dodać opcje zrobienia tabeli sql z tego ale trzeba by się z migracjami/zmianami tabeli pobawić
np jakieś cli albo api z:
/api/inc_table <key> change_schema from <max_entry_size> to <new_max_entry_size> <i cos nwego>
---

1. endpoint
2. dataManager - dodawanie req
    > nowy typ operacji "inc_read" "inc_save" "inc_delete"
3. faktyczna funkcja do obsługi
    > przy save write/create
    > przy read check czy istnieje
    > *checki tylko przy 1 interakcji

free/delete:
narazie 



dane w KV od incTable
będą zapisywane jako zwykłe dane
i tylko endpointy dla inc będą normalnie je czytały

jak ktoś użyje na tym zwykłego read to dostanie raw dane

## read
do odczytu:
1. od końca (najnowsze) -> dla odczytu 100 wpisów zaczynamy od końca pliku i czytamy 100x64

## save:
żeby nie czytać niepoprawnych danych robimy tak:
1. blok danych = entry_size + 1B (1b na specjalny znak)
2. jeżeli rozmiar wpisywanych danych jest równy ilości zapisywanych danyc to bytes[1+dane]
3. jeżeli rozmiar wpisywanych danych jest miejszy niż entry_size to:
    > zapisujemy zera aż do momentu kiedy będzie idealnie miejsca na dane -1 bit
    > zapisujemy bit == 1
    > zapisujemy dane
    > dzięki temu podczas odczytu czekamy na pierwszy bit == 1 i zaczynamy czytać od kolejnego


todo-add:
from 100
amount 100

zacznie od setnego wpisu i doda kolejne 100
tak aby muc odczytywać z środka

dodać opcje połączenia do subServer
po zasubskrybowaniu wysyłać notyfikacje jakby były by to zmiany samego wpisu w KV

funcja free

!!!!!!!!!!!!!!!!!!!!!!
TODO edytować docs i dodać tam inc_table