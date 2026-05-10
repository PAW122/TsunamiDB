# TsunamiDB

![Version](https://img.shields.io/badge/version-0.8.0-brightgreen.svg)

fast, simple key-value db

install
```
go get github.com/PAW122/TsunamiDB/lib/dbclient@v0.8.0
``` 

## Build binaries

```powershell
# Windows server binary
go build -buildvcs=false -o .dist\TsunamiDB.exe .
```

```bash
# Linux server binary
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
  go build -buildvcs=false -o .dist/TsunamiDB-linux .
```

```powershell
# Windows + Linux server binaries
.\build.bat
```

## DLL/shared-library wrapper
Native DLL/shared-library wrapper is available in `lib/dll`.

```powershell
# Windows DLL + generated C header
go build -buildvcs=false -buildmode=c-shared -o .dist\tsunamidb.dll .\lib\dll

# or
.\lib\dll\build.bat
```

```bash
# Linux .so + generated C header
go build -buildvcs=false -buildmode=c-shared -o .dist/libtsunamidb.so ./lib/dll

# or
bash ./lib/dll/build.sh
```

```powershell
# Linux .so from Windows, using WSL
.\lib\dll\build.bat linux
```

The wrapper exports the key-value, encrypted, network manager, public API, subscription and regex-key functions from `lib/dbclient`.

## Partial value patches
Use `POST /patch/{table}/{key}` when an application should send only a small change instead of saving the whole value again. TsunamiDB reads the current value, applies patch operations in order, stores the merged value, and streams a `patched` event to subscribers of that key.

Optional revision tracking can be enabled per key:

- `off` - default, no revision checks.
- `current` - stores current `rev`; patch requests must include matching `base_rev`.
- `history` - stores current `rev` and retained patch records for reconnect catch-up.

```bash
curl -X POST http://localhost:5844/revision/docs/doc-1 \
  -H "Content-Type: application/json" \
  --data '{"mode":"history"}'

curl -X POST http://localhost:5844/patch/docs/doc-1 \
  -H "Content-Type: application/json" \
  --data '{"base_rev":0,"ops":[{"offset":5,"insert":","},{"offset":7,"delete":5,"insert":"TsuDB"}]}'

curl "http://localhost:5844/revision/docs/doc-1/patches?from_rev=0"
```

Go client:

```go
state, err := TsuClient.SetRevisionPolicy("doc-1", "docs", TsuClient.RevisionModeHistory)
updated, state, err := TsuClient.PatchWithRevision("doc-1", "docs", state.Rev, []TsuClient.PatchOperation{
	{Offset: 5, Insert: ","},
	{Offset: 7, Delete: 5, Insert: "TsuDB"},
})
```

+ execute:
    go build -buildvcs=false -tags debug

    - local server without P2P network manager - ```.dist\TsunamiDB.exe```
    - server with P2P network manager - ```.dist\TsunamiDB.exe 5845```
    - Linux server joining another node - ```./.dist/TsunamiDB-linux 5845 127.0.0.1:5845```

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

## Benchmarks
### Relational Microbenchmarks
```powershell
# microbenchmarks: insert, read, select scan, select indexes, LIKE trigram, row_ref join
# use fixed benchtime so Go does not repeat expensive table setup during benchmark calibration
go test ./data/relational -run '^$' -bench BenchmarkRelational -benchmem -benchtime=100x -count=1

# smaller/larger seeded tables for read/select/join benchmarks
$env:TSU_REL_BENCH_ROWS='1000'
go test ./data/relational -run '^$' -bench 'BenchmarkRelational/(ReadByRowID|Select|Join)' -benchmem -benchtime=100x -count=1
```

Microbenchmark names:

| Benchmark | Meaning |
|---|---|
| `InsertNoIndex` | insert into table without secondary indexes |
| `InsertEqualityAndTrigramIndexes` | insert into table with equality and trigram indexes enabled |
| `ReadByRowID` | direct single-row lookup by `row_id` |
| `SelectEqualityScan` | equality `SELECT` without equality index, full scan |
| `SelectEqualityIndex` | equality `SELECT` with equality index |
| `SelectLikeScan` | `LIKE` query without trigram index, full scan |
| `SelectLikeTrigramIndex` | `LIKE` query using trigram index |
| `JoinRowRefIndexedPredicate` | `row_ref` join with indexed predicate |

Microbenchmark columns:

| Column | Meaning |
|---|---|
| `Runs` | number of benchmark iterations |
| `Time/op` | average time per operation |
| `Throughput` | processed MB per second when Go can derive byte volume for that benchmark |
| `Memory/op` | heap bytes allocated per operation |
| `Allocs/op` | number of heap allocations per operation |

### Relational Throughput Tests
```powershell
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
$env:TSU_REL_SAT_MODE='read'        # read, insert, select-eq, select-like, related-select, mixed
go test ./tests -run TestSpecialRelationalSaturation -count=1 -v

# one-command complex relational report
powershell -ExecutionPolicy Bypass -File .\tools\relational-perf-report.ps1
```

Throughput metrics:

| Metric | Meaning |
|---|---|
| `actions/s` | total API operations per second: `reads + inserts + equality selects + like selects + related selects` |
| `reads/s` | `ReadRow(...)` calls per second; in `read` mode one read is one row lookup |
| `inserts/s` | `InsertRow(...)` calls per second |
| `selects/s` | `SelectRows(...)` calls per second for equality and/or `LIKE` selects |
| `related/s` | `JoinRowRef(...)` calls per second |
| `rows/s` | rows returned per second by `SelectRows(...)` and `JoinRowRef(...)`; this is not the same as `actions/s` |

Mode meanings in `relational-perf-report.ps1`:

| Mode | Meaning |
|---|---|
| `read` | repeated `ReadRow(...)` lookups |
| `insert` | repeated `InsertRow(...)` writes |
| `select-eq` | repeated equality `SelectRows(...)` queries |
| `select-like` | repeated `LIKE` `SelectRows(...)` queries |
| `related-select` | repeated `JoinRowRef(...)` relation queries |
| `mixed` | combined loop of `ReadRow + SelectRows(eq) + SelectRows(like) + InsertRow` |

Example output:

| Benchmark | Runs | Time/op | Throughput | Memory/op | Allocs/op |
|---|---:|---:|---:|---:|---:|
| `BenchmarkRelational/InsertNoIndex-16` | 100 | 34,511 ns/op | 3.74 MB/s | 3,226 B/op | 52 allocs/op |
| `BenchmarkRelational/InsertEqualityAndTrigramIndexes-16` | 100 | 2,743,240 ns/op | 0.05 MB/s | 155,426 B/op | 1,324 allocs/op |
| `BenchmarkRelational/ReadByRowID-16` | 100 | 22,406 ns/op | 5.76 MB/s | 2,631 B/op | 50 allocs/op |
| `BenchmarkRelational/SelectEqualityScan-16` | 100 | 67,153,182 ns/op | 19.21 MB/s | 4,509,936 B/op | 91,986 allocs/op |
| `BenchmarkRelational/SelectEqualityIndex-16` | 100 | 2,601,748 ns/op | - | 449,419 B/op | 2,061 allocs/op |
| `BenchmarkRelational/SelectLikeScan-16` | 100 | 82,068,315 ns/op | 15.72 MB/s | 10,417,794 B/op | 449,818 allocs/op |
| `BenchmarkRelational/SelectLikeTrigramIndex-16` | 100 | 59,264,156 ns/op | - | 14,430,471 B/op | 31,642 allocs/op |
| `BenchmarkRelational/JoinRowRefIndexedPredicate-16` | 100 | 2,913,305 ns/op | - | 560,132 B/op | 5,235 allocs/op |

## Relational SQL
The relational engine accepts a compact SQL subset through `POST /rel/sql`.
The endpoint executes one statement per request and returns a JSON `SQLResult`.
The request body can be raw SQL (`text/plain`) or JSON:

```json
{"query":"SELECT * FROM products"}
```

PowerShell example:

```powershell
$body = @{
  query = "SELECT row_id, name, price FROM products WHERE name LIKE '%wid%' ORDER BY price DESC"
} | ConvertTo-Json

Invoke-RestMethod -Method Post -Uri "http://localhost:5844/rel/sql" -ContentType "application/json" -Body $body
```

curl example:

```bash
curl -X POST http://localhost:5844/rel/sql \
  -H "Content-Type: text/plain" \
  --data "SELECT row_id, name, price FROM products WHERE name LIKE '%wid%'"
```

## MySQL-compatible relational endpoint
The server also starts a minimal MySQL wire-protocol endpoint for SQL clients such
as HeidiSQL. It listens on port `3307` by default and maps client queries onto the
same relational SQL engine used by `POST /rel/sql`.

HeidiSQL connection settings:

```text
Network type: MySQL (TCP/IP)
Hostname: 127.0.0.1
Port: 3307
User: any value
Password: empty or any value
Database: tsunamidb
```

Change the port before starting the server:

```powershell
$env:TSU_MYSQL_PORT='3310'
go run .
```

The compatibility endpoint supports MySQL handshake/login, `USE`, `PING`, basic
session `SET` commands, metadata queries used by GUI clients (`SHOW DATABASES`,
`SHOW TABLES`, `SHOW FULL TABLES`, `SHOW TABLE STATUS`, `SHOW COLUMNS`,
`SHOW CREATE TABLE`, `SHOW VARIABLES`) and normal relational statements such as
`CREATE TABLE`, `INSERT`, `SELECT`, `UPDATE`, and `DELETE`.

Supported column types:

```text
uint64, int64, bool, float64, string(N), string[N], blob_ptr, row_ref
```

Supported statements:

```sql
SHOW TABLES;

CREATE TABLE products (
  id uint64 PRIMARY KEY,
  name string(32) INDEXED TRIGRAM,
  price uint64,
  active bool
);

INSERT INTO products (id, name, price, active)
VALUES (1, 'widget', 100, true);

INSERT INTO products (id, name, price, active)
VALUES (2, 'gadget', 250, false);

SELECT * FROM products;
SELECT row_id, name, price FROM products WHERE id = 1;
SELECT row_id, name, price FROM products WHERE name LIKE '%wid%';
SELECT row_id, name, price FROM products ORDER BY price DESC;

UPDATE products
SET price = 175, name = 'bluewidget'
WHERE row_id = 0;

DELETE FROM products WHERE id = 2;

CREATE INDEX products_price_idx ON products (price);
CREATE TRIGRAM INDEX ON products (name);
```

Result examples:

```json
{
  "operation": "insert",
  "table": "products",
  "row_id": 0,
  "rows_affected": 1
}
```

```json
{
  "operation": "select",
  "table": "products",
  "rows_affected": 1,
  "rows": [
    {
      "row_id": 0,
      "values": {
        "name": "widget",
        "price": 100
      }
    }
  ]
}
```

Current SQL limits:

- `WHERE` supports one predicate with `=` or `LIKE`.
- `row_id` is a synthetic column and can be used in `SELECT`, `WHERE`, and `ORDER BY`.
- `ORDER BY` supports `ASC` and `DESC`; numbers sort numerically, and string values that match `YYYY-MM-DD`, `YYYY-MM-DD HH:MM[:SS]`, or RFC3339 sort as dates.
- String literals use single quotes. Escape a quote by writing it twice, for example `'Bob''s item'`.
- Joins, `AND`/`OR`, ranges, aggregates, `ALTER TABLE`, `DROP TABLE`, and multi-statement batches are not implemented.
