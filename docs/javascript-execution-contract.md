# Owned JavaScript execution contract

This contract describes the optional **production-supported v1.0.0** processor module
`github.com/mellowforge/jsontrans-go/javascript`. The assessment in
`experiments/javascript` forwards its owned backend to this module. The [production approval](javascript-production-readiness.md) covers this
restricted profile; native root dependencies are unchanged.

## Construction and ownership

```go
engine, err := javascript.New(ctx, javascript.Config{})
if err != nil { return err }
defer engine.Close()
transformer, err := jsontrans.New(map[string]jsontrans.Processor{"js": engine})
if err != nil { return err }
result, err := transformer.Process(ctx, "js", input,
    `(input, bindings) => ({total: input.price * bindings.quantity})`,
    jsontrans.EvalOptions{Bindings: map[string]any{"quantity": 2}},
)
```

The initialization context applies only while constructing the engine. Canceling
it after success does not close the engine. Initialization checks the caller
context before and after trusted Wasm compilation, without claiming hard
preemption inside that compiler. Rule timeouts apply to rule compilation and
execution, rather than initialization. A failed constructor releases its resources.
The assessment constructors `New(OwnedQuickJS, config)` / `NewOwned(ctx, config)`
remain forwarding compatibility helpers; applications use public New(ctx, config).

The application owns the engine; transformers borrow it. Programs share its
immutable artifacts and cannot outlive its open lifetime. Close stops admission,
cancels admitted calls and waits for complete host conversion, guest execution,
diagnostic processing and guest cleanup before closing the runtime/cache.
Concurrent/repeated Close calls return the same cleanup result. Close has no
hard deadline and does not guarantee immediate release of physical RSS.

New calls after closure begins fail with `errors.Is(err, javascript.ErrClosed)`
before input conversion. In-flight calls either finish before observing closure
or preserve ErrClosed; contributing caller context cancellation remains reachable
through errors.Is. Nil contexts are errors. Compiled programs can be reused
concurrently; each Eval gets fresh guest state and independent detached data.

## Source, time and limits

Compile parses and compiles the expression without executing its initializer.
A function-producing IIFE initializes during each Eval. A valid non-function
expression can compile but fails with ErrConversion when evaluated. Promises
are rejected, async jobs are not pumped, and imports/host APIs are absent.
Code created through JS eval/Function stays inside the same guest/deadline.

The profile uses a **UTC time snapshot per guest**: Date.now/new Date() do not
advance during one evaluation. Math.random is guest-local, timestamp-seeded and
noncryptographic, without uniqueness/replay guarantees. No host clock, entropy,
filesystem, network or callback service is imported.

Supported per-call timeout overrides replace configured defaults, raising or
lowering them; caller context can stop execution sooner. Compilation, execution,
regexes and diagnostic coercion remain interruptible in Wasm. Go conversion uses
cooperative checks. Explicit MaxSteps/RegexTimeout and GuestStackBytes are
unsupported, with no JS instruction, separate regex or recursion quota promised.
Common/bridge conversion budgets still apply; registry metadata only masks
unsupported synthesized settings, never explicit unsupported requests.

Default guest linear memory is 64 MiB, configurable from 2–64 MiB in 64 KiB pages.
The C stack reservation is fixed at 1 MiB; transfers cap at 16 MiB and compiled
bytecode at 4 MiB. These do not bound total host memory, concurrent guests or RSS.
Inputs use detached JSON values and binary64 JS numbers. Output retains strict
JSON conversion: missing is Defined=false; null is a defined nil. Unsupported
results, cycles, accessors and nonfinite numbers fail rather than being silently
coerced into JSON.

## Diagnostics

```go
var diagnostic *jsontrans.Error
var exception *javascript.Exception
if errors.As(err, &diagnostic) {
    // Processor is "javascript"; Phase identifies compile/input/bindings/evaluate/output.
}
if errors.As(err, &exception) {
    // Name, Message, Stack, Location, Truncated; all text is bounded.
}
if errors.Is(err, javascript.ErrClosed) { /* engine was closed */ }
if errors.Is(err, context.DeadlineExceeded) { /* execution deadline */ }
```

Native exception names/messages are retained as reported data. JS has no stable
native code catalog, so jsontrans.Error.Code is empty; compile errors are not
all mislabeled SyntaxError. Constructor/Close failures are ordinary wrapped
errors because neither is an expression-processing phase.

Guest ABI v3 and the trusted bridge distinguish JS throws from genuine conversion and
budget failures. Throwing a marker such as `JSONTRANS_LIMIT: ...`, changing Error
names/prototypes or returning a protocol-shaped object cannot acquire a trusted
resource/conversion sentinel. Proxy exceptions during conversion remain JS
throws attributed to output. Wasm traps/protocol failures retain their causes;
message text alone never makes them ErrLimit.

Private traversal closures/state and failure/fallback tuples are reserved before
user execution. Trusted failures return reserved outcomes instead of throwing
tokens: QuickJS backtrace allocation under heap exhaustion can replace a thrown
token with OOM. Classification and phase cross the C boundary independently of
allocating diagnostic text, including when the guest heap is exhausted.

OOM and InternalError are terminal: a catch block cannot resume the evaluation,
no result is returned, and the next call creates an independent healthy guest.
This applies to source initializers, rule execution, output traversal and
diagnostic processing. A private, preallocated null-prototype phase record is
retained by C and passed only to the trusted bridge; C reads its existing own
integer data slot without getters or allocation when a terminal error bypasses
JavaScript catch blocks. Diagnostic JSON remains version 2.

The owned QuickJS patch reserves an uncatchable OOM error in full contexts,
with GC marking and cleanup. It prevents upstream's catchable-null fallback
when the heap cannot allocate an error object. Native errors keep available
typed fields or a bounded fallback; their names, messages and uncatchability
do not establish trusted quota provenance. Existing trusted initialization and
conversion/budget failures retain their classifications.

Encoded diagnostics cap at 64 KiB independently of ordinary output size.
Truncated records set Truncated. Formatting runs inside the same interruptible
guest under the remaining deadline. Secondary formatting failures use a bounded
generic description of the original throw. Returned errors own their text, with
lossless WTF-8 for isolated UTF-16 units, and retain no guest handles or input
references. Text can contain user data; there is no redaction guarantee.

### Source coordinates

Rules and bridge use distinct fixed filenames. The `(\n` rule wrapper shifts
native line numbers by one; mapped rule locations refer to the original expression.
Runtime locations set Reported=true: stacks and Error.prepareStackTrace are
user-modifiable and plausible forged frames cannot be authenticated.

The pinned v0.18 compiler reports native columns beyond column 1; ASCII, Unicode
and CRLF cases are calibrated separately. Regex syntax errors can omit locations.
This implementation preserves available native columns and mapped lines, with **ByteOffset=-1 and
common Error.Position=0**. It does not invent precise token offsets. Malformed,
bridge, wrapper-only, generated or out-of-range locations are omitted. More
precise byte offsets require independent coordinate-unit verification; bundle source maps and
authenticated runtime frames remain future work. Line mapping currently supports
LF/CRLF; standalone CR and Unicode line separators may omit Location.

## Evidence and navigation

The full owned assessment contains the original 18 probes plus lifecycle,
diagnostic classification and diagnostic-location cases. All owned cases must
report PASS in strict mode. Historical fixture/results directories remain intact.

Guest ABI 3 artifact: 1,034,164 bytes; SHA-256
`31aaf1d757a9071dafae75c424c49fb91d0ba70c455d5bde37afd37229b38dee`.
Canonical assets/notices live in `javascript/internal/quickjswasm`; QuickJS-ng
v0.18.0 and the separately hashed owned OOM patch are recorded in the manifest.
WASI SDK remains 34.0. Rebuild/hash/import gates and Linux
cross-host reproducibility protect the embedded boundary.
See the [upgrade and terminal-error record](javascript-quickjs-upgrade.md) for
the source patch, security evidence, measurements and remaining release gates.

See the [processor architecture atlas](processor-architecture.md),
[production acceptance record](javascript-production-readiness.md),
and [assessment commands](../experiments/javascript/README.md).
