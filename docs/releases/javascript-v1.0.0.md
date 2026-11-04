# JavaScript processor v1.0.0

Approved 2026-11-04. Module:
`github.com/mellowforge/jsontrans-go/javascript`; repository tag:
**`javascript/v1.0.0`**. This is the first production-supported release of the
optional processor. The native JSONPath/JSONata module is versioned separately
and acquires no JavaScript runtime dependencies.

## Install

Go 1.26 or newer is required. The repository is private; configure Git access
and add the repository prefix to your existing GOPRIVATE patterns before fetching:

```sh
GOPRIVATE=github.com/mellowforge/jsontrans-go go get github.com/mellowforge/jsontrans-go/javascript@v1.0.0
```

The required native-core minimum remains
`v1.1.1-0.20261103173822-e9b9c0c71fe0`, which contains registry limit forwarding.
Core v1.1.0 is too old for this adapter; Go resolves the declared minimum when
installing the JavaScript module. Consumers do not need the development replace.

## Supported behavior

- CGO-free QuickJS-ng/WebAssembly execution through Wazero, using the existing
  Processor/Program contracts and instance-local transformer registry.
- Synchronous function expressions receiving detached JSON input and bindings;
  binary64 numbers, missing versus null preservation and strict JSON results.
- Immutable compiled programs safe for concurrent reuse, with fresh guest state
  per evaluation. The application owns and closes the engine.
- JSON, prepared-document fallback and sequential batch integration.
- Typed native/phase diagnostics, cancellation and bounded conversion. Native
  OOM/InternalError terminate an evaluation; the next call starts fresh.

Initial supported targets are Linux amd64, macOS arm64 and Windows amd64 within
the [approved platform policy](../javascript-production-readiness.md).
The [execution contract](../javascript-execution-contract.md) is authoritative.

## Execution limits

Default call timeout is one second; guest linear memory defaults to 64 MiB and
is configurable from 2–64 MiB. Compiled artifacts cap at 4 MiB; transfers at
16 MiB. Host conversion checks are cooperative. Applications must control
admission/concurrency and per-call limit overrides.

The guest exposes no host imports/WASI instance, filesystem, network, Node APIs,
Go host functions or runtime module loader. Promises/async job pumping, runtime
pooling, instruction counting, separate regex deadlines and stack quotas are
unsupported. Time is a per-guest UTC snapshot; the RNG is noncryptographic.

Guest limits do not cap total process memory. Engine initialization does not
promise hard interruption inside trusted Wasm compilation. Close has no hard
shutdown deadline or immediate RSS-reclamation guarantee. Diagnostics are
untrusted and may contain sensitive input. Applications needing hard resource
isolation must provide a supervised process boundary.

## Verification and security

The [acceptance record](../javascript-production-readiness.md) contains 80 passing
local verification steps across Mac/Linux/Windows, including uncached native
normal/race tests, vet, module verification, fuzzing, outside-consumer packaging,
the Node oracle, all 21 owned probes, 1,200 recovery pairs, reproducible guest
builds and three-target Go vulnerability scans. Historical extended campaigns
supply 3,197,100 additional accepted recovery pairs with original source labels.
The release preserves the accepted runtime, dependency and guest pins.

Pins: QuickJS-ng v0.18.0 with the owned terminal-OOM patch, WASI SDK 34.0,
Wazero v1.12.0, guest ABI 3 / diagnostic schema 2. Guest artifact SHA-256:
`31aaf1d757a9071dafae75c424c49fb91d0ba70c455d5bde37afd37229b38dee`.
Go scans do not inspect embedded C code; the acceptance record includes the
scoped source/advisory review and its limitations. No universal memory-safety
or hard in-process isolation guarantee is made.

Report suspected vulnerabilities privately to
[koverboard@icloud.com](mailto:koverboard@icloud.com); the repository maintainer
handles triage. See [SECURITY.md](../../SECURITY.md).
