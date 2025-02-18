# HTTP docs

 - file name
    > use <file name> argument like tabe in sql db

! inportant - no matter what <file name> / <table> u are using one key value can be use only once
that means if u save table_1: key1 and then table_2: key1
key1 will be deleted from table_1

+ Save
    - POST
    - example url:
        > 127.0.0.1:5844/save/<file name>/<key>
        > 127.0.0.1:5844/save/data.bin/test
    - example body:
        > <binary obj>
        > Hello world

    - res:
        * "save" -> save succes
        * "Invalid url args" -> <file name> or <key> param is invalid
        * "Invalid body" -> error reading request Body
        * "Error saving to file: ..." -> error saving encoded body content to file
        * "Error saving to map: ..." -> error saving key to map (data is saved in file but metadata in memory is invalid, data may not be possible to read)

    - description:
        save data to database file using patern like in json: key: value.

    - process:
        1. read <file> and <key> arg from url path
        2. read body from req.Body
        3. encode data
        4. save encoded data to file
        5. add key & pointer to data in map
        6. return OK status

+ Read
    - GET
    - example url:
        > 127.0.0.1:5844/read/<file name>/<key>
        > 127.0.0.1:5844/read/data.bin/test
    - example body response:
        > <binary/string obj> (data may be decoded to string and not binary but all data is the same)
        > Hello world

    - res:
        * any data in body -> read succes, all data saved with this key is returned in response body
        * "Invalid url args" -> <file name> or <key> param is invalid
        * "Error retrieving element from map: ..." -> error occured when reading metatada for key value from map
        * "Error reading from file: ..." -> error occured when reading encoded file

    - description:
        read data from database file using key.

    - process:
        1. read <file> and <key> arg from url path
        2. get metadata assigned for this key
        3. read data from file based on metadata
        4. decode readed data from file 
        5. add data without metadata to response Body
        6. send res

+ Free
    - GET
    - example url:
        > 127.0.0.1:5844/free/<file name>/<key>
        > 127.0.0.1:5844/free/data.bin/test
    
    - res:
        * free -> succesfully freed space
        * "Invalid url args" -> <file name> or <key> param is invalid
        * "Error retrieving element from map: ..." -> error occured when reading metatada for key value from map

    - description:
        free space,
        free works in a similar way to free in C.
        it frees up previously occupied space and allows new data to be written without having to format the entire data file

    - process:
        1. read <file> and <key> arg from url path
        2. get metadata assigned for this key
        3. remove key and metadata from map
        4. mark space before taken by data as free
        5. return "free" response

+ Save Encrypted
    - POST
    - example url:
        > 127.0.0.1:5844/save_encrypted/<file name>/<key>
        > 127.0.0.1:5844/save_encrypted/data.bin/test
    - example body:
        > <binary obj>
        > Hello world
    - example header:
        > encryption_key <key>

    - res:
        * "save" -> save succes
        * "Invalid url args" -> <file name> or <key> param is invalid
        * "Invalid body" -> error reading request Body
        * "Error saving to file: ..." -> error saving encoded body content to file
        * "Error saving to map: ..." -> error saving key to map (data is saved in file but metadata in memory is invalid, data may not be possible to read)
        * Error Encryptiong data -> errpr ocured while encryption body content
        * Missing encryption_key header -> encryption_key is not provided

    - description:
        encrypt & save data based on provided encryption_key 

+ Read Encrypted
    - GET
    - example url:
        > 127.0.0.1:5844/read_encrypted/<file name>/<key>
        > 127.0.0.1:5844/read_encrypted/data.bin/test
    - example header:
        > encryption_key <key>
    - example body response:
        > <binary/string obj>
        > Hello world
    

    - res:
        * any data in body -> read succes, all data saved with this key is returned in response body
        * "Invalid url args" -> <file name> or <key> param is invalid
        * "Error retrieving element from map: ..." -> error occured when reading metatada for key value from map
        * "Error reading from file: ..." -> error occured when reading encoded file
        * "Error decryping data" -> error ocured while decrypting data
        * "Missing encryption_key header" -> encryption key header is missing

    - description:
        read data from database, decrypt using provided ecryption_key
        and return 