# Compatibility policy

Targets: JSONPath RFC 9535 (including RFC 9485 I-Regexp), JSONata 2.2.2.
See conformance/README.md for immutable fixture revisions. Upstream source
informs native implementations; Node and the JavaScript implementation are
test tools only. There are no complete-language runtime dependencies.

The pinned JSONata suite passes all 1,291 fixture files (1,686 cases), and the
JSONPath suite passes all 715 cases with the two explicit errata below. The
Node-only differential job additionally compares 585 selected expressions and
operator combinations against JSONata 2.2.2. No public release has been published.

Each fixture must run, including invalid syntax, native errors, missing results,
and explicitly permitted nondeterministic outputs. Passing upstream suites is
necessary but insufficient; extended Unicode, numeric, regex, formatting,
concurrency, cancellation, and resource tests are also required.

JSON objects are unordered at the common Go boundary. JSONPath object traversal
uses deterministic lexical key ordering, one ordering permitted by RFC 9535.
JSONPath selection preserves input numbers; arithmetic-free numeric comparison
does not round large integers to binary64. JSONata uses reference-compatible
binary64 values throughout its language boundary.

Raw JSON inputs retain serialized member order for JSONata enumeration; ordinary
Go maps have no such provenance and use lexical fallback. JSONata-created objects
preserve construction order, with integer index keys enumerated first. Raw callable
invocation preserves captured order metadata with evaluation-local mutable state.

JSONata regex operates on UTF-16 units like the pinned reference. The raw API
represents isolated units using WTF-8 strings internally; common results use the
lossless `UTF16String` and `UTF16Object` JSON marshalers only where needed. Adjacent
surrogate pairs canonicalize to ordinary Unicode scalars.

Trusted fixture runs raise step/time limits explicitly and honor per-case
`timelimit` and expression-frame `depth` guards. Production defaults remain
unchanged. JSONata's optional `WithEvaluationDepth` reports native U1001 while
retaining the underlying limit or deadline cause.

Confirmed upstream discrepancy: JSONPath CTS cases 516–517
(`functions, match, explicit caret/dollar`) interpret ^ and $ as anchors, while
RFC 9485 section 3 includes them in NormalChar. The fixture snapshot is preserved
unchanged. The implementation follows the RFC. The runner applies exactly two
guarded corrections, logs them, and executes both cases. See
[the errata record](../conformance/ERRATA.md) for the original expectations,
normative sources, and corrected results.
