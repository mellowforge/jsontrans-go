# JavaScript dependency and security review

Reviewed **2026-11-03**, against merged commit
`f5dd8ba6a3073cb5e0f00e82d824102aa1531827` (PR #10).
This review records the current implementation; it does **not** update dependency
pins or approve a production release. The JavaScript module remains a preview.

**Outcome: production release is blocked by the embedded QuickJS source pin.**
A bounded public-API reproduction traps on an out-of-bounds guest access. Go
dependency scanning does not detect this embedded C code. Go scanning also
identifies affected `x/text` versions in the native core, with conservative
symbol reachability. A staged remediation plan follows this review.

## Dependency inventory

| Component | Current pin | Role and review result |
|---|---|---|
| Go | 1.26.10 for these scans | Standard library included in source/binary scans; no reported standard-library findings for this toolchain. |
| Root library | `v1.1.1-0.20261103173822-e9b9c0c71fe0`, locally replaced by `..` | Optional adapter uses root contracts, budgets and JSON conversion. Local replacement is not used by outside consumers. |
| Wazero | v1.9.0 | Only external Go runtime module in the public example. No published advisory returned by the upstream endpoint; newer security hardening and maintenance fixes warrant an upgrade. |
| QuickJS-ng | `d01ca4491fb24ccfeccb4c7394e28a3b21fd5986`, version header 0.10.1, 2025-10-15 | Embedded runtime; known unsafe cleanup/allocation patterns remain. Release blocker. |
| WASI SDK | 34.0 | Build toolchain; verified release asset hashes and LLVM/wasi-libc submodule pins. No SDK/libc advisory returned by the reviewed upstream endpoints. |
| `regexp2/v2` | v2.12.0 | Native JSONata dependency, not linked by the JS-only example. No scanner finding. |
| `x/text` | v0.36.0 | Native JSONata dependency, not linked by the JS-only example. Two advisories reported in core scanning; see reachability below. |
| QJS / Goja | Assessment module only | Excluded from the public adapter's production import graph. Reviewed separately as development dependencies; the Go scan cannot inspect QJS's embedded C/Wasm. |

The actual JS-only example import graph contains the root module, the optional
JavaScript module and Wazero. It contains no native JSONata engine, `x/text`,
`regexp2`, QJS or Goja. The full selected module graph is a different inventory:
it also contains test/development requirements. See the machine-readable
[inventory](../javascript/testdata/security/2026-11-03/inventory.json).

## Findings

### P1: replace the embedded QuickJS pin before production

The current artifact reproduces the following small expression under the public
API with an **8 MiB guest cap** and a **100 ms caller deadline**:

```javascript
() => Reflect.ownKeys(new Proxy({}, {
  ownKeys() { return { length: 536870913, 0: "x" }; }
}))
```

The result is `wasm error: out of bounds memory access`, not an ordinary JS
TypeError. A subsequent independent evaluation returns 42. We observed a guest
trap and recovery, **not a host escape**. The cap does not prevent unchecked
32-bit allocation arithmetic from wrapping to a small allocation.

At the current source pin, `js_proxy_get_own_property_names` allocates
`sizeof(tab[0]) * len` in Wasm32 `size_t` without checking the product. The
reviewed v0.18.0 implementation grows the table incrementally and checks against
`SIZE_MAX / sizeof(tab[0])`. This is the same function family as
[GHSA-jp52-chwg-2p9p](https://github.com/quickjs-ng/quickjs/security/advisories/GHSA-jp52-chwg-2p9p),
but the published advisory describes count addition and gives an older affected
version range. We do not assign its identifier conclusively to this
multiplication reproduction or infer safety from that range.

Source inspection also confirms the cleanup pattern in
[GHSA-v647-xff9-jxf7](https://github.com/quickjs-ng/quickjs/security/advisories/GHSA-v647-xff9-jxf7):
`build_backtrace` frees a CallSite after `JS_DefinePropertyValueUint32` has already
consumed it on failure, while `js_new_callsite` transfers retained fields without
clearing the original record. `Error.prepareStackTrace` and allocation pressure
are available to user JavaScript in our profile. This path was inspected, not
reproduced with a native exploit or ASan in this review.

The zero-import Wasm boundary remains valuable isolation, but does not establish
C memory safety or trustworthy bridge state after guest memory corruption.
The current source must be replaced; adding a source-text blacklist or relying
only on timeout/memory caps would not address the underlying issue.

### P2: update the affected native-core x/text requirement

Core `govulncheck -test ./...` produces four finding messages representing two
advisories, rather than four distinct vulnerabilities:

- [GO-2026-5970 / CVE-2026-56852](https://pkg.go.dev/vuln/GO-2026-5970): invalid-UTF-8 normalization can loop; fixed in v0.39.0. Reported at module, package and symbol levels.
- [GO-2026-6629 / CVE-2026-56851](https://pkg.go.dev/vuln/GO-2026-6629): `secure/precis` Nickname transformation can panic; fixed in v0.41.0. Reported at module level only; this package is not imported by the library.

The first symbol trace passes through `cases.elUpper` to
`jsonata.builtinUnicodeCase`. The library explicitly constructs casers with
`language.Und`; `elUpper` is the Greek-locale variant. Static function-pointer
analysis is conservative, and this review has **not demonstrated an exploitable
JSONata input**. Do not describe the trace alone as proof of a reachable DoS.
The affected module requirement should still be updated rather than silenced.
The current verified release target is `x/text` v0.43.0 (Go 1.26 compatible).

This is a native-core dependency finding, not a vulnerability reported in the
JS-only runtime binary. Updating the root and nested module selections together
will also protect applications combining processors in one registry.

### P2: refresh the Wazero host runtime and its review gates

The upstream published-advisory endpoint returned an empty list for Wazero.
This is **not** evidence that v1.9.0 is vulnerability-free. The maintained target
is [v1.12.0](https://github.com/wazero/wazero/releases/tag/v1.12.0), commit
`2ab480b55fa408d6b35df97fe32a60d08bd6e201`.
The intervening [v1.10.1 notes](https://github.com/wazero/wazero/releases/tag/v1.10.1)
describe W^X hardening where supported; v1.11 fixes compiled-module reference
count initialization, and v1.12 includes integer-operation and runtime fixes.
These are upgrade reasons, not a claim that each fix is exploitable in our
trusted-artifact, zero-host-import configuration.

Wazero v1.12.0 retains the `github.com/tetratelabs/wazero` module path despite the
repository relocation. It requires Go 1.25 and adds `golang.org/x/sys` v0.44.0.
That new transitive runtime dependency must be inventoried/scanned. Do not import
WASI or turn on host capabilities as part of the upgrade.

### Release gate: establish an actual private reporting route

The repository is private. Repository metadata did not expose a security-analysis
configuration, and the private-vulnerability-reporting endpoint returned 404.
This does **not** prove a specific feature is disabled; capability/access cannot
be verified from those responses. No monitored reporting contact is identified
in SECURITY.md. Maintainers must select and test a private contact route before
public release. This review changes no repository settings and contacts nobody.

## Published QuickJS advisories reviewed

Ten active upstream advisories were returned. A publisher's affected-version
range is retained as evidence, not treated as an exact source-ancestry verdict.
No finding is hidden merely because a general Go scanner cannot see it.

| Advisory | Review disposition for this pin/profile |
|---|---|
| [v647-xff9-jxf7](https://github.com/quickjs-ng/quickjs/security/advisories/GHSA-v647-xff9-jxf7) | CallSite OOM cleanup ownership defect confirmed by source inspection. |
| [crgj-9jv8-xfhq](https://github.com/quickjs-ng/quickjs/security/advisories/GHSA-crgj-9jv8-xfhq) | Resolve-function cleanup leaves a freed value in the source. Published trigger uses module evaluation, which is not exposed; synchronous Promise machinery is still present. No claim of general immunity. |
| [rxpx-wj25-jhqq](https://github.com/quickjs-ng/quickjs/security/advisories/GHSA-rxpx-wj25-jhqq) | Published introduction is >=0.15.1, newer than this pin; exact applicability was not demonstrated. RegExp/OOM regression review remains required after upgrade. |
| [3jf7-4qfx-xc2h](https://github.com/quickjs-ng/quickjs/security/advisories/GHSA-3jf7-4qfx-xc2h) | Rope indentation disclosure was demonstrated upstream on newer releases. No matching rope representation found in this pin; not reproduced here. |
| [wm7w-r6qf-3c9w](https://github.com/quickjs-ng/quickjs/security/advisories/GHSA-wm7w-r6qf-3c9w) | Published <=0.8.0 range excludes the pin. Concurrent worker/host-detach trigger is not exposed; full source proof of applicability was not attempted. |
| [9wv5-5cgr-grj6](https://github.com/quickjs-ng/quickjs/security/advisories/GHSA-9wv5-5cgr-grj6) | Published trigger allocates a >1 GiB TypedArray, exceeding this guest cap. Range excludes the pin; no broad claim that caps fix integer overflow. |
| [jp52-chwg-2p9p](https://github.com/quickjs-ng/quickjs/security/advisories/GHSA-jp52-chwg-2p9p) | Related Proxy arithmetic sink reproduced as a guest trap despite the reported older range. Precise advisory identity is unconfirmed. |
| [52gx-967m-hxp3](https://github.com/quickjs-ng/quickjs/security/advisories/GHSA-52gx-967m-hxp3) | Requires embedder-owned external ArrayBuffers/custom free callbacks. The adapter transfers JSON strings and exposes no such buffer API. |
| [r6vf-p8wr-4vg9](https://github.com/quickjs-ng/quickjs/security/advisories/GHSA-r6vf-p8wr-4vg9) | Minimal shrink/grow/push example returned the expected `undefined` on this pin; not reproduced, not declared universally fixed. |
| [m53g-qx47-3h38](https://github.com/quickjs-ng/quickjs/security/advisories/GHSA-m53g-qx47-3h38) | Source publishes copy-array element count before slow-path getter initialization, matching the reported GC hazard. No native RCE reproduction run. |

The [advisory snapshot](../javascript/testdata/security/2026-11-03/upstream.json)
retains all ranges, dates and severities. Published native RCE severity must not
be restated as a demonstrated Go-host escape for this Wasm build.

## Host and artifact boundary review

- The fixed guest is hash checked before compilation. Rebuilding with the pinned
  source/SDK reproduced SHA-256 `29556e8cbe4b78704c71304aa3da83ae8881dd3d1447c65d27ade3237721d60e`, 929,138 bytes, byte-for-byte. Reproducibility also reproduces known unsafe source; it is not a safety certificate.
- Binary section inspection found **zero imports of any kind**, ten exported
  functions (initializer plus ABI), one exported memory, initial 32/max 1024
  pages. The host further clamps pages per engine. No WASI instance, filesystem,
  network service, callback registration or shared runtime pool is instantiated.
- JS source compilation does not execute initializers. Only private compiled
  bytecode is transferred to `JS_ReadObject`; no public bytecode-loading API is
  exposed. This matters because upstream explicitly excludes adversarial
  bytecode from its security model.
- Transfer/bytecode sizes and unsigned ABI limits are checked before calls.
  Host memory reads/writes are bounds checked; output/diagnostic buffers are
  copied before guest destruction. Diagnostic JSON validates version, fields,
  duplicate names, types and size; user names/messages do not become trusted
  limit sentinels.
- Calls use local budgets, detached values and fresh guest state. Close stops
  admission, cancels and drains complete calls. Registered host functions,
  runtime imports and async job pumping remain unavailable.
- Existing controls still do not promise a process-RSS/concurrency ceiling,
  hard constructor/Close deadline, or JS instruction/per-regex/stack quota.
  Treat exception text and reported locations as untrusted, potentially sensitive data.

No new host-capability exposure was identified in this targeted review. This is
not a formal verification of Wazero, the Go runtime, or the entire C engine.

## Scans, provenance and limitations

Scanner: **govulncheck v1.9.0**, built with Go 1.26.10. Database:
`https://vuln.go.dev`, last modified **2026-11-02T22:01:43Z** in every retained run.
Static target selections: darwin/arm64, linux/amd64 and windows/amd64. These are
source analyses, not new dynamic platform tests.

| Run | Reported findings |
|---|---|
| Optional module + tests, each of the three targets | 0; see local-replacement limitation below |
| JS-only CGO-free example binary | 0 |
| Native core + tests | Four messages, two advisory IDs, one conservative symbol trace |
| Assessment module + tests | 0; embedded comparison-engine C/Wasm is outside scanner coverage |
| Optional module-level scan | 0; this is not a complete selected-graph certificate |

**Important:** nested scans' SBOMs attribute local replacements as `..` and omit
`x/text`, although the selected graph contains it and module tests import
JSONata. Consequently a clean nested scan must not clear core/transitive
requirements. Scan the root independently and compare scanner coverage with
`go list` inventory. JSON scanner exit status zero alone is also insufficient:
the core run exited zero while emitting findings. Findings were parsed explicitly.
Future CI should use the scanner's normal failing text mode or a validated
finding-aware JSON gate, never a bare JSON command as a security pass condition.

Raw scanner streams are retained as deterministic gzip files, with configs,
SBOMs, findings and SHA-256 hashes in
[scans.json](../javascript/testdata/security/2026-11-03/scans.json).
Go module content verification passed. Source/SDK hashes and LLVM/wasi-libc pins
match upstream metadata. Existing guest MIT/Apache-family notices are present
and fingerprinted; an upgrade must refresh them from the selected source/SDK.
The official Wazero, WASI SDK and wasi-libc published-advisory endpoints returned
no entries; this is limited endpoint evidence, not an exhaustive LLVM/libc CVE audit.

Reproduction commands and the guest-only probe are in the
[evidence directory](../javascript/testdata/security/2026-11-03/README.md).
Review conclusions are time-bound: a future database or upstream security change
requires a new review. No third-party issue/report was filed by this work.

## Remediation recommendation

1. Update Go selections to `x/text` v0.43.0 and Wazero v1.12.0, inventory/scan
   `x/sys` v0.44.0, and add repeatable source/binary security gates across modules.
2. Upgrade the owned QuickJS build to v0.18.0, immutable commit
   `d7215b0ad3fc2c562e8317fd9bca058fd4863804`, keeping the pinned SDK unless new
   evidence requires a change. Rebuild/hash/license/capability verification is mandatory.
3. Explicitly handle the new terminal-error contract before that engine switch.
   v0.18.0 makes OOM and InternalError exceptions uncatchable by JS. Existing
   tests that catch OOM and then return a BigInt cannot simply retain their old
   expected result, nor should terminal failures be relabeled by message text.
4. Pass the full native and JS gates, remeasure, complete longer deployment
   soaks and choose a reporting route before a separately approved release.

The subsequent [Go dependency updates](javascript-go-security-updates.md) and
[QuickJS upgrade record](javascript-quickjs-upgrade.md) document the implemented
remediation. No dependency upgrade or terminal-error behavior change was
included in this historical audit PR.
