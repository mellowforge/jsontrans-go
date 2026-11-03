# Assessment-only QJS compatibility copy

This internal package copies the 15 runtime Go files and the unchanged Wasm
from `github.com/fastschema/qjs` **v0.0.6**, commit
`461716f4f380f81ffd09378751f1812919cddbca`. The upstream MIT license is retained
in LICENSE. `upstream.json` records every original file hash and the patched
runtime hash; `runtime.patch` is the complete change to upstream runtime code.
This is a development comparison backend, never the public JavaScript adapter.
The QJS tag's QuickJS-ng submodule is `d01ca4491fb24ccfeccb4c7394e28a3b21fd5986`;
its MIT notice is retained in LICENSE.quickjs. This unsafe source revision is
also identified in the historical security review.

Wazero v1.12 assigns function type IDs within a runtime. Upstream QJS creates its
compiled module in one runtime, then installs host imports and instantiates that
module in a different runtime. This rejects identical signatures with different
type IDs. The patch compiles the same guest bytes into the destination runtime
before installing imports, sharing the original machine-code cache. There is no
Wasm, bridge, capability, timeout or memory-profile change. Repeated runtime and
assessment semantic tests cover the failure. The decode/validation work on each
runtime is additional overhead: historical timings do not describe this copy.

The upstream module is deliberately retained as an advisory dependency in the
assessment import graph. The finding-aware gate rejects its reported Go
advisories even though execution uses the local copy. Copied source is also
analyzed as assessment code. Neither scan inspects the prebuilt C/Wasm engine,
nor proves that the copied code is free of vulnerabilities. Review both source
and upstream notices on updates. The upstream prebuilt asset's original build
provenance is not improved or certified by copying it.

Upstream filesystem mounts, host imports, shared configuration/cache machinery
and cleanup behavior remain comparison limitations. This patch does not harden
QJS or make it suitable for untrusted execution. Do not copy it into production
or use its historical assessment failures as accepted public-adapter behavior.

The runtime-local compilation handle is closed on both successful construction
and later setup failure. This balances its additional shared-cache reference
while preserving the global cache owner and the instantiated guest. A pinned
Wazero cache-inspection regression verifies that balance and a live evaluation;
the broader inherited cleanup limitations above remain unchanged.
