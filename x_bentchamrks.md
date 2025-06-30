# 30.06.2025

> opt amount of system call's
> in fileManager

> same setup
> UP write & read ~ 42K/s & 42K/s = combain ~84K/s
> UP write only went up from 40-45K/s -> ~70K/s
> read only same ~ 100K/s

# 26.06.2025

> new fileManager.go in data/dataManager
> rn limited by cpu

> read & write went up from +/- 20-25K/s -> 40-45K/s
> write only stays on arount same lvl of 40-45K/s
> read only went up from ~40K/s to ~100K/s

> I/O opt
> amount of data saved to disk /s is reduced from ~22Mb/s to ~2.6Mb/s = ~10x lower

> speads achived on db with: 7_088_061 entries ~ 7GB total size

* read & write at the same time  
PS D:\projects\DB-tests> go run main.go --workers=1000 --url=http://127.0.0.1:5844 --test_type=readwrite  
[STATS] RPS: 21118 | AVG SAVE: 24.37ms | GPS: 20522 | AVG GET: 21.11ms  
[STATS] RPS: 19398 | AVG SAVE: 27.33ms | GPS: 19267 | AVG GET: 24.49ms  
[STATS] RPS: 24090 | AVG SAVE: 22.52ms | GPS: 24376 | AVG GET: 18.67ms  
[STATS] RPS: 24579 | AVG SAVE: 21.89ms | GPS: 24586 | AVG GET: 19.06ms  
[STATS] RPS: 22079 | AVG SAVE: 23.40ms | GPS: 22353 | AVG GET: 21.29ms  
[STATS] RPS: 24193 | AVG SAVE: 21.91ms | GPS: 24124 | AVG GET: 19.13ms  
[STATS] RPS: 24443 | AVG SAVE: 22.30ms | GPS: 24029 | AVG GET: 19.09ms  
[STATS] RPS: 24150 | AVG SAVE: 22.02ms | GPS: 24602 | AVG GET: 18.88ms  
[STATS] RPS: 22489 | AVG SAVE: 23.61ms | GPS: 22188 | AVG GET: 21.14ms  
[STATS] RPS: 24677 | AVG SAVE: 21.66ms | GPS: 24737 | AVG GET: 18.90ms  
[STATS] RPS: 23717 | AVG SAVE: 22.47ms | GPS: 23820 | AVG GET: 19.35ms  
[STATS] RPS: 21026 | AVG SAVE: 25.31ms | GPS: 20948 | AVG GET: 22.31ms  
[STATS] RPS: 23847 | AVG SAVE: 22.41ms | GPS: 23895 | AVG GET: 19.31ms  
[STATS] RPS: 24097 | AVG SAVE: 22.41ms | GPS: 24035 | AVG GET: 19.23ms  
[STATS] RPS: 24449 | AVG SAVE: 22.05ms | GPS: 24187 | AVG GET: 18.95ms  
[STATS] RPS: 21852 | AVG SAVE: 23.85ms | GPS: 21887 | AVG GET: 21.74ms  
[STATS] RPS: 24297 | AVG SAVE: 22.24ms | GPS: 24302 | AVG GET: 19.00ms  
[STATS] RPS: 24094 | AVG SAVE: 22.54ms | GPS: 24108 | AVG GET: 18.86ms  
[STATS] RPS: 24674 | AVG SAVE: 21.84ms | GPS: 24553 | AVG GET: 18.67ms  
[STATS] RPS: 21888 | AVG SAVE: 24.33ms | GPS: 22344 | AVG GET: 20.87ms  
[STATS] RPS: 24270 | AVG SAVE: 22.41ms | GPS: 23968 | AVG GET: 18.97ms  
[STATS] RPS: 23785 | AVG SAVE: 22.50ms | GPS: 23854 | AVG GET: 19.57ms  
[STATS] RPS: 23442 | AVG SAVE: 22.29ms | GPS: 23452 | AVG GET: 19.18ms  
[STATS] RPS: 22546 | AVG SAVE: 23.68ms | GPS: 22625 | AVG GET: 21.41ms  
[STATS] RPS: 24237 | AVG SAVE: 22.42ms | GPS: 24310 | AVG GET: 18.66ms  
[STATS] RPS: 23282 | AVG SAVE: 22.89ms | GPS: 23013 | AVG GET: 20.17ms  
[STATS] RPS: 22455 | AVG SAVE: 23.27ms | GPS: 22281 | AVG GET: 20.61ms  

* first wire 400_000 elements then read 400_000 elements  
PS D:\projects\DB-tests> go run main.go -test_type=write>read -requests=400000 -workers=500  
[STATS] RPS: 25874 | AVG SAVE: 19.02ms | GPS: 0 | AVG GET: 0.00ms  
[STATS] RPS: 31446 | AVG SAVE: 15.84ms | GPS: 0 | AVG GET: 0.00ms  
[STATS] RPS: 31027 | AVG SAVE: 15.95ms | GPS: 0 | AVG GET: 0.00ms  
[STATS] RPS: 31100 | AVG SAVE: 16.12ms | GPS: 0 | AVG GET: 0.00ms  
[STATS] RPS: 30687 | AVG SAVE: 16.19ms | GPS: 0 | AVG GET: 0.00ms  
[STATS] RPS: 29903 | AVG SAVE: 16.74ms | GPS: 0 | AVG GET: 0.00ms  
[STATS] RPS: 32415 | AVG SAVE: 15.31ms | GPS: 0 | AVG GET: 0.00ms  
[STATS] RPS: 31681 | AVG SAVE: 15.75ms | GPS: 0 | AVG GET: 0.00ms  
[STATS] RPS: 31575 | AVG SAVE: 15.82ms | GPS: 0 | AVG GET: 0.00ms  
[STATS] RPS: 32397 | AVG SAVE: 15.28ms | GPS: 0 | AVG GET: 0.00ms  
[STATS] RPS: 32363 | AVG SAVE: 15.44ms | GPS: 0 | AVG GET: 0.00ms  
[STATS] RPS: 27387 | AVG SAVE: 16.46ms | GPS: 9351 | AVG GET: 5.33ms  
[STATS] RPS: 0 | AVG SAVE: 0.00ms | GPS: 97146 | AVG GET: 5.15ms  
[STATS] RPS: 0 | AVG SAVE: 0.00ms | GPS: 83697 | AVG GET: 5.89ms  
[STATS] RPS: 0 | AVG SAVE: 0.00ms | GPS: 100268 | AVG GET: 4.95ms  
[STATS] RPS: 0 | AVG SAVE: 0.00ms | GPS: 91403 | AVG GET: 5.43ms  

* other write only test:  
PS D:\projects\DB-tests> go run main.go --workers=1000 --url=http://127.0.0.1:5844 --test_type=write  
[STATS] RPS: 41529 | AVG SAVE: 23.13ms | GPS: 0 | AVG GET: 0.00ms  
[STATS] RPS: 38839 | AVG SAVE: 25.47ms | GPS: 0 | AVG GET: 0.00ms  
[STATS] RPS: 44535 | AVG SAVE: 22.60ms | GPS: 0 | AVG GET: 0.00ms  
[STATS] RPS: 44876 | AVG SAVE: 22.11ms | GPS: 0 | AVG GET: 0.00ms  
[STATS] RPS: 39788 | AVG SAVE: 25.26ms | GPS: 0 | AVG GET: 0.00ms  
[STATS] RPS: 36917 | AVG SAVE: 26.72ms | GPS: 0 | AVG GET: 0.00ms  
[STATS] RPS: 39488 | AVG SAVE: 25.48ms | GPS: 0 | AVG GET: 0.00ms  
[STATS] RPS: 45383 | AVG SAVE: 22.00ms | GPS: 0 | AVG GET: 0.00ms  
[STATS] RPS: 44718 | AVG SAVE: 22.21ms | GPS: 0 | AVG GET: 0.00ms  

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

450_000 keys = +/- 20MB of data in ./db/maps & +/- 100Mb of RAM

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