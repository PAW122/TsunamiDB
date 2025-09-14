## IncTables /save_inc

### example
POST 127.0.0.1:5844/save_inc/{table}/{key}

```params
table: "name of table in witch incTable key will be stored",
key: "entry name under witch incTable data will be saved"
```

headers:
```js
headers: {
    //max size of entry in [bytes] table (body)
    max_entry_size: "{uint64}",
    
    //optional header - if you want to set your own id, until its in range of arleady existing id's u cant do that
    // default = nex available id
    id: "{uint64}",

    // optional header - id header required
    // default = append
    // append - adds entry on specified id and move other entries down
    // overwrite - over writes data of specified id
    mode: "overwrite / append",

    // optional header - id header required
    // default = top
    // top - when using id 0 it will affect first entry in table (oldest)
    // bottom - when using id 0 it will affect last entry in table (newest)
    count_from: "top / bottom"
}
```

data:
```js
//body - data taht will be saved in entry
body: {
    test entry 10
}
```

response:
```js
body: {
    // returns id assigned to entry
    "id": "{number}"
}
```