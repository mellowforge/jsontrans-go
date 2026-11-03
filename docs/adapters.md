# Adding a processor

An external Go package implements `jsontrans.Processor` and returns a
`jsontrans.Program` from `Compile`. Programs accept JSON-shaped values and
evaluation options and return `jsontrans.Result`. Constructors and syntax stay
in the adapter package; the root package never imports adapters.

Register instances explicitly with `jsontrans.New(map[string]jsontrans.Processor{
"custom": processor})`. The transformer snapshots the map. Use an optional
`Limits() jsontrans.Limits` method on engines and programs to expose configured
defaults to common wrappers. Configured limits are defaults, not hard maximums:
`EvalOptions.Limits` replaces each nonzero field for one evaluation, raising or
lowering it. Zero fields in engine options select library defaults; zero fields
in an override keep the configured value.

Compiled state must be immutable. Create evaluation state per call; snapshot
configuration and function registrations when compiling. Do not mutate inputs
or bindings. Honor context, validate JSON results, and reject unsupported options
instead of silently ignoring them. Preserve `errors.Is` and native diagnostics
through `jsontrans.Error` where applicable.

The common API only accepts JSON-compatible results. Language-specific raw
evaluation may expose function values separately. Missing is not an error and
must use `Defined=false`; null is a defined nil. Each adapter documents its own
result shape, numeric semantics, bindings, and host-function API.

A future Tengo adapter can retain compiled bytecode and create a fresh VM for
each evaluation behind this contract. Tengo is not a v1 dependency or engine.

JSONata exposes `CompileRaw`, `EvalRaw`, `Callable`, and `Invoke` for language
values such as functions. Invoke creates a fresh bounded evaluation so returned
closures and transforms remain usable after their original evaluation finishes.
Its limits default to those of the engine that created the function.
Register host functions with `jsonata.WithFunction`. JSONata host results are
validated and detached: Go numeric types become numbers, while unsupported
types, cycles and non-finite numbers fail with `ErrConversion`. Plain host
errors report the function name as `Token`, and a registered function replaces
a built-in of the same name, as in JSONata. JSONPath's `WithFunction` uses its
RFC Value/Logical/Nodes type declarations and cannot replace standard functions.
In both engines, options apply only while `New` builds the engine, and repeated
`WithLimits` options refine earlier ones. These are language-specific extension
APIs, separate from the common JSON contract.

Processors can optionally implement `PreparedProgram.EvalPrepared` alongside
`Program.Eval`. The public `Document` and `Node` types provide read-only indexed
access; adapters do not need access to internal packages. Use `Root`, `Member`,
`Index`, `Kind`, `Text`, `Number`, `Boolean`, and `JSON` to inspect values. Kind
names are `object`, `array`, `string`, `number`, `boolean`, and `null`.
`Number` preserves the exact spelling; `Binary64` returns false on overflow.
Wrong-kind operations return false; `Len` is zero for scalars. `Keys` returns an
owned slice in input order, with duplicate names collapsed at their first
position and their last value retained. Apply language-specific key ordering in
the adapter. `Text` uses lossless WTF-8 for isolated surrogate units.

Node strings are immutable views that retain document storage; copy them when
returning independently owned results. `Materialize(ctx, PrepareOptions)` makes
a detached JSON-shaped tree. The root `EvalPrepared` helper validates input
dimensions, snapshots bindings, and validates/detaches results. Registry wrappers
preserve prepared dispatch and diagnostics. Programs without this capability
work through materialization and their existing `Eval` method.

Enforce compilation/evaluation work, depth, collection, output-size and context
limits inside adapters. Preparation and evaluation have separate step budgets:
charge the work actually performed, rather than recharging eliminated decoding.
Stricter evaluation byte/depth/collection limits must still constrain the stored
document. Document lifetime is managed by Go's garbage collector; there is no
Close method or automatic document/expression cache.

`NewBatch([]Program)` snapshots a program list; `Transformer.CompileBatch`
compiles a list for one registered processor. Batches execute in order and reuse
destination capacity through `EvalPrepared(ctx, document, dst, options)`.
The returned length equals the program count. Use a separate destination for
each concurrent call, and do not alias destination storage with bindings.
Passing nil destination requests fresh result storage. Results own their data
independently of the document and of other batch results.

On the first error, evaluation stops, returns nil results and a `*BatchError`
with zero-based Index and wrapped Cause. Populated destination slots are
cleared; other destination contents after an error are unspecified. Native
diagnostics and `errors.Is`/`errors.As` continue through the wrapper. External
programs get independent binding snapshots and the usual result validation.

Batch options bound the entire call, including input dimensions, program count,
deadline and aggregate output bytes. Each expression also keeps its configured
limits and work counter. Aggregate output counts the encoded bytes of defined
values; missing values contribute zero bytes. Default batch Timeout is one
second. Constructors cap program count at the default collection limit before
allocation; CompileBatch bounds the entire compilation with a shared deadline.
No expressions run concurrently inside a batch.

```go
batch, err := transformer.CompileBatch(ctx, "jsonata", []string{
    "name",
    "$sum(prices)",
})
if err != nil { return err }
document, err := jsontrans.PrepareJSON(ctx, input, jsontrans.PrepareOptions{})
if err != nil { return err }
destination := make([]jsontrans.Result, 2)
results, err := batch.EvalPrepared(ctx, document, destination, jsontrans.EvalOptions{})
// Reuse destination on the next call after consuming results.
```
