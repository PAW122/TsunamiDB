# json data type:

trzeba jakoś kofować jsona.


1)
na początek można zapisać raw json,
jakiś marker że to raw_json
opzwolić na akcje jak z tsuBot db

+ zapis danych:
jeżeli cały json nie przekracza danej wilkości jest zapisywany w 1 eleencie.


read zwróci całego jsona

/read/table/account.data
dla
account: {
    data: ""
}

specjalny save_tson (tsu json)
/save_tson/table
body: {
    account: {
    data: ">key<" <- odwołanie się od innego key
    }
}