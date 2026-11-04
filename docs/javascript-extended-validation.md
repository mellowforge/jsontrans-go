# Extended JavaScript validation campaign

This campaign follows merged PR #13 (`b5e7958`) and exercises the optional
preview processor without changing its public API, runtime or execution profile.
It is development validation, not a production release approval.

## Reproducible workload

The count-based `javascript/cmd/hardening` command remains available. Duration
mode adds bounded runs, a persistent engine or periodic recycling, exclusive
progress-file creation, and live/closed memory samples. Each batch compiles the
five existing expressions, runs 25 iterations per worker, and checks every
outcome followed by a healthy evaluation. This exercises repeated compilation
in a long-lived engine; it is not a compile-once throughput benchmark.

The cases cover detached input/bindings and fresh globals, ordinary exceptions,
cyclic output conversion, caller cancellation, and native allocation failure.
The allocation request is five times the configured guest cap. Native OOM must
not acquire trusted quota/conversion provenance. Partial results and progress
are retained on failure; counters cover fully completed batches, excluding an
interrupted in-flight batch. Failed, interrupted or incomplete runs cannot pass.
Normal duration expiry occurs only after a fully checked batch. An outer
watchdog is duration plus 30 seconds. Signal cancellation and progress/cleanup
errors return nonzero and mark the final report incomplete.

```sh
GOTOOLCHAIN=go1.26.10 CGO_ENABLED=0 go -C javascript build -o /tmp/js-hardening ./cmd/hardening
GOMAXPROCS=4 /tmp/js-hardening -duration 1h -workers 4 -guest-mib 8 \
  -sample-interval 30s -progress /tmp/js-persistent.jsonl > /tmp/js-persistent.json
GOMAXPROCS=4 /tmp/js-hardening -duration 1h -workers 4 -guest-mib 64 \
  -recycle 1m -sample-interval 30s -progress /tmp/js-recycled.jsonl > /tmp/js-recycled.json
```

Use fresh output paths. Duration is 1 second to 2 hours; workers 1–8; supported
campaign guest profiles are 2, 8 and 64 MiB. Sampling/recycle intervals are at
least 1 second. A zero recycle interval means one persistent engine. Explicit
count-mode iterations/cycles cannot be combined with duration options. Small
smoke runs must specify a sampling interval no longer than their duration.
The legacy count mode retains its five-minute watchdog and existing bounds.

## Resource fuzzing

`FuzzOwnedResourceLimits` varies scalar input and independent limit values at
2/8/64 MiB. Its programs generate arrays of up to 64 items, nesting up to 16,
ASCII strings up to 1024 bytes, small successful allocations up to 4096 bytes,
and allocations just above the guest cap. Exact success/output-limit/native-OOM
outcomes, phases, no-result behavior and healthy recovery are checked. Scalar
input/empty bindings avoid tiny output limits failing earlier in input handling.
An intentionally wrong array program is rejected by the seed oracle. No
unexpected deadline is discarded by this target.

```sh
go -C javascript test -run '^$' -fuzz '^FuzzOwnedResourceLimits$' \
  -fuzztime=100000x -parallel=2 -timeout=30m
```

The extended Linux run also exercises owned compilation (20,000 executions),
diagnostic decoding and location mapping (1,000,000 each), string and nested JSON
conversion (100,000 each), and terminal phases (10,000). Counts include normal
fuzz input rejection for out-of-domain inputs where documented in each target.
The existing compiler fuzzer permits syntax/resource/cancellation errors and
checks artifact bounds and crashes; it is not a source-language conformance
oracle or proof against every guest error. Failing corpus entries are retained.

## Measurement and assessment

The Linux profiles run concurrently for one hour, four workers and GOMAXPROCS=4
per process. Mac and native Windows run 15-minute recycled profiles, two workers
and GOMAXPROCS=2, from the same source commit. Per-process RSS is sampled
separately from Go runtime memory. Linux VmHWM/Windows peak working-set samples
and Mac RSS samples are labelled by their actual source; no process-memory hard
cap is inferred from guest limits.

Forced-GC samples identify baseline, live-engine and after-Close state. Analyze
live and closed samples separately, discard the first ten percent as warmup,
and compare beginning/end windows rather than calling retained Go Sys a leak.
The persistent profile has only one after-Close sample, so no closed-state trend
can be inferred for it. The report itself retains bounded timing/sample arrays;
their storage is part of measured Go heap and is reported when interpreting
small growth. These diagnostics perturb timings and provide no throughput claim.

All reports must include their source SHA, configuration, toolchains, exact
commands, exit codes, completed durations, checked counts and artifact hash.
Unexpected behavior or unexplained growth is investigated. A retry never
rewrites the original attempt. Historical PR #13 evidence remains unchanged.

## Results

All accepted profiles use committed source `dc29b60` and Go 1.26.10. Each
iteration below includes a subsequent checked healthy evaluation.

| Native profile | Guest cap | Actual duration | Verified recoveries | Closed engines |
|---|---:|---:|---:|---:|
| Linux, persistent | 8 MiB | 3,600.061 s | 1,394,900 | 1 |
| Linux, recycled | 64 MiB | 3,600.104 s | 1,389,500 | 60 |
| Mac, recycled | 8 MiB | 900.172 s | 217,400 | 15 |
| Windows, recycled | 64 MiB | 900.155 s | 195,300 | 15 |

That is **3,197,100 accepted recovery pairs**. All seven extended fuzz campaigns,
all-module normal/race/vet, consumer packaging, Node oracle, 21 owned probes and
the three-target Go security scan pass. No engine/pin/dependency change was made.
The [dated evidence](../javascript/testdata/stress/2026-11-04/README.md) retains
commands, source identities, reports, logs, sample data and checksums.

After ten-percent warmup, Linux live-heap window medians rose about 0.24 MiB,
while recorded timing values alone retain about 0.21 MiB plus slice capacity
and sample overhead. The recycled profile's closed-heap medians were about
0.39 → 0.64 MiB. Goroutines stayed at three in every accepted profile, including
baseline. Linux RSS window medians were approximately 51.6 → 53.9 MiB
(persistent) and 50.8 → 51.1 MiB (recycled). This is consistent with small harness
storage/allocator retention, not an observed growing engine leak; it is not a
proof against all leaks or a hard process-memory bound.

Mac RSS settled near 110 MiB after warmup. Windows retained-heap medians rose
about 0.07 MiB, with intermittent roughly 3.9 MiB after-Close observations that
later returned lower; the higher final snapshot is retained. Windows RSS ended
below its starting post-warmup window. The exact source of those transient
residual snapshots was not isolated. Close does not promise immediate physical
reclamation. Raw maxima and endpoints are exposed rather than replaced by only
favorable medians.

The first Windows runner lost the child's exit code despite a successful child
report. It remains a failed external verification. A harmless known-exit test
reproduced the PowerShell 5.1 behavior; caching the process Handle fixed the
wrapper, and a separate full 15-minute rerun passed with a reliable zero exit.
Both attempts and scripts are retained. A separate intentional SIGTERM control
also confirms incomplete runs exit nonzero, preserve partial counts and close
the engine.

Private reporting-route confirmation, supported-platform/production approval
and module versioning remain separate release gates. No release/tag is created.
