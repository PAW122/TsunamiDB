# TsunamiDB

![Version](https://img.shields.io/badge/version-0.8.0-brightgreen.svg)

fast, simple key-value db

install
```
go get github.com/PAW122/TsunamiDB/lib/dbclient@v0.8.0
``` 

+ execute:
    go build -tags debug

    - local server without P2P network manager - ```./TsunamiDB.exe```
        > ./TsunamiDB
    - server with P2P network manager - ```./TsunamiDB.exe 5845```
        > ./TsunamiDB <port for node's communication>
    - server joining another node - ```./TsunamiDB-linux 5845 127.0.0.1:5845```
        > ./TsunamiDB-linux <port for node's communication> <ip and port of other server>

+ performance:
+ I5-10400f
+ on my local pc Iam geting consistant 45K writes/s
+ and ~100K reads/s

### changeLog for v0.8.0
+ added subscription system
+ improved saving data speed
+ better stability in long run
  > tested stable with sizes around 12GB
  > maps system rework

# commands:
```bash
-tags debug
go test -v ./lib/dbclient

go test -run . -v ./data/dataManager/v2
go test -run . -v ./servers/public-api/v1/routes
```

# Code Testing
```bash
# coverage in %
go test -cover ./...

# create raport (example):
go test ./data/dataManager/v2 -coverprofile=coverage
go tool cover -html=coverage -o coverage.html

# run tests
go test -cover ./...
```

# Database Testing
Special tests are disabled by default because they run for minutes.
Enable them only when you want to measure database behavior.
```powershell
$env:TSU_SPECIAL_TESTS='1'

# run all database special tests:
# - resources: 10m
# - stability fuzz: 1h
# - performance: 5m
go test ./tests -run TestSpecial -v

# resource usage only: RAM, CPU runtime, disk usage, logical IO
go test ./tests -run TestSpecialResourceUsage -v

# stability fuzz only: API routes, Go library, lib/export DLL candidate
go test ./tests -run TestSpecialStabilityFuzz -v

# performance only: writes/s, reads/s, total actions/s
go test ./tests -run TestSpecialPerformanceThroughput -v

# short smoke run for local verification
$env:TSU_RESOURCE_DURATION='10s'
$env:TSU_STABILITY_DURATION='10s'
$env:TSU_PERF_DURATION='10s'
go test ./tests -run TestSpecial -v
```

## Optional knobs
```powershell
$env:TSU_RESOURCE_DURATION='10m'
$env:TSU_STABILITY_DURATION='1h'
$env:TSU_PERF_DURATION='5m'
$env:TSU_RESOURCE_WORKERS='8'
$env:TSU_STABILITY_WORKERS='8'
$env:TSU_PERF_WORKERS='8'
$env:TSU_PAYLOAD_BYTES='512'
$env:TSU_STABILITY_MAX_PAYLOAD_BYTES='4096'
```

## Test Tensor System
```bash
$env:TSU_TENSOR_ACCURACY_TEST='1'
$env:TSU_TENSOR_KEEP_DIR='1'
go test ./tests -run TestTensorAcuricy -count=1 -v
```

## Relational DB Benchmark
```powershell
# microbenchmarks: insert, read, select scan, select indexes, LIKE trigram, row_ref join
# use fixed benchtime so Go does not repeat expensive table setup during benchmark calibration
go test ./data/relational -run '^$' -bench BenchmarkRelational -benchmem -benchtime=100x -count=1

# smaller/larger seeded tables for read/select/join benchmarks
$env:TSU_REL_BENCH_ROWS='1000'
go test ./data/relational -run '^$' -bench 'BenchmarkRelational/(ReadByRowID|Select|Join)' -benchmem -benchtime=100x -count=1

# throughput-style relational performance test with useful logs
$env:TSU_SPECIAL_TESTS='1'
$env:TSU_REL_PERF_DURATION='30s'
$env:TSU_REL_PERF_WORKERS='1'
$env:TSU_REL_PERF_ROWS='1000'
go test ./tests -run TestSpecialRelationalPerformance -count=1 -v

# raise workers only when you want concurrent table load; setup creates one seeded table per worker
$env:TSU_REL_PERF_WORKERS='12'

# saturation profiles for pushing the relational layer harder
$env:TSU_REL_SAT_DURATION='30s'
$env:TSU_REL_SAT_WORKERS='12'
$env:TSU_REL_SAT_ROWS='1000'
$env:TSU_REL_SAT_MODE='read'        # read, insert, select-eq, select-like, mixed
go test ./tests -run TestSpecialRelationalSaturation -count=1 -v
```
