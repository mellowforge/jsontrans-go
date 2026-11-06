# Go 1.27 JSON performance assessment

Date: 2026-11-06. Scope: native JSON parsing/conversion boundaries and their
effect on JSONPath, JSONata, prepared batches and the optional JavaScript
processor. This is an experiment; no production backend, minimum Go version,
public API, prepared index or JavaScript guest is changed.

## Method

Configurations: Go 1.26.10; Go 1.27.3 with `GOEXPERIMENT=nojsonv2`;
Go 1.27.3 with its default JSON backend; and Go 1.27.3 with the opt-in
`jsontext_experiment` decoder. This separates toolchain effects from the
standard JSON-backend change and our explicit parser migration.
Go 1.27 backs the existing encoding/json API with the new implementation;
changing go.mod or replacing every import is unnecessary to measure that
benefit. See the [Go 1.27 release notes](https://go.dev/doc/go1.27).

Host: macOS arm64, Apple M5 Max. Six samples per configuration use 150 ms
calibrated benchmark windows and one Go execution thread. Configuration order
rotates across rounds. Compilation is outside measured processes; test and
build jobs are not run concurrently with performance measurements. These are
warm local descriptive measurements, not cold-start results, statistical
significance claims, throughput promises or cross-platform claims.

[Commands and fixture details](../../experiments/go127json/README.md),
[raw output and environment](../benchmarks/go127-json-2026-11-06/metadata.json),
[complete latency/allocation ranges](../benchmarks/go127-json-2026-11-06/summary.csv),
and [selected medians](../benchmarks/go127-json-2026-11-06/summary.md) are retained.
Fixtures contain exactly 1/8/64 KiB with eight items plus padding, and 1 MiB
with 18,724 items plus padding. Large strings and many small tokens exercise
different costs and must not be conflated.

Prepared batches reuse compiled expressions and destination storage. Native
1/16/64-rule batches use singular selections and a native sum projection; the
mixed 16-rule batch instead includes two uppercase interpreter fallbacks.
Each operation evaluates the whole set. Prepared results are checked against
ordinary Go-value evaluation before timing. The existing JavaScript benchmark
uses a tiny two-item input, so it does not establish large-payload conversion
performance. Its engine/compiled artifacts are reused, with a fresh guest per
evaluation.

## Compatibility and controls

Only internal materializing decoding changes in the prototype. It retains
legacy Clone validation, number spelling, duplicate-name last-write behavior
and first insertion position, object-order provenance, lossless UTF-16 escapes,
byte/item/depth/work limits and context error identities. This is not a one-pass
JSONata or JSONPath evaluator: sorting, grouping, joins and arbitrary document
references can require retained state even with a streaming syntax reader.

Borrowed jsontext tokens are never retained across decoder operations.
Returned strings and numbers own storage. Prepared docs keep their existing
owned bytes and index. AllowInvalidUTF8 is used only to admit surrogate escapes:
raw invalid UTF-8 is rejected before parsing, while raw string spans use the
existing lossless decoder. See [jsontext documentation](https://pkg.go.dev/encoding/json/jsontext@go1.27.3).

Cancellation is cooperative between tokens and during final validation. As in
the legacy path, an individual standard-library token scan and initial UTF-8
validation are bounded by input size but are not preemptible. This experiment
does not promise a stronger execution profile or hard process-memory cap.
The optional JavaScript execution contract and guest remain unchanged.

## Results and recommendation

Median microseconds per operation (whole batch for batch rows):

| Workload | Go 1.26 | Go 1.27 | Go 1.27 + jsontext |
|---|---:|---:|---:|
| 1 KiB decode | 20.95 | 12.76 | 11.03 |
| 64 KiB decode | 451.53 | 307.68 | 270.54 |
| 1 MiB array decode | 38,397.70 | 21,628.85 | 18,181.52 |
| 64 KiB JSONata EvalJSON | 671.01 | 500.09 | 471.55 |
| 64 KiB JSONPath EvalJSON | 661.04 | 495.24 | 478.45 |
| 64 KiB prepare | 40.99 | 41.96 | 43.84 |
| 64 KiB reused batch, 16 rules | 6.05 | 6.14 | 6.06 |
| 64 KiB reused batch, 64 rules | 23.60 | 23.95 | 23.78 |
| 64 KiB mixed reused batch, 16 rules | 28.79 | 29.63 | 29.51 |

The default Go 1.27 backend reduced 64 KiB decode latency by about 32% and
the 1 MiB array decode by about 44% versus Go 1.26. The explicit prototype
reduced those latencies by about 40% and 53% respectively versus Go 1.26,
or about 12% and 16% beyond Go 1.27's default decoder. For the 64 KiB
JSONata end-to-end call the corresponding medians are 671/500/472 us:
additional decoder gains are diluted by normalization, evaluation and output.

Allocation changes are less ambiguous than small timing differences. At
64 KiB, decode changed from 471,784 B/op and 491 allocations on Go 1.26, to
337,738 B/op and 253 on Go 1.27, and 271,198 B/op and 173 with the prototype.
For the 1 MiB array, counts were 1,029,890 / 486,888 / 318,364 allocations.
The prototype is materializing and remains far from allocation-free.

Native prepared reuse did not gain from replacing the materializing decoder.
The 16-rule batch stays near 6 us and the 64-rule batch near 24 us. The
mixed 16-rule batch with two interpreter fallbacks is near 29 us, which
points toward materialization/fast-plan coverage as a useful profiling target.
These are different rule sets, not a measured speedup from implementing a new
fast path. Preparation itself is about 41–44 us in this run; the prototype
does not replace that scanner. The full-suite prepare-and-batch timing varied
more than expected; see the isolated longer-window control below.

The tiny JavaScript fixture changed only modestly: EvalJSON medians were
775/756/738 us across Go 1.26/default 1.27/prototype. This does not justify a
guest/runtime change, and requires larger payload tests before generalizing.

Whole-suite peak RSS ranges overlap substantially. Native processes spanned
roughly 120–191 MiB; JavaScript processes roughly 163–209 MiB. These are
high-water marks for complete benchmark processes, not a per-document memory
cost or proof of a lower runtime memory requirement.

Recommendation:

1. Prefer Go 1.27 application builds to capture the standard-library gains.
   Keep the Go 1.26 minimum unchanged during this experiment; raising go.mod
   alone is not a performance optimization.
2. Keep jsontext opt-in until Linux measurements confirm the tradeoff and
   cancellation/compatibility review is complete for a production switch.
   A conditional decoder migration is sufficient; no engine rewrite is needed.
3. For many expressions per prepared document, profile materialization and
   extend native fast-plan coverage before replacing the existing index.
4. Treat reader/writer or NDJSON record streaming as a separate API design.
   This experiment accepts complete byte slices and produces owned results.
   It makes no claim of constant-memory whole-document JSONata evaluation.


## Isolated preparation control

The full-suite prototype prepare-and-batch median was 53.01 us versus
47.57 us on default Go 1.27. To check that unexpected difference, a separate
six-round run isolated the same 64 KiB/16-rule workload with 1-second windows.
Medians were 46.80 us on Go 1.26, 46.78 us on Go 1.27/nojsonv2, 46.53 us on
default Go 1.27, and 46.21 us with the prototype. The slowdown did not reproduce.
We do not identify its exact cause or attribute the short-window result to a
parser regression; no materializing decoder is used in this prepared workload.

The [control results](../benchmarks/go127-json-prepared-control-2026-11-06/summary.csv)
and [control environment/commands](../benchmarks/go127-json-prepared-control-2026-11-06/metadata.json)
are separate from the full-suite dataset. Both runs match the reviewed Go-source
and module-manifest SHA-256. This control supports treating prepared processing
as unchanged, rather than claiming the parser improves it.

## Verification

All 21 matrix checks passed on macOS arm64. Root tests, race detection and vet
ran on all four configurations. The Go 1.26 baseline and tagged prototype also
passed the complete optional JavaScript test/race/vet suites. Root tests include
pinned native JSONPath and JSONata conformance fixtures; no Node oracle job was
run for this experiment.

The final prototype passed bounded differential decoder fuzzing (15,000-case
budget), prepared-document differential fuzzing (27,000), and JavaScript JSON
conversion fuzzing (500). Dedicated tests cover exact numbers, Unicode and
surrogates, duplicate names/order, invalid input, ownership, concurrent calls,
cancellation and resource limits. Static review identified malformed closing
brackets under tight budgets; a failing regression was observed and fixed,
then affected checks were repeated. Review confirmed the fix without further
findings. Formatting and diff checks are clean; Python summary tests pass.

The [verification manifest](../benchmarks/go127-json-2026-11-06/verification.json)
records commands and exit codes. No native Linux or Windows measurements/checks
were added in this experiment. The existing JavaScript runtime/guest was not
changed or rebuilt.
