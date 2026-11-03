# Release preparation

## v1.0.0 — verified Go 1.25 baseline

Target commit: `1553fc1` (current `main` before this work).
A local annotated `v1.0.0` tag records this exact commit; no GitHub release has
been published by this work.

- Native JSONPath RFC 9535 and JSONata 2.2.2 processors.
- Explicit processor registry and shared JSON/missing/null contract.
- Immutable prepared JSON documents and sequential expression batches.
- Go 1.25 minimum, MIT license.

Verified from an isolated archive of the target commit using Go 1.25.13:
formatting, all tests including pinned fixture integrity and conformance,
race detection, vet, and the Node-based JSONata reference comparisons.

## v1.1.0 — Go 1.26 update (proposed)

- Go 1.26 minimum; Go 1.26.10 preferred development toolchain.
- CI, contributor instructions and nested development modules updated.
- Native runtime dependencies and public processor contracts unchanged.
- Optional JavaScript feasibility module and measured assessment. This is
  development tooling, not a supported JavaScript processor or sandbox.

Historical performance numbers remain labelled with their original Go 1.25.13
measurement environment. No Go 1.26 performance improvement is claimed without
new matched measurements.

Before publishing v1.1.0: review and merge the update, run CI on the merged
commit, tag that exact commit, then create the GitHub release with these notes.
Do not tag the JavaScript prototype as a production backend. The nested
assessment module is not a separately released module.
