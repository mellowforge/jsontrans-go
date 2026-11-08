# Longer one-worker NATS tail investigation, 2026-11-08

The ten-second trials do **not** reproduce the earlier single-rule prepared or
four-rule batch p99 regressions. They do show a persistent higher p99 for four
separate prepared evaluations with one Go execution slot. A separate runtime-slot
control makes scheduling configuration a plausible contributor; it does not
identify a defective processor operation or justify an engine change.

## Primary results

Both source revisions pass all **72 measured cases**, checking 3,600,000 inputs
across six alternating pairs. There are zero transformation, transport, bad
output, pending-drop, unaccounted-input or incomplete-fan-out errors. All twelve
measured brokers are removed. Every prepared/batch case is lossless (48/48).
The twelve four-rule direct control cases overload their application queues and
are retained without end-to-end comparison deltas.

JSONata objects, one transform worker and GOMAXPROCS=1, target 5,000 inputs/sec,
50,000 inputs per ten-second case. Figures are medians of six per-run percentiles,
not pooled percentiles. Each displayed row is lossless at matched actual load in
all twelve observations:

| Rules | API | Processing p50 µs B → M | Queue p99 ms B → M | E2E p99 ms B → M | E2E change | Main p99 higher in paired runs |
| --- | --- | ---: | ---: | ---: | ---: | ---: |
| 1 | prepared | 53.00 → 39.45 | 1.07 → 0.22 | 5.51 → 1.71 | -68.9% | 0/6 |
| 1 | batch | 53.21 → 38.99 | 1.26 → 0.20 | 6.41 → 1.62 | -74.7% | 0/6 |
| 4 | prepared | 72.09 → 46.17 | 0.80 → 0.57 | 3.18 → 3.92 | +23.2% | 6/6 |
| 4 | batch | 71.45 → 43.92 | 0.78 → 0.45 | 3.12 → 3.07 | -1.4% | 3/6 |

B is pre-optimization baseline; M is merged main. The four-rule prepared p99
ranges do not overlap in this sample: **2.86–3.45 ms** before versus
**3.51–4.58 ms** after. Four-rule batch ranges overlap substantially:
2.83–4.02 versus 2.21–3.66 ms. Batch p95 improves 2.08 → 1.01 ms, despite its
roughly unchanged p99. For four prepared calls, p95 also improves 2.08 → 1.03 ms.
Percentiles are not additive; subtracting processing/queue p99 from end-to-end
p99 would not locate a residual phase for the same messages.

The single-rule direct control is slightly worse on main: median p99
3.28 → 3.53 ms, higher in five of six pairs. Its processing median is near
unchanged (97.97 → 99.17 µs). Four-rule direct queue rejection is similarly
large on both versions (182,843 versus 182,964 inputs across six runs).
These controls remain visible and caution against attributing every tail shift
solely to optimized evaluator code.

Client-pipeline allocation and GC costs fall on all public paths. For four
prepared calls, median bytes per input fall **121,169 → 40,404** and GC cycles
per ten-second case **893.5 → 303**. Lower aggregate allocation/GC counts alone
do not explain a higher or lower latency percentile.

All cases, loss counts, actual offered load, p50/p95/p99, p99 ranges, allocation
and GC counters are in [comparison.csv](comparison.csv) and
[comparison.json](comparison.json). [Paired observations](paired-observations.csv)
retain the individual comparisons rather than only their medians.

## Separate diagnostics

Six additional five-second execution traces cover each version at N1/prepared,
N4/prepared and N4/batch. These are **instrumented diagnostics**, not primary
latency samples. They include case setup and cleanup as well as publishing.
The original trace runner source and build hashes are retained under
`diagnostics/`; raw traces and binary pprof profiles stay outside Git on Bertram.

GC stop-the-world ranges are short in these traces. N4/prepared's largest GC
world stop is **25.9 µs** on baseline and **41.5 µs** on main; all six traces'
maxima are below 49 µs. These observations do not contain millisecond-scale
world-stop pauses, but cannot rule out concurrent GC work, assists or scheduler
competition. They also cannot establish what happened in an untraced primary
message. [GC summaries](diagnostics/main-4-prepared/gc-stw.json) are derived from
paired Go 1.27 `RangeBegin`/`RangeEnd` events, rejecting incomplete ranges.

The N4/prepared scheduler-delay profile falls overall (about 2.66 → 1.44 seconds
of cumulative goroutine runnable delay), while NATS publish and receive wakeup
stacks remain prominent. Such profiles sum delay across goroutines; they are
not message latency and are not an attribution of the p99 regression. See
[baseline](diagnostics/baseline-4-prepared/sched-top.txt) and
[main](diagnostics/main-4-prepared/sched-top.txt) scheduler summaries, alongside
network and synchronization profiles.

### One worker versus one Go execution slot

A separate, untraced control keeps one transform worker and compares GOMAXPROCS
1 versus 2. It uses three five-second repeats per condition, 25,000 inputs,
N4/prepared, the same target rate, queue, source revisions, Go version and broker.
Only the development harness gains an explicit `ClientProcs` diagnostic override;
zero retains its existing Workers-based default. No library scheduler policy is
changed. The original primary/trace source copies and binaries remain untouched.

| Version | Go slots | Lossless repeats | Processing p50 µs | E2E p99 ms | Per-run p99 range ms |
| --- | ---: | ---: | ---: | ---: | ---: |
| Baseline | 1 | 3/3 | 72.17 | 3.33 | 3.26–3.75 |
| Baseline | 2 | 2/3 | 133.76 | 4.97 | 3.08–7.90 |
| Main | 1 | 3/3 | 44.91 | 4.94 | 3.51–5.38 |
| Main | 2 | 3/3 | 74.04 | 1.38 | 1.37–1.44 |

Main's one-slot/two-slot actual rates match and all six cases are lossless.
Its p99 changes substantially with runtime width despite increased measured
transform wall time. This supports **configuration sensitivity in the shared
client pipeline**, not a conclusion that more runtime slots always help. The
baseline two-slot condition rejects one input in one repeat, so it is not a
lossless across-version SLO comparator. Three repeats are exploratory evidence,
not a production sizing recommendation. Raw controls and
[derived observations](controls/observations.csv) retain that loss.

For application fan-out, the batch API remains the appropriate path to reuse
prepared input. Worker count and Go runtime capacity are separate application
choices: the library should not set global GOMAXPROCS to mask a harness-specific
tail. A precise mechanism would require further per-message phase correlation;
these traces and controls do not prove one. No speculative processor fix is made.

## Protocol and limitations

Primary source revisions and Go/broker settings match the preceding
[full-matrix comparison](../nats-optimization-linux-2026-11-08/README.md):

- Baseline `b21df4f8141c69e65bfc1a04c51cb4052e735fb2`, before both optimizations.
- Main `9e06911e37ac43dadd3c0f3655a3e2961e60ac55`, merged PR #23.
- Go 1.27.3 linux/amd64, default decoder and limits, no race build or profiling.
- Bertram: Ubuntu 24.04, i9-13900K, 128 GiB RAM; client affinity
  0,2,4,6,8,10,12,14; broker affinity 16,17, GOMAXPROCS=2, 512 MiB, loopback only.
- Core NATS 2.15.2-alpine at the pinned digest in `manifest.json`.

Each primary invocation has six cases: JSONata object N1/N4, one worker, 5,000
inputs/sec and all three strategies. Six pairs alternate B/M then M/B, with
case order modulo two and strategy order modulo three from offsets 0..5.
Each case has its own excluded 32-message warmup. Compilation, connections,
input generation and expected-result setup are outside timed processing.
Fresh per-message preparation and all rule evaluation/conversion/encoding are
inside. No host service or frequency setting changes. Other service containers
are active on this shared host; their identities are omitted from the retained
environment inventory, with their count preserved. No tests/profiles/controls
run concurrently with primary timing, and controls run after tracing finishes.

Longer trials also allocate larger per-run correlation/expected-result arrays
before timing (50,000 versus 5,000 input slots). Those retained arrays can affect
heap/GC behavior even though their setup allocations are outside the measurement
window. Thus this is not a pure duration-only perturbation of the old one-second
study. Before/after observations within each study remain matched. Results do
not establish that the old differences were solely random noise, and do not
establish universal SLO, capacity or GC-causation claims.

Validation requires exact matrix/rotation, twelve nonoverlapping successful
invocations, matching Go/image/server/resource metadata and complete input/output
accounting. All primary actual rates satisfy the existing ±5% target and 5%
pairwise matching filter. End-to-end deltas are omitted for any group with loss.
Reported percentiles are medians of per-run percentiles, not pooled quantiles.
Client allocation/GC counters include publisher, subscriber, pacing, validation
and transforms, rather than library-only costs. The broker is separate.

## Reproduction and provenance

The source archive and common [primary harness patch](harness.patch) hashes are
in `source.sha256`; executable hashes/build metadata accompany each source.
The primary patch adds focused configuration and report support without changing
engine code, defaults, the existing full-matrix duration limit or `Run`'s existing
50,000-message bound. The diagnostic command/runtime-slot override is excluded
from that frozen primary patch. Traces and controls have separately retained
source snapshots, diagnostic patch and executable hashes.

```sh
# Linux/Bertram with cached dependencies, Docker and matching CPU topology:
bash docs/benchmarks/nats-focused-linux-2026-11-08/reproduce.sh \
  "$PWD" /absolute/path/to/go1.27.3/bin/go /data/ci/new-focused-run
# Optional diagnostics, strictly after the primary comparison:
bash docs/benchmarks/nats-focused-linux-2026-11-08/diagnostics/reproduce.sh \
  /data/ci/new-focused-run /absolute/path/to/go1.27.3/bin/go
bash docs/benchmarks/nats-focused-linux-2026-11-08/controls/reproduce.sh \
  /data/ci/new-focused-run /absolute/path/to/go1.27.3/bin/go "$PWD"
# Revalidate retained data offline:
python3 -B experiments/messages/nats/compare.py docs/benchmarks/nats-focused-linux-2026-11-08
python3 -B docs/benchmarks/nats-focused-linux-2026-11-08/analyze-secondary.py
```

The version-matched trace tool can be built from cached Go sources using
`go build -o /tmp/go-trace cmd/trace` when the downloaded toolchain lacks a
prebuilt `go tool trace`. `trace_summary.py` streams its parsed events; it does
not write an enormous event dump. All raw traces, profiler binaries, frozen source
archives and original logs remain under `/data/ci/jsontrans-nats-focused-20261108`.
The primary driver's legacy terminal label says "ALL SIX RUNS COMPLETE";
`execution.tsv`, all twelve metadata records and validation are authoritative
for the six-pair/twelve-invocation count.


## Verification

Both frozen primary builds pass tests/race/vet, report checks and real-container
route integration before timing. Focused configuration tests fail before the
profile is implemented and pass after it is added; the default full/smoke
matrices and duration bounds remain covered. All primary source trees compare
identically in the Go workload module after the common patch.

The final harness passes Linux tests/race/vet, Python report/trace-parser tests
and real-container integration with default and independent worker/runtime
settings. Core tests/race/vet and formatting pass locally. Comparison tests cover
six-pair rotation, exact metadata and matrices, malformed counters/percentiles,
loss exclusions, actual-rate matching and per-pair output. The unchanged full
study remains exactly reproducible through the generalized analyzer.

Independent read-only review regenerates both studies' JSON/CSV, focused paired
observations and secondary-control summaries exactly, and confirms the report's
arithmetic and source/runner separation. Cleanup is independently checked for
all twelve primary, six traced and twelve runtime-control brokers; none remain.
Only maintained harness/tools, source reconstruction and relevant evidence are
retained. Plans, scratch, raw traces and executable/profile binaries stay out of
Git. Auxiliary text whitespace is trimmed; raw JSON/JSONL observations remain
unchanged. SHA256SUMS covers the retained files and excludes itself.
