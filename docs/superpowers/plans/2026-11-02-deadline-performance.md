# Native deadline-check optimization

Approved by user: try the next deadline optimization; no strict speed target.
Preserve the existing uncommitted implementation and current feature branch.
Before source: /private/tmp/jsontrans-before-deadline. No release or push.

## Design

Use `time.Until` for deadline expiration so Go can use a monotonic-only clock
read when the stored deadline includes one. Wall-only parent deadlines retain
Go's wall-clock fallback. Do not change Context Err/Done publication, sticky
terminal causes, cancellation ownership or lazy timer behavior.

The internal prepared evaluator already checks entry and successful exit. Add
an unexported request marker, set only by that evaluator, with entry/exit check
methods: native handlers may omit their duplicate boundary checks when enclosed
by this scope. Bare internal handler calls keep their checks. Do not change
Budget.Step/Enter/Collection, the 32-work-unit checkpoint interval, per-expression
counters, default limits, result size conversion, callback lifetimes or errors.
Public APIs and supported language semantics remain identical.

## Execution

- [x] Add regression tests for managed/bare native boundaries, canceled return,
  expired work/limits, and monotonic/wall-only deadline behavior. Observe missing
  boundary capability RED before implementation.
- [x] Implement minimal internal marker/check methods and use them in native
  JSONata/JSONPath prepared paths, with full interpreter checks left intact.
  Replace deadline expiration comparison with time.Until; run targeted tests.
- [x] Run full tests/race/vet/format, pinned/reference gates and bounded fuzz;
  obtain one independent final review and fix important findings with regression.
- [x] Freeze runtime; measure five matched before/after/gnata samples for primary
  1 KiB simple4/16/64, unique64/mixed16, plus representative 8/64KiB workloads.
  Include preparation and document reuse, allocations, source hashes and profile.
  Keep only measured useful changes; no safety relaxation or threshold chasing.
- [x] Document current results, exact scope and remaining costs. No throughput
  promise and no requirement to beat gnata or hit the former 2x goal.

Outcome: all 420 timing samples passed with unchanged before/after source hashes.
The 1 KiB simple preparation-inclusive cases improve 20–31%; distinct scalar64
improves35%. Allocation counts are unchanged. Tests/race/vet/oracle/format and
all four bounded fuzz gates passed; independent final review has no findings.
Raw data, medians/ranges, environment and a verified reverse patch are in
benchmarks/comparison/testdata/results/deadline. Results and limits are explained
in docs/performance.md. Work remains local and unreleased.
