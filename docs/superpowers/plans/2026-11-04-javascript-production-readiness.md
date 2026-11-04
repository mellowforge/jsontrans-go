# JavaScript Production Readiness Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Prepare a reviewable production-readiness PR with a supported profile, reporting contact, candidate release notes and fresh local verification evidence.

**Architecture:** Consolidate the existing execution contract and completed security/stress investigations into a release decision record. Keep the runtime, guest, dependencies and public APIs unchanged; production approval and release tagging remain explicit maintainer actions.

**Tech Stack:** Go 1.26.10/1.27.3, existing Python/Node verification tools, native local Mac/Linux/Windows machines, gh CLI.

**Spec:** The user-approved four-part readiness scope in this conversation, with `docs/javascript-execution-contract.md`, `docs/javascript-quickjs-upgrade.md`, `docs/javascript-extended-validation.md` and `docs/javascript-windows-memory.md` as the existing behavioral authority.

## Global Constraints

- Native JSONPath/JSONata remain unchanged; optional JavaScript module remains a preview pending production approval.
- No runtime/dependency/guest changes, tags, releases, visibility changes or hosted Actions runs.
- Designated private contact: maintainer's Git email `koverboard@icloud.com`; triage by the maintainer. No report email is sent by this task.
- Local verification uses isolated source copies with full Git history/tags and existing task-local toolchains/caches.
- Preserve actual source identities, toolchain versions, commands, exit codes and failed attempts. Never mark an unexecuted gate passed.

## Review Focus

- Untested OS/architecture/toolchain combinations must not acquire implied support.
- Historical security findings must remain historical, with current remediation and current notice checks distinguished.
- Go scanner passes must not be presented as C/Wasm engine clearance.
- Missing native execution or incomplete commands must not pass the release gate.
- Candidate profile/version proposals must not be presented as already-approved production support or a published release.

### Task 1: Prepare the readiness decision record

**Files:** Create `docs/javascript-production-readiness.md`; modify `SECURITY.md`, `javascript/README.md`, `docs/local-verification.md`; create the evidence directory in Task 2.

**Interfaces:** Consumes existing execution/profile documents and maintainer reporting contact; produces proposed platform policy, explicit approval checklist and candidate release notes.

- [ ] Write the record with current pins, tested-profile proposal, reporting route, remaining approval/tagging actions and proposed first module tag `javascript/v1.0.0`.
- [ ] Add the reporting contact and correct outdated current-blocker wording while preserving historical records.
- [ ] Check document links/pins; run root and optional-module normal tests. Expected: no broken local links, pins match the manifest, tests pass.
- [ ] Commit the candidate documentation, providing one exact source commit for native verification.

### Task 2: Verify, review and publish the readiness PR

**Files:** Create `javascript/testdata/readiness/2026-11-04/` with runners, reports, compressed logs, fresh advisory/source review and checksums; finalize the readiness record.

**Interfaces:** Consumes Task 1's source commit and existing local-verification commands; produces auditable platform/security evidence and a gh CLI PR.

- [ ] Run fresh normal/race/vet/module verification on all three modules on Mac/Linux with Go 1.26.10 and 1.27.3; run Windows public-module native vet/test/race/module/CGO-free example checks with Go 1.26.10.
- [ ] On Linux run all CI fuzz targets, Node oracle, packaging regressions/outside consumer, owned probes, bounded recovery soak, guest rebuild/byte comparison and three-target Go vulnerability scans. Expected: every required command exits zero, formatting is empty, guest bytes match and probe/security gates pass.
- [ ] Refresh QuickJS/Wazero/WASI SDK public notices and compare the actual pinned sources/remediation record. Expected: no unaddressed applicable published notice; limitations documented.
- [ ] Archive reproducible reports with exact source SHA and checksums. Link existing extended soaks and memory investigation rather than rerunning unchanged hour campaigns.
- [ ] Confirm the final diff changes only documentation/evidence; run final local formatting/tests/race/vet, evidence integrity and link/pin checks.
- [ ] Request one independent whole-branch review; resolve material findings and commit the final evidence.
- [ ] Push a `[skip ci]` tip and open the production-readiness PR with gh CLI. Keep production sign-off and tags pending maintainer review.
