# Message byte-path experiment

Approved scope: native Go benchmark comparison on Bertram first, then a separate
NATS/Testcontainers integration experiment. Preserve root APIs, production
backends, resource controls and optional JavaScript execution profile.

1. Add a separate development module with a correctness-tested message plan:
   independent precompiled EvalJSON, shared PrepareJSON + individual evaluations,
   shared PrepareJSON + Batch; encode every defined output with bounded conversion.
   Apply the same per-message deadline and aggregate output cap to all variants.
   Compiled state is immutable; each worker owns destination and evaluation state.
2. Add varied bounded JSON fixture corpora with independently expected results:
   selectors, object construction, arrays, interpreter calls, missing/null,
   bindings, configured limits and registered host functions. Test ownership,
   malformed/trailing input, native errors, cancellation and output bounds.
3. Benchmark full byte-to-byte calls on Bertram, Go1.27.3 default backend, fresh
   per-message state, sizes 512/1024/8192/65536 plus selected 1MiB arrays,
   fan-out 1/4/16 and native engines; separate JavaScript baseline. Compile/setup
   outside timing; compare single-threaded latency and 1/2/4/8-worker throughput.
   Retain repeated raw results/source identity. Selected Go1.26 and jsontext
   controls can establish toolchain/decoder interactions without repeating the
   full matrix. No production adapter is selected from prepared-only timing.
4. After native measurements, add nested optional NATS module pinned to resolved
   dependency versions and an explicit server image digest. Start one ready
   Testcontainers broker outside timing; use subject-to-plan routes, bounded
   workers/queues, correlated input/output IDs, full output checks, error/drop
   counters and deadlines. Keep Core NATS semantics explicit; record offered load,
   input-to-output latency and ready-to-publish processing/queue metrics separately.
   Broker/server startup, image pulls and compilation are excluded. Docker is
   available on Bertram. No privileged host changes or global service changes.
5. Verify both modules (test/race/vet), root gates and relevant fuzzing, static
   review, raw outputs and report. Document gains, fallback penalties, queue
   behavior and limitations; do not claim broker capacity from microbenchmarks.

Decisions: synthetic varied corpus is explicit until real payloads are supplied.
Both one-program and multi-program routes matter. Fan-in correlates outputs of
one input; cross-message joins/windows and JavaScript guest pooling are excluded.
Error/missing policy is message-atomic: failed messages publish no partial outputs;
missing stays distinct from null. Byte budgets bound encoded outputs; application
queues are separately bounded. This remains development-only and makes no
hard-process-memory or performance guarantee.
