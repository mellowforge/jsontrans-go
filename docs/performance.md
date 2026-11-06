# Performance measurements

## Batch setup consolidation (2026-11-03)

The retained experiment combines immutable wrapper/native limit configuration
when constructing prepared entries, caches scratch-budget reuse eligibility, and passes
immutable limit values internally by reference. Default calls avoid resolving
those limits for every expression; adjacent equivalent configurations share
per-call override resolution. Budgets, cancellation checkpoints, callback
lifetimes, binding/result ownership and failure attribution retain their behavior.
No public API, dependency or safety default changed.

The baseline is commit `127e13f`, which already includes the raw `Invoke` fixes:
result detachment uses output limits, preserves non-finite language values,
measures normalized numbers, rejects canonical-key collisions and propagates
sequence-conversion failures. Only `batch.go` and `document.go` differ between
the production sources in this performance comparison.

The initial seven-case run has 420 samples (five 200ms samples per operation).
It showed large timing shifts in some blocks, including gnata controls. A
confirmation run used fixed harness binaries, reversed before/current order
each round, and collected 150 additional samples (five 300ms samples for each
operation). Both datasets are retained. Measurements used Go 1.25.13, Apple M5
Max, darwin/arm64, default GOMAXPROCS=18, production safety defaults, compilation
outside timers and warmed gnata schema plans.

Confirmation medians below are microseconds per whole batch, using 1 KiB input
and a reused result destination. “Reuse” excludes preparation; “prepare+batch”
includes validation, ownership and index construction.

| Workload | Rules | Before reuse | After reuse | Before prepare+batch | After prepare+batch | Warmed gnata |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Simple | 64 | 12.33 | 11.37 | 16.49 | 17.66 | 13.96 |
| Mixed | 16 | 7.45 | 7.21 | 11.62 | 11.30 | 9.38 |
| Distinct predicates | 64 | 8.99 | 7.75 | 13.39 | 11.99 | 11.43 |

Document-reuse medians improve approximately 8%, 3% and 14% respectively.
Distinct predicates improve 10% including preparation, with non-overlapping
sample ranges in that confirmation. The simple preparation-inclusive median
is 7% slower, while its gnata control also shifts from 11.60 to 13.96 us; the
simple samples have broad overlapping ranges. These shared-machine results
support retaining the smaller setup path for repeated evaluation, but do not
establish a universal end-to-end improvement or statistical significance. The
initial broad run also contains apparent regressions and remains visible.
Allocation counts are unchanged across all matched operations in both runs:
reused simple64 is 1,281 bytes/68 allocations, mixed16 is 3,587–3,588/120 and
distinct64 is 384/4; B/op medians vary by up to one byte between runs. This is not a zero-allocation implementation.

The fixtures retain fixed collection cardinality; larger inputs grow unused
padding. gnata's stock fast path assumes valid JSON, whereas preparation here
validates and owns it. The broad dataset includes a validation-inclusive gnata
control and both destination modes. No throughput or gnata-relative target is
imposed.

Runtime hashes were unchanged throughout both accepted runs: baseline
`58ebdea4d46f598994f4dbeabdd46e00cb468ac2a2f374bd5d7b757aa88bf636`, retained
`886130a91ca056da2f942845760a0c91a1ef8bcc16fb48ff8099809d78b49481`.

[Alternating-order confirmation](../benchmarks/comparison/testdata/results/setup/confirmation/summary.md),
[confirmation samples and ranges](../benchmarks/comparison/testdata/results/setup/confirmation/medians.csv),
[broad seven-case results](../benchmarks/comparison/testdata/results/setup/summary.md),
and [broad samples and ranges](../benchmarks/comparison/testdata/results/setup/medians.csv)
are saved with raw outputs and environment information. Reproduce using
`python3 -B run-setup.py` and `python3 -B confirm-setup.py` in the comparison
module; its README explains reconstructing the baseline.

All sections below describe historical source versions; their “current” labels
mean current at the time of those measurements.

## Deadline checks before setup consolidation (historical)

This pass replaces repeated wall-plus-monotonic clock reads with Go's
monotonic-only `time.Until` path where available, and delegates duplicate native
entry/exit checks to the enclosing evaluator. Successful evaluations still check
cancellation before returning. Bare handlers, loop/resource accounting, the
32-work-unit checkpoint interval, owned callback scopes and all defaults retain
their behavior. Independent review, full correctness/reference/race gates and
bounded fuzz runs passed. There is no fixed latency or gnata-relative target.

Matched sequential before/after runs used Go 1.25.13, Apple M5 Max,
darwin/arm64, GOMAXPROCS=18 and five 200 ms samples per row. Below are median
microseconds per document, including preparation and reusing the result
destination. Compiled expressions and schema warming are outside timers.

| Input | Workload | Rules | Before | After | Warmed gnata |
| --- | --- | ---: | ---: | ---: | ---: |
| 1 KiB | simple | 4 | 5.79 | 4.64 | 2.99 |
| 1 KiB | simple | 16 | 9.09 | 6.76 | 4.77 |
| 1 KiB | simple | 64 | 22.97 | 15.87 | 11.23 |
| 1 KiB | mixed | 16 | 14.02 | 11.24 | 8.89 |
| 1 KiB | distinct scalar predicates | 64 | 20.48 | 13.29 | 11.32 |
| 8 KiB | mixed | 16 | 18.71 | 17.26 | 17.82 |
| 64 KiB | simple | 64 | 63.73 | 57.17 | 157.77 |

The 1 KiB simple cases take 20–31% less time; distinct scalar predicates improve
35%. On an already prepared document, the 64-rule simple batch improves from
17.96 to 11.32 us and distinct predicates from 15.64 to 8.89 us. Allocation counts
are unchanged: distinct scalar batches with reused document/destination still
use four allocations/384 bytes, while simple64 uses 68 allocations/1281 bytes
without preparation. Strings, numeric and container results still have their
normal ownership/conversion costs.

These are short development samples on a shared machine, not statistical
significance or a throughput promise. Larger input sizes grow an unused padding
string, not collection cardinality. gnata's stock byte fast path assumes valid
input; our preparation validates and owns it. The raw data also include gnata
with explicit `json.Valid`, fresh result destinations and document reuse.

All 420 samples completed successfully: 84 rows with exactly five samples each.
Production hashes were unchanged before/after the run: prior
`2e2ec16115470059b5a6f33238eee0d73708cbe7ec9d13e491629149e18e3098`, updated
`7680ae2b9c690ab5837a886e69210730d4c289f17d096cf42bd6c16e45d08ca5`.
A short follow-up CPU profile confirms wall-clock reads are no longer a leading
batch hotspot; monotonic clock checks, budget setup and result conversion remain
visible. Profile percentages are diagnostic and separate from the timing table.

[Full medians and allocation table](../benchmarks/comparison/testdata/results/deadline/summary.md),
[all medians and ranges](../benchmarks/comparison/testdata/results/deadline/medians.csv),
[profile summary](../benchmarks/comparison/testdata/results/deadline/profile-after.txt),
and [baseline restoration patch](../benchmarks/comparison/testdata/results/deadline/restore-before.patch)
are saved with raw samples and hashes. Run `python3 run-deadline.py` from
`benchmarks/comparison`; its README explains reconstructing the prior source.

## Compiled batches before deadline optimization (historical)

Compiled batches reduce repeated setup and result copying. Compact document
arenas reduce preparation allocations, and broader native JSONata array plans
remove interpreter fallback from the mixed fixture. Against the saved prior
runtime, the matched 1 KiB simple prepare-inclusive medians improved by
17–28%; the mixed fixture improved by approximately 8–50× across the measured
matrix. These are fixture results from a development tree, with no general
throughput guarantee.

Measured on 2026-11-02 with Go 1.25.13, darwin/arm64, Apple M5 Max, default
GOMAXPROCS=18 and production safety defaults. Every row has five **200 ms**
samples; numbers below are medians in microseconds per whole document/rule set.
The full matrix contains 1/8/64 KiB inputs, simple/mixed/unique workloads and
4/16/64 rules, with 1,350 timed samples in total. CPU-intensive implementation
and verification work stopped before the sequential runs. The machine remained
a shared development machine; these are short samples, not an isolated release
benchmark or a claim of statistical significance.

| 1 KiB simple workload | 4 rules | 16 rules | 64 rules |
| --- | ---: | ---: | ---: |
| Prior PrepareJSON + independent EvalPrepared loop | 6.87 | 11.81 | 32.11 |
| Current prepare + compiled batch, reused destination | 5.71 | 9.24 | 23.01 |
| Current prepare + compiled batch, fresh destination | 5.75 | 9.35 | 23.34 |
| Current reused document + reused destination | 1.52 | 4.95 | 18.23 |
| gnata warmed EvalMany | 3.05 | 4.74 | 11.11 |
| json.Valid + gnata warmed EvalMany | 4.38 | 6.22 | 12.68 |

The current prepare-inclusive ratios to warmed gnata are **1.88×, 1.95× and
2.07×**. The 64-rule simple case still misses the 2× target. gnata's stock byte
fast path assumes valid input; `json.Valid` adds validation but does not give
it owned input. Preparation validates and owns the source; this library returns
detached results. Compilation, schema warming, expected-result checks and
reusable-destination allocation are outside timers.

| 1 KiB simple preparation + evaluation | Prior bytes / allocations | Current fresh destination | Current reused destination |
| --- | ---: | ---: | ---: |
| 4 rules | 18,232 / 52 | 11,636 / 34 | 11,540 / 33 |
| 16 rules | 19,528 / 77 | 12,126 / 43 | 11,741 / 42 |
| 64 rules | 24,856 / 191 | 14,066 / 91 | 12,272 / 90 |

The `unique` fixture contains distinct `Account.Count > i` predicates, with
independent expected results `4 > i`. It shares path prefixes without repeating
whole expressions. Its reused-document/reused-destination batch uses **384 bytes
and four allocations** for each of 4, 16 and 64 rules; a fresh destination adds
one allocation (480/768/2,176 bytes total). The saved prior prepared loop used
6,656 bytes and 128 allocations for the matching 64-rule document-reuse case;
including preparation, that case drops from 167 to 26 allocations. This is a bounded scalar fixture,
not a zero-allocation claim for every workload. For example, the reused-document
1 KiB simple/mixed 16-rule batches allocate 752 bytes/20 objects and 3,588
bytes/120 objects respectively; detached string, array and numeric results have
additional costs.

| Input and workload | Rules | Prior prepare + loop | Current prepare + batch, reused dst | Warmed gnata |
| --- | ---: | ---: | ---: | ---: |
| 1 KiB mixed | 16 | 127.61 | 14.03 | 9.03 |
| 1 KiB unique | 64 | 34.44 | 20.45 | 11.37 |
| 8 KiB simple | 16 | 17.08 | 13.60 | 11.34 |
| 8 KiB mixed | 16 | 284.05 | 18.51 | 17.31 |
| 8 KiB unique | 16 | 17.20 | 13.11 | 8.40 |
| 64 KiB simple | 16 | 52.91 | 48.45 | 64.42 |
| 64 KiB mixed | 16 | 1,372.40 | 53.83 | 83.75 |
| 64 KiB unique | 16 | 53.70 | 46.79 | 34.42 |

These larger inputs grow an unused padding string while keeping the same
account/product structure. They expose scanning/preparation costs, not growth
in array cardinality or query complexity. The mixed fixture includes gnata's
four published expressions. Changes combine batch dispatch, arena storage and
native array plans; these measurements do not isolate each change's speedup.

| Input | Prior preparation µs | Current preparation µs | Prior bytes / allocations | Current bytes / allocations |
| --- | ---: | ---: | ---: | ---: |
| 1 KiB | 5.25 | 4.65 | 17,752 / 39 | 10,976 / 22 |
| 8 KiB | 10.18 | 8.67 | 24,920 / 39 | 18,144 / 22 |
| 64 KiB | 45.80 | 43.28 | 82,264 / 39 | 75,488 / 22 |

The preparation-only rows use simple/16; all rule sets receive identical input.
All result paths are checked independently before timing, including missing
versus null, mixed-engine result shapes, native error codes/positions and failed
batch destination cleanup. The prior runtime comes from
`/private/tmp/jsontrans-before-batch` through an alternate module-file replacement;
it runs the same fixture/checking harness and typed result sinks. A separate
`batch` build tag keeps new API references out of the prior-source build.
Typed sinks exclude artificial per-result interface boxing present in some
historical harness runs below, so their allocation counts are not directly
interchangeable with this matched comparison.

Reproduce from `benchmarks/comparison` with `sh run-batch.sh`.
[Full matrix medians](../benchmarks/comparison/testdata/results/batch/summary.md),
[machine-readable medians and ranges](../benchmarks/comparison/testdata/results/batch/medians.csv),
[prior raw samples](../benchmarks/comparison/testdata/results/batch/before-prepared.txt),
[current batch and gnata samples](../benchmarks/comparison/testdata/results/batch/current-batch.txt),
and [current preparation samples](../benchmarks/comparison/testdata/results/batch/current-prepare.txt)
are retained with the harness. It pins gnata v0.6.1 and its license/provenance.
No timing gate is imposed in CI.

SHA-256 of sorted production Go paths and contents (excluding tests,
benchmarks and examples) was
`2e2ec16115470059b5a6f33238eee0d73708cbe7ec9d13e491629149e18e3098`
both before and after all final samples. The saved prior-source hash was
`82b28d2f5a21c95f9eec69604dfb6e7e7588f9cad7f68691c031616dea568932`.
The earlier investigations and short profiling runs are excluded from this
final evidence.

## Initial ordinary-evaluation baseline (historical)

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

## Earlier prepared-loop measurements (historical)

The following section records the earlier prepared-loop implementation, before
compiled batches, compact document arenas and broader native array plans. Its
ratios and fallback counts do not describe the current implementation.

The isolated `benchmarks/comparison` module compares compiled JSONata programs
with gnata v0.6.1. It verifies independently specified results before timing,
including undefined and null. It uses exact-size documents and batches of
repeated fixed rules; the mixed set includes gnata's four published benchmark
expressions. Compilation is excluded. All production safety defaults remain
active. See that module's README and `testdata/results/` for commands, input
details, provenance and raw samples.

A local measurement on 2026-11-02, Go 1.25.13, macOS arm64, Apple M5
Max, used five 200 ms samples per primary row. Values are median
microseconds per document. The machine was shared, and the measured source was
a development tree; this is not a release benchmark. The SHA-256 of sorted
production Go paths and contents (excluding tests and benchmarks) was
`e98ce9516c92e035646bafc47a59627f5896b7b0036f04594f96b1f0f5b1a4e3`
both before and after the primary run.

| 1 KiB simple batch | 4 rules | 16 rules | 64 rules |
| --- | ---: | ---: | ---: |
| PrepareJSON plus EvalPrepared for every rule | 6.71 | 11.56 | 32.61 |
| gnata warmed EvalMany | 2.92 | 4.66 | 10.96 |
| json.Valid plus gnata warmed EvalMany | 4.19 | 6.04 | 12.29 |

The measured prepare-inclusive ratios to warmed gnata are 2.29×, 2.48× and
2.97×, so that earlier implementation did not meet the 2× performance target. The stock gnata byte
fast path assumes valid input, whereas preparation validates and owns a copy.
The `json.Valid` row includes validation but does not give gnata owned input.
Results are detached by this library. Both sets of measurements use the same
input and rules, with gnata's schema plan warmed before timing.

Five shorter 100 ms samples at 16 rules, taken before final correctness fixes,
give the following provisional wider context
(median microseconds per document):

| Input | Rules | Prepare + evaluate | gnata EvalMany | gnata + validation |
| --- | --- | ---: | ---: | ---: |
| 1 KiB | mixed | 130.9 | 8.86 | 10.51 |
| 8 KiB | simple | 20.6 | 11.34 | 23.57 |
| 8 KiB | mixed | 295.7 | 17.07 | 29.90 |
| 64 KiB | simple | 59.5 | 62.32 | 162.61 |
| 64 KiB | mixed | 1,445.9 | 80.25 | 181.53 |

Mixed rules include expressions that use the interpreter fallback; their cost
is sensitive to document size. By inspection of the plans and fixture shapes,
all 16 simple rules use the fast path and 10 of the 16 mixed rules do; the
remaining six mixed rules fall
back on array traversal or unsupported expressions. This is an inference, not
instrumented coverage. For the simple 16-rule batches, prepared-document reuse
without preparation measured roughly 8–9 µs, and preparation alone roughly 4.5, 10.5
and 50.7 µs for 1, 8 and 64 KiB respectively. The primary prepare-inclusive
rows allocated 18,328 / 19,912 / 26,392 bytes and 56 / 93 / 255 allocations
per document. Approximate retained live heap for 128 prepared documents after a
GC was 10.4 / 17.5 / 73.9 kB per document for 1 / 8 / 64 KiB input; this
measurement includes allocator and GC noise. Parallel results and allocation
details are in the raw output. No timing gate is imposed in CI.

In the same provisional 16-rule parallel reuse samples, the reported time per
operation was 2.03 µs for simple rules and 38.45 µs for mixed rules, compared
with 1.98 µs and 3.05 µs for gnata. Parallel ns/op reflects aggregate throughput
across workers, not individual-request latency. A separate comparison with the
archived baseline showed no greater-than-10% regression on the existing mixed
1 KiB ordinary-Eval workload; this is one fixture, not a general regression
claim. The N=64 allocation profile points primarily to document scanner allocation
and fast-budget allocation as opportunities for further work.

## Go 1.27 JSON boundary experiment

The [Go 1.27 assessment](assessments/go127-json.md) compares both standard-library
backends and an opt-in jsontext decoder. It separates JSON-boundary costs from
prepared-document evaluation, with commands, raw results and allocation ranges.
The experiment does not change the library's public API or minimum Go version.

The [message-processing workload](nats-workload.md) records both one-program and
multi-program routing for fresh NATS messages, including compilation reuse,
output encoding and concurrent-worker measurement requirements.

The [Linux reproduction](assessments/go127-json-linux.md) confirms JSON decoding
gains on Bertram and records the smaller prepared-route regression, rather than
treating the Go upgrade as a universal speedup.
