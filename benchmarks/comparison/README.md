# Isolated JSONata comparison

This development-only module pins gnata v0.6.1; it is not a dependency of the
root module. The exact-size 1,024/8,192/65,536-byte inputs are generated from
`core` in `comparison_test.go` by padding an unused string. `simple` contains
eight scalar rules and `mixed` starts with gnata's four published `bench_test.go`
expressions followed by the scalar rules, repeated in order to form batches of 4, 16 and 64. Expected results
are independent constants (floating point sums use an absolute 1e-9 tolerance).
`TestParity` checks all cases, including null versus undefined, and fails on any
compile/evaluation error. It runs before each benchmark's timer as well.

Run with Go 1.25.13 or later and network access for the initial module download:

```
GOTOOLCHAIN=local go test ./...
GOTOOLCHAIN=local go test -run '^$' -bench . -benchmem -benchtime=100ms -count=5
```

Compile is outside evaluation loops. `ours_Eval` uses a pre-decoded Go value;
`ours_EvalJSON` decodes, validates and encodes once **per expression**. gnata's
`EvalMany` handles one document per batch with a schema plan warmed before the
timer. Its stock byte fast path assumes valid input. `gnata_validate_EvalMany`
adds `encoding/json.Valid` per batch; it still does not provide ownership of the
input. The APIs and guarantees therefore differ. None of these baseline rows is
a prepared-document result, and ratios against them are not prepared-path claims.
The module uses default safety settings for both libraries. Tests/benchmarks do
not skip unsupported comparisons.

To reproduce the immutable baseline while the working tree changes, archive
commit `2832babc3c1ea7093f6bd126ba4aed0f6fc0e9ae`, copy `go.mod` and `go.sum`
to temporary files, and set a `-modfile` replacement for the root module to the
archive directory. `testdata/results/` holds raw output, not executable binaries.
The baseline used `-benchtime=100ms -count=5` with default GOMAXPROCS on an
Apple M5 Max, darwin/arm64, Go 1.25.13 (`GOTOOLCHAIN=local`). A 100ms sample is
short and the machine was not isolated, so interpret timing as indicative.

For the prepared API, run `GOTOOLCHAIN=local go test -tags prepared
./...` and `GOTOOLCHAIN=local go test -tags prepared -run '^$' -bench
'^BenchmarkPrepared' -benchmem -benchtime=100ms -count=5`. The tag keeps the
harness usable against the pre-API archive. The prepared benchmarks separately
measure preparation, reuse, preparation plus the complete batch, and parallel
reuse. Compare the prepare-and-evaluate row against `BenchmarkBatch`'s warmed
EvalMany rows with identical inputs, rule counts and machine settings.

The four published expressions come from [gnata v0.6.1 `bench_test.go`](https://github.com/RecoLabs/gnata/blob/v0.6.1/bench_test.go),
Copyright 2026 RecoLabs Inc., MIT licensed; the license is reproduced in
`testdata/GNATA-LICENSE`. The document independently retains the same order and
product values with added scalar fields and padding for the matched fixtures.

`BenchmarkRetained` reports an approximate `retained-B/doc` by holding 128
prepared documents over a forced GC and dividing the change in live heap by
128. Run it separately with `-tags prepared -run '^$' -bench '^BenchmarkRetained'
-benchtime=1x -count=5`; it is sensitive to other live allocations and GC noise.
Do not interpret its `ns/op` as an evaluation time.

## Compiled batch comparison

`-tags 'prepared batch'` enables `TestCompiledBatchParity`, the mixed-engine
error regression, and `BenchmarkCompiledBatch`. Each result is checked against
the independent fixture constants before timing, using both fresh and reusable
destinations. The mixed-engine regression checks missing/null, JSONPath result
shape, the failing expression index, native JSONata diagnostics, and destination
cleanup. These tests require the new batch API; the `prepared` tag alone remains
usable with the saved pre-batch runtime source.

The `unique` workload adds distinct `Account.Count > i` expressions for every
index, with expected result `4 > i`. It shares a singular path prefix without
repeating complete expressions. Simple and mixed workloads retain the repeated
rules from the earlier comparison. All three use the full 1/8/64 KiB and
4/16/64-rule matrix.

`BenchmarkCompiledBatch` measures preparation plus batch evaluation, and
evaluation of an already prepared document, each with a fresh destination or a
destination allocated once outside the timer. The two matched gnata rows use a
warmed schema plan, with and without `json.Valid`. A typed result sink avoids
artificial interface-boxing allocations in both the old prepared-loop and new
batch benchmarks. Preparation always uses production defaults; compile and
verification work are excluded.

After production changes are frozen and other CPU-intensive checks have stopped,
run the complete five-sample matrix with Go 1.25.13:

```sh
sh run-batch.sh
```

The script uses `GOTOOLCHAIN=local`, a temporary build cache, and an alternate
module file pointing at `/private/tmp/jsontrans-before-batch` for the old runtime.
It records environment details, preparation-only measurements, the old prepared
loop, new batch modes, gnata, and hashes of sorted production Go paths and
contents before and after. `BENCHTIME` defaults to `200ms` for this large matrix;
`SAMPLES` defaults to 5. Override `SOURCE_SNAPSHOT`, `RESULTS_DIR`, or `GOCACHE`
when needed. Timings are development evidence, with no CI speed threshold.

The final frozen-source run contains 1,350 samples (five 200 ms samples for
each of 270 rows). `summarize-batch.py` validates the complete sample count and
generates [all matched medians](testdata/results/batch/summary.md) and
[CSV medians/ranges](testdata/results/batch/medians.csv). The corresponding
source hashes and raw outputs are in the same directory. Current production
results and the remaining 1 KiB simple/N64 gap are discussed in
[the performance report](../../docs/performance.md); earlier files in
`testdata/results/` describe historical implementations.


## Deadline-check comparison

The current deadline pass is measured by `python3 run-deadline.py`. It runs
seven representative cases with five 200ms samples for all six batch/gnata
modes, alternating the prior/current source per case, and validates all 420
samples plus unchanged runtime hashes. Raw results, medians/ranges, environment,
profile summary and a reverse runtime patch live in `testdata/results/deadline`.
This pass has no fixed latency target.

The default prior source is `/private/tmp/jsontrans-before-deadline`; override it
with `DEADLINE_BEFORE_SOURCE=/absolute/path/to/prior`. To reconstruct it from the
exact current version (hash recorded in source-before.txt), copy the runtime
source tree and go.mod/go.sum to a disposable directory and apply
`restore-before.patch` there with `patch -p1`. The patch changes only the five
production files affected by this pass. Its resulting runtime hash must match
the recorded prior hash. Reconstruction was verified before reporting results.
Do not run other CPU-intensive checks during timed sampling.

## Batch setup experiment

`python3 -B run-setup.py` reuses the matched deadline harness for seven fixture
cases, six operation modes and five 200ms samples per mode. Results, runtime
hashes and environment details are written to `testdata/results/setup/`.
The pre-optimization runtime is commit `127e13f`, which already contains the
Invoke result fixes. Reconstruct it from the repository root with `git archive
127e13f | tar -x -C /absolute/path/to/empty-directory`, then set
`SETUP_BEFORE_SOURCE=/absolute/path/to/empty-directory` when running the script.
The default snapshot location is `/private/tmp/jsontrans-before-setup`.
Keep other CPU-intensive checks stopped during measurement. The script verifies
all 420 samples and checks that both runtime hashes remain unchanged.

The initial broad setup run encountered machine-load variation also visible in
gnata controls. `python3 -B confirm-setup.py` checks the three 1 KiB workloads
with fixed harness binaries, alternating before/current order each round.
It records five 300ms samples per operation (150 total, including both result
destination modes and warmed gnata) in `testdata/results/setup/confirmation/`.
Both runs are retained; neither is an isolated performance or significance claim.
