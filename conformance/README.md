# Pinned conformance data

- JSONata 2.2.2: jsonata-js/jsonata commit
  `6c7e95fdbf4405a1e741852a7cd8cd985b4305bb`.
  `testdata/jsonata` contains its language-neutral `test/test-suite` and MIT license.
- JSONPath RFC 9535: jsonpath-standard/jsonpath-compliance-test-suite commit
  `20ebdcce5008c8a777ee9c80ad853ecf606950b5`.
  `testdata/jsonpath` contains cts.json, its schema, and license.

Fixtures are unchanged upstream snapshots. Normal Go tests require no network
or Node. Update snapshots only with reviewed provenance and recorded compatibility
results. Never add per-case ignore lists to make incomplete implementations pass.

Original projects:
https://github.com/jsonata-js/jsonata
https://github.com/jsonpath-standard/jsonpath-compliance-test-suite
