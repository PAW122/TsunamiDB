# Subscription Benchmark Comparison

Command before and after:

```powershell
go test ./servers/subscriptions -run '^$' -bench BenchmarkSubscriptionServer -benchmem -benchtime=100x -count=1
```

| Scenario | Before ns/op | After ns/op | Before B/op | After B/op | Before allocs/op | After allocs/op |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| clients=10 keys=1 | 49,955 | 56,105 | 9,099 | 9,139 | 107 | 107 |
| clients=10 keys=10 | 4,387 | 8,807 | 1,599 | 1,589 | 16 | 16 |
| clients=100 keys=1 | 425,818 | 548,172 | 81,262 | 81,047 | 1,009 | 1,008 |
| clients=100 keys=10 | 50,493 | 62,156 | 8,998 | 8,923 | 107 | 107 |

Additional longer after-change run:

```powershell
go test ./servers/subscriptions -run '^$' -bench BenchmarkSubscriptionServer -benchmem -benchtime=1000x -count=1
```

| Scenario | After 1000x ns/op | After 1000x B/op | After 1000x allocs/op |
| --- | ---: | ---: | ---: |
| clients=10 keys=1 | 67,720 | 8,711 | 106 |
| clients=10 keys=10 | 5,934 | 1,440 | 16 |
| clients=100 keys=1 | 563,198 | 80,946 | 1,007 |
| clients=100 keys=10 | 58,116 | 8,678 | 106 |

Allocation counts stayed flat or slightly lower. The old full-value notification path still uses `NotifySubscribers` and the implementation only adds a separate `NotifyPatchSubscribers` path beside it, so the observed timing movement is most likely normal short-run WebSocket benchmark variance rather than an added allocation or hot-path regression.
