# 21.06.2025 - v0.7.6

> test of write capacity per second.

setup:
i5-10400f
300mb/s SSD

results:
stable 29_000 - 48_000 writes/s
4-5ms / write

DB:
10 GB/data
1.25 GB/map data
in total 19M saved objects
3.8GB RAM usage - idle
6GB RAM usage - runing

System usage:
+/- 20 mb/s avarage
40% cpu usage avarage (db.exe)
*with clients sending requests and other programs cpu usage = 100%


# odl
Total save time: 35.3311759s, Avg save time: 3.533117ms
Total read time: 2.0534737s, Avg read time: 205.347µs
Total execution time: 37.3851652s

edit async saveData
Total save time: 32.7199948s, Avg save time: 3.271999ms
Total read time: 435.9027ms, Avg read time: 43.59µs
Total execution time: 33.1558975s

edit mapManager
Total save time: 28.4566859s, Avg save time: 2.845668ms
Total read time: 436.7484ms, Avg read time: 43.674µs
Total execution time: 28.8954282s

edit saveData.go
Total save time: 29.0531096s, Avg save time: 2.90531ms
Total read time: 505.3292ms, Avg read time: 50.532µs
Total execution time: 29.5594363s

5. 
Total save time: 30.330817s, Avg save time: 3.033081ms
Total read time: 471.9001ms, Avg read time: 47.19µs
Total execution time: 30.8037135s

[DEBUG] Timing Stats:
defragmentation [loadFreeBlocks] - avg: 203ns, total: 4.0691ms, count: 20000
defragmentation [GetBlock] - avg: 867ns, total: 8.6798ms, count: 10000
save-to-file - avg: 1.682237999s, total: 4h40m22.3799994s, count: 10000
fileSystem [GetElementByKey] - avg: 91.339999ms, total: 15m13.3999979s, count: 10000
decode - avg: 1.498µs, total: 14.9808ms, count: 10000
fileSystem [SaveElementByKey] - avg: 3.506740476s, total: 9h44m27.4047606s, count: 10000
fileSystem [saveMap] - avg: 8.276895458s, total: 22h42m6.0657604s, count: 9874
encode - avg: 49.615µs, total: 496.1541ms, count: 10000
defragmentation [SaveBlockCheck] - avg: 749ns, total: 7.4982ms, count: 10000
read-from-file - avg: 844.086µs, total: 8.4408663s, count: 10000

6. 
Total save time: 2.6978825s, Avg save time: 269.788µs
Total read time: 453.2166ms, Avg read time: 45.321µs
Total execution time: 3.1510991s

[DEBUG] Timing Stats:
save-to-file - avg: 1.378749694s, total: 3h49m47.4969431s, count: 10000
fileSystem [SaveElementByKey] - avg: 8.83µs, total: 88.3086ms, count: 10000
encode - avg: 12.628µs, total: 126.285ms, count: 10000
defragmentation [GetBlock] - avg: 869ns, total: 8.6935ms, count: 10000
defragmentation [SaveBlockCheck] - avg: 249ns, total: 2.4978ms, count: 10000
read-from-file - avg: 1.220392ms, total: 12.2039276s, count: 10000
decode - avg: 56.507µs, total: 565.0756ms, count: 10000
defragmentation [loadFreeBlocks] - avg: 125ns, total: 2.5143ms, count: 20000
fileSystem [GetElementByKey] - avg: 7.105924ms, total: 1m11.0592431s, count: 10000

7. 
Total save time: 2.6356614s, Avg save time: 263.566µs
Total read time: 513.0278ms, Avg read time: 51.302µs
Total execution time: 3.1486892s

[DEBUG] Timing Stats:
defragmentation [GetBlock] - avg: 632ns, total: 6.3228ms, count: 10000
fileSystem [GetElementByKey] - avg: 176.631µs, total: 1.7663148s, count: 10000
defragmentation [SaveBlockCheck] - avg: 450ns, total: 4.5029ms, count: 10000
decode - avg: 1.708µs, total: 17.0897ms, count: 10000
save-to-file - avg: 1.335791717s, total: 3h42m37.9171794s, count: 10000
encode - avg: 22.068µs, total: 220.6867ms, count: 10000
defragmentation [loadFreeBlocks] - avg: 151ns, total: 3.0278ms, count: 20000
read-from-file - avg: 9.345124ms, total: 1m33.451243s, count: 10000
fileSystem [SaveElementByKey] - avg: 9.919µs, total: 99.1946ms, count: 10000

8. retest
Total save time: 2.561245s, Avg save time: 256.124µs
Total read time: 461.3913ms, Avg read time: 46.139µs
Total execution time: 3.0236343s

[DEBUG] Timing Stats:
save-to-file - avg: 1.278677373s, total: 3h33m6.7737391s, count: 10000      
read-from-file - avg: 9.198135ms, total: 1m31.9813513s, count: 10000        
defragmentation [loadFreeBlocks] - avg: 100ns, total: 2.0162ms, count: 20000
defragmentation [GetBlock] - avg: 735ns, total: 7.3539ms, count: 10000      
encode - avg: 24.316µs, total: 243.163ms, count: 10000
defragmentation [SaveBlockCheck] - avg: 450ns, total: 4.5002ms, count: 10000
decode - avg: 9.969µs, total: 99.693ms, count: 10000
fileSystem [SaveElementByKey] - avg: 4.189µs, total: 41.8969ms, count: 10000
fileSystem [GetElementByKey] - avg: 183.09µs, total: 1.8309061s, count: 10000

9. 100 000K
PS D:\projects\TsunamiDB> ./TsunamiDB.exe     
Total save time: 24.3367455s, Avg save time: 243.367µs
Total read time: 4.4130776s, Avg read time: 44.13µs
Total execution time: 28.7524813s

[DEBUG] Timing Stats:
defragmentation [GetBlock] - avg: 742ns, total: 74.2878ms, count: 100000
fileSystem [GetElementByKey] - avg: 593.136203ms, total: 16h28m33.6203027s, count: 100000
defragmentation [SaveBlockCheck] - avg: 612ns, total: 61.2321ms, count: 100000
save-to-file - avg: 12.045102155s, total: 334h35m10.2155519s, count: 100000
fileSystem [SaveElementByKey] - avg: 269.026µs, total: 26.9026254s, count: 100000
encode - avg: 19.797µs, total: 1.9797254s, count: 100000
defragmentation [loadFreeBlocks] - avg: 69ns, total: 13.9983ms, count: 200000
read-from-file - avg: 2.441859ms, total: 4m4.185911s, count: 100000
decode - avg: 1.504µs, total: 150.4232ms, count: 100000


===

[DEBUG] Timing Stats:
fileSystem [GetElementByKey] - avg: 17.138µs, total: 171.3848ms, count: 10000
fileSystem [SaveElementByKey] - avg: 3.692µs, total: 36.9295ms, count: 10000
defragmentation [GetBlock] - avg: 796ns, total: 7.9684ms, count: 10000
defragmentation [loadFreeBlocks] - avg: 0s, total: 0s, count: 20000
read-from-file - avg: 6.875055ms, total: 1m8.7505525s, count: 10000
defragmentation [SaveBlockCheck] - avg: 398ns, total: 3.9897ms, count: 10000
save-to-file - avg: 1.013458007s, total: 2h48m54.580079s, count: 10000
decode - avg: 16.387µs, total: 163.8716ms, count: 10000
encode - avg: 9.114µs, total: 91.141ms, count: 10000