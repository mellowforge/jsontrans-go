# Invoke fixes and batch setup experiment

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans. Execute the approved fixes and batch-setup experiment inline with regression-first tests and a final independent review.

**Goal:** Correct raw Invoke result detachment, then reduce repeated batch configuration work if measured evidence supports retaining the experiment.

**Architecture:** Keep input conversion strict. Give raw results a separate bounded detachment boundary that uses output limits and preserves language-only values. For batches, move immutable configuration decisions to construction and reuse resolved per-call configuration across adjacent equivalent entries, keeping expression budgets and callback scopes independent.

**Tech Stack:** Go 1.25; existing native engines and standard library; no new dependencies.

**Spec:** User-approved review findings and the batch setup consolidation proposal in this chat (2026-11-03).

## Global constraints

- Preserve JSONPath RFC 9535 and JSONata 2.2.2, native diagnostics, missing/null, caller ownership, and concurrent reuse.
- Keep per-expression counters, configured limits, option overrides, error attribution, host lifetimes and cancellation checkpoints.
- Raw non-JSON scalar values remain raw; JSON-shaped values use exact encoded JSON size for output accounting. Missing/functions/non-finite scalars count one bounded token when nested.
- No new public API, global cache, execution-check frequency change, dependencies, pushes or releases.
- Run formatting, tests, race, vet, oracle/parity and focused fuzz checks. Retain optimization only with useful measured evidence and no correctness regression.

## Review focus

- Nested functions, undefined, NaN/Infinity and UTF-16 strings must survive raw result detachment.
- Output size must be cumulative for nested containers; input size must not constrain output; inputs and host results remain strictly validated.
- Alternating registry/native configurations and raising/lowering overrides must not leak limits across entries or calls.
- Callback retention/cancellation and first failing expression attribution must survive configuration reuse.
- Independent concurrent calls must not share mutable resolved configurations, budgets or memo state.

## Task 1: Fix Invoke results

Files: `jsonata/core.go`, new `jsonata/raw_result.go`, new `jsonata/invoke_result_test.go`, `docs/adapters.md`, `docs/compatibility.md`.
Consumes: existing `Invoke`, `cloneRawArgs`, budget and JSON conversion contracts.
Produces: `cloneRawResult(Value, *budget.Budget) (Value, error)`; input conversion stays separate.

- [x] Add regressions for no-argument long output with MaxInputBytes=4, exact output size thresholds, escaped/nested output, raw nonfinite values, detached captured containers, callable/undefined values, cycles, cancellation and host/input rejection.
- [x] Run focused tests; observe wrong input/output budget and nonfinite failures.
- [x] Implement bounded result-specific detachment and call it only from Invoke.
- [x] Run focused and whole-module tests; document output accounting.
- [x] Commit the Invoke fixes.

## Task 2: Batch setup experiment

Files: `batch.go`, `document.go`, new `batch_setup_test.go`, performance evidence under `benchmarks/comparison/testdata/results/setup/`.
Consumes: Task 1 green runtime; existing prepared entry and native limits behavior.
Produces: constructor-resolved entry metadata and evaluation-local reuse of resolved configuration.

- [ ] Pin mixed native/wrapper limits, overrides, independent steps, external options mutation, cancellation and concurrency with regression tests; demonstrate any new internal configuration reuse with a focused test before implementing.
- [ ] Consolidate immutable guards/defaults at construction and repeated per-call resolution across equivalent entries; do not skip execution checks.
- [ ] Run targeted tests, full checks, and a sequential matched before/after benchmark with immutable source snapshots, raw output, hashes, allocation counts and medians.
- [ ] Retain only justified changes; document the outcome without extrapolating small fixtures to large collections.
- [ ] Commit the retained experiment and evidence.

## Task 3: Final verification and independent review

- [ ] Run gofmt, go test ./..., go test -race ./..., go vet ./..., oracle and nested benchmark parity, and bounded relevant fuzz targets.
- [ ] Obtain one fresh whole-change review; fix any important findings with regression tests.
- [ ] Report measured results, limitations, branch and verification; leave integration and pushing to the user.
