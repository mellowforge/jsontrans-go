# JavaScript production hardening: first PR

> **For agentic workers:** Use superpowers:executing-plans inline, then one independent whole-branch review.

**Goal:** Exercise the existing synchronous execution contract under hostile values, concurrent calls and repeated failure/recovery, and refresh measurements of the public optional module. This implements the approved assessment's “Harden and measure” stage without changing the execution profile.

**Spec:** [Execution contract](../../javascript-execution-contract.md) and [approved production roadmap](../../javascript-assessment.md#production-implementation-plan-and-revised-cost).

## Global constraints

- Preserve public interfaces, guest/ABI/dependency pins, fresh guest state, unsupported limits and preview status. No pooling, host capabilities, release, tag or additional quota promise.
- Preserve root runtime dependencies and native engine behavior. Do not change macOS runner, timeout, retry or matrix settings.
- Use observable guest-entry barriers for lifecycle tests, rather than assuming a goroutine has entered Wasm after a sleep. Keep instrumentation test-only.
- Cancellation tests must prove cancellation; memory rejection tests must prove the allocation failed, not merely accept any error.
- Report guest memory separately from Go heap/process RSS; samples are evidence rather than a hard memory guarantee. Report benchmark environment and commands.

## Review focus

- Tests must fail if a shared guest leaks mutations, bindings are shared, admission/draining is removed, trusted conversion checks are bypassed, or resource failures poison later evaluations.
- Stress tooling must validate expected errors and results, propagate worker failures, bound input/iterations/concurrency, and stop promptly on failure.
- Fuzz inputs must remain bounded and cannot inject unbounded source/collections into workers. Legitimate resource errors must not hide semantic mismatches on small successful inputs.
- Measurements must include complete public calls, distinguish construction from compilation/evaluation, and avoid claiming guest caps bound process memory.

### Task 1: Public lifecycle and hostile conversion regression coverage

**Files:** javascript/hardening_test.go; implementation only if a regression exposes a contract violation.

**Interfaces:** Existing New/Compile/Eval/Close and root errors; Wazero function listeners are test-only barriers.

- Add synchronized public-engine tests for Close during admitted compilation/evaluation, concurrent Close, admission after closure, and timeout overrides reaching guest execution.
- Add concurrent compiled-program reuse with detached input/bindings, hostile intrinsic mutation and fresh-state checks.
- Cover hostile result shapes, prototype/key edge cases and genuine output limits; verify typed error phase/classification and healthy reuse after each failure.
- Run focused tests and race detection; fix any semantic defect with a witnessed RED/GREEN regression. Coverage for existing behavior need not invent production changes.

### Task 2: Bounded fuzzing, repeatable soak and public benchmarks

**Files:** javascript/fuzz_test.go, hardening benchmark/test utilities or a public-API-only command under javascript/cmd/hardening.

**Interfaces:** No production API addition. Command flags select bounded iteration/worker/cycle counts; JSON report contains actual successful/expected failure counts, timing and Go memory samples.

- Expand JSON tree conversion fuzzing with a reference comparison for successful bounded inputs.
- Add a standalone fresh-runtime soak mixing successful concurrent evaluation, conversion failures, thrown exceptions, cancellation and guest allocation rejection. Check recovery and Close between cycles. Tool validates the outcome of every call.
- Add benchmarks for public construction, Compile, fresh Eval, EvalJSON, prepared fallback and sequential expressions over one document. Separate construction/setup from steady-state operations.
- Run bounded fuzz campaigns and short benchmarks, plus a longer soak outside normal tests; retain reproducible commands and raw results. No cross-library comparison.

### Task 3: Continuous gates and operational documentation

**Files:** .github/workflows/ci.yml, javascript/README.md, SECURITY.md, docs/javascript-hardening.md; measurement artifacts.

- Add bounded JSON-tree fuzz/soak to Linux JavaScript gates and Windows public-module test/race/vet coverage, preserving all native/macOS configuration.
- Document supported controls, application-owned admission/concurrency limits, source/bytecode trust boundaries, error text handling, update/rebuild policy and remaining release gates. Keep preview status explicit.
- Run formatting, tests/race/vet in root and both nested modules, all 21 strict owned probes, relevant fuzz checks, packaging consumer verification and artifact fingerprint checks.
- Obtain one independent review, address important findings with regression evidence, then commit/push/open PR with gh. Do not merge or release.
