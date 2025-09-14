## IncTables /read_inc

### example
POST 127.0.0.1:5844/read_inc/{table}/{key}

```js
params:{
    table: "name of table where key is",
    key: "from witch key db should read data"
}
```

headers:
```js
headers: {
    // how you want to read data from incTable
    // default - by_id
    // by_id - read data of specified entry - requires id header
        // optional header [id] -  specifies witch entry to read
    // last_entries - read x amount of entries starting from bottom (newest)
    // first_entries - read x amount of entries starting fro top (oldest)
    read_type: "by_id / last_entries / first_entries / (default: by_id)",

    // how mutch you want to read
    amount_to_read: "{number}"
}
```

response:
```js
body: {
    - 200 OK + JsonList:[{id: {number}, data: "data"}, {...}]
}
```
