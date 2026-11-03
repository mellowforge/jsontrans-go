# Implementation progress

Plan: docs/superpowers/plans/2026-11-02-json-transformers.md

Pre-flight: core interfaces are shared by both engines; engine packages import
the core, which never imports engines. Conversion and budgets must not import
engine packages. JSONata internal runtime interfaces must be agreed before
splitting interpreter and standard-library work.

Ruling: initialize a new implementation branch in the requested empty directory
instead of creating another worktree — there is no existing checkout or work
to isolate; cost if wrong is relocating the new repository.

Status: implementation complete. Pinned compatibility suites and final
verification pass. Independent scoped reviews have no outstanding blockers.
No release has been published.

Shared adapter tests pass. Upstream fixtures downloaded at pinned commits.
Ruling: use parallel-agent skill for independent engine packages with exclusive
file ownership; establish a stable JSONata runtime contract before separating
its standard library. Cost if wrong: integration rework, not shared-file loss.

Project guidance, MIT license, module, CI, examples, fixture integrity lock, and
Node-only reference job have been created. Shared API regression tests pass.

Ruling (user decision): RFC9485 is authoritative over the two conflicting CTS
anchor expectations. Preserve snapshots; apply only explicit guarded errata,
documented in conformance/ERRATA.md. Cost: upstream CTS counts require disclosure.

Core/JSONPath review: seven findings fixed and independently verified, including
slice identity, undefined cancellation, binding ownership, work/depth accounting,
filter grammar, counted regex repetitions, and rune-aware character classes.
JSONata's full pinned suite passes all 1,291 files / 1,686 cases. The separate
reference job passes 585 expressions. Full normal, race, vet and formatting checks
have passed; final runs follow the last scoped review fixes.

Ruling: add lossless UTF16String/UTF16Object wrappers only for isolated surrogate
results and keys, plus native evaluation-depth instrumentation — required to
preserve pinned language semantics through Go's JSON boundary. Ordinary JSON
shapes are unchanged. Cost: rare Unicode results have explicit wrapper types.

Independent reviews found and drove fixes for malformed-signature panic,
returned-callable lifetime and alias races, generated-value traversal budgets,
preallocation checks, regex Unicode/continuation semantics, native diagnostics,
surrogate canonicalization, and captured object-order provenance. Regression
tests and scoped re-reviews cover these paths. No findings were silently waived.

Final verification (2026-11-02): `go test ./...`, `go test -race ./...`,
`go vet ./...`, empty `gofmt -l .`, the 585-case Node reference job, and fixture
integrity checks pass. Bounded JSON helper, JSONPath and JSONata fuzz runs pass.
The runnable example prints `jsonpath: [4,7]` and `jsonata: 11`.
Local benchmark measurements are recorded in docs/performance.md.

Final regression review verified invocation-local order caches for returned
closures and transforms under the race detector. All review findings are fixed;
there are no deferred review findings. Work remains local and reviewable.


Prepared performance work (2026-11-02): added owned immutable documents,
read-only public nodes, optional prepared dispatch with safe adapter fallback,
JSONata builtin-parent environments and conservative indexed plans in both
engines. Extended fixtures and Node comparisons run both evaluation paths.
Full correctness checks and independent review pass. Matched prepare-inclusive
1 KiB simple batches of 4/16/64 rules take 6.71/11.56/32.61 microseconds versus
warmed gnata's 2.92/4.66/10.96; the 2x goal remains open. Profiled opportunities
are budget/arena allocation and broader array plans. See docs/performance.md and
docs/superpowers/plans/2026-11-02-prepared-performance.md for exact evidence and
remaining work. The project remains unreleased.

Compiled batch follow-up (2026-11-02): NewBatch and Transformer.CompileBatch
snapshot ordered programs, reuse result destinations and preserve fail-fast
native diagnostics. Exact-owner native handlers remove duplicate conversion;
callback scopes remain independent and pure handlers reuse local budget scratch.
Compact arenas preserve document semantics while reducing preparation allocation.
Conservative nested projection/filter/arithmetic-sum and typed-comparison plans
expand direct evaluation; shared prefix state is local and bounded.

Independent task/final reviews and full verification pass. Five-sample matched
1 KiB simple prepare-inclusive results are 5.71/9.24/23.01 us at 4/16/64 rules
versus prior 6.87/11.81/32.11 and warmed gnata 3.05/4.74/11.11. Ratios
1.88/1.95/2.07x leave the strict 2x goal open for 64 rules. The mixed 16-rule
fixture improves 127.61 to 14.03 us; unique 64-rule document reuse reduces
128 allocations to four. See docs/performance.md for all 1,350 samples, source
hashes, allocation/ownership caveats and the complete matrix. Work is local;
no release or publication was performed.


Deadline follow-up (2026-11-02): user replaced the earlier strict 2x threshold
with safe improvements guided by profiling. Monotonic-only deadline reads and checked
internal native boundaries reduce redundant clock calls without changing loop
checkpoint frequency, limits or callback ownership. Five-sample matched 1 KiB
simple preparation-inclusive medians improve 5.79/9.09/22.97 to
4.64/6.76/15.87 us at 4/16/64 rules; warmed gnata is 2.99/4.77/11.23.
Distinct scalar 64 improves 20.48 to 13.29 us including preparation; reused-document
latency 15.64 to 8.89 us with unchanged four allocations/384 B. Full checks and
independent review passed; all 420 timing samples and unchanged hashes recorded
in docs/performance.md. No release/publication.

## 2026-11-03: Invoke result fixes and batch setup experiment

- Raw Invoke results now use output rather than input size limits and preserve
  native non-finite numbers. Strict input/registered-host validation is retained.
- Output sizing uses normalized numeric values; canonical key collisions and
  sequence-conversion failures are rejected. Regressions and a raw-result fuzz
  target cover these boundaries.
- Prepared entries combine immutable configuration at construction; batch calls
  reuse adjacent equivalent override resolution and pass limits by reference
  internally instead of copying them at each hop.
  Per-expression budgets, cancellation checks and callback scope ownership remain.
- Retained after 570 accepted samples across broad and alternating-order runs.
  Document-reuse confirmation medians improve 8% simple, 3% mixed and 14% distinct
  predicates; preparation-inclusive results are mixed and documented in full.
  Allocation counts are unchanged. No general throughput improvement is claimed.
- Full tests, race, vet, formatting, oracle/parity and bounded fuzz checks pass.
  Independent review findings were addressed with RED→GREEN regressions; none deferred.
