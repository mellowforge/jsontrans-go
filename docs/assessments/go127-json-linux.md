# Go 1.27 JSON: Linux validation and message-processing implications

Date: 2026-11-06. Follow-up to the [macOS assessment](go127-json.md).
The experiment's production defaults, minimum Go version and public API remain
unchanged. Both one-program and multi-program transformations of fresh incoming
messages are target workloads; see the [NATS workload record](../nats-workload.md).

## Source and machine

Bertram: Ubuntu 24.04, Linux 6.8.0-147-generic, amd64, Intel Core i9-13900K.
A clean isolated checkout under /data/ci used commit
`dbd3057f96cf2ecaa1e31816255407132a3cc8a6` and existing Go 1.26.10/1.27.3 toolchains.
Go/module sources match both macOS datasets exactly:
`63fdd9919d55ce2393fdc9ca8e2b5ede2e17e2dc80ca9148f43b004ef37c8f5f`.

Tests used GOMAXPROCS=8. After verification finished, the benchmark driver and
its children were pinned to logical CPU 8 (a performance core, core ID 16,
5.8 GHz reported maximum); benchmarks used GOMAXPROCS=1 and -test.cpu=1.
The governor remained powersave; frequency was not locked. Affinity applied
only to the benchmark processes. No host policy, services or shared source
checkout was changed. Toolchains/modules/build cache were reused under /data/ci,
with GOPROXY=off during checks and measurements. [Host details](../benchmarks/go127-json-linux-2026-11-06/host.json)
record affinity and load; this is not an otherwise isolated/constant-frequency
machine or a test of the whole 32-thread system's throughput.

The four configurations and six rotating rounds match the Mac method:
Go 1.26, Go 1.27/nojsonv2, default Go 1.27, and the tagged jsontext prototype.
The full suite uses 150 ms windows; a separate prepared control uses 1 second.
Compilation is excluded from timing. These are warm single-threaded medians,
not tail-latency distributions or NATS end-to-end measurements.

## Results

Microseconds per operation; batch rows represent the complete rule set.
EvalJSON includes input parsing and JSON output; prepared batches return Go
values and therefore exclude output serialization.

| Workload | Go 1.26 | Go 1.27 | Go 1.27 + jsontext |
|---|---:|---:|---:|
| 1 KiB JSONata EvalJSON | 45.01 | 34.13 | 32.09 |
| 64 KiB decode | 451.53 | 290.88 | 269.54 |
| 1 MiB array decode | 51,774.28 | 27,237.35 | 24,353.97 |
| 64 KiB JSONata EvalJSON | 635.55 | 468.77 | 449.05 |
| 64 KiB JSONPath EvalJSON | 632.45 | 467.10 | 447.87 |
| 64 KiB prepare + 16 rules, Go results | 53.23 | 56.16 | 56.37 |
| 64 KiB reused batch, 16 rules | 7.72 | 7.81 | 7.72 |
| 64 KiB reused batch, 64 rules | 30.34 | 30.86 | 30.82 |

Default Go 1.27 reduced 64 KiB decode latency by about 36% and 1 MiB array
decode latency by about 47% versus Go 1.26. The prototype reduced them by
about 40% and 53% versus Go 1.26, adding approximately 7% and 11% over default
Go 1.27. The 64 KiB JSONata byte-to-byte medians were 636/469/449 us:
about 26% lower latency from the standard backend and another 4% from the
prototype. This reproduces the direction of the Mac results, not identical
magnitudes or a guarantee for other transformations.

At 64 KiB decode allocation counts were again 491/253/173 and allocated bytes
about 472/338/271 kB per operation. The 1 MiB array counts were again
1,029,890/486,888/318,364. The prototype remains a materializing decoder;
these reductions are not a zero-allocation implementation.

Native prepared evaluation stays approximately unchanged. Fresh preparation
and batching showed a small Go 1.27 slowdown on Linux. The longer isolated
control for 64 KiB preparation plus 16 rules measured 52.93 us on Go 1.26,
53.75 us on Go 1.27/nojsonv2, 56.30 us on default Go 1.27, and 55.86 us with
the prototype. Default Go 1.27 was about 6% slower than Go 1.26 here. This
reproduces the Linux short-window direction, unlike the earlier Mac control.
The experiment does not identify its cause. No materializing decoder is used
by this prepared workload, and the prototype adds no useful improvement to it.
Profile this route before describing a Go-version change as a universal gain.

JavaScript's small EvalJSON benchmark was 1,049/1,051/1,056 us: no useful
improvement is established. Guest initialization/execution remains separate
from host decoding. This does not cover larger JavaScript inputs or concurrent
guests. Whole-process RSS ranges are retained separately and must not be read
as per-message memory or a guest memory cap.

## Evidence and verification

- [Full medians](../benchmarks/go127-json-linux-2026-11-06/summary.md),
  [ranges and allocations](../benchmarks/go127-json-linux-2026-11-06/summary.csv),
  [toolchains and commands](../benchmarks/go127-json-linux-2026-11-06/metadata.json).
- [Longer prepared control](../benchmarks/go127-json-linux-prepared-control-2026-11-06/summary.md)
  and [control metadata](../benchmarks/go127-json-linux-prepared-control-2026-11-06/metadata.json).
- [21-check manifest](../benchmarks/go127-json-linux-2026-11-06/verification/results.json)
  and adjacent complete logs, all exit codes zero.

Root tests/race/vet passed on all four configurations; root tests include pinned
native conformance fixtures. Go 1.26 and the tagged prototype additionally
passed the complete optional JavaScript test/race/vet suites. Bounded tagged
fuzzing covered decoder differential behavior (15,000-case budget), prepared
materialization (27,000) and JavaScript JSON conversion (500). Formatting,
diff checks and Python summary tests passed. Source hashes match the two Mac
runs and the final local Go sources. No Node oracle, NATS broker, Windows
prototype check, or concurrent-throughput/tail-latency experiment ran here.

## Decision for the message-processing workload

Go 1.27 is useful for fresh byte-to-byte processing. The additional jsontext
prototype gain is real enough to continue evaluating a conditional backend,
but it does not justify rebuilding the engines, forcing a new minimum version,
or claiming every route improves. Keep the prototype opt-in for this PR.
A production switch needs its own reviewed compatibility/CI change; the Linux
prepared-route slowdown remains a profiling item.

For both single-program and multi-program NATS routes, compile at setup and
reuse compiled state. Add a representative byte-to-byte benchmark matrix before
choosing the next implementation: direct EvalJSON versus complete preparation,
native evaluation and result encoding for one program; shared preparation/batch
versus independent calls for multiple programs; then subject routing and bounded
worker concurrency. Include object/array transformations, output expansion,
per-message bindings and configurations that disable fast plans. Retain native
semantics and budgets in every path.

The prepared index can avoid eager whole-tree conversion for supported plans,
so a compiled byte-input fast path is a worthwhile candidate for both patterns.
It is not implemented by this validation, and prepared-only timings cannot
establish its total cost. NATS transport, delivery policy, fan-in correlation,
queue wait and ordered output need a subsequent integration-level benchmark.
