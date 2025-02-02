# /encoding

V1:

encoding:
2bytes -> version
4bytes -> start ptr
4bytes -> end ptr
4bytes -> len
?bytes -> data

= 14bytes + data.len -> bin

# do przemyslenia:
czy chcemy w mapach zapisywac start pointery np do calego elementu zapisanego (versja, ptr, ptr, len, data)