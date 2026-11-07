# Go 1.27 JSON boundary experiment

This experiment compares the unchanged decoder with Go 1.26.10, Go 1.27.3's
legacy JSON backend (`GOEXPERIMENT=nojsonv2`), Go 1.27.3's default JSON backend,
and an opt-in `jsontext` materializing decoder on Go 1.27.3.
The root module still supports Go 1.26; the prepared index, public API,
JSONata/JSONPath engines and JavaScript guest are unchanged.

## Reproduce

Use installed native toolchain binaries. The runner sets `GOTOOLCHAIN=local`,
`GOWORK=off`, clears `GOFLAGS`, and explicitly selects `GOEXPERIMENT` for each
configuration. Ordinary Go module dependencies must already be cached when
running offline. Python 3 and the native `/usr/bin/time` utility are used;
RSS extraction supports macOS and Linux. Windows timing can run, but RSS is
reported unavailable. No Node or comparison engines are required.

```sh
python3 experiments/go127json/run.py \
  --go126 "$GO126" --go127 "$GO127" \
  --output /tmp/jsontrans-go127-results --count 6 --benchtime 150ms
python3 experiments/go127json/summarize.py /tmp/jsontrans-go127-results
python3 -m unittest discover -s experiments/go127json -p 'test_*.py'
```

`GO126` and `GO127` are absolute paths to the chosen toolchain's `bin/go`.
The output directory must be empty. `--skip-javascript` omits the optional
module. Test binaries are built once before measurement. Configurations run
sequentially with rotating order, `GOMAXPROCS=1` and `-test.cpu=1`; no test or
build jobs should run concurrently. Results include six warm, calibrated
samples, not cold startup latency. There is no CPU affinity/frequency lock.

Raw benchmark output, whole-process peak RSS, toolchain/environment metadata,
source SHA-256, complete commands, medians and min/max ranges are retained.
The hash covers Go sources and module manifests, including untracked files;
`metadata.json` also records the base commit and working-tree status.
RSS includes the whole benchmark suite's setup, fixture allocations, runtime
and guest caches. It does not measure per-operation memory or retained docs.
The source tree must remain unchanged throughout measurement.

Fixtures contain exactly 1/8/64 KiB with eight small item objects plus padding,
and exactly 1 MiB with 18,724 item objects plus padding. Native measurements
cover bounded decode/encode, preparation, JSONata/JSONPath EvalJSON, identity
output, compiled batches of 1/16/64 native fast-path rules with reused result storage, and a
16-rule batch with two interpreter fallbacks. Batch latency is per whole set,
not per expression. The large array probes conversion only; it is not used
for the 64-rule batch. JavaScript's existing benchmark uses a tiny two-item
input and measures Eval, EvalJSON and prepared fallback with a fresh guest
per evaluation; the processor/compiled artifact are reused.

## Prototype and correctness

`-tags=jsontext_experiment` selects the new decoder only with Go 1.27 and
jsonv2 enabled. Other toolchains/disabled-jsonv2 configurations intentionally
fail to build with this tag rather than silently measuring the old backend.
Without the tag, the original decoder is used.

The prototype keeps the final Clone validation and work/size accounting.
Duplicate keys keep their last value and first insertion position. Raw string
spans use the existing lossless UTF-16 decoder; number spelling is copied
without a binary64 conversion. Borrowed jsontext tokens are never retained.
Input UTF-8 validation remains strict even though the parser option admitting
isolated surrogate escapes is enabled. Malformed input wraps ErrInvalid;
limits and cancellation keep their sentinel identities.

Cancellation is checked between tokens and in final conversion. As in the
existing decoder, an individual standard-library token scan and the initial
UTF-8 validation are bounded by the input-byte cap but are not preemptible.
This is a parsing-boundary experiment, not a new streaming evaluator, zero
allocation API, or hard process-memory guarantee.

Representative verification (repeat root tests/race/vet without the tag on
both toolchains and with `GOEXPERIMENT=nojsonv2` on Go 1.27):

```sh
"$GO127" test -tags=jsontext_experiment ./...
"$GO127" test -race -tags=jsontext_experiment ./...
"$GO127" vet -tags=jsontext_experiment ./...
"$GO127" test -tags=jsontext_experiment ./internal/jsonvalue -run '^$' \
  -fuzz '^FuzzJSONTextCompatibility$' -fuzztime=15000x -parallel=2
"$GO127" test -tags=jsontext_experiment ./internal/document -run '^$' \
  -fuzz '^FuzzPrepareAndMaterialize$' -fuzztime=27000x -parallel=2
"$GO127" -C javascript test -tags=jsontext_experiment ./...
"$GO127" -C javascript test -race -tags=jsontext_experiment ./...
"$GO127" -C javascript vet -tags=jsontext_experiment ./...
```

Set `GOTOOLCHAIN=local`, `GOEXPERIMENT=''` and a writable `GOCACHE` as needed.
The root suite includes pinned native conformance tests; full tagged tests
also cover missing/null, order provenance and bounded output behavior.
The dedicated decoder differential tests compare legacy/prototype success,
values, error classes, root object enumeration and exact step boundaries.

See [the assessment](../../docs/assessments/go127-json.md) and
[the recorded results](../../docs/benchmarks/go127-json-2026-11-06/summary.md).

A focused control isolates a single unchanged prepared workload with longer
windows (the report records this separately from the main suite):

```sh
python3 experiments/go127json/run.py \
  --go126 "$GO126" --go127 "$GO127" \
  --output /tmp/jsontrans-go127-prepared-control --count 6 --benchtime 1s \
  --skip-javascript \
  --native-pattern 'BenchmarkJSONAPI$/64KiB$/prepare_batch$/N16$'
python3 experiments/go127json/summarize.py /tmp/jsontrans-go127-prepared-control
```
