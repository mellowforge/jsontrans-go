# Fresh-message byte-path experiment

**Follow-up (2026-11-07):** The proposed byte-result helpers are implemented as
public `EvalPreparedJSON` and `Batch.EvalPreparedJSON`. The
[final NATS validation](../benchmarks/message-nats-public-api-2026-11-07/README.md)
exercises those APIs and records 486 valid cases, including single-rule and
fan-out routes. The sections below retain the original prototype's source,
measurements and recommendations.

Date: 2026-11-06. Native measurements ran first on Bertram (Ubuntu24.04,
amd64/i9-13900K), then the NATS/Testcontainers phase. Production APIs, decoder
defaults and execution profiles remain unchanged. Both one transformation and
several transformations over each new message are target workloads.

## Native method and evidence

The [development module](../../experiments/messages/README.md) compares three
precompiled routes, all returning encoded JSON: independent EvalJSON; one owned
PrepareJSON plus individual EvalPrepared and bounded encoding; and one owned
PrepareJSON plus Batch.EvalPrepared and bounded encoding. Every operation
prepares fresh input where required. All use the same outer message deadline
and aggregate encoded-output limit plus existing per-expression controls.
Work counters differ because strategies perform different work. The experiment
uses internal bounded encoding helpers and is not a new public library API.

Eight deterministic inputs vary shape/item count/escaping/Unicode. Scalar/object
fan-out expressions are distinct; repeated whole-output and four-pattern
JSONPath union cases are declared explicitly. Shapes/expressions differ between
engines, so comparisons are within each workload, not engine rankings. Target
sizes are minimums: the extra object fixture fields exceed1KiB. The inventory
records every actual input/output byte count and expression/corpus hash.

Six warm rounds rotate strategy order. Sequential profiles use Pcore CPU8 and
GOMAXPROCS1. Routed parallel profiles use eight independently compiled subject
routes and worker-local scratch, allowing one thread per physical Pcore with
GOMAXPROCS1/2/4/8. Their ns/op is aggregate wall-time/message, not per-message
latency. No broker, tests or other benchmark configurations ran concurrently.
CPU frequency/governor settings were unchanged. Source was frozen at7c4d784;
the recorded source hash includes core/JS/native-helper Go/module/Wasm sources
and excludes the subsequent separate NATS module. Final source hash verified.

[Raw results/commands/source identity](../benchmarks/message-native-linux-2026-11-06/metadata.json),
[all618 median/range rows](../benchmarks/message-native-linux-2026-11-06/summary.csv),
[fixture inventory](../benchmarks/message-native-linux-2026-11-06/inventory.json),
[host settings](../benchmarks/message-native-linux-2026-11-06/host.json), and
[11 Linux verification checks](../benchmarks/message-native-linux-2026-11-06/verification/results.json)
are retained. The source hash still matches the measured source after adding
NATS code because that nested module is explicitly outside the native artifact.

## Native results

Sequential Go1.27.3 medians, microseconds per complete input message, including
preparation, required result conversions and encoding of every output:

| Workload | Fan-out | Direct | Prepared individually | Prepared batch |
|---|---:|---:|---:|---:|
| JSONata scalar,1KiB target |1|61.74|9.31|9.39|
| JSONata scalar,1KiB target |16|982.43|23.07|19.76|
| JSONata object,1KiB target |1|124.64|35.03|35.90|
| JSONata object,1KiB target |16|2010.57|327.30|336.72|
| JSONPath scalar,1KiB target |1|57.72|9.41|9.39|
| JavaScript object,1KiB target |1|1418.87|1343.56|1338.75|

For JSONata scalar N1 the prepared route reduces time about85% and allocations
from910 to49; for object N1 about72%, with1966 to454 allocations. At scalar N16,
batching helps further (19.76 vs23.07us), whereas object N16 individual prepared
calls are slightly faster than batching. It is not correct to assume batching
wins every family. All measured native families favored shared preparation in
this corpus, including fallback/whole-output cases, but tiny/empty JSON and
arbitrary external adapters are not covered. Do not enable an automatic
production switch from this finite synthetic corpus alone.

Eight-worker JSONata object N4 routed aggregate times were140.10us/message for
direct,32.74 for prepared and33.32 for batch. These are throughput-normalized
wall times under concurrent load, not p50 or p99 latency. JavaScript sees only
modest benefits because the approved fresh guest remains the dominant cost;
its startup/compilation budget was30seconds, with the per-call timeout restored
to1second. No guest pooling was introduced.

Native unit/race/vet passed on Go1.26.10, Go1.27.3 and tagged-jsontext Go1.27.3.
Default/tagged differential fuzzing each used12,000-case budgets. Tests cover
exact JSONPath numbers, JSONata binary64 conversion, missing/null, surrogate
code units, malformed/trailing input, native errors, cancellation, aggregate
output limits and concurrent reuse. The output checker rejects lossy surrogate
normalization and trailing data. Reporting rejects missing strategy/workload/CPU
or round groups and changed/incomplete source snapshots. Root gates, formatting,
Python tests and static review also passed.

## NATS phase

The separate [transport harness](../../experiments/messages/nats/README.md) used
Testcontainers-Go0.45.0, NATS Go client1.55.0 and official NATS server2.15.2-alpine,
pinned by digest. The quiet Core NATS broker (no TLS/persistence/acks) bound only
to loopback,512MiB, two Ecores16/17; client Pcores0,2,4,6,8,10,12,14 with
GOMAXPROCS1/4/8. The source snapshot was01fbb04. No system CPU policy changed.
Broker/image startup, plan compilation, subscription readiness and32-message
warmups were excluded. Each run used fresh connections/plans and new messages.

Real-container mixed-route correctness passed under race detection. The six
smoke strategy cases were lossless. Three strategy/case-rotating rounds then ran
scalar/object/mixed eight-route families, fan-out1/4/16, workers1/4/8, requested
1000/5000 inputs/sec for1second per case:486 valid runs. Of those,436 were
lossless;50 intentionally overloaded the bounded drop-on-full application queue.
All queue rejection was retained (102,659 inputs across the full sweep). There
were no unaccounted inputs, incomplete fan-outs, invalid outputs, NATS pending
drops, publish errors or async transport errors. Broker cleanup succeeded and
an explicit post-run image-filtered container listing was empty.

[Raw per-run results](../benchmarks/message-nats-linux-2026-11-06/runs.jsonl),
[full summary/achieved-load/loss data](../benchmarks/message-nats-linux-2026-11-06/summary.csv),
[broker image/settings and cleanup](../benchmarks/message-nats-linux-2026-11-06/metadata.json),
[host/dependency graph](../benchmarks/message-nats-linux-2026-11-06/host.json),
[seven passing unit/race/vet/integration checks](../benchmarks/message-nats-linux-2026-11-06/verification/results.json)
and [separate smoke results](../benchmarks/message-nats-smoke/summary.json) are retained.
Diagnostic stderr/build-info text has trailing whitespace normalized; timing
samples, JSON run records and their metrics are unchanged.

Selected eight-worker cases, roughly5000 actual inputs/sec. Processing p50 and
end-to-end p99 are medians of the three per-run percentiles, not pooled quantiles.
E2E is actual publish to receipt of every required output, microseconds:

| Family | Fan-out | Strategy | Completion % | Lossless rounds | Processing p50 | E2E p99 |
|---|---:|---|---:|---:|---:|---:|
| object |1|direct|100|3/3|293.8|1589.1|
| object |1|prepared|100|3/3|109.6|1177.8|
| object |1|batch|100|3/3|111.7|1161.3|
| object |16|direct|57.7|0/3|3267.8|222071.4|
| object |16|prepared|100|3/3|339.8|5536.4|
| object |16|batch|100|3/3|349.2|6572.9|
| mixed |16|direct|82.2|0/3|1881.2|149669.7|
| mixed |16|prepared|100|3/3|320.3|4258.0|
| mixed |16|batch|100|3/3|322.4|3347.1|

The transformation savings survive this transport harness, especially under
fan-out pressure, but do not imply uniformly better tails. One-worker object N1
was lossless for every strategy yet had E2E p99 medians3162us direct,4195 prepared
and4048 batch, despite lower prepared processing time. Scheduling, pacing, GC
and transport can dominate the end-to-end tail; this experiment does not isolate
the cause. More workers also do not automatically reduce individual processing
time. No universal strategy/worker count is selected from these samples.

End-to-end percentiles include completed inputs only and exclude pacing lateness
before actual publish. Requested/achieved rates, maximum pacing lateness, queue
rejection, transform errors and queue wait remain alongside them. Runs are brief,
deterministically paced, local and instrumented; the two-Ecore broker and client
share a host. These results are not a production capacity/SLO guarantee. Mixed
bindings are static route-local metadata, not variable message headers. Cross-
message joins, retries, JetStream, remote networks and JavaScript transport were
not measured. Runtime allocation/GC deltas cover the entire instrumented client,
not just the transformation library. All output encoding and correlation work is
included; transformations are message-atomic, publications are not transactional.

## Recommendation

The data justifies designing a public bounded byte-result path using the existing
prepared machinery for both one-program and multi-program routes. Prefer a
single-program helper for one expression; make batch reuse available when it
helps, without assuming it wins every expression family. Keep compilation outside
message handling and worker state local. Retain ordinary fallback behavior and
native semantics/resource guarantees; profile tiny/scalar-root payloads and
unsupported adapters/configurations before any automatic default switch.

No production API or default was changed. The prototype uses internal helpers
and still needs a separately reviewed API/diagnostic/limit design before adoption.

The experiment is a feasibility measurement, not production integration code.
A public bounded byte-result helper or compiled byte-input fast path needs a
separate reviewed design covering diagnostics/limits across adapters. NATS
transport, queue policy, delivery reliability and cross-message fan-in remain
application concerns. See the [workload record](../nats-workload.md).
