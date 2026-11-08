# Prepared-result encoding on Linux, 2026-11-08

Prepared JSON-result calls reuse the exact encoded size established during
result validation and detachment. The encoder checks traversal limits without
cloning containers, normalizing strings or measuring scalar encodings a second
time. It still marshals to independently owned bytes. External adapters retain
their validation/detachment boundary. Public APIs, safety defaults, minimum Go
version and JavaScript guest behavior are unchanged.

This is a focused optimization, not a zero-allocation implementation. The
single-rule workload remains important, but preparation dominates its measured
cost. The clearest gains are in scalar fan-out.

## Measured results

Bertram: Ubuntu 24.04, linux/amd64, Intel Core i9-13900K, 128 GiB RAM. Native
benchmarks use CPU 8, GOMAXPROCS=1 and production limits. The primary comparison
uses Go 1.27.3 with GOEXPERIMENT empty; a separate Go 1.26.10 control covers the
minimum supported Go version. No host frequency or service settings changed.

These are medians per complete incoming message, including preparation,
evaluation, conversion and byte encoding. Rules are compiled outside timing.
For each workload, eight varied documents cycle through a worker-local result
destination. The harness checks every strategy against independent expected
JSON before starting its timer. The input-size column is a target minimum;
actual lengths and expressions are in [the corpus inventory](corpus-inventory.json).

Go 1.27.3, target 1 KiB, public batch API:

| Workload | Rules | Baseline µs/message | Candidate µs/message | Time reduction | Allocations before → after | Bytes allocated before → after |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| JSONata scalar | 16 | 20.66 | 18.16 | 12.1% | 216 → 153 | 17,659 → 17,149 |
| JSONPath scalar | 16 | 25.60 | 22.09 | 13.7% | 295 → 231 | 19,905 → 18,625 |
| JSONata object | 16 | 339.78 | 330.77 | 2.7% | 6,620 → 6,508 | 370,527 → 364,360 |
| JSONPath object | 16 | 228.16 | 214.05 | 6.2% | 5,725 → 5,613 | 343,167 → 336,237 |

For a single rule through `EvalPreparedJSON`, the same scalar cases are
9.47 → 9.39 µs (JSONata) and 9.61 → 9.37 µs (JSONPath); object cases are
36.27 → 35.48 µs and 29.16 → 28.48 µs. These roughly 1–2.5% changes are small
relative to shared-machine variation. The corresponding allocation reductions
are three, four, seven and seven allocations per message.

At target 64 KiB, scalar batch16 takes 91.22 → 88.73 µs for JSONata and
93.92 → 89.76 µs for JSONPath (2.7% and 4.4% less time). Object batch16 improves
about 2%; single-rule cases are essentially unchanged. Larger sizes grow unused
padding, not collection cardinality. This is not evidence for arbitrary large
collections or parallel routing throughput.

The Go 1.26.10 control also improves scalar batch16: 18.12 → 16.85 µs for
JSONata and 22.71 → 18.85 µs for JSONPath (7.0% and 17.0%). Object batch16 takes
3.6% and 4.2% less time. Different Go versions have different marshal costs;
compare before/after within each toolchain rather than across toolchains.

## Method and profiling evidence

The primary matrix contains 48 cases: two engines, two result families, two
input sizes, two rule counts and three strategies. Five paired rounds alternate
baseline/candidate order; strategy order rotates independently. Each benchmark
runs for 300 ms. All 480 rows passed. The Go 1.26 control uses 24 cases at target
1 KiB, three paired 200 ms rounds, and passed all 144 rows.

The unchanged `direct` strategy (`EvalJSON`) is an environmental control. Its
median shifts range from -1.8% to +0.6%. Allocation counts are mostly unchanged;
the JSONPath scalar 64-KiB/16-rule control varies from 14,095 to 14,150
allocations/message despite its unchanged code path. Retained samples show
this background variation.
No confidence interval, significance claim or universal throughput promise is
inferred from these short local measurements. Full min/max sample ranges and
all strategies, including controls, are retained in
[Go 1.27 medians](medians-go127.csv), [Go 1.26 medians](medians-go126.csv), and the
raw `baseline`/`candidate` round files. B/op is allocated bytes, not peak RSS or
live heap. These tests do not measure a long-running subscriber's GC behavior.

CPU and allocation profiles run separately from timing samples. Each profile
isolates one exact benchmark: target 1 KiB, either N1/prepared or N16/batch,
for each engine/result family. Every slash-delimited selector is anchored so
N1 cannot also match N16. CPU sampling uses 2-second runs. Allocation profiling
uses a separate 1-second run with memprofilerate=1. Those instrumented ns/op
values must not be used for performance comparisons. Allocation profiles also
include setup; their total MiB and percentages are not comparable per-operation
measurements. Use the uninstrumented B/op and allocs/op tables for that purpose.

In the baseline scalar JSONata batch16 CPU profile, `jsonvalue.Encode` accounts
for 23.9% cumulative CPU; cloning alone accounts for about 10.5%. The candidate
encoder accounts for 8.6%, and its allocation call-site profile contains only
the final marshal, without the removed clone. These sampled shares diagnose
where work moved; they are not standalone latency estimates. Object JSONata
batch16 instead spends about half its baseline CPU in document materialization,
which explains the smaller whole-message gain. JSONPath object evaluation also
materializes values before encoding. Reducing that materialization is a separate
future experiment.

Generated text has trailing whitespace and extra terminal blank lines trimmed;
CSV summaries use LF line endings. Benchmark values are unchanged.
Profile summaries retain CPU, alloc_space, alloc_objects, encoder call sites
and raw instrumented benchmark output in each version directory. Raw profiler
binaries stay outside Git on Bertram and can be regenerated with the script.

## Reproduction and source identity

Baseline: `b21df4f8141c69e65bfc1a04c51cb4052e735fb2` (merged main).
Candidate: that exact source plus [candidate.patch](candidate.patch), containing
only the production change and its regression/fuzz tests. Documentation is
outside the measured source delta. The fixed binaries, source and original raw
profiles remain under `/data/ci/jsontrans-encoding-20261108` on Bertram.

Original transferred archive SHA-256:

- Baseline: `d4f685e2a47a70ebe2654bf276324a3415cf46aad7cf23bc1d2abc4ee6450b90`
- Candidate: `0ec90c7c506017b3f981d617f4c5816d63bcdb7feb41f4068bc792d00631d0aa`

[Build records](build-info.txt) identify both Go versions and benchmark binary
hashes. [Environment](environment.txt) records the measured host. Source archives
include unrelated repository content; the patch is the reproducible code delta.

With cached module dependencies, absolute paths to both Go toolchains and CPU 8
available on Linux:

```sh
bash docs/benchmarks/prepared-encoding-linux-2026-11-08/reproduce.sh \
  "$PWD" /absolute/path/to/go1.27.3/bin/go /absolute/path/to/go1.26.10/bin/go \
  /tmp/jsontrans-encoding-reproduction
python3 -B docs/benchmarks/prepared-encoding-linux-2026-11-08/analyze.py \
  /tmp/jsontrans-encoding-reproduction
```

The reproduction script builds from the pinned archive plus patch, performs the
same alternating timing matrix and generates fresh exact-selector profiles.
Source reconstruction was checked with `git apply --check`; the script also
passes `bash -n`. The recorded experiment originally collected baseline profiles
before implementation, then collected candidate profiles and the Go 1.26 control
after the primary paired matrix. No profiling ran concurrently with timings.

## Verification

The [verification logs](verification/) record:

- Core tests, race detection and vet on Linux with Go 1.26.10 and 1.27.3, including
  pinned conformance tests; core race/vet on macOS with Go 1.26.10 also pass.
- Optional JavaScript, assessment and native-message modules: tests, race and vet
  against the modified core through a private workspace on macOS.
- Pinned Node JSONata reference comparisons on macOS.
- 20,000 requested encoder fuzz executions, 5,000 prepared-result executions and
  5,000 message-strategy executions, all passing (coordinator totals can exceed
  the requested count slightly).
- NATS harness tests, race and vet on Linux; Docker/Testcontainers integration
  checks across direct/prepared/batch strategies, fan-out 1/4/16 and four workers.
  All nine route cases validate 256 complete, lossless messages apiece. Container
  cleanup passes. This validates transport correctness, not new end-to-end p99 or
  production-capacity numbers; the earlier load matrix was not repeated here.

Formatting and source/documentation whitespace checks pass (the reproduction
patch is excluded because unified-diff context has intentional whitespace). Regression tests cover independent output
storage, numbers, escaping, lone surrogates, collection/depth/step/byte limits,
and cancellation during long-string/large-array encoding traversal. Existing
prepared-result tests cover adapter rejection, aggregate escaped-byte limits,
missing/null, diagnostics and concurrent reuse. Standard-library marshaling
remains cooperatively bounded rather than forcibly interruptible; exact work
counter thresholds can change as redundant work is removed.
