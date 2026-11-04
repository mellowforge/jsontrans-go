# QuickJS Upgrade and Terminal Errors Implementation Plan

> Use superpowers:executing-plans inline, then one independent whole-branch review.

**Goal:** Implement PR B's owned QuickJS upgrade with terminal native failures,
truthful phases and fresh recovery, using local verification.
**Approval:** The user's instruction to move on to PR B authorizes the existing
PR B design and its terminal OOM/InternalError behavior.
**Spec:** [PR B and invariants](2026-11-03-javascript-security-remediation.md).
**Architecture:** Keep stage in a C-retained, preallocated null-prototype JS
object with own writable integer slot 0: evaluate=0, output=1. Pass it only to
the trusted bridge; read its own data descriptor without calling getters.
Increment the private guest ABI to 3; diagnostic JSON remains version 2.
**Tech stack:** Go 1.26.10, Wazero 1.12.0, QuickJS-ng v0.18.0, WASI SDK 34.0.

## Global constraints

- Pin source d7215b0ad3fc2c562e8317fd9bca058fd4863804 and archive SHA-256
  a49defe70cbcae9226d9fac025af116e87ebae2440dcff1ca3c80b79e1cdef0b.
- Preserve public APIs, fresh guests, native processors, Go dependencies,
  timeouts, 2–64 MiB page limits, transfers and conversion budgets.
- Zero imports of any kind; no WASI instance, new host capability, async or pooling.
- Never clear uncatchability or infer ErrLimit from native names/messages.
- Preserve historical fixtures and measurements; no release/tag or hosted CI.
- Work on codex/quickjs-terminal-errors from merged main 3fc979a. Windows SSH
  setup is pending; no native Windows pass can be inferred from cross-builds.

## Review focus

- User initializers fail before bridge invocation: phase must remain evaluate.
- Terminal OOM in output traversal/diagnostic getters: preserve original phase.
- Prototype mutation, proxies and forged errors cannot access the private stage
  or acquire trusted limit/conversion provenance.
- Failures at minimal guest memory and cleanup/cancellation must remain bounded
  with independent recovery; do not raise limits to pass.
- New guest bytes, ABI signatures and packaging must agree on all build hosts.

### Task 1: Upgrade the owned guest and fix the Proxy reproduction

**Files:** javascript/terminal_errors_test.go; internal/quickjswasm/guest/{lock.json,build.py,LICENSE.quickjs}; internal/quickjswasm/quickjs.wasm.
**Interfaces:** Existing public Compile/Eval and ABI 2 initially; no public changes.

- [ ] Add TestOwnedProxyKeyListSafety: the 536870913-key array-like fixture at
  8 MiB must yield a native exception, no result/trap/limit classification,
  followed by healthy 42. Include small, empty, holes, duplicates, symbols and
  throwing-getter controls with explicit result/phase expectations.
- [ ] Run the focused test; expect failure on the existing Wasm trap.
- [ ] Update source pin/hash/license, key cached source archives by commit,
  build the owned guest with SDK 34.0 and record its actual hash.
- [ ] Run the focused Proxy test and existing artifact/cancellation tests;
  expect pass. Record baseline and upgraded benchmarks separately.
- [ ] Commit the guest upgrade and security regression.

### Task 2: Preserve terminal-failure phases through the private ABI

**Files:** javascript/terminal_errors_test.go, bridge.js; internal/quickjswasm/{runtime.go,artifact_test.go,guest/abi.c,guest/lock.json,quickjs.wasm}.
**Interfaces:** Private ABI 3, existing abi_eval arguments/exports unchanged.
Bridge gets a ninth private stage-record argument after ownedMode. Slot 0
contains only immediate integers 0/1; set output before traversing the result.
C creates it before user initialization and validates its own data descriptor
on exceptional returns, without allocating atoms or invoking user getters.

- [ ] Add TestOwnedTerminalFailures with initializer/execution/output and
  evaluate/output diagnostic-getter OOM cases; assert no result, native typed
  exception or bounded native fallback, exact phase, no ErrLimit/ErrConversion,
  and subsequent healthy evaluation.
- [ ] Run against upgraded ABI 2; expect output-phase cases to fail.
- [ ] Implement the C-retained record and trusted bridge update, free it on
  every exit, version ABI 3 and rebuild/hash the guest. Never expose the record
  on globals/functions. Validate invalid private records as internal failures.
- [ ] Add/execute prototype/forgery and ABI integrity checks, verify the focused
  terminal/Proxy tests and existing cancellation tests pass, then commit.

### Task 3: Migrate obsolete OOM continuation tests and document semantics

**Files:** javascript/diagnostics_test.go, terminal_errors_test.go,
README.md, ../SECURITY.md, ../docs/javascript-execution-contract.md,
../docs/javascript-quickjs-upgrade.md; relevant owned probes if needed.
**Interfaces:** Unchanged common errors; native Exceptions preserve available
fields, including InternalError, without trusted quota classification.

- [ ] Run the public suite; retain each obsolete catch-OOM-and-continue failure.
- [ ] Replace those expectations with terminal/no-result/exact-phase/recovery
  assertions. Add separate bounded nonterminal pressure cases for trusted
  conversion/limit classification and retain forged-error/ordinary Proxy tests.
- [ ] Test cancellation during native and diagnostic work, minimal guest memory,
  and concurrent reuse; preserve existing limits and meaningful assertions.
- [ ] Update current documentation and the upgrade record, leaving the
  historical audit and assessment-only QJS compatibility copy unchanged.
- [ ] Run public/assessment tests and focused runtime fuzz targets; expect pass,
  then commit the semantic tests and docs.

### Task 4: Local verification, measurements and handoff

**Files:** dated evidence under javascript/testdata/security; upgrade record;
benchmark results recorded separately from historical measurements.
**Interfaces:** Reuse docs/local-verification.md and the private Bertram tools.

- [ ] Reproduce owned guest bytes independently on Mac and Bertram. Recheck all
  import kinds, exact exports/signatures, ABI, page/transfer boundaries and notices.
- [ ] Run formatting, tests/race/vet in all three modules; native oracle,
  packaging regressions/consumer, bounded fuzz, all owned probes, Go security
  matrix and a longer deployment-host soak. Capture actual failures, never skip.
- [ ] Run fresh Windows tests/race/vet once SSH is available; otherwise clearly
  retain Windows as pending and do not claim complete platform acceptance.
- [ ] Remeasure cold construction, compilation, first/fresh evaluation,
  conversion, prepared fallback/batch, allocations, guest/process memory and
  binary size on the same pinned toolchain; preserve before/after data.
- [ ] Review current upstream notices, record source inspection/reproductions,
  provenance, limitations and remaining release gates. Obtain the single final
  independent review, fix important findings with regression evidence, then
  prepare PR B with CI skipped if publishing. No merge or release.
