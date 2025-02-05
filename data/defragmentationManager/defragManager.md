# /data/defragmentationManager

right now TsunamiDB dont have function like "delete data", delete operation requires time and locks file
for deletion time, if data blob is big it can slow db.

instead of deleteing data TsunamiDB is using system similar to RAM mempry allocation.
after using ```defragmentationManager.MarkAsFree()``` space is marked as free in /db/maps/free_blocks
it allows to reuse same space in file without wasting time to ovverwrite data or reformating whole file.

TODO:
add "low performance" function like: deleteAndFormat()
that will delete data and format file to dont left any not used space behaind

function will be intended to use only for db disk space optimization