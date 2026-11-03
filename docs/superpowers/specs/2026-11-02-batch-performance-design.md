# Compiled batches and allocation reduction

User approval: “let's try your proposal” after the recommendation to add batch
evaluation, remove duplicate native-result copies, compact the document index,
and expand compiled array operations. Prioritize batch/copy costs, then arena
costs. Retain the existing language/ownership/resource contracts.

Add `NewBatch(programs []Program) (*Batch, error)` and
`(*Transformer).CompileBatch(ctx, processorName, expressions []string) (*Batch, error)`.
`(*Batch).EvalPrepared(ctx, document, dst []Result, options EvalOptions)
([]Result, error)` overwrites/reuses dst capacity and returns results in program
order. Compiled batches snapshot the program slice and may be evaluated
concurrently with independent destinations. They support mixed external/native
programs. Empty batches return an empty result; nil programs/doc/context fail.

Evaluation is sequential and fails at the first error, returning nil results
and `*BatchError{Index, Cause}` with Unwrap. A failed evaluation clears populated
destination slots; callers must not rely on the destination contents after
failure. Each result owns mutable data independently of the document and other
results. Bindings are validated/snapshotted independently for every expression
that can invoke external code. The destination and bindings must not alias.

One outer scope bounds the entire batch using DefaultLimits plus options.Limits
(Timeout defaults to one second). Each expression also retains its configured
limits and per-expression counters. The outer deadline may therefore stop the
batch before another expression starts. A batch's program count and aggregate
output are bounded by its MaxCollectionItems and MaxOutputBytes respectively;
per-expression output limits remain effective. Outer input/depth/collection
dimensions also apply before allocating destination or plan scratch. Aggregate
output is the sum of encoded defined-value bytes (missing contributes zero).
Constructors cap program count at DefaultLimits.MaxCollectionItems before
allocation; NewBatch uses a default compilation deadline and CompileBatch uses
one configured deadline across all compilation and optional planning. No
implicit parallel execution.

Introduce an internal native prepared evaluator capability with an immutable,
opaque handler bound to the exact owning compiled-program identity. An external
adapter cannot forge it or inherit trust merely by embedding a native program
and overriding public evaluation. Its request supplies the already validated
document, owned bindings, resolved limits, and evaluation-local Budget. Native
handlers guarantee detached validated JSON results, so root wrappers do not
clone them again. Ordinary external adapters retain root validation/cloning.
Direct native EvalPrepared delegates to the common helper so guarantees remain
identical. Preserve processor/native error diagnostics through registry wrappers.

Compact document metadata into shared arenas and offsets. Preserve exact
numbers, source member ordering, last-duplicate values/first-key position,
isolated surrogates, overflow metadata and read-only public Node behavior. No
unsafe borrowing, global caches, global pools, or new runtime dependencies.

Expand native JSONata plans for nested object-array projections and simple
filters/aggregate loops only with differential tests and bounded accounting.
Share deterministic singular path prefixes across batch expressions where it
actually helps, using evaluation-local scratch (never document/global mutable
caches). Complex shapes, overrides, bindings and instrumentation retain safe
fallback. Retain all source positions, language shapes and native errors.

Measure verified 1/8/64 KiB simple and mixed batches of 4/16/64 against the prior
prepared-loop implementation and warmed gnata. Include preparation and allocated
bytes/objects; report reused destination and freshly allocated output separately.
Keep a non-repeating rule fixture to expose prefix sharing without exploiting
the repeated baseline expressions. No promised speedup or relaxed safety limits.

Existing uncommitted work belongs to the prior approved phase; preserve it.
Use /private/tmp/jsontrans-before-batch as its runtime source snapshot. Changes
remain local; no remote publication/release.
