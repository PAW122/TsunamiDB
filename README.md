# TsunamiDB
 
+ TODO
dodac zapisywanie z db jak w TsunamiBocie dla testów

server-setwork -> sieć pomiędzy serwerami
client -> łączy się do sieci i udostępnia publiczną komunikację
    usera z serwerami

+ Flagi
    > -UI //uruchamia web ui
    > -LogAll // loguj wszystkie akcje od pliku
    > -server // uruchom serwer
    > -client // uruchom clienta
    > -clientAdmin // uruchom Admina
     

# komunikacja P2P
sieć server-network zrobić na P2P pomiędzy wszystkimi serwerami,
podczas uruchamiania serwera do podania w configu lub w argumencie będą podane parametry,
na ich podstawie serwer albo stworzy nową sieć, albo połączy się do już istniejącej

*można dodać jakiś system integracji sieic (np w przypadku 2 oddzielnych sieci można by je połączyć)
np komendą (admin console > server-network-1 > $ move-to-network <addr>),
spowoduje to przeniesienie serwerów do innej sieci