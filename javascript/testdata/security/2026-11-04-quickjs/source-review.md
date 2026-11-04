# Pinned upstream source review, 2026-11-04

Source: QuickJS-ng v0.18.0, commit
`d7215b0ad3fc2c562e8317fd9bca058fd4863804`, archive SHA-256
`a49defe70cbcae9226d9fac025af116e87ebae2440dcff1ca3c80b79e1cdef0b`.
Review uses the extracted archive, independently rebuilt into the owned guest,
plus the separately documented complete-exhaustion patch. The patch changes
OOM reporting/context ownership, not the advisory sinks below.

The [published advisory index](https://github.com/quickjs-ng/quickjs/security/advisories)
contains ten advisories at this review. `upstream-advisories.jsonl` retains
IDs, URLs, dates and published ranges. No additional published advisory was
found. Version ranges are metadata, not proof of applicability or safety.
The historical Proxy overflow existed despite a range excluding our old pin.

| Advisory | Source inspection at this pin |
|---|---|
| [GHSA-v647-xff9-jxf7](https://github.com/quickjs-ng/quickjs/security/advisories/GHSA-v647-xff9-jxf7) | `js_new_callsite` clears transferred filename/function/name fields; `build_backtrace` no longer frees the value again after the consuming property helper fails. The reported double cleanup is removed. |
| [GHSA-crgj-9jv8-xfhq](https://github.com/quickjs-ng/quickjs/security/advisories/GHSA-crgj-9jv8-xfhq) | `js_create_resolving_functions` initializes both slots and resets the first after failure cleanup. Runtime module evaluation/job pumping remain unavailable in the adapter. |
| [GHSA-rxpx-wj25-jhqq](https://github.com/quickjs-ng/quickjs/security/advisories/GHSA-rxpx-wj25-jhqq) | `re_parse_disjunction` checks `dbuf_error` before patching the emitted branch operand. The reported write after failed growth is guarded. |
| [GHSA-3jf7-4qfx-xc2h](https://github.com/quickjs-ng/quickjs/security/advisories/GHSA-3jf7-4qfx-xc2h) | `JS_JSONStringify` normalizes string spacing with `JS_ToStringFree` and checks failure before treating it as a flat string. |
| [GHSA-wm7w-r6qf-3c9w](https://github.com/quickjs-ng/quickjs/security/advisories/GHSA-wm7w-r6qf-3c9w) | `js_atomics_store` frees the converted BigInt value if the target becomes out of bounds. Concurrent workers and host detach callbacks are unavailable in this adapter. |
| [GHSA-9wv5-5cgr-grj6](https://github.com/quickjs-ng/quickjs/security/advisories/GHSA-9wv5-5cgr-grj6) | `js_typed_array_sort` uses checked `js_calloc` for indices and `size_t elt_size` for copy sizes. Its published >1 GiB allocation trigger also exceeds the unchanged guest cap; caps alone are not treated as an overflow fix. |
| [GHSA-jp52-chwg-2p9p](https://github.com/quickjs-ng/quickjs/security/advisories/GHSA-jp52-chwg-2p9p) | `js_proxy_get_own_property_names` incrementally grows the key list using int64 arithmetic and checks `SIZE_MAX / sizeof(tab[0])`. The concrete old-pin 536870913-key fixture now returns a native exception instead of a guest out-of-bounds trap, with healthy recovery. |
| [GHSA-52gx-967m-hxp3](https://github.com/quickjs-ng/quickjs/security/advisories/GHSA-52gx-967m-hxp3) | `js_array_buffer_transfer` uses the buffer-specific realloc callback, rejects absent callbacks on resize, and transfers backing-store ownership explicitly. The adapter exposes no external-buffer C API. |
| [GHSA-r6vf-p8wr-4vg9](https://github.com/quickjs-ng/quickjs/security/advisories/GHSA-r6vf-p8wr-4vg9) | `set_array_length` clears each freed fast-array slot to JS_UNDEFINED before later regrowth. |
| [GHSA-m53g-qx47-3h38](https://github.com/quickjs-ng/quickjs/security/advisories/GHSA-m53g-qx47-3h38) | `with`, `toReversed`, `toSpliced` and `toSorted` use `js_allocate_fast_array`, which fills all published slots with JS_UNDEFINED before any reentrant property getter executes. |

The exact source is available under the immutable
[upstream tree](https://github.com/quickjs-ng/quickjs/tree/d7215b0ad3fc2c562e8317fd9bca058fd4863804).
The [release notes](https://github.com/quickjs-ng/quickjs/releases/tag/v0.18.0)
include terminal OOM/InternalError changes and context-creation OOM cleanup.
The owned patch additionally closes the catchable-null error-allocation edge,
validated by Wasm regression tests and the native sanitizer/GC harness.

Only the concrete Proxy case is dynamically compared against the old guest
here. This table is focused source review, not a claim of exhaustive advisory
reproduction, a full engine audit or general memory safety. Go vulnerability
scans do not analyze this C code. WebAssembly isolation and guest caps remain
necessary even after these known source defects are replaced. Private reporting,
production approval and prolonged deployment testing remain release decisions.
