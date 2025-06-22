## POST /api/book/add

oczekiwane body:
```json
{
    "title": "example_title", 
    "author": "ex_author", 
    "publisher": "ex_publisher", 
    "year": "2025", 
    "isbn": "ex_isbn", 
    "pages": "100", 
    "description": "ex_description", 
    "tags": "ex_tags", 
    "shelf": "4C", 
    "copies": "{copies json}"
}
```

po otrzymaniu danych następuje:

1. stworzenie nowego UUID dla książki i zapisanie go w DB w liście książek
2. pod tym UUID zapisanie danych o egzemplarzu ks z jsona