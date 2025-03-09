# /encoding

b = 8B

V1:

encoding:
2bytes -> version
4bytes -> start ptr
4bytes -> end ptr
4bytes -> len
?bytes -> data
= 14bytes + data.len -> bin

V1.1:

encoding:
1b -> version
1b -> pointerSize
1b - 4b -> startPtr 
1b - 4b -> endPtr
?b -> data
= 4bytes + data.len -> bin

# do przemyslenia:
czy chcemy w mapach zapisywac start pointery np do calego elementu zapisanego (versja, ptr, ptr, len, data)

# sql:


polecenie POST:
body:
{
    query: insert,
    tableName: userdata
    data: {
        id: 1
        emapl: example@a.com
    }
}