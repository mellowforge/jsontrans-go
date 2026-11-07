# Prepared JSON results

**Goal:** Add bounded production byte-result APIs for single transformations and
fan-out, preserving every existing API and its defaults.

**Architecture:** `EvalPreparedJSON(ctx, program, document, options)` and
`Batch.EvalPreparedJSON(ctx, document, dst, options)` return existing `JSONResult`
values. Reuse prepared dispatch, native batch plans, and the bounded JSON encoder.
No required adapter interface changes, engine imports, runtime pooling, or Go
version change.

**Tech:** Go 1.26, existing internal document, budget, prepared, and jsonvalue packages.

**Spec:** This document records the approved additive API contract. Results own
their encoded bytes; missing is undefined with nil bytes, null is defined `null`.
Numeric and Unicode behavior follows existing engine semantics. Compilation and
document preparation remain separate. Evaluation and encoding share a deadline;
per-expression configured limits apply throughout. Batch additionally bounds
expression count and the sum of actual encoded output bytes, returning no partial
results. Preflight errors leave dst untouched; runtime failures clear populated
slots and carry the failing expression index. Destination slice capacity may be
reused, but prior output byte buffers are never overwritten. Independent calls
are concurrent-safe with separate destinations. Registry and native diagnostics,
unsupported-limit rejection, and external callback lifetime guarantees survive.

**Global constraints:** Existing APIs retain behavior. Preserve input ownership,
registry/program limit intersections, native language semantics, cancellation,
and optional JavaScript execution controls. No NATS production dependency.

**Review focus:** Deadline coverage after child evaluation scopes close; real
encoded aggregate bytes; destination clearing and byte ownership; limit forwarding
and external fallback; errors and native diagnostics; concurrency.

## Task 1: Contract and implementation

Write failing public API tests for result shapes, parity, ownership, limits,
cancellation, diagnostics, and destination handling. Add shared batch preflight
and byte encoding dispatch with local execution scopes. Run focused tests red
then green; retain existing prepared tests.

## Task 2: Integration, documentation, and performance

Exercise optional JavaScript fallback and unsupported limits. Add examples and
API documentation, bounded fuzz coverage and focused byte-result benchmarks.
Check performance on Bertram without changing existing defaults.

## Task 3: Verification and review

Run formatting, root/JavaScript tests, race detection, vet, and relevant bounded
fuzzing. Obtain one fresh read-only review; resolve findings with regression
tests. Record evidence, then commit locally. Publishing is outside this task.
