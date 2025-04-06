# todo list:

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
