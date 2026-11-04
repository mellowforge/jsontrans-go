# Release preparation

## v1.0.0 — verified Go 1.25 baseline

Target commit: `1553fc1` (current `main` before this work).
An annotated `v1.0.0` tag records this exact commit. Its historical GitHub release
entry remains a draft; `v1.1.0` is the current published release.

- Native JSONPath RFC 9535 and JSONata 2.2.2 processors.
- Explicit processor registry and shared JSON/missing/null contract.
- Immutable prepared JSON documents and sequential expression batches.
- Go 1.25 minimum, MIT license.

Verified from an isolated archive of the target commit using Go 1.25.13:
formatting, all tests including pinned fixture integrity and conformance,
race detection, vet, and the Node-based JSONata reference comparisons.

## v1.1.0 — Go 1.26 update

- Go 1.26 minimum; Go 1.26.10 preferred development toolchain.
- CI, contributor instructions and nested development modules updated.
- Native runtime dependencies and public processor contracts unchanged.
- Optional JavaScript feasibility module and measured assessment. This is
  development tooling, not a supported JavaScript processor or sandbox.

Historical performance numbers remain labelled with their original Go 1.25.13
measurement environment. No Go 1.26 performance improvement is claimed without
new matched measurements.

Published at merged commit `c0519ea48d8022721b0d847e3def176d5a99e361` after
post-merge CI passed: [v1.1.0](https://github.com/mellowforge/jsontrans-go/releases/tag/v1.1.0).
The tag and release target both name that exact commit.
That root release did not ship a supported JavaScript processor. The nested
assessment module is not a separately released module.

## javascript/v1.0.0 — first supported JavaScript processor

Approved on 2026-11-04 following merged PR #16 and the maintainer's explicit
release instruction. This nested-module release uses the verified merge commit
of its release PR; the annotated tag and GitHub release target that same commit.
The native root release remains `v1.1.0`.

See the [release notes](releases/javascript-v1.0.0.md) and
[production approval](javascript-production-readiness.md) for the supported
profile, installation, platform policy and acceptance evidence.

Release: [javascript/v1.0.0](https://github.com/mellowforge/jsontrans-go/releases/tag/javascript%2Fv1.0.0).
