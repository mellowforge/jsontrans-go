# Optional JavaScript Module Extraction Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans inline, task-by-task, then one independent whole-branch review.

**Goal:** Extract the approved owned JavaScript processor into an optional, separately consumable module.

**Architecture:** The `javascript` submodule owns the single QuickJS/Wazero implementation and embedded artifact. The historical assessment keeps QJS/Goja and forwards owned calls to the public module. Ordinary native root dependencies remain separate.

**Tech Stack:** Go 1.26+, Wazero 1.9.0, unchanged ABI 2 guest/source/SDK pins, MIT.

**Spec:** [Approved execution contract](../specs/2026-11-03-javascript-contract-diagnostics-design.md), [current contract](../../javascript-execution-contract.md), and user's explicit extraction request after merging PR #8. This is packaging of that contract, not a new execution profile or release approval.

## Global Constraints

- Preserve lifecycle, diagnostics, source-coordinate limitations, limits and fresh guests from merged e9b9c0c.
- Keep root Processor/Program/EvalOptions/Error and root go.mod/go.sum unchanged.
- Runtime module `github.com/mellowforge/jsontrans-go/javascript`, package javascript; canonical constructor `New(context.Context, Config) (*Engine,error)`.
- Keep Config, Engine, Program, ErrClosed, Exception, SourceLocation, LimitsProvider and EvalLimitSupport semantics. Config retains explicitly unsupported GuestStackBytes.
- Runtime dependencies: root library and Wazero; no QJS, Goja, assessment imports or duplicated guest/owned engine.
- Root minimum `v1.1.1-0.20261103173822-e9b9c0c71fe0`, resolved by Go from merged commit. Local `replace root => ..` supports development; consumer verification ignores it and resolves the real version.
- Go 1.26+, CGO_ENABLED=0, MIT; embedded artifact remains hash 29556e8cbe4b78704c71304aa3da83ae8881dd3d1447c65d27ade3237721d60e / 929138 bytes.
- No release/tag, clock/RNG changes, pooling, new APIs for runtime internals, quota changes, performance optimization or deferred semantic fixes.
- Preserve all 21 owned assessment gates and historical evidence. Private guest memory statistics move with internal runtime tests/benchmarks; assessment facade verifies actual public large/small memory ceilings without exposing stats API.
- Do not modify native/macOS matrix jobs, timeout/retry/runner configuration. Add/change Linux JS jobs only and move guest rebuild paths.

## Review Focus

- Consumer outside the repository builds with no replace/workspace and correct minimum root version.
- Application importing only root acquires no JS runtime dependency; JS application acquires no comparison engine.
- Fresh guest, complete-call Close, typed errors and all 21 gates survive wrapper changes.
- Wasm, bridge and license assets are present in the module zip; no checkout-relative runtime files.
- Private memory/capability tests remain effective after moving internals; packaging must not weaken gates.

### Task 1: Extract the canonical runtime and contract tests

**Files:** Create javascript/go.mod, go.sum, LICENSE, public processor/owned/lifecycle/diagnostics files; move experiments/javascript/internal/quickjswasm and owned contract tests to javascript; add javascript/external_test.go.

**Interfaces:** New(ctx, Config), *Engine Compile/Close/Limits, *Program Eval/Limits/UnsupportedEvalLimits; all root interface signatures unchanged. Internal runtime import becomes github.com/mellowforge/jsontrans-go/javascript/internal/quickjswasm.

- [x] Add external-package TestPublicProcessorRegistry, compile/evaluate JSON, bind variables, typed exceptions, unsupported overrides, prepared/batch and Close. Run and observe missing public module implementation (RED).
- [x] Move owned implementation and tests, trimming baseline dispatch; reuse the existing bridge/artifact unchanged and copy MIT license. Keep Goja/QJS outside public source/test dependencies. Public constructor names New, internal/new test helpers updated mechanically.
- [x] Run module tests, race detection and vet; verify asset fingerprint unchanged and no forbidden dependency in go list -deps. Existing tests prove semantics rather than adding duplicate cases. Commit extraction.

### Task 2: Forward assessment calls to the public implementation

**Files:** experiments/javascript/processor.go, owned.go, probes_owned.go, go.mod/go.sum, cmd/owned-min/main.go; remove duplicate owned files/tests/guest assets now in public module; preserve generic cross-backend tests/benchmarks and baseline bridge.

**Interfaces:** Existing New(OwnedQuickJS,Config)/NewOwned(ctx,Config) delegate to public New; owned Compile returns public Program directly. Expose diagnostic aliases in assessment only for existing tooling. Baseline Program remains for QJS/Goja.

- [x] Add TestOwnedAssessmentUsesPublicProgram and verify RED before forwarding.
- [x] Replace owned branches with forwarding, remove legacy owned runtime imports and duplicate implementations. Update guest-memory probe to test a successful 40 MiB buffer under default cap and rejected allocation under 8 MiB through public APIs. Keep low-level page growth/statistics/capability tests with runtime; retain cache contamination probe against QJS.
- [x] Build owned-min using public API, run assessment tests/race/vet and all 21 strict owned probes; unchanged counts/no skipped cases. Record new extraction evidence separately from history. Commit facade.

### Task 3: Prove packaging and wire development/CI

**Files:** javascript/scripts/check_module.py, javascript/README.md, examples, docs/javascript-execution-contract.md, CONTRIBUTING.md, README.md, AGENTS.md, .github/workflows/ci.yml.

**Interfaces:** `python3 javascript/scripts/check_module.py` builds a zip/proxy fixture from public module source, obtains minimum root source with Go's normal git-module download against local repository history, then runs a fresh outside consumer with GOWORK=off, no replaces and a fresh module cache. Dependencies are read from the already downloaded Go module proxy cache; no network/authentication needed during verification. Require full git history for this Linux CI step.

- [x] Write consumer fixture using only public imports and assert registry behavior/typed errors. Run it against unextracted baseline or temporarily omit bridge/Wasm from fixture and observe a real missing-package/asset failure (RED).
- [x] Implement the bounded local fixture/proxy checker using Python standard library and Go tooling; use Go's own local-git fetch for canonical root zip/pseudo-version validation, not a local root replace. Verify compiled dependencies exclude QJS/Goja/experiments and root consumer excludes JS/Wazero.
- [x] Add a Linux javascript-module job (tests/race/vet/fuzz/packaging); assessment job retains comparison tests and 21 public-backed probes; move owned fuzz and reproducibility paths to canonical module. Leave the native test job byte-for-byte unchanged.
- [x] Document private-repository installation, nested module/tag convention javascript/vX.Y.Z for a later release, minimum core version, preview status, architecture/ownership and reproducible commands. No tag/release now. Update project rule to allow the requested optional module extraction.
- [x] Run gofmt -l . (empty), git diff --check, root and both nested module go test ./..., go test -race ./..., go vet ./..., native reference checks, relevant bounded owned fuzz runs and full 21 probes. Rebuild canonical guest and compare byte-for-byte.
- [ ] One independent review; fix blocking findings with RED/GREEN regressions, commit, push and open PR with gh. Check exact-commit Linux JS and reproducibility jobs. Do not merge or alter macOS pipelines.
