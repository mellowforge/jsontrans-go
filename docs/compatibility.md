# Compatibility policy

Targets: JSONPath RFC 9535 (including RFC 9485 I-Regexp), JSONata 2.2.2.
See conformance/README.md for immutable fixture revisions. Upstream source
informs native implementations; Node and the JavaScript implementation are
test tools only for the native engines. The root module has no complete-language
runtime dependencies. The supported optional JavaScript processor and the
development-only assessment live in separate modules.

The pinned JSONata suite passes all 1,291 fixture files (1,686 cases), and the
JSONPath suite passes all 715 cases with the two explicit errata below. The
Node-only reference job additionally compares selected expressions, operator
combinations and the JSONata fast plans against JSONata 2.2.2. A reference
error without a code must match a Go error that also has none. The native-library
release [v1.1.0](https://github.com/mellowforge/jsontrans-go/releases/tag/v1.1.0)
requires Go 1.26 and does not bundle JavaScript. The independently versioned
[JavaScript v1.0.0 module](../javascript/README.md) is production-supported under
its [restricted execution profile](javascript-execution-contract.md) and
[platform policy](javascript-production-readiness.md).

Core [v1.2.0](releases/v1.2.0.md) introduces the prepared JSON-result helpers and
optional adapter limit-support declarations while preserving these language
targets and the Go 1.26 minimum. The release notes include merged-commit
installation instructions until the stable tag is published.

Pinned suites execute both ordinary evaluation and prepared evaluation for
every representable JSON input. JSONata fixtures with undefined input retain the
raw API because undefined is not a JSON document. The Node comparison also runs
prepared evaluation with production default limits, exercising specialized plans
alongside the interpreter. Preparation preserves complete input validation,
duplicate-key behavior, exact JSONPath numbers and lossless surrogate handling.

JSONata currently specializes conservative singular paths, scalar predicates,
selected builtins, direct array count/sum and simple object-array projections
and filters. Root object constructors with unique literal keys can select scalar
fields directly, including conservative numeric `+`, `-` and `*` expressions.
JSONPath specializes singular selectors and materializes only a selected
container subtree. Whole-input dimension checks remain effective. Unsupported
syntax or shapes use the full evaluator. JSONata bindings, host functions, nondefault
limits, explicit expression-depth instrumentation and documents with 10,000 or
more nodes also use that fallback. Batch prefix memoization applies only when
every program in the batch is native JSONata. These are optimization
boundaries, not language restrictions.

Compiled batches share an overall deadline, retain independent expression work
counters and enforce both registry and underlying native limits. Exact native
dispatch validates and detaches results once; external wrappers retain their
conversion boundary. Native callbacks get independent canceled scopes after
each expression; reusable budget scratch is limited to certified pure handlers.
The document index uses shared arenas, preserving the same public Node readers
and exact/lossless JSON representation. Nil and zero-value documents are rejected.

External programs may declare unsupported evaluation limit fields through
`EvalLimitSupport`. Registry, JSON, prepared and batch helpers reject explicit
unsupported overrides and omit unsupported synthesized fields, while retaining
common conversion budgets. Programs without this optional interface retain
legacy behavior. See [adapter guidance](adapters.md). The original QuickJS
assessment's 18-probe feasibility result is historical. The supported optional
processor passes all 21 owned probes plus the native/security/resource gates
recorded in its [production acceptance evidence](javascript-production-readiness.md).

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

JSONata regex and string ordering (comparison operators, order-by and `$sort`)
operate on UTF-16 code units like the pinned reference. The raw API
represents isolated units using WTF-8 strings internally; common results use the
lossless `UTF16String` and `UTF16Object` JSON marshalers only where needed. Adjacent
surrogate pairs canonicalize to ordinary Unicode scalars.

## Behavior pinned beyond the upstream suites

`jsonata/reference_semantics_test.go` pins cases the upstream suite does not
cover, with expected values recorded from JSONata 2.2.2 and checked on
ordinary, prepared and batch paths:

- a lone empty or keep-array (`[]`) array from a path's final step;
- keep-array results passed to functions and array constructors;
- array constructor flattening, including when `%` appears elsewhere;
- `@`/`#` tuple-stream steps flattening their results one level;
- regex literal scanning, with S0301/S0302 and compile-time pattern validation;
- S0217 at compile time for `%` after a step without a derivable parent;
- unbound parent references inside group-by pairs, which are undefined;
- NaN, infinite, huge and fractional arguments, following the JavaScript
  conversions the reference applies (`Array.slice`, `ToUint32`, `toFixed`,
  `Number.prototype.toString`), NaN falsiness and D1001 for infinite numbers in
  boolean contexts and indexes;
- negative zero, which survives arithmetic but prints as 0;
- dates outside ±8.64e15 ms, whose numeric components format as NaN, year
  widths, and timezones read as `parseInt` with `floor(v/100)` hours and
  `v % 100` minutes (so `"-0530"` is `-06:30`).

Computed object keys that evaluate to missing omit the entry without evaluating
its value, matching JSONata 2.2.2. This corrects an earlier `T1003` error for that
case; null and other non-string keys still report their native type error.
Regression coverage is in `jsonata/prepared_object_test.go` and the extended
Node reference comparison.

## Differences at the Go boundary

These are deliberate and documented rather than conformance failures:

- Non-finite arithmetic results (for example `1e308 * 10`) are values inside
  the language, as in the reference, but cannot be returned as JSON and fail
  with `ErrConversion`. Raw `Invoke` results preserve these numbers, including
  inside containers, and use output rather than input size limits.
- Host function results are validated: Go numbers become JSONata numbers, and
  unsupported types, cycles, NaN and Infinity fail with `ErrConversion`. The
  JavaScript reference passes host values through unchanged.
- Errors the reference raises without a code, such as invalid regular
  expressions (rejected while compiling, as in the reference) and plain host
  function errors, are `*jsontrans.Error` values with an empty `Code`. Host
  errors carry the reference's call position and function name as `Token`.
- Where the reference hangs, overflows its stack or fails with an internal
  JavaScript error, Go returns an error without a code: words, letters and
  roman numerals for non-finite integers, exponent pictures for non-finite
  numbers, names and words for invalid dates, and non-numeric timezones (D3134).
  Grouping separators inserted into the text of non-finite or exponent-form
  numbers may differ.
- JSONPath treats strings and object keys containing isolated surrogates as
  ordinary JSON. `length()` counts each isolated surrogate as one character,
  and `match`/`search` let `.` consume it as one character (as U+FFFD).

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
