# Third-party notices

This repository's own code is MIT licensed. Portions of its native JSONata
algorithms follow the MIT-licensed JSONata reference implementation, copyright
IBM Corp. and JSONata contributors, as noted in source files. The upstream
license is retained in conformance/testdata/jsonata/LICENSE.

Pinned JSONPath fixtures retain their upstream license in
conformance/testdata/jsonpath/LICENSE. Fixture commits and SHA-256 hashes are
recorded in conformance/README.md and fixtures.lock.json.

Runtime supporting utilities are
[regexp2](https://github.com/dlclark/regexp2) (MIT) and
[Go text](https://pkg.go.dev/golang.org/x/text) (BSD-3-Clause).
Their source distributions contain their original licenses. Neither dependency
is a JSONPath or JSONata engine. The JavaScript JSONata npm dependency is pinned
solely for the separate development reference job and is not linked into Go code.

The optional JavaScript development module has separate dependency pins, artifact
provenance and licensing notes in [its README](../experiments/javascript/README.md).
It does not alter the native library dependency graph.

The owned experimental Wasm guest additionally retains QuickJS-ng and linked
WASI C-runtime/LLVM compiler-runtime notices in
[its guest directory](../experiments/javascript/internal/quickjswasm/guest).
Source, toolchain and artifact hashes are pinned in `lock.json` there.
