# Initial performance baseline

Measured on 2026-11-02 with Go 1.25.13, macOS arm64, Apple M5 Max:

```sh
go test ./examples/basic -run '^$' -bench . -benchtime=200ms
```

| Operation | Time per operation | Bytes allocated | Allocations |
| --- | ---: | ---: | ---: |
| JSONPath compile | 410 ns | 976 | 14 |
| JSONPath evaluate | 1.21 µs | 1,801 | 28 |
| JSONata compile | 2.22 µs | 6,168 | 31 |
| JSONata evaluate | 6.38 µs | 11,633 | 56 |

The benchmark input contains two items priced 4 and 7. Queries are
`$.items[*].price` and `$sum(items.price)`. It calls each engine directly with
decoded Go values and production default limits. Engine construction is outside
the measurement; evaluation includes input validation and detachment. The source
is examples/basic/benchmark_test.go.

These are single-run local baselines. Larger documents, complex expressions,
host functions, JSON encoding, and transformer wrappers have different costs.
Use the committed benchmark to measure the workload and deployment hardware
that matter to an application. There is no throughput guarantee.
