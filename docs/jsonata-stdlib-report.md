# JSONata standard library implementation status

The native JSONata standard library includes numeric, string, collection, higher-order, encoding, regex, nested evaluation, and date/time functions, including integer, number, and date pictures. The pinned standard-function, regex and matcher groups pass, as does the complete pinned interpreter suite. Passing tests documents the tested compatibility baseline rather than proving every possible input.

The implementation refers to the [MIT-licensed JSONata 2.2.2 reference](https://github.com/jsonata-js/jsonata/tree/v2.2.2/src); pinned fixture provenance and license are in `conformance/testdata/jsonata`. Regex matching uses regexp2/v2 with evaluation-local matcher instances, deadlines, and no shared mutable matcher state between continuations. UTF-16 matching preserves unpaired surrogate units as WTF-8 internally, with JSON projection through the shared JSON value support. Unicode case mappings use the focused Go text package.

Verification during development includes the pinned function, regex and matcher groups, plus regressions for nested budget consumption, output and collection limits, regex timeout and concurrency, large-number formatting, Unicode casing, UTF-16 matching, and lossless surrogate JSON output. Consult the full conformance run for current results; no complete conformance claim is made.
