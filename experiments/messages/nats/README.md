# Core NATS/Testcontainers transport experiment

Separate development module; production dependencies and APIs are unchanged.
Requires Docker for explicit integration tests and the load command. Ordinary
unit tests do not start Docker. Testcontainers-Go/NATS module0.45.0 and NATS Go
client1.55.0 are pinned in go.mod. The official server2.15.2-alpine image is
pinned by its registry digest in DefaultImage, resolved by docker pull/inspect
before timing.

The broker is Core NATS: no persistence, TLS, authentication or acknowledgements.
It is bound only to loopback on a dynamic host port, quiet (module default debug/
trace flags overridden), limited to512MiB, GOMAXPROCS2 and, on Bertram, Ecores
16/17. Client native code is allowed Pcores0,2,4,6,8,10,12,14. GOMAXPROCS equals
worker count1/4/8. No host-wide CPU/frequency/service settings change.

One broker is started and becomes ready before the suite. Each case creates two
connections and eight subject-specific precompiled plans, flushes subscriptions,
and handles fresh message bytes through a bounded worker queue. Every input is
assigned a correlation ID. Each defined transformed output is published with ID
and expression index; the receiver checks its full JSON value, rejects duplicate/
unknown outputs and measures arrival of the complete fan-out. Input bytes are
copied at enqueue so asynchronous workers own them. Worker scratch is local.
Correlation headers are not passed to expressions; bindings in mixed routes are
static route-local fixture metadata, not variable per-message headers.

The message worker now calls public `jsontrans.EvalPreparedJSON` for the
`prepared` strategy and `Batch.EvalPreparedJSON` for `batch`; `direct` retains
precompiled `jsontrans.EvalJSON`. Preparation and encoding remain inside each
message's timed processing window. Reports identify these entry points with
`api_profile: public-prepared-json-v1` and an `entry_points` map in metadata.
Historical reports from 2026-11-06 used the earlier internal encoding prototype
and retain their original source identities.

The [2026-11-07 public API validation](../../../docs/benchmarks/message-nats-public-api-2026-11-07/README.md)
records the completed three-round run, full counters, limitations and native
Go 1.26/1.27 checks.

Cases: JSONata scalar, JSONata object, and eight mixed JSONata/JSONPath routes
covering scalar/object/array/bindings/host functions; fan-out1/4/16, workers1/4/8,
paced offered rates1000/5000 inputs/sec. Default3 rounds rotate case/strategy order;
32-message warmups are excluded. Setup/compilation/connections/image pulls and
broker startup are outside timing. Each publish window is1second by default.
End-to-end timing starts at actual publish, not the ideal scheduled arrival.
Clock/pacing lateness and achieved offered rate are retained; sleep-based pacing
may produce catch-up bursts and is not a Poisson traffic model.

The application drops when its input queue is full (64slots/worker), and records
that loss. It never waits for completed messages to pace publication. Core NATS
pending drops, async errors, publish failures, unaccounted inputs, wrong outputs
and incomplete fan-out make a run invalid. App-queue rejection is a valid
measurement of overload, but not lossless service. Input/output barrier messages
on the same connections drain preceding frames; there are no acknowledgements
for individual application messages. Transformation errors publish no outputs;
multiple output publications are not a broker transaction.

Reports include total processing throughput, requested/actual offered load,
processing/queue wait/end-to-end p50/p95/p99, completion and all loss/error counts.
End-to-end percentiles include completed messages only; do not rank a lossy run
as an SLO improvement. Queue/processing distributions include processed inputs,
including failed transformations. Runtime allocation/GC deltas include the entire
instrumented client pipeline and differ from native library-only B/op. Test runs
are brief, local, unencrypted and instrumented; they do not establish production
capacity or internet/network performance. Actual JSON sizes/expressions are those
of the parent deterministic corpus; targets are not exact sizes.

```sh
go test ./...
go test -race ./...
go vet ./...
go test -tags=integration -run '^TestContainerRoutes$' -count=1 ./...
go build -o /tmp/message-load ./cmd/load
# Run on Bertram; another host needs suitable affinity/CPU settings.
taskset -c 0,2,4,6,8,10,12,14 /tmp/message-load \
  -output /data/ci/message-nats-smoke -smoke -rounds 1
# Start a new ready broker and run the full matrix:
taskset -c 0,2,4,6,8,10,12,14 /tmp/message-load \
  -output /data/ci/message-nats-results -rounds 3 -duration 1s
```

Use an empty/new output directory. Configured broker containers are removed on
exit with bounded cleanup; Testcontainers also manages its reaper. Metadata
records the image/build/container settings, successful completion and cleanup.
Any report persistence error makes the command fail. Detailed native source,
conformance and fuzz verification remains in the parent experiment. This harness
has no JavaScript transport optimization or guest pooling.
