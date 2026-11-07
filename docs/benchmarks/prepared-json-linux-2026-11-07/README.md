# Public prepared JSON-result APIs: Linux check

Measured on Bertram, Ubuntu 24.04 / Linux amd64, Intel Core i9-13900K,
Go 1.26.10, 2026-11-07. Single logical CPU 0, `GOMAXPROCS=1`, five warmed samples,
300 ms per sample. Raw output and environment details are in this directory.

This is a focused public-API microbenchmark, not a broker throughput or latency
test. Each evaluation returns a small JSON object from an input containing
1,024 padding characters. Sixteen-expression cases use distinct compiled rules.
All paths include JSON result encoding. Compilation is excluded. Document
preparation is excluded from the prepared and batch paths: these numbers measure
an already prepared document shared across expressions, and must not be read as
the full cost of preparing and transforming each incoming message. The existing
message experiments measure preparation-inclusive workloads separately.

Median results per complete operation (one or sixteen expressions):

| API | Expressions | Latency | Bytes/op | Allocs/op |
| --- | ---: | ---: | ---: | ---: |
| `EvalJSON` loop | 1 | 18.54 µs | 12,961 | 136 |
| `EvalPreparedJSON` loop | 1 | 4.30 µs | 5,176 | 55 |
| `Batch.EvalPreparedJSON` | 1 | 4.35 µs | 5,392 | 58 |
| `EvalJSON` loop | 16 | 300.65 µs | 207,382 | 2,176 |
| `EvalPreparedJSON` loop | 16 | 71.91 µs | 82,820 | 880 |
| `Batch.EvalPreparedJSON` | 16 | 68.97 µs | 81,596 | 868 |

The APIs retain the benefit of skipping repeated input materialization for these
native JSONata rules. Small single-rule batches have extra bookkeeping; use the
single helper for one rule. Encoding still allocates and uses bounded validation.
These measurements do not establish gains for every expression or payload.

Reproduce from the root module with Go 1.26.10:

```sh
GOTOOLCHAIN=local GOWORK=off GOEXPERIMENT='' GOMAXPROCS=1 taskset -c 0 \
  go test . -run '^$' -bench '^BenchmarkPreparedJSON$' \
  -benchmem -benchtime=300ms -count=5 -cpu=1
go test ./...
go test -race ./...
go vet ./...
```

The Linux source archive SHA-256 is recorded in `environment.txt`; the archive
contained the uncommitted implementation following `d5922ab`, before adding this
evidence report. Production source/tests remained frozen during measurement.
The first transfer mistakenly included macOS metadata sidecars, which made the
fixture readers fail. The corrected archive was checked to contain no sidecars;
the passing test/race/vet logs here come from that corrected source. No fixtures
were filtered or changed.

Additional macOS arm64 verification passed: root Go 1.26.10 tests/race/vet,
optional JavaScript module tests/race/vet, ordinary Go 1.27.3 tests and the tagged
Go 1.27 `jsontext_experiment` tests. Formatting and diff whitespace checks passed.
The bounded `FuzzPreparedJSON` run passed 30,915 executions in approximately
11 seconds with two workers. No Go version or default decoder selection changed.
