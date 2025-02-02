# /data/fileSystem
wybieranie z którego pliku co odczytać,
do któego co zapisać

input (save) -> encoder -> fs -> dataManager -> file.bin
input (read) -> fs -> dataManager -> file.bin -> decoder -> return data.bin

* zapisywac mapy na hashMapach, [Key, startPtr, endPtr]