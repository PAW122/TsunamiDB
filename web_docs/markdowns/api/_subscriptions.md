## subscriptions webscoket

### subServer endpoints:
* ws://127.0.0.1:5845/sub
* 127.0.0.1:5844/subscriptions/disable
* 127.0.0.1:5844/subscriptions/enable

- endpoint powinien być wystawiony jako publiczny
- służy on łatwej synchronizacji danych
    > możesz wygenerować auth_key dla użytkownika używając `127.0.0.1:5844/subscriptions/enable` co pozwoli mu zasubskrybować key dla którego wygenerowałeś auth_key.
    > klucz auth_key jest ważny 60s po wygenerowaniu.
- kiedy użytkownik połączy się ws i zasubskrybuje dany key będzie dostawał wiadomości informujące o aktualizacji danych.

- łączenie
    > po połączeniu się z ws user musi wysłać wiadomość:
    ```
    {
	"auth_key": "auth_key"
    }
    ```
    > po jej wysłaniu zostanie automatycznie dodany do listy użytkowników subskrybujących dany klucz.

- użytkowik może subskrybować wiele kluczy w tym samym czasie

- użycie:
    ```
    127.0.0.1:5844/subscriptions/enable
    body: {
        "key": "test"
    }
    ```
    spowoduje usunięcie subskrybcji wszystkim użytkownikom który otrzymywali wiadomości o aktualizacji danych dla danego key

- testowanie
    > zapisz jakiś key w DB
    > użyj endpointu enable do wygenerowania key
    > połącz się z WS i wyślij klucz
    > edytuj dane poprzez wysyłanie /save dla tego key
    > db powinna wysłać wiadomość o aktualizacji danych do wszystkich połączonych użytkowników

    > gotowe endpointy do testowania dla Insomnia znajdziesz w reposytorium github pod ścieżką: `docs/setup`