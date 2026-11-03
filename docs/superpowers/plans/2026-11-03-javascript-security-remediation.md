# Proposed JavaScript security remediation

**Status:** Proposed after the [dependency/security review](../../javascript-security-review.md).
This audit PR does not execute upgrades or approve changed terminal-error
behavior. No release/tag is included.

**Goal:** Remove the confirmed unsafe QuickJS pin, refresh Go dependencies, and
keep the optional processor's bounded synchronous execution profile explicit.

## Invariants

- Root Processor/Program/Result interfaces and registry semantics remain unchanged.
- JSONPath and JSONata stay native. Runtime dependencies remain in the optional
  module; the root gains no JavaScript dependency.
- Fresh guests; immutable programs; detached inputs/bindings; zero host imports;
  no WASI, filesystem/network, host-function registration, async jobs or pooling.
- Keep timeout, page, transfer, bytecode and conversion controls. Verify minimal
  supported memory configurations; do not silently raise a limit or add a host
  capability to make a candidate pass.
- Preserve native error data when available and errors.Is cancellation/closure.
  Exception names, messages and uncatchability alone are not trusted quota codes.
- Retain historical measurements/probes. Record new hashes and results
  separately; do not rewrite old runs as new pins.
- Leave existing macOS runner/timeout/retry configuration unchanged.

## PR A: Go dependency updates and automated scanning

1. Pin `golang.org/x/text` v0.43.0 in the root and both nested module selections.
   The core scan currently reports GO-2026-5970; this version also clears the
   older module-only precis advisory. Use the existing finding as scanner RED,
   then verify the update is GREEN; do not add a version-string unit test.
2. Pin `github.com/tetratelabs/wazero` v1.12.0 in the optional module, resolve the
   assessment consistently through MVS, and inventory/scan its new dependency
   `golang.org/x/sys` v0.44.0. Verify checksums and Go 1.26 compatibility.
3. Keep guest source/SDK/ABI pins unchanged in this PR. Run native compatibility
   suites, oracle comparisons, tests/race/vet in all modules, all 21 owned probes,
   public lifecycle tests, consumer packaging and relevant fuzz gates. Watch
   W^X, code-cache ownership, context interruption and Close on each platform.
4. Add a Linux security job using pinned govulncheck. Scan the root and both
   nested modules independently, compare coverage with module inventory, and
   scan a CGO-free public example. Use normal failing scanner mode or a tested
   finding-aware JSON gate. A successful JSON command alone is not a pass;
   scanner/network failures must not be recorded as clean.
5. Keep the C advisory review explicit. A clean Go job does not clear QuickJS.
   Update before/after evidence and keep the production block open.

## PR B: QuickJS update and terminal-error contract

### Proposed observable behavior to approve before implementation

Use QuickJS-ng **v0.18.0**, immutable source commit
`d7215b0ad3fc2c562e8317fd9bca058fd4863804`. This release makes OOM and InternalError
exceptions uncatchable by user JavaScript. Preserve that fail-closed upstream
behavior: an evaluation stops, no result is returned, and the next evaluation
gets a fresh healthy guest. Do not clear uncatchability to preserve a previous
test that catches OOM and continues.

Ordinary thrown exceptions and trusted conversion/limit outcomes retain their
current treatment. Terminal native errors remain typed native Exceptions when
their text can be recovered, with the existing bounded fallback otherwise.
Do not infer ErrLimit from `InternalError`, `"out of memory"`, or an
uncatchability flag: user code can construct errors, and internal failures are
not all configured quota violations.

Keep phase attribution trustworthy when terminal errors bypass the JS bridge
catch. The proposed mechanism is a private, preallocated stage record retained
by C and passed only to the trusted bridge. It is created before source
initializers, starts in evaluate, and is updated before output conversion.
On an exceptional return C reads a validated existing integer slot without
calling user getters. Never put the record on a public function/global property.
The guest introduces no Go/host callback or import. Version the internal ABI if
the guest/bridge protocol changes; the public Go interfaces need no change.
If this cannot preserve the contract under allocation failure, report the
blocker rather than fabricating a phase or suppressing a failing case.

### Implementation sequence

1. Add failing regressions for the large Proxy key-list reproduction. Expect a
   bounded native JS error, not a Wasm out-of-bounds trap, then verify healthy
   recovery. Keep small/hole/duplicate/symbol/getter/Proxy cases as controls.
2. Compute/check the immutable archive digest; update source/version provenance
   and licenses. Build with WASI SDK 34.0 first. Use the owned ABI build, not an
   upstream reactor/prebuilt wrapper. Verify zero imports of **every** kind,
   exact export signatures, memory limits and artifact integrity. Reproduce bytes
   across build hosts and record the new manifest.
3. Implement private terminal-stage handling and calibrate diagnostics against
   the new pin. Keep ByteOffset unavailable unless coordinates are separately
   verified; never label user stacks authenticated.
4. Replace OOM-catch-and-continue expectations with approved terminal-failure
   assertions. Document why old control flows are unreachable. Keep conversion/
   limit provenance checks under nonterminal pressure, forged errors, nested
   failures and ordinary Proxy exceptions. Add terminal OOM during initialization,
   execution, output traversal and diagnostic formatting; verify no result,
   truthful phase, cancellation and fresh recovery.
5. Confirm no new runtime-import, promise-job, clock, RNG or host-entropy service
   is enabled. Reject a candidate requiring an unapproved execution-profile
   change; do not silently fall back to the old vulnerable engine.
6. Run all owned gates with the new documented semantics, add security
   regressions without skipped failures, broaden ABI/resource fuzzing, and repeat
   Linux/macOS/Windows tests/race/vet and native compatibility gates. Verify the
   outside-consumer archive embeds the new asset and retains notices.
7. Remeasure cold construction, Compile, first/fresh Eval, JSON conversion,
   prepared fallback, multi-expression costs, allocations, binary size, guest
   pages and process RSS. Repeat long deployment-platform soaks; retain old and
   new measurements separately.

## Release decision

After both PRs, rerun current Go/upstream security review, verify a monitored
private reporting route, and review the terminal-error/platform contract and
prolonged-soak evidence. Production approval and nested-module versioning
(`javascript/vX.Y.Z`) remain separate decisions.

Planning estimate: 1–2 engineer-days for Go updates/scanning, 3–5 for the owned
engine/terminal-error migration, plus 1–2 for verification and measurements.
Soak wall time is additional. These estimates assume no required upstream engine
modification; re-estimate if a preserved gate cannot be met.
