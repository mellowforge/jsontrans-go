# Fresh-message byte-to-byte experiment

Development-only module. Production JSON APIs and backends are unchanged.

`Plan` compiles subject-specific transformations once. Each `Worker.Eval` accepts
fresh JSON bytes and returns owned JSON output bytes for every defined result:

- `direct`: independent precompiled EvalJSON calls.
- `prepared`: one PrepareJSON, individual EvalPrepared calls, bounded encoding.
- `batch`: one PrepareJSON, Batch.EvalPrepared, bounded encoding of every result.

All variants have a common per-message deadline and aggregate encoded-output cap,
plus the engines' existing per-expression limits. Actual work counters differ
between strategies. The adapter is an experiment, not a proposed production API.
Failures return no outputs; missing and null remain distinct. Transform atomicity
does not imply transactional delivery of multiple NATS publications.

Compiled plans are immutable and shared; each worker owns scratch/destinations.
The returned result slice is reused at the worker's next Eval. JSON byte buffers
own storage and are not overwritten. Close plans after workers finish. No worker
or JavaScript guest pooling changes are made. Intermediate Go results are cleared
after encoding. JavaScript initialization/compilation gets a 30-second setup
budget for cold/race builds; per-evaluation timeout is restored to one second
through a supported option, with all other execution controls unchanged.

## Corpus and correctness

Eight deterministic inputs vary IDs, item counts, Unicode and JSON escaping.
Families: scalar selection, object construction/selection, explicit array
filtering, interpreter/function/union fallback, bindings, configured limits,
registered host functions and whole-document output. Single-message fan-out is
1/4/16. Scalar/object expressions are distinct at N16; whole-document output is
intentionally repeated, and JSONPath fallback unions reuse four patterns.
JSONPath has no bindings, so that inapplicable benchmark is explicitly absent.
Engines have different result shapes and sometimes different expressions;
compare strategies within a workload, not engine-versus-engine throughput.

Sizes 512/1024/8192/65536 are minimum targets, not exact byte counts: metadata and
object fixtures can exceed them, and escaping varies the encoded length. The
selected 1MiB target array case contains many small values. inventory.json records
actual input/output bytes, exact expressions and corpus hashes. Expected outputs
are calculated from fixture data independently of the evaluator. Tests retain
exact numbers, UTF-16 surrogates, missing/null, duplicate names, native errors,
input ownership, cancellation, aggregate output bounds and concurrent reuse.

Compilation, expected-value generation and eight-sample correctness preflight
are outside benchmark timers. Every timed operation includes fresh preparation
where applicable, conversion and output encoding. Sequential benchmark result
sinks are typed; parallel workers use KeepAlive. Strategy order rotates across
six process rounds. These are calibrated warm measurements, not cold starts.

## Run on Bertram

Use installed Go1.26.10 and Go1.27.3 binaries with cached dependencies:

```sh
go test ./...
go test -race ./...
go vet ./...
go test -run '^$' -fuzz '^FuzzByteStrategies$' -fuzztime=12000x -parallel=2
python3 -m unittest discover -p 'test_*.py'
python3 run.py --go126 "$GO126" --go127 "$GO127" \
  --output /data/ci/message-results --rounds 6 --benchtime 75ms
python3 summarize.py /data/ci/message-results
```

The runner is Linux/Bertram-specific: sequential profiles use logical CPU8;
parallel profiles allow one hardware thread from each of the eight performance
cores (0,2,4,6,8,10,12,14), with GOMAXPROCS/-cpu 1/2/4/8. No system policy or
frequency setting changes. Check affinities before running on another host.
No tests or other benchmark configurations should execute concurrently.

Primary Go1.27 profiles cover both native engines, 1/64KiB targets and 1/4/16
fan-out; additional profiles cover 512B/8KiB selectors, 1MiB arrays, JavaScript
object baseline, selected Go1.26 controls and selected tagged-jsontext controls.
Parallel profiles route across eight independently compiled plans through an
immutable subject map; each goroutine owns eight workers. They measure aggregate
wall-time/message and allocation costs at each CPU count, not individual latency
or p95/p99. Inputs cycle through a deterministic corpus; this is not a production
traffic model. JS retains a fresh guest per evaluation and is measured separately.

Raw outputs, command exit codes, Go/module/embedded-Wasm source hash, toolchains,
CPU affinity, corpus inventory and median/min/max allocations/latencies are
retained. RSS is a whole-benchmark-process high-water mark, not per message or a
hard memory guarantee. No NATS broker is involved in these native profiles.

A later nested `nats` module owns Testcontainers/client dependencies and the real
message transport/load harness. It is deliberately separate from native timing
and from the production library's module graph.
