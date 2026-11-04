# JavaScript production-readiness evidence — 2026-11-04

Candidate source: `acbf01cb0271089c08de2f63ece849067a2137fb`, built from merged
main `6c8c72e`. The [readiness record](../../../../docs/javascript-production-readiness.md)
contains the proposed profile/platform policy and explicit pending maintainer
approval. This preparation creates no supported module tag or production sign-off.

## Fresh local acceptance

| Native host | Toolchains | Recorded steps | Result |
|---|---|---:|---|
| Ubuntu 24.04.5 LTS amd64 / Bertram | Go 1.26.10 and 1.27.3 | 49 | PASS |
| macOS 27.2 arm64 | Go 1.26.10 and 1.27.3 | 25 | PASS |
| Windows NT 10.0.22621.0 amd64 VM | Go 1.26.10 | 6 | PASS |

All tests and race runs use `-count=1`. Mac/Linux run vet, tests, race detection
and module verification on all three modules with both Go versions, plus empty
formatting output. Windows runs the public module's native vet/tests/race/module
checks and builds/runs its example with CGO disabled. Go race checks use the
existing C toolchains. The Mac/Linux 1.27.3 optional-module checks exceed the
current hosted workflow's stable core-only matrix; all executed checks are kept.

Linux additionally executes all eleven CI fuzz targets at the workflow counts,
Node 22.22.3 reference comparisons, packaging regressions, the outside consumer,
all 21 owned probes, and 1,200 checked failure/success-to-healthy recovery pairs
across three engine lifecycles. A checksum-verified WASI SDK 34 rebuild reproduces
the owned Wasm byte-for-byte. Go security finding/coverage gates scan source/tests
in all three modules and a CGO-free public binary for each of linux/amd64,
windows/amd64 and darwin/arm64. Cross-target scans are separate from native tests.
No hosted Actions runner or credits are used.

`mac/report.json`, `linux/report.json` and `windows/report.json` retain commands,
source identity, toolchains, guest hash, completion state and exit codes. Step
logs and raw scanner streams are gzip-compressed without changing their bytes;
append `.gz` to a referenced raw filename when needed. Windows logs preserve
native output encoding; its JSON report has a UTF-8 BOM. Host metadata records
OS/toolchain versions. No binary executable, guest artifact or Git bundle is
committed here.

## Security/source review and continuity

The [fresh notice review](notices/source-review.md) explains the scope and limits.
Raw published advisory and latest-ten-release snapshots are under `notices/`,
with request provenance. QuickJS metadata exactly matches all ten previously
reviewed notices; Wazero/SDK published repository advisory indexes are empty.
The snapshot's latest stable releases match the pins. Absence from those indexes
is not proof of vulnerability absence. Go scans do not inspect the C engine.

`runtime-identity.json` hashes 60 production/module/guest-source files and proves
they are identical to extended-campaign source `dc29b60`. This supports reusing the
existing long soaks, strict resource-fuzz follow-up, sanitizer/source review and
Windows monitor investigation with their original source labels. It does not
claim they were rerun for this documentation commit or that their test sources
are identical. Current candidate tests include the stricter resource oracle.

## Reproduction and integrity

`runners/verify.py` implements the Mac/Linux commands and `runners/windows.ps1`
the Windows commands. They use the task-local paths recorded in each report and
existing toolchains/caches; adapt workspace/cache paths for another host.
Create a fresh isolated checkout with full history **and tags**, checked out at
the exact candidate source. For the original Linux/Windows setup, a full bundle
was transferred before cloning; Windows uses `core.autocrlf=false`. The Mac ran
from the clean candidate checkout. Never overwrite an earlier output directory.

```sh
python3 verify.py linux  # on Bertram after preparing the isolated checkout
python3 verify.py mac    # on the Mac with recorded tools available
```

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File windows.ps1
```

The runners capture failures and leave incomplete work marked pending/failed.
They enforce process timeouts for test steps; an interrupted driver/report is
not a pass. The Windows runner uses synchronous `Start-Process -Wait`, avoiding
the previously investigated asynchronous ExitCode loss. The prior long-run
orchestration failure remains in its original evidence, not rewritten here.

From the repository root:

```sh
python3 javascript/testdata/readiness/2026-11-04/verify_evidence.py
```

The verifier checks all 80 successful steps, required native matrices, log
presence, fuzz/probe/soak accounting, raw security stream hashes, current runtime
identity, and every file in `SHA256SUMS`. It validates this dated evidence; it is
not a new public API or a guarantee that future runs/platforms must produce the
same timings or memory readings. The manifest excludes only itself.

Final documentation-only checks are retained in `final-local-verification.json`
and its log. `final-packaging.json` records an additional passing outside-consumer
check of the evidence-bearing commit. `review.json` records the independent
whole-branch review with no actionable findings and its scope limits. Production approval, repository visibility changes and release
version/tagging remain maintainer actions.
