# Release-candidate notice review — 2026-11-04

Candidate source: `acbf01cb0271089c08de2f63ece849067a2137fb`.
Review scope: current published repository advisories/release notes for QuickJS-ng,
Wazero and WASI SDK, fresh Go source/binary scans and identity of the previously
reviewed C/guest sources. This is a targeted release review, not a new exhaustive
engine/LLVM/libc audit or an independent memory-safety certification.

The raw API snapshots, request URLs, retrieval timestamps and hashes accompany
this record. Advisory queries are paginated. Release-note queries retain the
latest ten releases, including prereleases. At this capture, the newest stable
release in each index is already the pin used by the candidate.

| Component | Refreshed public notices | Candidate assessment |
|---|---|---|
| [QuickJS-ng](https://github.com/quickjs-ng/quickjs/security/advisories) | Ten published advisories; every retained ID, title, publication/update timestamp, URL and affected/patched range matches the prior fixture exactly. Latest stable release is v0.18.0. | Existing source-level remediation assessment still applies to the identical v0.18.0 source plus owned terminal-OOM patch. No new or changed advisory metadata was found. |
| [Wazero](https://github.com/wazero/wazero/releases/tag/v1.12.0) | Requested through the historical tetratelabs repository URL, which redirects to wazero/wazero. Published repository advisory index is empty. Latest stable release is v1.12.0. | Pinned release includes its documented masking and 32-bit fd_prestat_get overflow fixes. The adapter instantiates no WASI/host modules and supports only the tested 64-bit targets. Empty advisory metadata is not proof of safety; fresh Go scans also cover this dependency. |
| [WASI SDK](https://github.com/WebAssembly/wasi-sdk/releases/tag/wasi-sdk-34) | Published repository advisory index is empty. Latest stable release is SDK 34; its release notes document the LLVM and wasi-libc updates already pinned by the manifest. | Rebuilt from checksum-verified SDK/source archives and compared byte-for-byte. No newly published SDK repository advisory or later stable SDK notice was identified. This does not establish absence of vulnerabilities in every transitive compiler/libc component. |

The [prior ten-advisory source table](../../../security/2026-11-04-quickjs/source-review.md)
remains the detailed assessment, including the concrete Proxy reproduction and
limits of dynamic coverage. `../advisory-comparison.json` records the refreshed
metadata comparison. `../runtime-identity.json` independently hashes 60 production
Go/module/guest-source files and confirms identity with extended-campaign source
`dc29b60`; this includes the guest manifest, bridge, C sources and owned patch.
The owned patch's GC/context initialization/sanitizer and partial error-construction
results remain historical evidence, rather than being relabeled as fresh runs.

Fresh `govulncheck` v1.9.0 source/tests and CGO-free public-binary finding/coverage
gates pass for linux/amd64, windows/amd64 and darwin/arm64 using Go 1.26.10.
The scanner's recorded database modification time is `2026-11-02T22:01:43Z`;
raw scanner config/SBOM/finding streams are preserved. The Go scanner does not
inspect embedded C code. No unaddressed applicable published notice was identified
within this review scope; future notices or pin changes require another review.
