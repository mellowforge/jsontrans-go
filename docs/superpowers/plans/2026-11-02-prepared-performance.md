# Prepared JSON Performance Implementation Plan

> For agentic workers: use superpowers:executing-plans or superpowers:subagent-driven-development task by task. Track the checkboxes and verification evidence.

**Goal:** Approach gnata performance for many compiled expressions per JSON document while preserving language and ownership semantics.

**Architecture:** An immutable prepared document owns validated JSON and a neutral structural index. Optional prepared-program methods execute conservative compiled plans and retain the full interpreter as fallback. Immutable builtin environments remove repeated setup work for both paths.

**Tech stack:** Go 1.25+, existing native engines, project-owned document index; gnata v0.6.1 and benchmark analysis tooling in an isolated development module.

**Spec:** ../specs/2026-11-02-prepared-performance-design.md

## Global constraints

- JSONata 2.2.2 and JSONPath RFC 9535; retain existing public interfaces and default limits.
- Primary workload is 4/16/64 precompiled expressions per document; include preparation in end-to-end results.
- Inputs are owned/immutable after preparation; results stay detached; no unsafe public borrowed slices.
- Pure Go; no complete-engine runtime dependency. Comparison module only uses pinned gnata v0.6.1.
- Use Context7 for supporting library/tool APIs. Keep tests and fixtures offline after dependency installation.
- No timing-based CI gates; claim performance gains only from matched measurements with recorded configuration.

## Review focus

- Escaped or duplicate keys and Unicode surrogates preserve the legacy decoder's results and ordering.
- A prepared document survives caller/result mutation and concurrent custom-function calls without sharing mutable data.
- Stricter limits, canceled preparation, and failed lazy view construction cannot be bypassed by reuse.
- Specialization cannot bypass shadowed builtins, bindings, native error positions, or array sequence rules.
- Benchmark loops include preparation where claimed, consume valid results, and compare identical operations.

## Task 1: Matched benchmark baseline

**Files:** create benchmarks/comparison/{go.mod,go.sum,comparison_test.go,README.md} and fixed testdata; update docs/performance.md.

**Interfaces:** consumes current compiled Program.Eval and gnata Compile/Eval/EvalBytes/StreamEvaluator; produces result-verified benchmark fixtures and baseline tables.

- [ ] Pin gnata v0.6.1 in this separate module with a replace to the local root module; main go.mod must not gain a complete-engine dependency.
- [ ] Define fixed 1/8/64 KiB documents and 4/16/64-expression simple and mixed rule sets; include gnata's four published bench_test.go expressions unchanged as a named fixture.
- [ ] Write result-parity tests before timing, including undefined/null handling. Fail explicitly on an unsupported comparison case; do not silently skip it.
- [ ] Measure current compiled Go-value evaluation, compile cost, and per-document processing; warm gnata schema plans outside measured steady state.
- [ ] Run five samples on Go 1.25.13 with GOTOOLCHAIN=local and the same CPU settings. Record baseline latency, allocations and machine details.

Expected: functional comparisons pass; baseline documents the actual gap without changing production behavior.

## Task 2: Reduce evaluation setup

**Files:** create jsonata/environment.go and environment_test.go; update jsonata/core.go and existing external regression tests.

**Interfaces:** consumes immutable engine builtin/custom-function snapshots; produces parent bindings plus evaluation-local writable frames, used by evaluator and closures.

- [ ] Add failing tests for builtin shadowing, assignments, mutually recursive closures, returned functions, nested eval and concurrent Invoke across local frames.
- [ ] Replace copying every builtin into each evaluation map with an immutable base lookup and local overlay. Keep bindings and lexical writes evaluation-local.
- [ ] Allocate ancestry/order/depth structures only when required; use profiles to remove verified duplicate conversions without changing ordinary Program guarantees.
- [ ] Run the complete suite and race detector, then repeat the existing two-item sum and matched baseline benchmarks. Record bytes/allocations separately from timing.

Expected: all semantic checks pass and setup allocation falls measurably; no speculative speedup claim.

## Task 3: Prepared document contract and index

**Files:** create document.go, document_test.go, internal/document/{document.go,scan.go,document_test.go}; extend integration tests.

**Interfaces:** implement PrepareOptions, PrepareJSON, opaque Document, PreparedProgram and EvalPrepared exactly as the spec. Internal document nodes expose immutable kinds/spans/children to built-in engines; public fallback materializes owned values.

Document and read-only Node accessors are public as listed in the spec, so an
external PreparedProgram can use JSON/index access without importing internals.
Preparation and evaluation charge their own actual work; eliminated/cached work
is not charged again, while stored shape/byte constraints are enforced per call.

- [ ] Write failing public tests for repeated/concurrent evaluation, buffer mutation after preparation, invalid suffixes, nil arguments and fallback through an existing third-party processor.
- [ ] Add an external-package PreparedProgram test using Document.JSON/Root/Node readers, wrong-kind behavior and owned Materialize results.
- [ ] Build a bounded validator/index over a copied input buffer; record exact numbers, last duplicate values, member order, escaped keys, surrogate strings and limit summaries.
- [ ] Add cancellation/depth/collection/byte tests during scanning and metadata construction before large allocations.
- [ ] Test separate preparation/evaluation step accounting and stricter stored shape constraints; failed cached initialization must not poison later calls.
- [ ] Implement detached fallback materialization; ensure canceled/failed initialization does not poison future use. Add callback/result-mutation regressions.
- [ ] Preserve optional prepared dispatch through the checked-program wrapper. Test an unsupported external processor using the compatibility fallback.

Expected: prepared infrastructure passes tests without introducing a semantic fast path yet.

## Task 4: Conservative JSONata and JSONPath plans

**Files:** create jsonata/prepared.go, jsonata/fastplan.go and their tests; create jsonpath/prepared.go and tests; update internal/budget only as required.

**Interfaces:** implement PreparedProgram on built-in compiled programs; compile immutable fast plans from their own ASTs, with full-evaluator fallback.

- [ ] Add independent expected results for singular paths/indexes, scalar comparison/boolean rules, exists/type/count/length/sum, and JSONPath singular selectors.
- [ ] Execute against indexed document nodes, preserving numeric conversion and detached output. Validate evaluation limits against stored preparation metadata.
- [ ] Add forced-fast/fallback differential cases for missing/null, arrays, escaped/duplicate keys, huge numbers, surrogate Unicode, function overrides and runtime bindings.
- [ ] Keep unsupported shapes, dynamic functions and explicit expression-depth instrumentation on the full evaluator. Fallback must happen before partial effects.
- [ ] Add bounded timer-free checks for simple plans while retaining cancellation/deadline guarantees. Test timeout, step/depth, output size, and callback context behavior.
- [ ] After singular plans pass, add simple filter/projection and aggregate loops with preallocation guards and incremental budgets. Preserve full array/sequence semantics or fall back.

Expected: optimized plans match fallback/reference results and accelerate the selected common workloads.

## Task 5: Compatibility, regression gates and performance target

**Files:** extend conformance runners, prepared fuzz/race tests and benchmark comparison; update README.md, docs/adapters.md, docs/compatibility.md and docs/performance.md.

- [ ] Run all representable pinned fixtures through prepared evaluation; cover raw missing-input language fixtures separately without silently dropping them.
- [ ] Run the Node reference comparisons, normal/race tests, vet, formatting and bounded preparation/evaluation fuzzing.
- [ ] Extend the isolated comparison with prepare-only, prepared steady-state and prepare-plus-N-evaluations rows; compare gnata EvalMany on the same input/rules.
- [ ] Label stock gnata's validation/ownership difference and also report its validation-inclusive variant; never omit our preparation cost from end-to-end claims.
- [ ] Record five-sample median per-document results for N=4/16/64 on the 1 KiB simple set. First performance gate is no more than 2x gnata latency at each batch size.
- [ ] Report 8/64 KiB, mixed/complex, allocation, retained-memory and parallel results alongside the primary target. Investigate complex-case regressions above 10%.
- [ ] If the target is missed, use the profiles to identify and implement the next bounded optimization; record the gap rather than marking performance achieved.
- [ ] Document examples, preparation cost/lifetime, fallback behavior, measured fast-path coverage and exact remaining gaps. Preserve production safety defaults.

Final correctness commands: `go test ./...`, `go test -race ./...`, `go vet ./...`, empty `gofmt -l .`, reference job, pinned fixture integrity and bounded fuzz checks. Benchmark commands run from the isolated module and are recorded in its README.
