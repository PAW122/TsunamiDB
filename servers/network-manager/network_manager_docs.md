# /servers/network-manager

Aktualny `network-manager` działa jako P2P overlay dla instancji TsunamiDB.

## Co robi teraz

- utrzymuje połączenia WebSocket między peerami,
- wymienia handshake `hello` z adresem noda i katalogiem lokalnych tabel,
- rozgłasza aktualizacje katalogu tabel (`catalog`) po lokalnym `save` / `free`,
- wysyła requesty P2P jako jawne ramki protokołu:
  - `hello`
  - `catalog`
  - `request`
  - `response`
  - `ping`
  - `pong`
- routuje `read` do peerów, które reklamują daną tabelę,
- fallbackuje do broadcastu, gdy katalog tabel nie jest jeszcze znany,
- zwraca statystyki sieci wraz z lokalnymi i zdalnymi tabelami.

## Szyfrowanie

Warstwa P2P wspiera szyfrowanie wiadomości współdzielonym sekretem:

- `TSUNAMI_NETWORK_SECRET`

Jeśli sekret jest ustawiony, ramki protokołu są szyfrowane AES-GCM przed wysłaniem przez WebSocket.

## Konfiguracja adresu reklamowanego

- `TSUNAMI_NETWORK_ADVERTISE_ADDR`

Jeśli nie jest ustawione, node próbuje zbudować adres z lokalnego IP i portu `network-managera`.

## Ograniczenia obecnego reworku

- routing po katalogu tabel jest wdrożony dla ścieżki KV `read`,
- `save` i `free` aktualizują katalog tabel, ale nie robią jeszcze pełnej dystrybucji/partycjonowania danych,
- pojęcie "jednej bazy" jest obecnie realizowane na poziomie zdalnego odczytu po tabeli, nie globalnego planera zapisów.
