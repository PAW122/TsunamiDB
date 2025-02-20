# funcs
funkcje pod które trzeba zrobić odpowiednie opcje optymalizacyjne:

* time table
    > np db przejazdów taksówek (dużo danych typu uniqueKey: small Data), duże pliki

// json jak db z bota
* /save_json
* /read_json

* sql commands
    dla sql db: /db/sqlData/
    > mogą działać na jsonie
    > dodatkowo dla poleceń sql zrobić specjalny rodzaj tabeli gdzie w map
    będą zapisywane dodatkowe dane:,
    > nowy enkoder: dla np tabeli [id, login, password]
    podzielić tabelę na 3 subTable, każda będzie oddzielną kolumną,
    w mapie zapis na ten styl:
    > map: 1: [id: {startPtr, endPtr}, login:{startPtr, endPtr}, password:{startPtr, endPtr}]

    + optymalizacja:
        - każdy taki jak wyżej przedstawiony element ma mieć zawsze identyczną wielkość w []byte
        na podstawie tej wielkości oraz id będzie można obliczyć offset w którym powinna być dana wartość w mapie
        i nie będzie trzeba wykonywać query przez plik.

        = szybki odczyt wartości key z pointerami = szybki read()
        save musiało by działać tak samo jak obecny - na podstawie free(), getBlock() oraz saveAsync()

* import:
    > .db
    > .sql

    > dane mogą być przenoszone na typ json 

* db mode
    > tryby pracy baz danych

    1. backup / raid1
    2. readSrc
        > defaultowe, tylko odczyt danych jak nie ma ich lokalnie
     