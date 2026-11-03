# JavaScript Execution Contract and Diagnostics Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the owned experimental JS backend's execution lifecycle and structured diagnostics match a precise, tested contract before public-module extraction.

**Architecture:** Keep the multi-backend assessment package, routing owned calls through full-call lifecycle admission and structured diagnostic handling. Extend the owned ABI/bridge to distinguish trusted failures from user throws, map available rule locations, and retain fresh state for each evaluation.

**Tech Stack:** Go 1.26+, pinned QuickJS-ng C/Wasm guest, Wazero 1.9.0, existing lossless JSON utilities; no new runtime dependency.

**Spec:** [Execution contract and diagnostics design](../specs/2026-11-03-javascript-contract-diagnostics-design.md). Proposed design and plan: await user review before implementation.

## Global Constraints

- Go 1.26 or newer; MIT; CGO_ENABLED=0 application builds.
- Root Processor, Program, EvalOptions and Error layouts remain unchanged.
- JSONPath and JSONata remain native; root dependencies remain unchanged.
- Only the owned backend gains the new contract. Original QJS/Goja remain historical assessment baselines, with their existing limitations explicit.
- Fresh guest state per evaluation; immutable program artifacts; no pooling, sessions, imports, Node APIs, network/filesystem APIs, async job execution or host-function registration.
- All 18 owned probes remain required; no silent skips or weaker limits.
- Change the internal ABI to 2 with a rebuilt, pinned artifact; keep source/SDK dependency pins absent a documented blocker.
- No public-module extraction or production release in this PR.

## Review Focus

- Close racing with host conversion or error formatting must wait for complete admitted calls (Task 1).
- A raised per-call timeout must reach the guest rather than being capped again by an engine default (Task 2).
- Thrown marker strings, forged envelopes and mutated intrinsics cannot acquire trusted limit/conversion status (Task 3).
- Diagnostic allocation/coercion failure must remain bounded and cancellable, retaining the original failure category (Task 3).
- Unicode/CRLF and generated-code frames must not produce invented compiler offsets; runtime coordinates remain explicitly reported data (Task 4).

## File responsibilities

All paths below are relative to the repository root.

| Files | Responsibility |
|---|---|
| `experiments/javascript/processor.go` | Existing public experimental types/dispatch and baseline compatibility |
| New `experiments/javascript/owned.go` | Owned constructor, Compile/Eval orchestration, error phase attribution |
| New `experiments/javascript/lifecycle.go` | Whole-call admission, cancellation and idempotent shutdown |
| New `experiments/javascript/diagnostics.go` | Exception/SourceLocation, typed failure translation and rule mapping |
| `experiments/javascript/bridge.js` | Existing strict conversion; opt-in owned diagnostic mode, captured intrinsics/private failure identity |
| `experiments/javascript/internal/quickjswasm/runtime.go` | Runtime resources, named source compilation, effective guest timeout and ABI calls |
| New `experiments/javascript/internal/quickjswasm/diagnostics.go` | Bounded ABI v2 record decoding and internal Failure type |
| `experiments/javascript/internal/quickjswasm/guest/abi.c` | Trusted origin/stage, bounded native diagnostics and fixed rule/bridge filenames |
| Corresponding `*_test.go` and fuzz files | Focused contract regressions, hostile inputs and ABI validation |
| Guest `quickjs.wasm`, `guest/lock.json`, `guest/build.py`, `artifact_test.go` | Synchronized ABI v2 artifact/provenance/capability verification |
| `probes_owned.go`, `run_probes.py`, `.github/workflows/ci.yml`, docs | Full assessment acceptance, commands and accurate contract |

Implement one task at a time; commit after its focused tests pass. Keep helpers
private except the explicitly proposed experimental API. New code files should
separate lifecycle/diagnostics from processor.go rather than expanding that file.

### Task 1: Owned construction and complete-call lifecycle

**Files:** Create `experiments/javascript/owned.go`, `lifecycle.go`, `lifecycle_test.go`; modify `processor.go`, `internal/quickjswasm/runtime.go` and its lifecycle tests.

**Interfaces:**
- Produces `NewOwned(context.Context, Config) (*Engine, error)` and public `ErrClosed` aliasing the internal sentinel.
- Private `newEngineLifetime() *engineLifetime`, `(*engineLifetime).begin(context.Context) (context.Context, func(), error)` and `(*engineLifetime).close(func() error) error`.
- Engine stores an owned-only `*engineLifetime`; existing `Compile`, `Program.Eval` and `Close` signatures stay unchanged.
- Later tasks use the operation context and deferred completion returned by begin; they must not release admission before conversions/diagnostics/guest cleanup.

- [ ] Write `TestOwnedConstructorContextLifetime`: nil/canceled initialization context errors; canceling a successfully completed constructor's context leaves Compile/Eval usable; failed initialization releases resources. Keep legacy `New(OwnedQuickJS, Config)` working.
- [ ] Write `TestOwnedCloseWaitsForCompleteCalls`: channel barriers pause an admitted operation before guest entry and in diagnostic/output processing; Close cancels it, refuses new operations before input conversion, and waits for its deferred completion. Include concurrent Close calls and repeated Close with stable cleanup error.
- [ ] Write `TestOwnedClosePreservesContextCauses`: independently canceled/deadline-expired caller plus closure retains ErrClosed and any observed contributing context sentinel. An open engine with pre-canceled context does not convert cyclic input; a closed engine reports ErrClosed first.
- [ ] Run from `experiments/javascript`: `go test ./... -run 'TestOwned(Constructor|Close)' -count=1`. Confirm failures demonstrate missing NewOwned/whole-call shutdown behavior.
- [ ] Implement constructor validation/snapshot/partial cleanup in owned.go. Route only the owned branches of Compile/Eval through admission, leaving baseline branches intact. Normalize closure through the shared sentinel without discarding caller causes. First Close cancels the adapter lifetime, drains admitted calls, then closes the owned runtime/cache. Do not hold a mutex during user/Wasm execution or while waiting for calls to finish.
- [ ] In internal `New(ctx, Config)`, remove the extra rule-timeout wrapper around trusted-artifact initialization. Continue pre/post context checks; ensure its constructor context is not the engine lifetime context.
- [ ] Run focused tests and `go test -race ./... -run 'TestOwned(Constructor|Close)|TestCloseInterruptsActiveOwnedGuest' -count=1`. All must pass, without raising explicit cancellation deadlines. Commit: `feat: define owned JavaScript construction and shutdown`.

### Task 2: Effective timeouts and explicit evaluation profile

**Files:** Modify `owned.go`, `processor.go`, `internal/quickjswasm/runtime.go`; create `experiments/javascript/contract_test.go`; extend internal runtime tests and `README.md`.

**Interfaces:**
- Consumes Task 1 admission and constructor.
- Add `Timeout time.Duration` to internal `quickjswasm.Limits` (zero selects its existing configured default for direct internal calls).
- Internal `(*Engine).operation(ctx context.Context, timeout time.Duration) (context.Context, func(), error)` resolves this timeout; Compile uses configured rule timeout, Eval uses the adapter's resolved `Limits.Timeout`.
- Public Config and EvalOptions stay unchanged; no clock/RNG/stack option is added.

- [ ] Write `TestOwnedTimeoutOverrideReachesGuest`: internal lifetime test confirms supplied 200 ms is not capped by a configured 20 ms default; adapter/registry tests verify an override can both raise and lower it. Use a test hook/barrier to inspect the effective context deadline and avoid relying on how fast a busy loop runs.
- [ ] Write `TestOwnedCompileDoesNotRunInitialization`: compile `(()=>{throw new TypeError('init');})()` successfully, then Eval reports a thrown TypeError; compile `42` successfully, then Eval reports ErrConversion. A looping initializer cancels during Eval and a later independent evaluation succeeds.
- [ ] Write `TestOwnedTimeSnapshotProfile`: initialize a test guest using a known ABI timestamp, assert Date.now/new Date()/UTC behavior and the same value on repeated observations; verify fresh globals and guest-local random state. Do not assert random uniqueness, cryptographic strength or a stable sequence. Keep the test-only ABI fixture private; add no public injected clock.
- [ ] Add representative `TestOwnedStrictResultContract` cases for missing/null, Promise/function/BigInt/nonfinite/cyclic results, JSON input ownership and independent bindings. Reuse existing coverage instead of duplicating an entire conversion suite.
- [ ] Run the focused tests and confirm the raised-timeout regression fails under the previous internal default cap.
- [ ] Implement effective-timeout forwarding and evaluate-time callable/initialization semantics; document the retained snapshot/randomness profile and cooperative/guest-memory distinctions. Do not run user code to validate callable shape during Compile.
- [ ] Run `go test ./... -run 'TestOwned(Timeout|Compile|Time|Strict)' -count=1` and the actual registry regression `go test -run '^TestRegistryForwardsSupportedLimitsOnly$' -count=1`. Commit: `fix: honor owned JavaScript execution limits and profile`.

### Task 3: Authenticated bounded diagnostic protocol

**Files:** Create internal `diagnostics.go`, `diagnostics_test.go`; create experimental `diagnostics.go`, `diagnostics_test.go`; modify `owned.go`, `processor.go`, `bridge.js`, guest `abi.c`, runtime.go, artifact manifest/build/checks and Wasm.

**Interfaces:**
- Internal `type Failure struct { Kind, Stage, Name, Message, Stack, Source string; Line, Column int; Truncated bool }`, implementing `error`. Allowed kinds: `exception`, `conversion`, `limit`, `internal`; stage: `compile`, `evaluate`, `output`. Internal failures do not infer sentinels from text.
- Private `decodeFailure(ctx context.Context, data []byte) (*Failure, error)` rejects malformed/oversized/unknown-version records. Error strings preserve the existing lossless WTF-8 representation. Encoded shape: `{"version":2,"kind":"exception","stage":"evaluate","name":"TypeError","message":"...","stack":"...","truncated":false,"source":"rule.js","line":2,"column":1}`. Source/line/column are optional together; other fields required. Unknown fields, invalid kinds/stages, negative/overflowed positions and wrong field types fail as protocol errors.
- Experimental `Exception`/`SourceLocation` and `(*Exception).Error()` exactly as the spec; Task 3 may leave Location nil until Task 4.
- Private `ownedDiagnostic(phase string, err error) error` produces *jsontrans.Error{Processor:"javascript", Phase:..., Code:"", Cause:...}. It translates only typed internal failures, never string markers; phase comes from trusted stage where available.
- Bridge's existing seven arguments retain legacy behavior. Add an eighth optional owned-mode flag, passed as true only by owned C ABI; default mode keeps original baseline behavior. Kind/stage records are built inside the trusted bridge/C boundary.
- ABI version 2 retains existing export names; `abi_compile` gains a third source-kind argument, 0 rule / 1 bridge, rejecting other values. Non-success output is the structured record; success output stays bytecode or current result envelope.

- [ ] Write `TestOwnedDiagnosticClassification`: real SyntaxError, runtime TypeError, primitive/null throws, non-callable source, bridge conversion rejection, actual conversion exhaustion, host traps and constructor failures. Check correct phase, Code empty, errors.As(*Exception) for throws and correct sentinels only for trusted failures.
- [ ] Write `TestOwnedDiagnosticForgery`: throw each old marker string, forged objects/records, custom Error names and mutated Error/JSON/Reflect prototypes. Return an object shaped like the protocol as ordinary JSON. None acquires a trusted sentinel or bypasses result conversion; original QJS/Goja tests remain unchanged.
- [ ] Write `TestOwnedDiagnosticBoundsAndCancellation`: encoded payload <=65,536 bytes even with escaping/isolated surrogates and MaxOutputBytes=1; flags truncation, huge messages/stacks, failed allocations and throwing/infinite coercion/prepareStackTrace/Proxy hooks. Cancellation uses the remaining original deadline and fresh evaluations recover. Secondary formatter throws produce a bounded generic original-throw diagnostic, not a replacement exception.
- [ ] Add `TestDiagnosticProtocolValidation` for every required field/version/enum/type/size, including unknown fields and overflowed positions. Lossless UTF-16 text uses existing internal JSON decoding facilities with a bounded context-aware budget, not encoding/json replacement of isolated units.
- [ ] Run focused tests to demonstrate missing types and current marker forgery/blanket SyntaxError behavior fail.
- [ ] Implement private bridge failure identity and stage-aware try/catch. User invocation and conversion exceptions become exception records; only the bridge's own private failures become limit/conversion records. Use captured intrinsics. Build diagnostics incrementally within the encoded-byte budget, with bounded constant fallback; never build an unbounded JSON encoding before truncating it.
- [ ] Implement C compile/initializer exception extraction and ABI v2 records. Native property access/coercion remains guest-local and interruptible. Do not give user-controlled values authority over kind/stage. Add typed Go decoding/translation and preserve raw trap/protocol/context causes. In runtime output(), choose the 65,536-byte diagnostic cap based on status independently of the ordinary result limit, including MaxOutputBytes=1. Keep public names/messages as data, not a guaranteed native intrinsic classification.
- [ ] Rebuild via `python3 internal/quickjswasm/guest/build.py --output /tmp/jsontrans-contract-quickjs.wasm` from the nested module, update the embedded artifact/hash/ABI manifest and version assertions together. Verify zero imports and exports remain the expected set; retain source/toolchain pins and licenses. Rebuild a second output and compare bytes.
- [ ] Run focused tests, `go test ./...`, `go test -race ./...` in the nested module and the full strict owned assessment. Commit the coherent source/artifact change: `feat: add trusted bounded JavaScript diagnostics`.

### Task 4: Rule source mapping and common-wrapper propagation

**Files:** Modify `diagnostics.go`, `owned.go`, internal runtime.go and guest abi.c as needed; extend diagnostics/contract/registry tests and manifest/artifact if C changes.

**Interfaces:**
- Internal `(*Engine).CompileNamed(ctx context.Context, source, name string) ([]byte, error)` permits only `rule.js`/`bridge.js`; existing Compile delegates with rule.js for assessment/internal compatibility. Uses Task 3 ABI source-kind argument.
- Rule source wrapper is exactly `"(\n" + expression + "\n)"`; bridge compiles under bridge.js with no rule mapping.
- Private `mapRuleLocation(expression string, source string, line, column int, reported bool) *SourceLocation` verifies coordinates and removes the one-line wrapper prefix. Preserve raw native column. Validated compiler locations may acquire ByteOffset after calibration; runtime locations always set Reported=true and ByteOffset=-1. Unknown/generated/bridge positions cannot become original offsets.
- Program retains at most MaxExpressionBytes of immutable original expression/mapping metadata. *jsontrans.Error.Position gets a known compiler-origin byte offset, zero otherwise; no root layout change.

- [ ] Write table tests `TestOwnedRuleLocations` for first-line, multiline, emoji, combining marks, LF/CRLF and expression-boundary syntax errors. For expected known offsets use byte indexes into the original expression, not assumed native columns. Include runtime TypeError rule frame with Reported=true/ByteOffset=-1 and trusted bridge separation.
- [ ] Write `TestOwnedUnmappableLocations`: deleted/malformed stack, custom prepareStackTrace, eval/Function/sourceURL frames, impossible lines/columns and wrapper suffix errors yield no invented compiler offset. A plausible in-range forged rule frame may expose reported coordinates, but must set Reported=true/ByteOffset=-1 and common Position=0. A known source frame preserves diagnostic text even when no byte offset is available.
- [ ] Write `TestOwnedDiagnosticsThroughWrappers` using actual owned backend and errors.As/Is through direct calls, registered alias, ProcessJSON/EvalJSON, EvalPrepared and mixed batch. Verify processor javascript, precise phase, Exception fields, BatchError index, context and ErrClosed causes. No caller-owned input/bindings or prior error/result is mutated on later calls.
- [ ] Run these tests against current mapping to confirm failure. Calibrate/document the pinned QuickJS column units with the fixtures before implementing compile-time byte conversion; explicitly report unavailable offsets where engine data is insufficient.
- [ ] Implement named compilation, immutable bounded source mapping and typed error propagation. Do not parse arbitrary message substrings for source positions; use the bounded engine diagnostic source/frame data and validate against the expression. Runtime stacks remain reported, user-modifiable data, never authenticated execution locations; they never populate common byte offsets.
- [ ] Run focused tests, bounded Unicode/malformed-location fuzz cases, and existing native `errors_test.go` tests to prove the common wrapper contract is unchanged. If C changes, rebuild/hash/compare the ABI v2 artifact again. Commit: `feat: map JavaScript diagnostics to original rule source`.

### Task 5: Acceptance, documentation and PR handoff

**Files:** Modify `README.md`, owned probes, `run_probes.py`, `.github/workflows/ci.yml`, `docs/quickjs-runtime-foundation.md`; create `docs/javascript-execution-contract.md` and new dated results directory `experiments/javascript/testdata/results/contract/`.

**Interfaces:**
- New owned-only probe cases: `lifecycle-contract`, `diagnostic-classification`, `diagnostic-locations`; keep all original 18 cases. Baseline backends are not asked to pass owned-only gates.
- New `FuzzDiagnosticRecord` in internal diagnostic tests uses bounded records, context and no guest instantiation; fuzz the guest-facing path with existing owned compiler/conversion fuzzers.
- Contract document describes the actual implemented native column units/availability, API and restrictions, not an unverified promise.

- [ ] Add the three probe cases and protocol fuzz seeds for malformed/missing fields, oversized escaped payload, UTF-16, versions, enum forgery and integer overflow. Add scripts' owned-only case selection without changing historical reports.
- [ ] Write usage/error examples with NewOwned(ctx,...), defer Close, registry borrowing, errors.As(*Exception) and errors.Is(ErrClosed/context/limit). Explain compile-only initialization, snapshot time, nonsecure RNG, unsupported controls and complete-call shutdown.
- [ ] Run `gofmt -l .` (empty) and `git diff --check`. From root run `go test ./...`, `go test -race ./...`, `go vet ./...`, and `go test -tags oracle ./conformance ./jsonata` with the existing pinned Node oracle installed. From `experiments/javascript` run `go test ./...`, `go test -race ./...`, `go vet ./...`.
- [ ] From the nested module run `go test ./internal/quickjswasm -run '^$' -fuzz '^FuzzDiagnosticRecord$' -fuzztime=1000x -parallel=2`, existing `FuzzOwnedCompile` (200x), and `FuzzOwnedStringConversion` (200x). All must pass; do not write implementation-mirroring tests or expand fuzzing without a reason.
- [ ] Build `CGO_ENABLED=0 go build -o /tmp/jsontrans-contract-assess ./cmd/assess`; run `python3 run_probes.py --binary /tmp/jsontrans-contract-assess --backend owned-quickjs --strict --output testdata/results/contract/probes.json`. Require all 21 owned cases, no skipped failures. Keep full owned CI strict and Linux reproducibility; update only to reflect new cases/artifact.
- [ ] Review the complete change independently once, resolve blocking findings with regressions, then commit docs/evidence. Push feature branch, open PR using gh with exact validation/results, and check its exact-commit CI. Do not merge, tag, extract a production module or claim production JS support.

## Completion and scope limits

This PR is done when the contract document, examples and typed diagnostics match
actual owned behavior; shutdown/timeouts are regression-tested; all native and
owned gates, race checks and reproducible guest builds pass. An inability to map
a reported native column reliably must be documented with unavailable offsets,
not hidden or replaced with a guessed location. Any failure of required syntax
location fixtures or trusted classification blocks completion and needs a
specific design adjustment, not relaxed tests.

The next PR can extract the optional single-backend module without reopening
these contracts. Source-map file ingestion, live clocks, runtime pooling and
performance work are outside this plan.
