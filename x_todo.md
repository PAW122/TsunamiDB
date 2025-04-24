# efi todo:
* tak samo jak jest flaga -debug to zrobic -bentchamr
     i pododawać logi z info o wydajności db -> co ile czasu zamuje itp

    > sprobować dla obecnej najwydajniejszej wersji zaimplementować używanie handle
    tak aby zwiększyć prędkość przynajmiej avg read.

    można użyć handle nawet dla samych r/s

* opt2:
    > obecnie przy każdym save jest read -> free -> save
    można pokombinować coś żeby nie trzeba było wykonywać tego read & free albo samego read
    i będzie te .5ms szybciej

bdw jaki jest rate save - read
możliwe że db obsługuje ilość req/s * 2 albo req/s / 2 (1k read & 1k save)

# test
- zrović test dzie będa wielokrotnie używane te same key & używane free
    + save & read encrypted
    i dawać avg staty z wszystkiego

# todo list:


* rozdzielić mapę na oddzielne mapy dla pojedyńczych tabel
    - każda tabela będzie miała swoja własna mapę
    + optymalizacja - można usunąć pierdoły z daych w mapie np status czy jakos tak

* krok2
    > optymalizacja max wielkości mapy
    kiedy mapa osiągnie daną wielkość to tworzys ei droga

!! zmienić kod tak aby można byłu używać 2 identyczych key w 2 różnych tabelach, w free() wymagać zarówno key jak i table
> albo w map zamiast używać key i table
to użyć key: "<table>@<key>"
i w razie potrzeby parsować key do pierwszego @ w stringu,
zakazać używania @ w nazwie tabeli 

* poprawka - read (tabela, key)
ma używać tabela jako nazwy mapy.
w ten sposób rozłoży się "load" z mapy

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
