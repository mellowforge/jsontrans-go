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

Programs compiled through a `Transformer` receive the effective limits in
`EvalOptions.Limits` on ordinary, prepared and batch evaluation alike: the
stricter of the processor's and the program's configured ceilings, with the
program's regex timeout, then any per-call override. When every configuration
is the default and there is no override, `EvalOptions.Limits` stays nil so
engines may keep their default fast paths.

Programs that cannot accept certain evaluation limit fields can implement the
optional `EvalLimitSupport` interface, without changing `Processor` or `Program`:

```go
func (*Program) UnsupportedEvalLimits() jsontrans.LimitSet {
    return jsontrans.LimitSteps | jsontrans.LimitRegexTimeout
}
```

The eight `LimitSet` flags correspond to the eight `Limits` fields. Registry and
batch constructors snapshot this immutable declaration; unknown bits fail with
`ErrUnsupported`. Programs without the interface retain existing forwarding.
Wrappers must forward the declaration explicitly if they want this behavior.
Synthesized options omit declared unsupported fields on an independent copy.
Common input, binding and result validation still use fully resolved budgets;
the declaration does not remove common validation or context cancellation.

An explicit nonzero unsupported override fails with `ErrUnsupported`, even when
it equals the default. Zero means unspecified. Negative values remain `ErrLimit`.
Rejection precedes input conversion and callbacks. A batch checks every entry
before running expressions, reporting the first unsupported entry as a
`BatchError`; a supplied destination is untouched on this preflight failure.
Adapters must also reject unsupported settings in direct `Eval` calls and
honor context cancellation. This interface describes accepted option fields,
not a guarantee that common work counters measure work inside another runtime.

Compiled state must be immutable. Create evaluation state per call; snapshot
configuration and function registrations when compiling. Do not mutate inputs
or bindings. Honor context, validate JSON results, and reject unsupported options
instead of silently ignoring them. Preserve `errors.Is` and native diagnostics
through `jsontrans.Error` where applicable.

Programs receive detached JSON-shaped input: `map[string]any`, `[]any`,
strings, booleans, nil, and numbers as `json.Number` (from JSON bytes) or the
caller's finite Go numeric types. Strings and object keys containing isolated
surrogates arrive as `jsontrans.UTF16String` and `jsontrans.UTF16Object`; treat
them as ordinary strings and objects.

The common API only accepts JSON-compatible results. Language-specific raw
evaluation may expose function values separately. Missing is not an error and
must use `Defined=false`; null is a defined nil. Each adapter documents its own
result shape, numeric semantics, bindings, and host-function API.

A future Tengo adapter can retain compiled bytecode and create a fresh VM for
each evaluation behind this contract. Tengo is not a v1 dependency or engine.

JSONata exposes `CompileRaw`, `EvalRaw`, `Callable`, and `Invoke` for language
values such as functions. Invoke creates a fresh bounded evaluation so returned
closures and transforms remain usable after their original evaluation finishes.
Its limits default to those of the engine that created the function, and each
call returns a detached result owned by the caller. Argument conversion uses
`MaxInputBytes`; result conversion uses `MaxOutputBytes`. Raw results preserve
non-finite language numbers, functions, and missing values. JSON-shaped results
are charged their encoded JSON size; language-only scalar values consume one
bounded token each (a top-level missing result is returned without conversion). Unrepresentable results, such as cycles or canonical key
collisions, fail with `ErrConversion`. Registered host results and JSON inputs
remain strictly validated.
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

On the first error, evaluation stops and returns nil results. A failure while an
expression runs, including the overall deadline or cancellation noticed before
it starts or as it returns, is a `*BatchError` with that zero-based Index and the
wrapped Cause. Failures not tied to an expression (nil arguments, invalid
options, program count, document limits) are returned unwrapped. Populated destination slots are
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

For callers that need encoded bytes, `jsontrans.EvalPreparedJSON` and
`Batch.EvalPreparedJSON` reuse the same dispatch and limit forwarding without
requiring another adapter interface. Each expression is encoded immediately
under its continuing deadline. Batch destinations are `[]JSONResult`; capacity
can be reused, while each output byte buffer remains independently owned. See
[prepared JSON results](prepared-json-results.md) for output accounting and
failure semantics.

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
