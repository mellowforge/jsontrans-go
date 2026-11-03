# Compiled Batch Performance Implementation Plan

Historical note: the later deadline pass supersedes these measurements. The user
explicitly removed the strict gnata-relative target; follow the current plan in
2026-11-02-deadline-performance.md for subsequent performance work.

> For agentic workers: use subagent-driven-development for isolated tasks and
> independent review. Preserve the current feature branch and existing work.

**Goal:** Reduce repeated prepared-evaluation setup/copies and document allocation.
**Architecture:** Exact-owner internal native handlers, immutable compiled
batches with reusable destinations and bounded local state, compact document
arenas, then measured native JSONata path/loop specialization.
**Tech stack:** Go 1.25+, current pure Go runtime; isolated gnata v0.6.1 harness.
**Spec:** ../specs/2026-11-02-batch-performance-design.md

## Global constraints and review focus

Preserve JSONata 2.2.2 / JSONPath RFC9535, detached owned results, cancellation,
default limits, concurrent reuse and native errors. Context7 for Go docs.
Test embedding wrappers that override native EvalPrepared; mixed configured
limits; caller-binding mutation; cross-result aliasing; failed destination reuse;
surrogate/duplicate-key metadata; and cancellation between expression calls.

## Task 1: Trusted native dispatch and compiled batches

Files: internal/prepared/*, document.go, batch.go, batch_test.go,
jsonata/{core,prepared}.go, jsonpath/{jsonpath,prepared}.go.

- [x] Write failing ownership/identity tests, including external embedding and
  overridden public evaluation; implement immutable exact-owner native handlers.
- [x] Route root/native prepared evaluation through one conversion boundary;
  direct and registry APIs preserve limits, errors and detached results.
- [x] Write failing NewBatch/CompileBatch/reusable-destination tests: program
  snapshot, empty/nil, mixed engines, ordering, missing/null, fail-fast Index/
  Unwrap and cleared destination, bindings, independent mutable results and race.
- [x] Implement the public signatures in the spec. Share outer deadline and
  native request setup; keep independent expression budgets and aggregate bounds.
- [x] Verify full tests/race/vet; independently review before dependent work.

## Task 2: Compact immutable document storage (independent of task 1)

Files: internal/document/*.go and tests only; public methods stay unchanged.

- [x] Add failing regression/allocation benchmarks for nested arrays/objects,
  duplicate and escaped names, source order, large numbers, surrogates, shallow
  and deep inputs, collection limits and cancellation.
- [x] Replace per-container child/member allocations with compact shared arena
  metadata. Bound scanner scratch and lookup construction; no unbounded scans.
- [x] Run existing differential fuzz and fixtures, tests/race/vet; report
  preparation-only bytes/allocations/time against the saved source snapshot.
- [x] Independently review the representation/limits before integration.

## Task 3: Shared prefixes and broader compiled array paths

Files: jsonata/fastplan.go, new jsonata batch planning support as needed,
internal/prepared contracts only with root agreement; regression tests.

- [x] Write independent/differential cases for nested projection/filter/sum,
  singleton/empty/nested arrays, native type errors, missing/null and Unicode.
- [x] Extend conservative direct loops for the published gnata path/filter
  workload; retain fallback for semantics not proven and all safety guards.
- [x] Add immutable batch planning with evaluation-local shared singular prefix
  reads; compare unique-rule fixture. No expression-result cache to game repeats.
- [x] Verify tests/race/oracle/fuzz and independently review semantic coverage.

## Task 4: Benchmarks, integration and evidence

Files: benchmarks/comparison, examples/prepared, README, docs/performance,
adapters/compatibility/progress and this plan.

- [x] Add batch parity tests and benchmarks for reusable/new destinations,
  prepare+batch and reused documents; pin nonrepeating workload results.
- [x] Compare five matched samples against saved old prepared source and gnata;
  include default safety settings, allocations, preparation and mixed workloads.
- [x] Document exact API/error/budget/lifetime behavior and measured gaps.
- [x] Run full tests/race/vet, empty gofmt, fixture integrity, Node references,
  bounded document/engine fuzz, isolated module checks and final scoped review.

Execution notes and deviations go in
.superpowers/sdd/2026-11-02-batch-performance/progress.md.

## Outcome (2026-11-02)

All implementation tasks, independent scoped/final reviews and verification
gates passed. Final matched matrix contains 1,350 samples, five per row, with
unchanged production hash before/after. For 1 KiB simple prepare-inclusive
batches, prior 6.87/11.81/32.11 us became 5.71/9.24/23.01 us at 4/16/64 rules;
warmed gnata is 3.05/4.74/11.11 us. Ratios are 1.88/1.95/2.07x, so the strict
2x goal remains open at 64 rules. Mixed 16-rule 1 KiB latency improved from
127.61 to 14.03 us (gnata 9.03). Unique 64-rule document reuse drops from
128 allocations/6656 bytes to four allocations/384 bytes; preparation-inclusive
allocations drop 167 to 26. This is not an end-to-end zero-allocation guarantee.

Exact methods, limits, sample ranges, source hashes, larger fixtures and caveats
are in docs/performance.md and benchmarks/comparison/testdata/results/batch.
No release, push or remote publication. Existing branch/work remains local.
