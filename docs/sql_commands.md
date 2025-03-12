# sql commands:

file = table


FROM <table> SELECT <regex for key's> LIMIT 5
z <pliku> wybierz wszystkie key które spełaniją <regex> z max wybranych 5

# założenia:
dodać pełną obsługę poleceń SQL.
tak aby użytkownik mógł korzystać zarówno z non sql jak i z sql.

* oddzielne TABLE mają być trzymane w oddzielnych plikach dla wydajności

* rekomendować jak największa separacje danych
    > nie trzymać wszystkiego w 1 tabeli tylko rozdzielać na wiele miejszy
    > takie rozwiązanie powinno znacząco zmiejszyć czas potrzebny
    > na wykonanie query (znalezienie konkretnej wartości w kolumnie)

    > np dla kolumny users szybciej jest znależć usera jeżeli np:
    10M userów podzielimy na 10 tabel typu users_eu users_us ...
    niż szukać w 1 wielkiej tabeli.

# sql tables:
1. tworzenie tabeli:
    tabela składa się z:
       - automatycznie ustawianego ID
       - kolumn o konkretnym rozmiarze danych

2. logika:
    znając id oraz rozmiar wszystkich kolumn można łatwo obliczyć
    offset w którym miejscu w pliku będa znajdowały się dane konkretnego
    id i konkretnej kolumny tego id co powinno pozwolić na bardzo szybki
    odczyt i aktualizacje danych

    podczas odczytu i zapisu zrobić coś na styl GetBlock, ale zamiast bazować
    na samych pointerach można bazowac na id.
    dzięki temu będzie miej danych w GetBlock i szybsze czasy dostępu

2. zapis:
    - new value -> O_APPEND, autoincrement id

3. odczyt:
    na podstawie podanego id obliczamy offset i odczytujemy z niego odpowiednią
    ilość danych

    dla akcji wymagających przejścia po tabeli:
    - obliczay offset
    - odczytujemy wartość
        > jeżeli wartość się nie zgadza to idziemy dalej
    - uruchamiać odczyt od kilku miejsc:
        > początek
        > środek w góre
        > środek w dół
        > od dołu

    