# Compatibility policy

Targets: JSONPath RFC 9535 (including RFC 9485 I-Regexp), JSONata 2.2.2.
See conformance/README.md for immutable fixture revisions. Upstream source
informs native implementations; Node and the JavaScript implementation are
test tools only. There are no complete-language runtime dependencies.

The pinned JSONata suite passes all 1,291 fixture files (1,686 cases), and the
JSONPath suite passes all 715 cases with the two explicit errata below. The
Node-only differential job additionally compares selected expressions and
operator combinations against JSONata 2.2.2. No public release has been published.

Pinned suites execute both ordinary evaluation and prepared evaluation for
every representable JSON input. JSONata fixtures with undefined input retain the
raw API because undefined is not a JSON document. The Node comparison also runs
prepared evaluation with production default limits, exercising specialized plans
alongside the interpreter. Preparation preserves complete input validation,
duplicate-key behavior, exact JSONPath numbers and lossless surrogate handling.

JSONata currently specializes conservative singular paths, scalar predicates,
selected builtins, direct array count/sum and simple object-array projections
and filters. JSONPath specializes singular scalar selectors. Unsupported syntax
or shapes use the full evaluator. JSONata bindings, host functions, nondefault
limits and explicit expression-depth instrumentation also use that fallback.
These are optimization boundaries, not language restrictions.

Compiled batches share an overall deadline, retain independent expression work
counters and enforce both registry and underlying native limits. Exact native
dispatch validates and detaches results once; external wrappers retain their
conversion boundary. Native callbacks get independent canceled scopes after
each expression; reusable budget scratch is limited to certified pure handlers.
The document index uses shared arenas, preserving the same public Node readers
and exact/lossless JSON representation. Nil and zero-value documents are rejected.

Additional conservative plans cover nested scalar projections, scalar filters,
and numeric arithmetic projection sums. Batch-local singular-prefix memoization
contains document nodes only, with bounded compile work/scratch and no global
cache. Numeric-literal comparisons can avoid intermediate interface boxing.
Unsupported projection shapes still use ordinary native evaluation.

Prepared documents retain shape and byte summaries so stricter evaluation
limits remain effective. Step counters account separately for preparation and
each evaluation's actual work; eliminating repeated work can change the exact
step threshold at which an evaluation stops. Simple plans check cancellation at
entry/exit and at most every 32 work units; output conversion and array/string
loops remain bounded. Host callbacks retain ordinary context deadlines.

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
