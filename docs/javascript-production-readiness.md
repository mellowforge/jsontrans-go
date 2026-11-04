# JavaScript production readiness

**Status: candidate awaiting maintainer approval.** This record proposes the
first supported release of `github.com/mellowforge/jsontrans-go/javascript`.
Preparing or merging verification evidence does not publish a module version.
The module remains a preview until the maintainer explicitly approves the
profile and supported-platform policy below. A tag/release requires a separate
instruction after approval.

## Proposed support policy

Initial support covers Linux amd64, macOS arm64 and Windows amd64, with native
verification on Ubuntu 24.04, macOS 27.2 and Windows NT build 10.0.22621.0.
Other OS versions and architectures have no native acceptance evidence in this
record. They are not part of this initial support commitment. Go 1.26 is the
module minimum; verification records exact patch/toolchain versions rather than
promising compatibility with all future Go releases. The final acceptance matrix
uses Go 1.26.10 and 1.27.3 on Mac/Linux and Go 1.26.10 on Windows.

The supported profile is the [execution contract](javascript-execution-contract.md):

- Synchronous function expressions over detached JSON input/bindings, binary64
  numbers, missing versus null preservation and strict JSON result conversion.
- Immutable compiled programs, independent fresh guest state per evaluation,
  concurrent program reuse and explicit engine ownership/Close.
- Default one-second call timeout and 64 MiB guest linear-memory limit; memory
  configurable from 2–64 MiB in 64 KiB pages. Bounded cooperative host conversion,
  16 MiB transfers and 4 MiB compiled artifacts.
- Cancellation during guest execution, including built-ins and diagnostic
  processing. OOM/InternalError terminate that evaluation and cannot be caught
  to resume; a subsequent evaluation starts fresh. Native diagnostics retain
  their typed fields without granting trusted quota/conversion provenance.
- No host imports/WASI instance, filesystem, network, Node APIs, Go host-function
  registration, async job pumping, runtime module loading or runtime pooling.
  Per-guest UTC time snapshot and noncryptographic guest-local RNG.

Instruction counts, separate regex deadlines and stack quotas are unsupported;
explicit unsupported requests are rejected. Guest caps do not bound Go/native
code allocations, concurrency or total RSS. Applications must bound admission,
control limit overrides and treat diagnostics as untrusted potentially sensitive
data. Close drains admitted adapter calls but has no hard deadline or immediate
physical-memory guarantee. The [Windows investigation](javascript-windows-memory.md)
explains asynchronous internal monitor retention across a GC cycle.

Applications requiring hard process-memory or shutdown guarantees must use their
own supervised process boundary. This release does not supply one.

## Security ownership and pins

Private reports go to [koverboard@icloud.com](mailto:koverboard@icloud.com),
triaged by the repository maintainer. The maintainer authorized this address for
the policy; this task does not send a test email or claim independent mailbox
availability verification. The repository is private at preparation time.
[SECURITY.md](../SECURITY.md) defines reporting instructions. If made public,
enable GitHub private vulnerability reporting as an additional route and verify
that it is monitored; changing visibility is outside this PR.

| Component | Candidate pin |
|---|---|
| QuickJS-ng | v0.18.0, `d7215b0ad3fc2c562e8317fd9bca058fd4863804` |
| Owned terminal-OOM patch SHA-256 | `41e04680ad186ec1d230874e949593bf0723c443c31966941509e25461eb146b` |
| WASI SDK | 34.0, per-host source/toolchain checksums in the manifest |
| Wazero | v1.12.0 |
| Guest ABI / diagnostic schema | 3 / 2 |
| Guest artifact SHA-256 | `31aaf1d757a9071dafae75c424c49fb91d0ba70c455d5bde37afd37229b38dee` |

The [manifest](../javascript/internal/quickjswasm/guest/lock.json) and Go module
files are authoritative. This PR changes no runtime, guest, dependency or API.
The [upgrade record](javascript-quickjs-upgrade.md) supersedes the old guest's
historical release blocker and describes the separately owned OOM patch.
A Go scanner does not inspect the embedded C engine; current upstream notices
and the pinned source/patch must also be reviewed. No general engine memory
safety certification is implied by a clean scan or bounded fuzz campaign.

## Acceptance evidence

All **80 recorded steps passed** on candidate source
`acbf01cb0271089c08de2f63ece849067a2137fb`, using local machines without hosted
Actions. The [dated evidence](../javascript/testdata/readiness/2026-11-04/README.md)
retains exact commands, source/toolchain identities, exit codes, raw logs and
checksums. Results remain technical acceptance evidence, not maintainer sign-off.

| Machine / toolchains | Fresh acceptance checks | Result |
|---|---|---|
| Mac arm64, Go 1.26.10 and 1.27.3 | All three modules: uncached tests/race, vet and module verification; formatting | 25/25 steps passed |
| Ubuntu 24.04 amd64, Go 1.26.10 and 1.27.3 | Same native matrix, Node oracle, packaging regressions/outside consumer, 21 probes, 1,200 recovery pairs, 11 CI fuzz targets, guest rebuild/byte comparison and three-target Go security gates | 49/49 steps passed |
| Windows amd64, Go 1.26.10 | Public JavaScript module native vet, uncached tests/race, module verification, CGO-free example build and execution | 6/6 steps passed |

The [refreshed notice review](../javascript/testdata/readiness/2026-11-04/notices/source-review.md)
finds the same ten QuickJS advisory metadata records as the prior source review;
no new published Wazero/SDK repository advisories or later stable releases were
listed. The Go scanner database reports modification time
`2026-11-02T22:01:43Z`. Raw notices and scan streams are retained; these sources
are not a guarantee of complete vulnerability coverage.

Sixty production/module/guest-source files match extended-campaign source
`dc29b60` exactly. The final readiness changes are documentation/evidence only;
the evidence verifier checks their current runtime hashes. Final local checks
and independent review are recorded with the publication evidence.

Existing evidence retained as part of the review:

- [QuickJS remediation](javascript-quickjs-upgrade.md): source/artifact rebuilds,
  terminal-error/GC/initialization regressions, sanitizer harness and source review.
- [Extended campaigns](javascript-extended-validation.md): 3,197,100 accepted
  recovery pairs across Linux, Mac and Windows, plus extended strict fuzzing.
- [Windows memory analysis](javascript-windows-memory.md): pending cancellation
  monitors explain the reproduced spikes; an isolated join control removes them.

These campaigns ran the same runtime/guest/dependency pins. They are retained
with their original source identities, not relabeled as fresh runs. The known
minor observer-write-after-progress test remains a development-tool coverage
improvement, not an observed processor failure or a hidden acceptance pass.

## Candidate release notes and versioning

Proposed first supported module tag: **`javascript/v1.0.0`**, subject to maintainer
approval. No tag or release is created by this readiness PR. The root module is
versioned separately; its current required minimum is the real merged
`v1.1.1-0.20261103173822-e9b9c0c71fe0`, which contains registry limit forwarding.
Core v1.1.0 is too old for this adapter. A separate stable core tag may replace
the pseudo-version in a later release change; it is not necessary for consumers
to resolve the existing valid requirement.

Candidate release summary:

- Optional CGO-free JavaScript processor using owned QuickJS-ng/Wasm and Wazero;
  native JSONPath/JSONata remain independent of JavaScript dependencies.
- Existing processor registry, JSON, prepared-document and sequential batch
  integration with detached values and independent evaluations.
- Typed phase/native diagnostics, cancellation, bounded conversion, terminal
  native OOM/InternalError handling, and documented capability/resource limits.
- Native acceptance evidence for the proposed platform matrix, reproducible
  guest pins and a monitored private reporting contact.

Before tagging, approve the exact merged commit, check that runtime/dependency
pins have not changed since acceptance, and confirm the reporting contact is
active. Replace preview wording only with that approval recorded; publish the
nested module tag and release notes as a separately authorized action.

## Maintainer decision

The PR review should explicitly accept or amend:

- [ ] The execution profile and its resource/cancellation/diagnostic limitations.
- [ ] The initial platform policy and recorded native verification matrix.
- [ ] The designated reporting address and maintainer triage responsibility.
- [ ] The security/source review and complete acceptance evidence.
- [ ] The first-release version proposal and follow-up tagging procedure.

Approval is pending; no unchecked item is represented as completed production
sign-off. The pull request is the concrete artifact for that decision.
