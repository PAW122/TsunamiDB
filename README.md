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

# todo:
dodac funkcje nadpisywania danych jezeli key juz istnieje (jezeli pointery nie pozwalaja na zapisanie w danym miejscu to zapisac na koncu pliku
i dodac pointery do tego miejsca)
    + opcja czyszczenia po usuniediu - trzeba usunac stare dane albo oznaczyc jako (tak jak w alokatorze w C) ze dane miejsce jest puste,
    jezeli cos bedzie chcialo sie zapisac to zostanie pierw wcisniete w wolne miejsce zamiast na koniec pliku.
    + funkcja naprawiania defragmentacji Pliku ->
        > po wlaczeniu zapisywac nowe dane do innego pliku dopuki ten sie nie skonzcy formatowac
        > poprzesuwac all dane, przeliczyc na nowo pointery, zaktualizowac mapy
        > przeniesc dane z pliku tymczasowego do głównego

todo fs:
    + zrobic max wielkosci 1 pliku bin i wtedy zapisywac do innego
    + zamienic jsona na hash mape o ile jeszcze nie jest mapa w ramie
    + zrobic opcje w map do odnoszenie sie do subMap tak aby mozna bylo miec wiele plikow map
        > np jezeli jest duzo wpisow z account.??? to mozna z automatu zrobic nowa mape dla account
        > i dodac zasade, arg[0] w key to account to odczytoj i zapisauj juz donowej mapy i nowego bin

# lib
+ zrobić bibliotekę na początek do go do obsługi db

# dodac typy danych
any -> tak jak teraz dziala
string -> tylko string
numbers -> wszystkie liczbowe
big_json -> json
    > zapisywac normalnie same sciezki key jsona + pointery do wartosci.
    > tak zeby jak najszybciej pracowac na danych
file -> obslugiwane przez fs (nie zapisywane do pliku db)
    > w bin dodac bit odp za sprawdzanie czy dane to plik, jezeli tak to
    w data powinien byc odnsnik do mapy i przechowysania plikow 

# api funcs:
write
read
over_write
increment

# commands:
$ - admin
> - user

$ add user <username> <password> <r/w perms> -> user_api_key
$ users list
$ user password reset <user> <new_password>
> user password reset <old_password> <new_password>

# todo kiedys:
+ serwer typu network-backup -> zajmuje miejsce innego jeżeli ktorys padnie
+ serwer typu data-backup -> jakiś sys typu RAID do dublowania danych tak aby zawsze był backup
+ docs web

+ opcja uruchomienia db w trybie smb
    - db zapisywała by pliki i inne rzeczy tak samo jak wcześniej, ale zamiast z poleceń nrmanych
    była by obsługiwana np przez windowsa który widział by ją jako dysk sieciowy.