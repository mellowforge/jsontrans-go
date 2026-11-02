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
