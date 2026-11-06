# High-throughput message transformations

## Workload

A NATS subscriber receives a JSON message, selects predefined transformations
by subject, transforms the message, and hands the result to the next stage.
Both patterns matter:

- One incoming message runs through one precompiled transformation.
- One incoming message fans out to several precompiled transformations.

Multiple subjects and transformer configurations can be active concurrently.
Each message is fresh input: reuse compiled expressions across messages, not a
prepared document containing a previous message. Reuse one prepared document
within a message when several expressions consume that same input.

When a message contains a complete JSON value, a continuous subscription is a
stream of messages; it does not itself require an incremental JSON-document
parser. Faster byte parsing and avoiding unnecessary materialization are the
immediate opportunities for this workload.

NATS remains an application integration. The core library does not acquire a
NATS dependency, own subscriptions, publish outputs, correlate separate messages,
or define acknowledgement/retry policy.

## Setup and hot paths

Construct processors/registries at application startup, compile expressions,
and publish an immutable subject-to-program or subject-to-batch routing table.
Different subjects may use different processor configurations. Compiled programs
are safe for concurrent evaluations, so a separate instance per message or per
worker is unnecessary. Configuration differences, rather than concurrency alone,
are a reason to create separate processors/transformer registries. Route updates
should replace a snapshot rather than mutate a table workers are reading.

For one expression, the existing byte-to-byte path is `EvalJSON(ctx, program,
msg.Data, options)`. It includes input decoding, evaluation, result validation
and JSON output. `ProcessJSON` also compiles the expression, so repeated calls
with a fixed expression do extra work; compile once outside message handling.
`Defined=false` and defined JSON null must remain distinct in application output
policy. Errors and cancellation must be handled before publishing a result.

For several expressions over one message, compile a `Batch` once, call
`PrepareJSON` once per arriving message, and call `Batch.EvalPrepared` with
per-worker result storage. The batch evaluates its ordered rule set within the
call; it is not a parallel executor. Independent workers can process different
messages concurrently using the same batch. Result destinations, bindings and
other mutable scratch must not be shared between simultaneous evaluations.

Prepared and batch APIs currently return Go values. A NATS republisher also
needs JSON output encoding and its associated ownership, byte limits and error
handling. Comparing a prepared Go-value result with `EvalJSON` alone therefore
is not a complete byte-to-byte comparison. Container-producing transformations,
large output and input validation must remain in the timed message path.

Fan-out within one process can share an immutable Document. Separate subscribers
or processes do not share its index automatically. Fan-in across different
messages needs application-level correlation, ordering, windows and aggregation;
it is distinct from merging the results of expressions on one message.

## Dispatch and concurrency

The NATS Go client's ordinary asynchronous callback delivery is serialized
within a subscription; different subscriptions have independent delivery loops.
A slow callback can therefore limit a hot subject even when other cores are
idle. See the [NATS Go client delivery implementation](https://github.com/nats-io/nats.go/blob/main/nats.go).

Use an explicit bounded worker/queue strategy and decide where ordering is
required. A shared worker pool may reorder completion; subject/key partitioning
or result resequencing is needed when ordered output is required. Queue groups
distribute messages between group members; independent subscriptions provide
broadcast fan-out. Adding subscriptions or a worker pool does not itself define
how multiple transformed outputs should be joined.

Measure queue depth, pending bytes, processing errors and the selected overflow
policy. Core NATS slow-consumer limits can drop messages; JetStream delivery
and acknowledgement choices have different semantics. These transport choices
belong to the integration. Library execution budgets do not bound application
queues or total process memory. Registered Go functions must still cooperate
with cancellation and be safe for concurrent evaluations.

## What the current experiment measures

The Go 1.27 experiment reuses compiled expressions. Its `eval_json` cases
repeat full byte-to-byte decoding/evaluation/encoding for every input; they do
not reuse a decoded document. Its `prepare_batch` cases include preparation on
every message, then evaluate 1/16/64 rules and return Go values. Its `reuse`
cases omit preparation and are only the within-message evaluation component.

The fixed fixtures reuse identical payload bytes to isolate library costs.
They are single-threaded warm microbenchmarks, not realistic message-shape
variation, an NATS subscription benchmark, sustained multi-worker throughput,
or p95/p99 end-to-end latency. Their medians must not be advertised as NATS
messages/second. No NATS server or client was run for the Go 1.27 assessment.

## Next performance experiment

Keep both one-expression and multi-expression paths in the acceptance matrix.
Use representative transformations producing the actual JSON output, including
scalar selection, object construction, array projection/filtering, and cases
that trigger interpreter materialization. Vary payload size, item count, string
escaping, schema/shape and output expansion independently. Include per-message
bindings for subject/header metadata, configured limits and registered host
functions: these configurations can change fast-plan eligibility and must not be
represented only by default-configuration selector benchmarks.

Compare the existing `EvalJSON` path against a candidate compiled-byte fast path
that validates each message once, uses the native prepared plan when applicable,
and encodes the required result. The candidate must retain full input validation,
numeric/missing/null semantics, errors, budgets and owned results. It is not yet
implemented, and should be compared byte-to-byte rather than inferred from
prepared-only timings. Include preparation/index allocation and encoding cost.

For one-message/many-expressions, compare a shared prepared document and batch
with independent calls, including encoding each required output. For both
patterns, test multiple subject-specific routes and bounded worker counts;
compiled state may be shared, while evaluation state and output buffers stay
local. Start with library-only routed/concurrent benchmarks to isolate CPU and
allocations, then add a separate NATS transport harness.

Measure sustained messages/second, bytes/second, B/op, allocations, GC work,
queue growth, and p50/p95/p99 latency from receipt to ready-to-publish output.
Track processing latency separately from queue wait and broker/network time.
Specify offered load, worker count, ordering requirements, missing/error policy,
and delivery mode before treating an end-to-end result as a capacity claim.

Run the JavaScript processor separately: the approved execution profile creates
an independent guest per evaluation. Go JSON decoder improvements do not remove
that initialization/execution cost; runtime pooling would need a separate design
and security review. No processor behavior or pooling policy changes as part of
this workload record.
