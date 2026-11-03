# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

Read `AGENTS.md` first. It holds the binding project rules: native engines only, immutable compiled programs, no mutation of caller JSON, budgets and cancellation everywhere, pinned fixtures, and the required checks. This file adds commands and architecture only.

## Commands

The required checks before completion match CI (`.github/workflows/ci.yml`):

```sh
gofmt -l .            # must print nothing
go vet ./...
go test ./...
go test -race ./...
```

Fuzz targets (run for parser/runtime changes; CI runs fixed execution counts of roughly 10s each, see `ci.yml`). A timed run that fails with only `context deadline exceeded` and no crasher is a race in Go's fuzz coordinator at expiry, not a finding; rerun it:

```sh
go test ./jsonpath -run '^$' -fuzz '^FuzzCompileAndEval$' -fuzztime=10s
go test ./jsonata  -run '^$' -fuzz '^FuzzCompileAndEval$' -fuzztime=10s
go test ./internal/document -run '^$' -fuzz '^FuzzPrepareAndMaterialize$' -fuzztime=10s
```

Also fuzz the target closest to a change: `FuzzIRegexp` (`./jsonpath`), `FuzzPreparedNestedPlans` (`./jsonata`), `FuzzInvokeResultJSON` (`./jsonata`, Invoke/raw results), `FuzzJSONHelpers` (root).

Single tests and conformance cases:

```sh
go test ./jsonata -run '^TestCompiledBatchFallbackAndBudgets$'
go test ./conformance -run 'TestJSONataConformance/array-constructor/case003'  # subtest = fixture path under testdata/jsonata/groups
go test ./conformance -run 'TestJSONPathCTS/516_'                              # subtest = <index>_<name>
```

Node 22 reference comparison (`oracle` build tag; covers `conformance` and the JSONata fast paths in `jsonata/fastplan_oracle_test.go`). Use the pinned reference in `conformance/oracle/node_modules/jsonata` to derive expected values for new JSONata regression tests:

```sh
npm ci --ignore-scripts --prefix conformance/oracle
go test -tags oracle ./conformance ./jsonata
```

`benchmarks/comparison` is a separate Go module that pins gnata for comparison. It is not a dependency of the root module. It uses the `prepared` and `batch` build tags. See its README before running or recording benchmarks.

## Architecture

**Dependency direction.** The root package `jsontrans` defines the processor contract and never imports the engines. `jsonpath/` and `jsonata/` import the root package. `internal/budget`, `internal/jsonvalue`, `internal/document`, `internal/prepared` and `internal/errchain` must not import engine packages.

**Root package (`transformer.go`, `document.go`, `batch.go`).**
- `Processor.Compile` returns a `Program`. `Transformer` is an explicit, snapshotted name→Processor registry. Registry programs are wrapped in `checkedProgram`, which resolves limits, detaches bindings, and validates results.
- `Result.Defined=false` means missing; JSON null is `Defined=true, Value=nil`.
- `Error` carries native diagnostics (code/position) and must keep `errors.Is`/`errors.As` working.

**Three evaluation paths:**
1. `Program.Eval` takes a detached Go value (`map[string]any`, `[]any`, `json.Number`, …).
2. `PrepareJSON` → `*Document` (`internal/document`): validated JSON plus an arena-backed structural index. `EvalPrepared` dispatches to a native handler if one exists. Otherwise it materializes the document and falls back to `Eval`.
3. `Batch` (`NewBatch` / `Transformer.CompileBatch`) evaluates ordered programs over one document. It has one overall deadline but keeps per-expression limits and work counters. It reuses the `dst` slice and reports failures as `*BatchError{Index}`.

**Native prepared dispatch (`internal/prepared`).**
- A `prepared.Handler` is bound to its owning compiled program, so external adapters cannot forge or rebind it.
- Engines implement the public `PreparedProgram` and expose a handler via `NativePrepared()`.
- An optional `BatchCompiler`/`BatchPlan` lets an engine compile a whole batch. JSONata uses this in `jsonata/batch_plan.go` for singular-prefix memoization with batch-local state only.
- External processors go through the `evalExternal` conversion boundary in `document.go`.

**Budgets (`internal/budget`).**
- `Limits` are positive ceilings. Zero fields select `DefaultLimits()`: 1s timeout, 1M steps, depth 256, 16 MiB input/output, 50ms regex timeout.
- Preparation and evaluation charge separate step budgets.
- `fast.go` is a cheaper budget used by simple plans, which check cancellation at entry/exit and at most every 32 work units.

**JSONata (`jsonata/`).**
- `core.go` contains the lexer, Pratt parser, AST `node`, tree-walking `evaluator`, closures, tail calls, transforms, and partial application.
- `runtime_contract.go` defines `Value`, `Undefined`, `Sequence`, `Callable`, and `CallContext`, the shared contract for the builtins.
- The standard library lives in `builtins*.go`, `datetime*.go`, `regex.go` (regexp2-based), and `signature.go` (function signature validation).
- Host functions registered with `WithFunction` are wrapped so their results are validated and detached.
- `fastplan.go` specializes conservative shapes: singular paths, scalar predicates, count/sum, simple projections and filters. Anything unsupported falls back to the full evaluator. Bindings, host functions, nondefault limits, and depth instrumentation also force the fallback. Optimizations must never change semantics; they are optimization boundaries, not language restrictions. These paths and `batch_plan.go` are benchmark-tuned: even small helper extractions have cost ~1.5%, so compare `benchmarks/comparison` before and after changing them.
- Numbers are binary64. Raw JSON input keeps object member order for key enumeration; plain Go maps fall back to lexical order.
- `CompileRaw`, `EvalRaw`, and `Invoke` expose language values such as functions outside the common JSON contract.

**JSONPath (`jsonpath/jsonpath.go`).**
- Implements RFC 9535, including the I-Regexp matcher (RFC 9485) and host functions typed by Value/Logical/Nodes.
- Preserves exact input numbers and compares large integers without binary64 rounding.
- Always returns a node list. Object traversal uses lexical key order.
- `prepared.go` specializes singular scalar selectors.

**`internal/jsonvalue`.** Detached-value conversion and validation, cycle detection, and key ordering. It also defines `UTF16String`/`UTF16Object` for strings and keys with isolated surrogates, in both results and processor input; engines must treat them as ordinary strings and objects.

**Errors.** `jsontrans.Error` carries native code and position (JSONata: UTF-16 units; JSONPath: bytes). `internal/errchain.Find` locates a processor's own diagnostic in a wrapped chain without crossing another processor's; keep wrapped causes reachable with `errors.Is`.

**Options and limits.** Engine `Option`s act on unexported construction state, so they cannot alter a live engine. Configured limits are defaults: `EvalOptions.Limits` replaces each nonzero field and may raise them. Registry wrappers forward wrapper ∩ program limits (program regex timeout, then the override) to the program on every evaluation path, and forward nil when everything is default.

## Conformance

- Fixtures in `conformance/testdata` are unmodified upstream snapshots at pinned commits (see `conformance/README.md`). `fixtures.lock.json` and `TestPinnedFixtureIntegrity` verify their checksums.
- The pinned suites run every representable input through both ordinary and prepared evaluation.
- The only allowed deviations are the two guarded JSONPath CTS errata (cases 516/517) in `conformance/ERRATA.md`. Never add skip lists or output normalization.

## Docs to update with behavior changes

- `README.md`: public API overview.
- `docs/adapters.md`: processor/prepared/batch contract.
- `docs/compatibility.md`: what is specialized versus what falls back.
- `docs/performance.md`: measured results.
- `SECURITY.md`: execution safety.

Design specs and implementation plans live in `docs/superpowers/{specs,plans}/`, and the running status in `docs/progress.md`.
