# Go dependency security updates

Reviewed **2026-11-04**, on `codex/go-security-updates`, based on merged PR #11
(`f5c53c4652151eda8c5c939c29fd74754b15edcc`). This records the Go dependency
remediation following the [JavaScript security review](javascript-security-review.md).
The [2026-11-03 audit](javascript-security-review.md) remains a historical record.
**The JavaScript module remains a preview: its embedded QuickJS pin still blocks
production release.** No release or terminal-error contract change is included.

## Updated selections

| Dependency | Before | After | Scope |
|---|---|---|---|
| `golang.org/x/text` | v0.36.0 in core/public selection | v0.43.0 in all three modules | Native JSONata Unicode utilities; not linked by the JS-only example. Assessment already selected v0.43.0. |
| `github.com/tetratelabs/wazero` | v1.9.0 | v1.12.0 | Optional JavaScript module and assessment only. |
| `golang.org/x/sys` | Not selected | v0.44.0 | New Wazero dependency; inventoried per target and included in scans where imported. |

Go remains 1.26+, using go1.26.10 for these local checks. The root acquires no
JavaScript dependency. Public interfaces, limits, bindings, clocks, RNG behavior,
guest lifecycle and capability restrictions are unchanged. QuickJS source
`d01ca4491fb24ccfeccb4c7394e28a3b21fd5986`, WASI SDK 34.0, ABI 2 and the owned
929,138-byte Wasm artifact are unchanged. Its SHA-256 remains
`29556e8cbe4b78704c71304aa3da83ae8881dd3d1447c65d27ade3237721d60e`;
the cached pinned toolchain reproduced identical bytes.

## Before and after Go findings

The baseline normal-mode `govulncheck -test ./...` exited **3** and reported
[GO-2026-5970](https://pkg.go.dev/vuln/GO-2026-5970) against x/text v0.36.0,
with a conservative trace from `builtinUnicodeCase` through `cases` to `norm`.
This is a scanner reachability finding, not a demonstrated JSONata exploit:
the recorded Greek case branch differs from the library's `language.Und` use.
The historical module-level [GO-2026-6629](https://pkg.go.dev/vuln/GO-2026-6629)
concerns `secure/precis`, which is not imported by this library.

After the update, the same normal-mode core command exited **0** with no
vulnerabilities found. Finding-aware JSON scans separately cover all three
modules, including tests, and a CGO-free public example binary. Complete streams
and inventories are retained in the [dated evidence](../javascript/testdata/security/2026-11-04/README.md).
The database revision is recorded in each scan; results describe that revision,
not future advisories or a proof of safety.

## Automated gate and coverage policy

The separate [Go security workflow](../.github/workflows/security.yml) runs on
Linux hosts for Linux/amd64, Windows/amd64 and macOS/arm64 targets. It cross-builds
and analyzes binaries; it does not execute Windows/macOS binaries on Linux.
Existing native, macOS and Windows test jobs retain their settings.

`scripts/check_go_security.py` uses pinned govulncheck **v1.9.0** against
`https://vuln.go.dev`. JSON mode can exit zero with findings, so the gate parses
every finding and rejects findings at any reported level, including module-only
findings. Process/network errors, malformed/incomplete protocol data, unexpected
tool/config/toolchain metadata and missing dependency/version coverage fail.
The gate's Python regression tests run offline.

For each source scan, the gate checks package roots against `go list -deps
-test`. It compares the union of scanner SBOMs against actual imported/test
modules at their selected versions. Local replacements are mapped only to the
explicitly scanned workspace modules. This compensates for govulncheck's nested
local-replacement SBOM omission: scanning a submodule alone can omit native
transitive modules. The core must be scanned independently. Own workspace module
version strings are normalized; external versions must match exactly.

The binary has an independent runtime import inventory and coverage check; it
cannot borrow source coverage. The pinned scanner reports its main module as
the binary root and uses `go1.x` for binary stdlib versions versus `v1.x` for
source scans. The gate accounts for these formats while checking the exact
toolchain version. Reports also retain complete selected module inventories and
`selected_not_imported`: unused tooling/test requirements are inventoried,
**not certified scanned** by this gate. Default build tags are scanned;
additional tag configurations require additional runs.

## Assessment-only QJS correction

Wazero v1.12 exposed a compatibility failure in QJS v0.0.6: it creates a compiled
module in one runtime and installs imports/instantiates in another, carrying
runtime-local function type IDs across the boundary. Even identical printed
signatures then fail import validation. Public owned guests use the same
runtime for compilation and instantiation and do not take this path.

With explicit approval, the assessment now uses a
[provenance-tracked internal copy](../experiments/javascript/internal/qjscompat/README.md)
of QJS v0.0.6. A small recorded patch compiles the unchanged guest bytes into
the destination runtime before installing imports, reusing cached machine code.
Its license, original file hashes, unchanged Wasm hash and patch are retained.
The upstream QJS requirement stays in the import graph for conservative Go
advisory detection, while the copied source is analyzed as assessment code.
This is not a new production backend or a QJS security endorsement. Original
shared configuration, host imports and filesystem limitations remain.

Each fresh QJS runtime now incurs additional decode/validation work. Historical
QJS measurements describe the original wrapper and are not rewritten as
measurements of this compatibility copy. The public adapter's guest, bridge and
runtime source have no changes in this PR.

## Verification and remaining release work

Verification commands cover normal/race tests, vet and module checksums in all
three modules; native pinned conformance and Node oracle comparisons; public
outside-checkout packaging; bounded conversion/compiler/diagnostic fuzzing;
the owned 21-probe suite; a 400-recovery public soak; and the byte-identical
guest rebuild. Local execution is macOS/arm64; supported-platform execution is
also required through the existing Linux/macOS/Windows CI jobs before merge.
Cross-target scans complement those jobs rather than replacing them.

A clean Go scan does **not** inspect QuickJS C code, its Wasm dependencies or
their memory-safety properties. The confirmed Proxy key-list guest trap and
unsafe source cleanup paths from the historical audit remain unresolved here.
PR B must update the engine with an explicitly approved terminal OOM/InternalError
contract, fresh recovery and truthful phase attribution. Re-review upstream
advisories and the rebuilt artifact, remeasure/soak, and verify a monitored
private reporting route before considering a production release.

## Independent review disposition

The whole-branch review found no critical or important issues. Its initially
deferred assessment-only cache-reference cleanup is now fixed: each additional
runtime-local QJS `CompiledModule` is closed when construction returns, including
failure paths. The instantiated guest remains usable, as Wazero permits closing
the compilation handle after instantiation. The regression reproduced two
references before the fix and now verifies that only the deliberate global cache
owner remains after successful construction or host-function setup failure.
The recorded patch and source hash are updated. The public adapter's separately
owned runtime/cache lifecycle is unchanged; inherited QJS cleanup limitations
are not addressed by balancing this added acquisition.

The review retained these scope decisions: inherited QJS capability/cleanup
limitations remain assessment-only; QuickJS security/terminal-error work remains
PR B with production blocked; native platform execution requires CI; unused
dependencies/alternate build tags are outside scan claims; retained evidence is
dated rather than a guarantee against future advisories. Unified-diff whitespace
inside `runtime.patch` is upstream context, not a Go formatting change.
