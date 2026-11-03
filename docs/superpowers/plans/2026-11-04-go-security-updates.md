# PR A: Go dependency security updates

> Use superpowers:executing-plans inline, then one independent whole-branch review.

**Approval:** User's “go” implements PR A of the [remediation plan](2026-11-03-javascript-security-remediation.md).
**Spec:** [Security review](../../javascript-security-review.md) and that plan's invariants.

## Global constraints

- Go 1.26+, same public APIs and execution limits, fresh guests and detached data.
- Pin x/text v0.43.0, Wazero v1.12.0 and its x/sys v0.44.0 dependency through normal module resolution.
- Preserve QuickJS/source/SDK/ABI/artifact pins, bridge code and native engine implementation.
- Preserve native/macOS workflow jobs. Add a separate Linux Go security job.
- No release/tag or PR B terminal-error changes. QuickJS remains a production blocker.

## Review focus

- A scanner process exit zero with findings must fail the gate, as must scanner/network/protocol failures.
- Coverage must be checked against actual imported/test module versions and the production example's own module inventory; local replacements must not hide third-party modules.
- Retain selected-but-unimported requirements as inventory, without calling them security-cleared.
- Keep package/runtime boundaries intact, including outside-checkout consumption and CGO-free builds.
- Do not loosen cancellation tests, budgets, import restrictions or conformance checks to accommodate the runtime update.

### Task 1: Update dependencies with scanner RED/GREEN

**Files:** root/public/assessment go.mod and go.sum.
**Interfaces:** Existing runtime APIs only.

- Observe the current core govulncheck finding in failing mode; capture baseline evidence.
- Update versions using Go tooling; tidy all modules and verify selected versions/checksums.
- Run core/public tests and the core scanner; investigate any compatibility failures at their source.
- Commit dependency updates; task completion command is core and public module tests.

### Task 2: Implement finding-aware scanning and coverage checks

**Files:** scripts/check_go_security.py and scripts/test_check_go_security.py; .github/workflows/security.yml.
**Interfaces:** Python standard-library CLI accepts a pinned govulncheck executable and output directory. It scans core/public/assessment source with tests, inventories imports and selected modules, builds/scans a CGO-free public example, and emits retained JSON evidence.

- Write failing behavioral tests for zero-exit findings, malformed/incomplete streams, scanner process failure, version/coverage gaps and local-replacement coverage across modules.
- Implement parsing and source-union coverage, plus independent binary coverage. Findings at any reported level fail. Verify tool/config/SBOM metadata; errors remain errors.
- Keep complete source/binary scanner streams and inventory/report output; no network calls from the unit tests.
- Add Linux CI with pinned govulncheck v1.9.0, gate tests and real source/binary scans. Existing CI file remains unchanged.
- Run offline unit tests and the real gate; task completion command is the Python test suite.

### Task 3: Verify compatibility and document current status

**Files:** docs/javascript-go-security-updates.md, current README/security guidance and contribution commands; new evidence dated 2026-11-04. Historical audit evidence remains intact.

- Run formatting, tests/race/vet in all three modules, native oracle/conformance, all 21 owned probes, packaging checks, relevant bounded fuzz and supported-platform CI.
- Verify artifact hash/pins, native module isolation, new runtime dependency inventory and byte-identical guest rebuild.
- Document before/after Go findings, coverage policy, scanner limitations and the remaining QuickJS blocker.
- Commit, obtain final independent review, fix important findings with regression evidence, push and open a PR with gh. Do not merge or release.
