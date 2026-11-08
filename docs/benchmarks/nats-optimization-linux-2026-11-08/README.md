# Matched NATS load comparison, 2026-11-08

This experiment compares pre-optimization `b21df4f` with merged main `9e06911`
after PRs #22 and #23. Both use the public prepared byte-result APIs, with
preparation and output encoding included in message processing. The native CPU
benchmarks established evaluator gains; this comparison measures the complete
instrumented publisher/queue/worker/output-validation pipeline.

## Results

All **972 measured cases were valid**, with zero transformation, transport,
output-validation, pending-drop, unaccounted-input or incomplete-fan-out errors.
All six measured brokers were removed. Each version received 1,458,000 inputs.

**All 324 public prepared/batch cases on main were lossless**, compared with
314/324 on the baseline. Queue rejection on those paths fell from **14,456 to
zero**. Every single-rule case was lossless in both versions (162/162 each).
The remaining overload on main belongs to the direct `EvalJSON` control path,
which still decodes each rule independently. This result covers only the
tested rates/windows; it is not a long-term capacity guarantee.

| Version | All cases lossless | Prepared/batch lossless | Total queue rejections | Completed inputs |
| --- | ---: | ---: | ---: | ---: |
| Baseline | 436/486 | 314/324 | 102,926 | 1,355,074 |
| Main | 447/486 | 324/324 | 88,185 | 1,369,815 |

Selected target-5,000/sec cases below use medians of three per-run metrics.
B → M means baseline → main. Lossless-round counts retain any loss even when
the median completion percentage happens to be 100%. An end-to-end delta is
omitted whenever either version loses messages.

| Route | Rules | Workers | API | Lossless rounds B → M | Processing p50 µs B → M | E2E p99 ms B → M | E2E change |
| --- | ---: | ---: | --- | ---: | ---: | ---: | ---: |
| object | 1 | 1 | prepared | 3/3 → 3/3 | 46.83 → 37.36 | 3.38 → 3.62 | +7.1% |
| object | 16 | 1 | batch | 0/3 → 3/3 | 272.89 → 45.45 | 70.44 → 5.01 | not comparable: loss |
| object | 16 | 8 | batch | 3/3 → 3/3 | 353.08 → 113.32 | 7.85 → 1.15 | -85.3% |
| scalar | 1 | 1 | prepared | 3/3 → 3/3 | 25.72 → 25.30 | 1.16 → 1.07 | -7.6% |
| scalar | 16 | 1 | batch | 3/3 → 3/3 | 35.87 → 32.52 | 4.69 → 1.12 | -76.1% |
| scalar | 16 | 8 | batch | 3/3 → 3/3 | 52.08 → 46.18 | 0.83 → 0.80 | -3.0% |
| mixed | 1 | 1 | prepared | 3/3 → 3/3 | 44.25 → 40.41 | 3.75 → 3.41 | -9.1% |
| mixed | 16 | 1 | batch | 2/3 → 3/3 | 141.41 → 88.22 | 25.62 → 3.04 | not comparable: loss |
| mixed | 16 | 8 | batch | 3/3 → 3/3 | 330.13 → 286.54 | 3.83 → 1.75 | -54.4% |
| object | 4 | 1 | batch | 3/3 → 3/3 | 71.45 → 40.48 | 2.76 → 4.01 | +45.1% |

The one-worker object batch16 completes a median **52.06% → 100%** of
inputs at the 5,000/sec target; main is lossless in all three runs. Its
baseline tail describes only completed messages and is not an SLO comparator.

The lossless eight-worker object batch16 has a repeatable large improvement:
per-run end-to-end p99 ranges are **7.60–9.65 ms** before and **1.09–1.25 ms**
after. Its instrumented allocation bytes per completed input fall from about
**388,370 to 60,989**, mallocs from **6,847 to 639**, and median GC cycles per
one-second case from **607 to 107**. These are client-pipeline measurements;
they do not isolate a GC cause for the latency change.

**Processing gains do not uniformly lower tails.** Single-object/one-worker
processing falls about 20%, while p99 increases from 3.38 to 3.62 ms (+7.1%).
The one-worker/four-rule object batch saves about 43% processing time but
p99 rises 2.76 → 4.01 ms (+45.1%). Their main per-run tails vary widely
(1.18–4.34 ms and 1.74–5.42 ms respectively). Pacing, scheduling, queueing
and the shared host remain material. Longer focused trials are needed before
making SLO conclusions about those configurations.

Across all 162 workload/strategy groups, 144 have six lossless observations
at matched actual load. Of those, 108 have a lower median per-run p99 and
36 a higher one; this count includes controls and is descriptive, not a
significance test. All groups passed the actual-rate matching check. Every
group, p50/p95/p99, p99 range, loss count, allocation and GC metric remains in
[comparison.csv](comparison.csv) and [comparison.json](comparison.json).

The comparison supports retaining the optimizations for lower CPU/allocation
cost and better overload behavior on these prepared paths. It does not
establish a universal end-to-end latency improvement.

## Protocol

The existing full matrix runs three times per version: JSONata scalar and
object routes plus mixed JSONata/JSONPath routes; fan-out 1/4/16; workers 1/4/8;
1,000/5,000 offered inputs/sec; direct/prepared/batch strategies. Each of the
972 cases uses a one-second paced publication window after a separate
32-message warmup. Eight subject-specific precompiled plans select transformations.
Inputs and outputs carry correlation IDs; every defined output value and full
fan-out completion are checked. Compilation, connections, broker startup and
warmups are outside timing. Preparation, conversion, evaluation and encoding
are inside timed processing.

Runs are serial: baseline/main, main/baseline, baseline/main. The common
`-round-offset` CLI patch rotates configuration and strategy order by 0/1/2
across the paired invocations. Local report round IDs remain 1; pair identities
are in directory names, metadata and `execution.tsv`. The comparison validator
checks the exact matrix/order, all six run metadata records, nonoverlapping
execution intervals, derived rates, integer counts and input/output accounting.
No test or profiling work runs concurrently with load on Bertram.

Both binaries use Go 1.27.3 linux/amd64, the default decoder and production limits,
without race instrumentation or experimental build tags. The parent message
harness, NATS harness, corpus and dependencies are identical at both source
revisions. Both source archives receive only the identical CLI rotation patch.
The later comparison/report code runs offline and is not part of timed native
execution.

Bertram runs Ubuntu 24.04 on an Intel Core i9-13900K with 128 GiB RAM. Client
code is pinned to P cores 0,2,4,6,8,10,12,14; per-case GOMAXPROCS equals worker
count. A fresh Testcontainers broker starts for each invocation, pinned to
E cores 16/17 with GOMAXPROCS=2, a 512-MiB cap and loopback-only dynamic ports.
The image is Core NATS 2.15.2-alpine at immutable digest
`sha256:afae2fdbe2e326c65e1246cb83b9d05d319188734f3bd577ac8072daae78a10f`.
No host service or frequency setting changed. Other service containers run on
this shared machine; alternating version order reduces but does not eliminate
background variation. Host counters before/after each run are retained.

## Reading the measurements

Processing covers all transformations for one input. End-to-end latency starts
at actual input publish and ends when the complete output fan-out arrives.
Queue wait is separate. Reported p50/p95/p99 figures are medians of three
per-run percentiles, not pooled percentiles. Each full comparison row retains
per-run p99 ranges and every underlying observation.

The application queue has 64 slots per worker and rejects at enqueue when full.
Queue rejection is a valid overload measurement, not lossless service. Transport
pending drops, publish/async errors, bad output values, unaccounted inputs,
incomplete fan-out, transformation errors and cleanup failures invalidate the
experiment. Each measured broker is independently checked for removal.

All end-to-end percentiles describe completed messages only. The comparison
omits an end-to-end percentage change unless all six observations for that case
are lossless, each actual offered rate is within 5% of its target, and each
pair's actual rates differ by at most 5% of the lower actual rate. This is a
conservative descriptive filter, not evidence of statistical significance or
identical burst patterns. Lossy/mismatched observations remain visible. A lower
processing median alone does not establish an end-to-end improvement.

Allocations, mallocs and GC counts cover the entire instrumented client process,
including input/output transport, pacing, validation and transformation. The
broker is a separate process. Per-sent and per-completed allocation/malloc
figures are both retained, especially for lossy cases. GC counts are per run;
this harness does not record GC pause distributions, live heap or peak RSS.
Do not compare these allocation figures directly to library-only Go benchmarks.

The payload size is a 1-KiB minimum target; exact dimensions, expressions and
corpus hashes for all native route fixtures are in `corpus-inventory.json`,
reused from the earlier public-API validation because the fixture source is
unchanged. Larger representative documents,
TLS, persistence/JetStream, acknowledgements, remote networking, production
arrival distributions and long-duration soak tests are outside this experiment.
The two tested offered rates do not locate maximum sustainable capacity.

## Sources and reproduction

- Baseline: `b21df4f8141c69e65bfc1a04c51cb4052e735fb2`, immediately before both
  optimizations; source archive SHA-256
  `d4f685e2a47a70ebe2654bf276324a3415cf46aad7cf23bc1d2abc4ee6450b90`.
- Main: `9e06911e37ac43dadd3c0f3655a3e2961e60ac55`, merged PR #23; source archive
  SHA-256 `5c553143b98e61688edd6d2f6fa6e9d269c28845e49b9b9e0273f8c7602e1236`.
- Identical [harness rotation patch](harness-rotation.patch): SHA-256
  `ddd6c5b17d622713986ea7b7f01ab40f417b2b79eea09b4759937048e457f5d7`.

Fixed executables, frozen archives and full original logs remain on Bertram under
`/data/ci/jsontrans-nats-compare-20261108`. Build records and binary hashes identify
both measured executables. `manifest.json`, `execution.tsv`, raw per-run JSONL,
broker metadata and the complete paired CSV/JSON retain source and measurement
provenance. Process plans and binaries stay outside Git.

From a checkout containing the comparison tools, with cached dependencies, Docker,
Go 1.27.3 and the CPU topology described above:

```sh
bash docs/benchmarks/nats-optimization-linux-2026-11-08/reproduce.sh \
  "$PWD" /absolute/path/to/go1.27.3/bin/go /data/ci/new-nats-comparison
```

The script reconstructs the two pinned sources, applies the common patch,
verifies/compiles both binaries, runs all six invocations serially, checks
container cleanup, and generates paired reports. To revalidate retained data:

```sh
python3 -B experiments/messages/nats/compare.py \
  docs/benchmarks/nats-optimization-linux-2026-11-08
python3 -B -m unittest discover -s experiments/messages/nats -p 'test_*.py'
```


## Verification and retained evidence

Before load, both frozen sources pass native message tests, NATS tests/race/vet,
parent/NATS report tests and real Testcontainers route integration at fan-out
1/4/16 across all three strategies. The offset rejection check runs before any
output directory or broker creation. Their compiled Go workload source trees
and fixture files compare identically after applying the common CLI patch.
Verification streams and binary build hashes are retained in this directory.

Local core formatting, tests/race/vet, report/comparison tests and shell syntax
checks pass. A fresh independent read-only review reproduced the retained JSON
and CSV exactly from all 972 raw reports and found no remaining actionable
issue. The comparison tests reject missing/duplicate cases, wrong rotation
or execution order, overlapping runs, mismatched metadata, cleanup failures,
transformation failures, non-finite/fractional percentiles and negative/bool/
fractional counters. Separate tests prove that processing gains cannot imply
latency gains and that loss or materially different actual load suppresses the
end-to-end comparison. Validation creates no extra per-run summary artifacts.

The committed environment inventory omits identifiers of unrelated background
service containers and records their count instead; the original inventory
remains on Bertram. Raw measured reports and broker metadata remain intact.
Checksums cover all retained evidence; the checksum list excludes itself.
