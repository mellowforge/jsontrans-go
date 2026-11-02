# Prepared documents and performance design

## Intent and success criteria

Make gnata performance the optimization target, prioritizing many compiled
expressions over each JSON document. The user chose an optional prepared-document
API and this workload explicitly. Existing APIs, JSONata 2.2.2 / JSONPath RFC 9535
semantics, ownership guarantees, and production safety defaults remain supported.

Primary benchmark: prepare one input document, evaluate 4, 16, and 64 precompiled
expressions, then discard the document. Include preparation in per-document
latency and allocation counts. Also measure preparation alone, already-prepared
evaluation, plain Go-value evaluation, compilation, and parallel reuse separately.
Use identical inputs, expressions, Go toolchain, machine, and worker count for
both libraries. First milestone: at most 2x gnata's median per-document latency
on each simple-rule batch size for the shared 1 KiB workload. Continue reducing
the gap; this is a measured acceptance target, not a promised speedup.

Pin gnata v0.6.1 in an isolated benchmark module. Its module declares Go 1.25.6;
use the installed Go 1.25.13 for both libraries with GOTOOLCHAIN=local. The main
library keeps its Go 1.25 baseline. Published gnata numbers remain contextual
references, not a substitute for the matched local comparison.

## Public contract

```go
type PrepareOptions struct { Limits *Limits }

func PrepareJSON(ctx context.Context, input []byte, options PrepareOptions) (*Document, error)

type PreparedProgram interface {
    Program
    EvalPrepared(ctx context.Context, document *Document, options EvalOptions) (Result, error)
}

func EvalPrepared(ctx context.Context, program Program, document *Document, options EvalOptions) (Result, error)
```

`Document` is opaque and immutable. Preparation copies caller bytes once,
validates the complete input, and records reusable structural and numeric
metadata. No caller-owned byte slice or mutable Go map is retained. A document
can be shared concurrently across programs and engines. Garbage collection
governs lifetime; there is no Close method or document-buffer pool.

Results remain independently owned. Internal readers may borrow prepared
storage during evaluation; public RawMessage results cannot alias it. Scalar
booleans/numbers can have zero result allocations; strings and containers may
require ownership copies. Do not introduce an unsafe borrowed-result API.

Expose read-only document access to external processor packages as well as builtins:
`Document.JSON() string` returns immutable owned JSON, `Document.Root() Node`
returns a read-only handle, and `Document.Materialize(ctx, PrepareOptions)` returns
detached JSON-shaped values with an error. Node exposes `Kind() string`,
`Member(string) (Node, bool)`, `Index(int) (Node, bool)`, `Len() int`, `Keys() []string`,
`Text() (string, bool)`, `Number() (json.Number, bool)`,
`Boolean() (bool, bool)`, and `JSON() string`. Kind names are object, array,
string, number, boolean and null. Wrong-kind operations return false (Len is 0
for scalars). Keys returns an owned slice in original member order with duplicate
names collapsed at their first position; language-specific enumeration rules
are applied by engines. A zero Node has empty Kind/JSON and failed lookups.
Returned strings are immutable views and retain their backing
document; consumers wanting independent bytes copy them. No method exposes a
mutable backing buffer or map. Text uses the project's lossless WTF-8 convention
only for isolated surrogate units. Builtins and external adapters use the same
capability; callers need not import an internal package.

Existing processors need no new method. EvalPrepared dispatches to an optional
PreparedProgram; its fallback materializes detached JSON-shaped values and calls
the existing Program.Eval. The checked-program wrapper preserves this capability
and its diagnostic behavior. Invalid nil documents/programs return errors.

## Document representation and compatibility

Use a language-neutral node arena over an owned raw buffer. Record kind, byte
span, object member order and lookup information, array children, and exact
number spelling. Build it with bounded validation; preserve last-value-wins
duplicate-key behavior, escaped names, UTF-16 surrogate handling, and integer
key enumeration. Do not substitute GJSON path syntax for either language.

Keep size, maximum nesting/collection dimensions, and JSONata binary64-overflow
metadata, allowing evaluation to enforce stricter limits without revalidating
the entire document. Preparation limits are not authority to ignore a program's
evaluation limits. A JSONPath document may contain exact numbers JSONata rejects.

Preparation and evaluation have separate work counters. MaxSteps measures actual
work performed in that call; it need not fail at the same threshold as ordinary
decoding/copying or an uncached evaluation. Cached work is not charged again.
Byte/depth/collection constraints still apply through stored summaries, and
deadlines/cancellation apply to all work actually performed. Document this
distinction and test it explicitly. Optional native expression-depth
instrumentation retains its semantics through the full-evaluator fallback.

Complex expressions initially use the existing interpreter after bounded
materialization. A document may cache one immutable canonical decoded view;
publish successful views only, never cache a caller's cancellation or a failed
initialization. Fallback passes detached data to ordinary processors and trusted
Go callbacks, preserving document immutability. Engine-specific mutable views
and unbounded expression/schema caches are excluded from this phase.

## Optimizations and boundaries

First replace per-evaluation copying of all builtins with immutable parent
bindings and evaluation-local overlays. Preserve shadowing, assignment, mutual
recursion, closure lifetimes, dynamic eval, custom functions, and concurrent Invoke.
Remove duplicate boundary validation/copies only where ownership is explicit.

At compile time classify conservative fast plans for singular member/index
access, scalar comparison/boolean combinations, and known builtin functions
including exists/type/count/length/sum. Expand to simple collection filtering
only after singular plans and their differential checks pass. Start JSONPath
with singular selectors. Unsupported expressions/shapes use full evaluation.

Use the owned arena for indexed reads. A raw reader such as GJSON is optional
supporting machinery only if matched benchmarks justify it and semantic tests
prove equivalence; the initial indexed-reader implementation is project-owned.
No dependency on another complete engine and no native/CGO code.

Guard function identity and runtime bindings before specialization. Handle
missing/null, arrays and singleton preservation, number conversion, object
order, Unicode, native errors and source positions exactly. Any unsupported
condition falls back before exposing a partial result or invoking a callback.
Explicit expression-depth instrumentation can use the legacy evaluator first.

Fast plans keep work, depth, size, deadline and cancellation checks. Simple
bounded plans may use monotonic deadline checks instead of allocating a derived
context/timer; callbacks and fallback retain context deadline behavior. No
benchmark configuration may silently disable production safety defaults.

## Validation and measurement

Run old APIs, prepared fallback, forced-fast and auto-selected paths against
independently expected results and the pinned Node reference where applicable.
Test caller-buffer mutation, result mutation, concurrent reuse, returned
callables, callback mutation, stricter post-preparation limits, failed/canceled
view construction, invalid JSON suffixes, duplicates/escapes, numeric overflow,
surrogate pairs, and fast-plan rejection of function overrides.

Run both pinned conformance suites through prepared evaluation as well as the
existing paths; retain the two explicit JSONPath errata. Expected-error fixtures
with missing raw input continue to use the existing raw API as appropriate.
Keep format, normal tests, race, vet, fuzz and reference checks mandatory.

Comparisons report preparation-inclusive latency/throughput, bytes/allocations,
retained memory, fast-path coverage, and errors. Compile outside steady-state
loops. Compare our prepare-plus-loop against gnata EvalMany with a warmed schema
plan; compare ordinary Eval and bytes APIs in separate rows. Build one rule set
per batch, validate result parity before timing, and pin all harness inputs.
Record that gnata's stock byte fast path assumes valid input whereas our
preparation validates and owns it; the primary deployment comparison includes
that cost. Also publish a validation-inclusive gnata row and steady-state
prepared rows to distinguish those guarantees.

Benchmarks cover the common 1 KiB input plus 8 KiB and 64 KiB documents, simple
and mixed-complexity rule sets, single-expression cost, and parallel reuse.
Use at least five samples and a benchmark comparison tool; keep timing thresholds
out of shared-runner CI. After broadening coverage, characterize any regression
over 10% in an existing complex-expression benchmark before accepting it.

Cost: approximately 2–4 human engineer-weeks for common paths, as a planning
range. Broader complex-expression parity is further profile-driven work.
