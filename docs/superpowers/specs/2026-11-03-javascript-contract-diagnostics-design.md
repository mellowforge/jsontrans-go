# JavaScript execution contract and diagnostics

Status: proposed for review; this document authorizes no production release.
First implementation PR after registry integration, based on merged `b03b7c6`.

## Intent and scope

Make the owned QuickJS/Wazero experiment's execution and error behavior precise
and testable before extracting a public module. Keep the work inside
`experiments/javascript`. Implement the contract here; package extraction,
production security approval and release are later PRs.

The owned backend passes all 18 recorded probes. It already has fresh guest
state, zero host imports, guest memory caps and cancellation. Remaining concrete
gaps include string-based error classification, blanket SyntaxError labels for
compile failures, missing source information, initialization without a caller
context, and no public guarantee that Close waits for complete adapter calls.
The internal runtime also reapplies its configured timeout during Eval, which
can prevent a supported per-call timeout override from raising that default.

The following API and behavioral choices are proposals by the author, based on
the approved synchronous processor direction, rather than prior user decisions.

## Constraints

- Go 1.26 or newer; MIT; CGO_ENABLED=0 application builds.
- Root Processor, Program, EvalOptions and Error layouts remain unchanged.
- JSONPath and JSONata remain native; root dependencies remain unchanged.
- Only the owned backend gains the new contract. Original QJS/Goja remain
  historical assessment baselines, with their existing limitations explicit.
- Fresh guest state per evaluation; immutable program artifacts; no pooling,
  sessions, imports, Node APIs, network/filesystem APIs, async job execution or
  host-function registration.
- All 18 owned probes remain required; no silent skips or weaker limits.

## Construction and ownership

Add an owned-only constructor in the experimental package:

```go
func NewOwned(ctx context.Context, config Config) (*Engine, error)
var ErrClosed error // same sentinel as internal/quickjswasm.ErrClosed
```

Keep `New(backend, config)` for assessment compatibility; its OwnedQuickJS path
calls NewOwned with context.Background. Examples for the proposed contract use
NewOwned. Nil context is an error, not a panic.

The constructor validates and snapshots Config, checks artifact integrity and
capabilities, compiles the trusted Wasm artifact and initializes engine-owned
caches. Its context applies to initialization only; canceling it after successful
return does not close the engine. On failure, all partially acquired resources
are closed. Initialization checks cancellation before and after Wasm compilation
and uses the caller context, but cannot promise a hard wall-clock stop inside
Wazero's trusted artifact compiler. The rule timeout does not impose an additional
one-second initialization deadline.

The caller owns Engine and calls Close. Programs retain their engine, share its
immutable compiled artifacts and remain valid only while it is open. The
transformer borrows processors and does not close them. There is no finalizer,
implicit global cache, serialized bytecode input or independent Program.Close.

### Lifecycle and concurrency

Lifecycle: open -> closing -> closed. The first Close atomically stops admission,
cancels admitted operations, waits for their entire adapter call (including Go
conversion, diagnostics and deferred guest cleanup), then closes owned runtime
and caches. Concurrent Close calls wait for the same completion and return the
same cleanup error. Close is idempotent, has no caller context, and does not
promise immediate physical-memory reclamation or a hard process deadline.

Compile and Eval can run concurrently while open. Admission occurs before input
conversion. New calls after closing begins return ErrClosed before conversion or
guest allocation. Calls admitted before Close either finish before observing
closure or fail with errors.Is(err, ErrClosed). Close cannot return while an
admitted call is still using runtime/cache resources. If caller cancellation or
deadline expiration also contributes, preserve its sentinel through errors.Is.
Once a call has completed, later closure must not mutate its result or error.

An already-closed engine takes precedence over input validation. On an open
engine, an already-canceled context takes precedence over conversion. Tests use
admission/guest-entry barriers rather than sleeps to assert the shutdown races.
The runtime may keep a private admission gate for direct internal use; the outer
adapter gate must span complete calls, not just Wasm exports.

## Compilation and evaluation

Compile parses/compiles the function expression without executing its top-level
initialization. A syntactically valid non-function expression can compile; Eval
rejects its evaluated non-callable value with ErrConversion. A function-producing
IIFE executes afresh inside each Eval and is covered by that evaluation's limits.
No argument binding, initializers, getters or user code run during Compile.

Input and bindings remain detached JSON-shaped values; JS numbers use binary64.
Top-level undefined is missing; null is defined. Existing strict output rules
remain: promises, functions, symbols, BigInt, cycles, nonfinite numbers, array
holes/accessors and non-JSON objects are rejected. Native JS eval/Function syntax
is not newly disabled; code created by those features runs in the same isolated
guest and budget, with source locations possibly unavailable. Async jobs are not
pumped and promises are not awaited.

The adapter uses resolved per-call Timeout, including overrides that raise a
configured default. The internal guest must not silently apply a second,
stricter configured timeout. Caller context and engine closure still stop the
call earlier. Source compilation, function initialization, execution, conversion
and diagnostics remain cancellable. Go conversion is cooperative, not a hard
process-memory or preemption guarantee.

### Time and randomness

For this PR retain the current **UTC time snapshot per fresh guest**. Date.now
and new Date() observe that snapshot throughout the evaluation. Explicit date
arguments retain ordinary JS behavior. Math.random has guest-local state seeded
by the prototype's timestamp; it is neither cryptographically secure nor promised
to be unique or reproducible across evaluations. No live clock or entropy import,
clock injection, seed option or deterministic replay API is introduced.
This is an explicit restricted execution profile, not full environment parity
with a browser or Node. A live clock requires a separate capability decision.

### Limits

Keep configured defaults and EvalLimitSupport behavior from registry PR #7.
Explicit MaxSteps and RegexTimeout remain unsupported for JS execution even if
they equal defaults. Common/bridge conversion work remains bounded. Explicit
GuestStackBytes remains unsupported. Keep the 2–64 MiB guest-memory range in
64 KiB pages, default 64 MiB, fixed 1 MiB reserved C stack, 16 MiB transfers and
4 MiB bytecode cap. Guest memory is distinct from aggregate process memory.
Do not classify every Wasm trap or out-of-memory-looking message as ErrLimit;
only trusted limit outcomes carry that sentinel. Depth remains a conversion
limit, not a JS recursion quota. Unsupported and negative controls retain their
existing error categories; this PR does not redesign shared option validation.

## Error model

Owned processing errors use the existing *jsontrans.Error with processor
`javascript` and the correct compile/input/bindings/evaluate/output phase.
Constructor and Close errors remain ordinary wrapped errors because these are
not expression-processing phases. Preserve errors.Is/As through registry,
EvalJSON, EvalPrepared and BatchError. Do not alter original baseline diagnostics.

Expose in the experimental package:

```go
type SourceLocation struct {
    Source string // "rule.js" or other engine-reported source
    Line, Column int // mapped 1-based line; engine-native column, zero if absent
    ByteOffset int // zero-based original expression byte offset; -1 if unavailable
    Reported bool // true for user-modifiable runtime diagnostic coordinates
}
type Exception struct {
    Name, Message, Stack string
    Location *SourceLocation // nil when unavailable
    Truncated bool
}
func (*Exception) Error() string
```

An *Exception describes a JS throw, including throws of primitive values. The
native name/message/stack are diagnostic data, may be changed by user code, and
are not authentication of an intrinsic error class. Strings preserve isolated
UTF-16 units using the existing lossless WTF-8 convention. Retain no guest
handles or references to input, bindings or source in returned errors; keep
bounded text only. Thrown messages may themselves contain user
data, so diagnostics are not promised to redact secrets.

JS defines no stable numeric/native error-code catalog, so jsontrans.Error.Code
is empty. Name holds reported names such as SyntaxError or TypeError; arbitrary
throws may have an empty Name. Never label all compile failures SyntaxError.
Error.Position is a mapped zero-based UTF-8 byte offset only for validated
compile-time locations, otherwise
zero as in the common API; SourceLocation.ByteOffset=-1 distinguishes unknown
from a failure at byte zero. Error.Token stays empty unless genuinely known.

### Classification without trusting messages

Replace owned-backend substring checks for JSONTRANS_LIMIT/JSONTRANS_CONVERSION
with a versioned, bounded internal diagnostic protocol. The origin and stage
are set by the owned C ABI/trusted bridge, not by thrown name/message text.

- Guest syntax/throw: *Exception, no conversion/limit sentinel by message.
- Trusted non-callable or JSON-result rejection: ErrConversion; phase evaluate
  for non-callable source, output for result conversion.
- Trusted conversion work/depth/items/bytes exhaustion: ErrLimit; also
  ErrConversion when result conversion fails, matching common output behavior.
- Input/bindings conversion: appropriate phase, preserve existing sentinels.
- Timeout/cancellation/closure: preserve context sentinel/ErrClosed; no fake JS
  exception/code. They take precedence over diagnostics if observed before return.
- Wasm trap/protocol/artifact/internal bridge failure: preserve the actual cause;
  no fabricated native syntax, conversion or resource classification.

A user can throw "JSONTRANS_LIMIT: ...", change Error.prototype, return an
object shaped like the protocol, or throw from a Proxy trap. None may impersonate
a trusted limit/conversion failure. The trusted bridge must separate invocation
from conversion stages and identify its own failures using private identity,
not a public property/name/prefix. Capture intrinsics before evaluating user
source. Arbitrary user exceptions remain exceptions; exceptions during conversion
use output phase without acquiring ErrConversion solely from their text.

Bump the internal ABI from 1 to 2. Define a strict tagged JSON diagnostic record
for non-success output with version, kind, stage and bounded name/message/stack
and optional source line/column. Go validates versions, enums, field types and
sizes; malformed records are protocol failures, never user throws or limits.
The trust boundary is the immutable bridge/C code, not JSON shape alone. Success
output keeps its existing value envelope; user values cannot bypass conversion
to become protocol records. Baseline QJS/Goja retain their legacy bridge mode.

Keep the entire encoded diagnostic payload at most 64 KiB, independent of a
small MaxOutputBytes; ordinary result limits still apply separately. Text fields
have a combined budget, account for JSON escaping, and set Truncated when reduced.
A constant bounded fallback is required on diagnostic allocation/extraction
failure. User-controlled coercion, stack formatting, getters and Proxy traps may
run only inside the interruptible guest, under the original remaining deadline;
never run them in Go or retry with a fresh timeout. A formatter that throws must
not erase the original thrown value with a secondary exception. If extraction
cannot safely recover fields, return a generic JS-throw diagnostic with fields
unavailable; cancellation still wins. Do not restart the guest for formatting.

### Source positions

Give rule and trusted bridge distinct fixed filenames (`rule.js`, `bridge.js`).
Use source wrapper `(\n` + expression + `\n)` so original rule lines shift by one
without changing columns. Store only bounded mapping metadata in Program.

Map a compiler rule location or the first usable reported runtime rule frame
to the original expression's line coordinates; never attribute a
bridge/generated-wrapper frame or sourceURL/eval string to the caller expression.
Compile failures must map ordinary multiline/Unicode rule syntax locations where
the pinned engine reports them. Runtime locations are best effort because stacks
and Error.prepareStackTrace can be user-controlled. They set Reported=true and
ByteOffset=-1; the common Error.Position stays zero. An in-range forged frame
can be indistinguishable from native text and may appear as reported coordinates;
there is no promise to detect such forgery. Reported data is not proof of an
execution location. Only compiler-origin coordinates, obtained before user code
runs and validated against the original expression, may acquire a byte offset.
Missing, malformed or out-of-range data yields no mapped coordinates. No external source-map
format, minified bundle mapping, arbitrary filenames or source contents in errors.

Calibrate native column units against the pinned engine using ASCII, emoji,
combining characters, LF and CRLF fixtures before converting a column to bytes.
Preserve native column data even if an offset cannot be established; document
actual units in the resulting contract. Failure to establish reliable compile-time byte
mapping must remain explicit, never silently claim complete source mapping.

## Validation and delivery

Implement inside the experiment only. Add contract regressions for all lifecycle
transitions, supported timeout overrides, clock profile, error forgery, malformed
protocols, arbitrary throws, formatter cancellation, Unicode source mapping,
small output budgets and wrapper propagation. Keep old evidence; add new probe
cases rather than replacing or skipping the existing 18.

Required checks: gofmt -l . (empty), root and nested go test ./..., go test -race
./..., go vet ./..., existing reference/conformance jobs, bounded ABI/diagnostic
fuzzing, strict full owned probe run and cross-host guest reproducibility.
Update the ABI manifest/hash and notices only for actual artifact changes; keep
source/toolchain pins unless a documented blocker requires separate review.
No performance optimization or new latency promise belongs in this PR.

## References and later work

- [Owned runtime checkpoint](../../quickjs-runtime-foundation.md)
- [Registry integration](../../registry-limit-forwarding.md)
- [Wazero ownership model](https://wazero.io/docs/how_do_compiler_functions_work/)
- Pinned QuickJS-ng source/SDK and licenses: experiments/javascript/internal/quickjswasm/guest/lock.json.

Next PR extracts a single-backend optional module using this tested contract.
Production hardening, platform stress, engine security updates and refreshed
measurements follow. A live clock, instruction/stack metering, async, modules,
pooling and prepared-input optimizations remain separate decisions.

## Design review resolution

The independent builder check identified runtime-stack provenance as the only
consequential ambiguity. Author decision: keep runtime coordinates explicitly
reported/untrusted and omit runtime byte offsets. Compile-time offsets use
validated engine-origin information. No engine patch for authenticated runtime
frames is included. This is a proposed scope choice for user review.

## Implementation calibration

The pinned engine's js_parse_error passes its coarse parse-state column (1)
rather than the token column. ASCII, Unicode and CRLF fixtures confirm that
ordinary syntax failures retain usable line numbers but no reliable byte
offsets. Regexp compilation failures can have no stack. This PR preserves native
coordinates and sets ByteOffset=-1 and common Position=0 for this pin; it does
not add an upstream source patch or guess token positions. Runtime coordinates
remain explicitly Reported. More precise compiler columns require a separately
verified engine change.
