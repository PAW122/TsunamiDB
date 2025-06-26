# todo list:

[] jednoczesny zapis i odczyt
    > podczas jednego typu akcji na 1 pliku do jest w zstanie wykonać
    około 35-45k operacji/s na moim CPU
    > podczas użycia jednoczesnego read & write wydajność jest usinana o 50% do:
    około 10K write/s
    około 10K read/s
    > limit wynika z ciągłego lockowania i odblokowywania pliku mutexem
    (zakładając że wszystkie operacje sykonywane są na 1 pliku danych)

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
