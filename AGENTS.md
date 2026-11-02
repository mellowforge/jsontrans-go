# Project instructions

- Use Context7 for current dependency and library documentation.
- Module: `github.com/mellowforge/jsontrans-go`; package: `jsontrans`; Go 1.25 or newer; MIT license.
- Implement JSONPath RFC 9535 and JSONata 2.2.2 natively. Do not depend on complete JSONPath/JSONata engines or embed JavaScript. Focused pure Go utilities are allowed.
- Keep processor adapters public, registration explicit and instance-local, and language-specific syntax and runtime values inside each engine.
- Preserve native language semantics, numeric behavior, missing/null distinctions, and native error codes and positions.
- Compiled programs are immutable and safe for concurrent evaluations. Keep bindings, budgets, clocks, RNGs, and matcher state local to evaluations.
- Never mutate caller-owned JSON values. Validate inputs, reject cycles and unsupported types, and bound conversion work.
- Check cancellation and resource budgets during parsing, evaluation, built-ins, regex operations, and nested evaluation. Host functions are trusted Go code and must cooperate.
- Add meaningful regression tests for semantic changes. Pin upstream fixtures with provenance and license notices; never hide conformance failures or call incomplete engines conformant.
- Before completion run `gofmt -l .` (no output), `go test ./...`, `go test -race ./...`, and `go vet ./...`; run relevant fuzz checks for parser/runtime changes.
- Update documentation when public behavior or compatibility changes. Do not release until all pinned compatibility gates pass.
