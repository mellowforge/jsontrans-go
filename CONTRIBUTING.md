# Contributing

Read AGENTS.md and the [approved design](docs/superpowers/specs/2026-11-02-json-transformers-design.md)
before changing public behavior.
Use Go 1.26 or newer. Core runtime dependencies must be pure Go supporting utilities,
not complete language engines. Use Context7 when checking dependency APIs.

Before submitting a change:

```sh
gofmt -w .
go test ./...
go test -race ./...
go vet ./...
go test ./jsonpath -run '^$' -fuzz '^FuzzCompileAndEval$' -fuzztime=10s
go test ./jsonata -run '^$' -fuzz '^FuzzCompileAndEval$' -fuzztime=10s
go test ./internal/document -run '^$' -fuzz '^FuzzPrepareAndMaterialize$' -fuzztime=10s
```

Fuzz the target closest to a change as well: `FuzzIRegexp` (`./jsonpath`),
`FuzzPreparedNestedPlans` (`./jsonata`), `FuzzInvokeResultJSON` (`./jsonata`,
for `Invoke` and raw results) and `FuzzJSONHelpers` (root package).

Go tests and committed fixture snapshots work offline after downloading Go
module dependencies. Every conformance failure is a release blocker; do not
introduce silent skips or broad output normalization to hide incorrect results.
Changes to semantic behavior need independent expected values and regression
tests, including malformed input and resource accounting where relevant.

The optional reference comparison uses Node 22 only for development:

```sh
npm ci --ignore-scripts --prefix conformance/oracle
go test -tags oracle ./conformance ./jsonata
```

The `oracle` tag also runs `TestNestedFastPlansReference`, which checks the
JSONata fast paths against the reference implementation.

Fixture updates must use immutable upstream commit IDs. Record source URLs,
revisions, checksums, licenses, changed expectations, and full test results.
Keep upstream files unchanged; document any confirmed upstream discrepancy
separately. Public release tags require complete compatibility verification.

The optional JavaScript assessment has a separate module and test commands in
[its README](experiments/javascript/README.md). It does not add JavaScript
runtime dependencies to the core library. Historical benchmark environments
remain recorded as measured; use Go 1.26.10 for new development measurements.

The optional JavaScript processor is a separate preview module in `javascript`.
Run its tests, race detection and vet with `go -C javascript ...`; the historical
assessment remains in `experiments/javascript` and forwards owned calls to it.
The root go.mod/go.sum must not acquire JavaScript runtime dependencies.
`python3 javascript/scripts/check_module.py` validates actual module-zip
consumption with no consumer replace/workspace. Run module tests first to cache
dependencies and use a checkout with full Git history for minimum-core validation.
Future JS release tags use `javascript/vX.Y.Z`; extraction does not tag a release.
