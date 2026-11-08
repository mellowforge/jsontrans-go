# Selective materialization on Linux, 2026-11-08

Eligible prepared object transformations now read fields directly from the
immutable document index. JSONata builds root object constructors from singular
scalar fields, literals and conservative numeric `+`, `-` and `*` expressions.
JSONPath singular selectors materialize only the selected container subtree.
This removes repeated whole-document conversion for these transformations.

Whole-input validation and dimension checks, detached results, deadlines, work
accounting, byte caps and batch error attribution remain in place. Unsupported
shapes use the native interpreter. There are no public API, dependency, Go
minimum or JavaScript execution-profile changes.

## Measurements

Bertram: Ubuntu 24.04, linux/amd64, Intel Core i9-13900K, 128 GiB RAM. Go 1.27.3,
GOEXPERIMENT empty, GOMAXPROCS=1, CPU 8, production default limits. Rules are
compiled outside timing; preparation, evaluation, conversion and encoding are
inside each message's timed call. No host-wide service or frequency settings
changed. All results below are medians across five alternating paired rounds.

Target 1 KiB, one rule through `EvalPreparedJSON` or sixteen rules through
`Batch.EvalPreparedJSON`:

| Workload | Rules | Before µs/message | After µs/message | Less time | Allocations before → after | Allocated bytes before → after |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| JSONata object | 1 | 35.43 | 17.54 | 50.5% | 448 → 70 | 49,852 → 30,023 |
| JSONPath object | 1 | 28.50 | 16.88 | 40.8% | 395 → 72 | 48,332 → 30,100 |
| JSONata object | 16 | 330.70 | 46.55 | 85.9% | 6,508 → 414 | 364,361 → 43,720 |
| JSONPath object | 16 | 216.08 | 44.76 | 79.3% | 5,613 → 446 | 336,227 → 44,527 |

The sixteen-rule object batches are approximately 7.1× and 4.8× faster in these
fixtures, with 88% and 87% fewer allocated bytes. Target 64 KiB object batches
improve from 579.85 to 120.87 µs for JSONata (79.2% less time) and 414.12 to
113.03 µs for JSONPath (72.7%). Single-rule 64-KiB calls improve about 25% and
20%; preparation takes a larger share of the call.

Scalar prepared/batch controls remain near baseline: the primary scalar medians
shift between -2.1% and +1.5%. In particular, target-1-KiB scalar JSONPath batch16
is 1.5% slower in this sample. The unchanged direct API controls shift between
-0.9% and +0.6%. See the complete tables for allocation variation and sample
ranges rather than treating small differences as established gains or regressions.
These are short local measurements, not confidence intervals or universal
throughput/capacity claims. B/op measures allocation traffic, not peak RSS.

The Go 1.26.10 control confirms these gains: target-1-KiB object batch16
improves from 326.80 to 42.48 µs for JSONata (87.0% less time) and from 213.93
to 43.99 µs for JSONPath (79.4%).

## Workloads and interpretation

The unchanged `experiments/messages` harness cycles eight varied deterministic
documents and checks each strategy against independent expected JSON before its
timer starts. It reuses worker-local result destinations; every output buffer
remains independently owned. The three strategies are repeated `EvalJSON`,
prepare-once then `EvalPreparedJSON`, and prepare-once then
`Batch.EvalPreparedJSON`. Every strategy includes its own required decoding or
preparation and byte encoding.

The object corpus includes sixteen small sibling objects plus metrics, customer,
item and padding fields. JSONata constructs two fields per rule:
`{"name":customer.name,"code":meta.id+N}`. JSONPath selects `$.objects.oN`.
Previously each object rule materialized the entire document. The candidate
reads the needed scalar paths or materializes the selected two-field object.
This explains the large fan-out gain; the result does not establish the same
speedup for whole-document outputs, grouping or arbitrary scripts.

Target-1-KiB object inputs actually span 1,024–1,206 bytes; scalar inputs span
1,024–1,040. Target-64-KiB inputs span 65,536–65,552. Sizes grow mostly unused
padding, not collection cardinality. [The inventory](corpus-inventory.json)
records all expressions, byte lengths and corpus hashes. Fixtures are unchanged
from the preceding encoding experiment.

JSONata fast eligibility remains conservative: root constructors need unique
literal keys and supported scalar expressions. Array-valued paths, container
values, computed/duplicate keys, missing/nonnumeric arithmetic operands and
non-finite arithmetic use fallback. Bindings, host functions, custom limits,
expression-depth instrumentation and documents with at least 10,000 nodes retain
the existing fallback guards. JSONPath non-singular queries also use fallback.
Whole-document numeric-overflow rejection remains effective for JSONata, even
when the overflowing field is unselected.

## Profiles and protocol

The main matrix contains 48 cases (two engines, scalar/object, target 1/64 KiB,
N1/N16 and three strategies), with five paired 300-ms samples per version: all
480 rows passed. Binary order alternates across rounds and strategy order
rotates. A separate Go 1.26.10 minimum-version control uses the 24 target-1-KiB
cases and three paired 200-ms samples per version. All individual samples and
min/max ranges are retained; use [analyze.py](analyze.py) to check coverage and
regenerate the median CSVs.

CPU and allocation profiles are separate from timing runs and isolate four
object cases: each engine with N1/prepared or N16/batch at target 1 KiB. Every
slash component of the profile selector is anchored. CPU runs use 2 seconds;
allocation runs use 1 second and memprofilerate=1. Instrumented ns/op values
are not performance samples. Allocation profiles include setup, so totals and
percentages are not per-operation before/after allocation comparisons.

The candidate JSONata object profiles contain no sampled allocations attributed
to `document.Node.materialize`. JSONPath still materializes selected containers,
as intended, rather than the whole input for each rule. The driver records an
explicit absence message if pprof has no matching materialization allocation
stack; other profiler errors remain fatal. Profile summaries retain CPU,
alloc_space, alloc_objects, materialization call sites and instrumented output.
Raw profiler binaries remain outside Git on Bertram.

## Source identity and reproduction

Baseline: merged main `f533e6020b5321d4c506a48ae481df8c9a523661` (PR #22).
Candidate: that source plus [candidate.patch](candidate.patch), including
production changes and regression/reference/fuzz tests. Documentation is outside
the measured delta. Fixed binaries, archives and raw profiles remain under
`/data/ci/jsontrans-selective-20261108` on Bertram.

Transferred archive SHA-256:

- Baseline: `33c86e5df50fc53b87c458c2474880afcb8996a6c2a03dc4d53f1a4454e15bd5`
- Candidate: `85dacc3c6764ec4d460f0eee9612163e1e71573491ceecb172ad0e3462806d2d`

The expanded reference comparisons also exposed a pre-existing interpreter
mismatch: a missing computed object key incorrectly raised T1003. The candidate
omits that entry without evaluating its value, matching JSONata 2.2.2. Null or
other non-string keys still error. Dedicated failing-then-passing regressions
and the pinned oracle cover this correction. Literal-key benchmark expressions
do not exercise the correction; direct object cases serve as controls.

With cached dependencies, absolute toolchain paths and CPU 8 available on Linux:

```sh
bash docs/benchmarks/selective-materialization-linux-2026-11-08/reproduce.sh \
  "$PWD" /absolute/path/to/go1.27.3/bin/go /absolute/path/to/go1.26.10/bin/go \
  /tmp/jsontrans-selective-reproduction
python3 -B docs/benchmarks/selective-materialization-linux-2026-11-08/analyze.py \
  /tmp/jsontrans-selective-reproduction
```

The script reconstructs pinned source plus patch, compiles fixed binaries,
compares the timing matrix, and generates profiles. Source patch application and
shell syntax are checked. The recorded run collected baseline profiles before
implementation, then primary timings, candidate profiles and the minimum-version
control. No profiling or correctness test ran concurrently with timing on Bertram.
Generated text has trailing whitespace/terminal blank lines trimmed; CSVs use LF.
Benchmark values are unchanged. The reproduction patch retains unified-diff
whitespace. Build identities, host information and verification logs are retained
alongside the samples; SHA256SUMS covers the evidence files.


## Verification

[Retained logs](verification/) record core tests, race detection and vet on Linux
with Go 1.26.10 and 1.27.3, including the pinned conformance suites. macOS
Go 1.26.10 passes core tests/race/vet, the expanded pinned Node reference
comparison, and optional JavaScript, assessment and message module tests/race/vet
against the modified checkout using a private workspace. No dependency or guest
runtime change is involved.

Bounded fuzz execution targets pass: 20,000 object-plan, 5,000 prepared-byte,
10,000 document, 5,000 JSONata parser/runtime, 10,000 JSONPath parser/runtime and
5,000 message-strategy executions. Coordinator totals can slightly exceed the
requested counts. Regression tests cover missing/null, fallback arrays and
containers, numeric overflow/type errors and source positions, surrogate keys
and values, host overrides/bindings, ownership, concurrent homogeneous/mixed
batches, output aggregation, whole-input dimensions and cancellation.

Linux NATS harness tests/race/vet and Docker/Testcontainers integration pass.
All nine direct/prepared/batch × fan-out1/4/16 route cases complete 256 lossless
messages apiece with four workers; container cleanup succeeds. This is transport
correctness validation, not a repeated end-to-end load matrix or a new p99 claim.

Formatting, shell syntax, evidence coverage and source reconstruction checks
pass. A fresh independent read-only review found no actionable defects. Profiles
and timings use the same frozen production/test source. Plans, review scratch
and raw profiler binaries remain outside Git. Standard-library marshaling and
individual allocation/copy operations retain their cooperative in-process
limits rather than hard process-memory or preemption guarantees.
