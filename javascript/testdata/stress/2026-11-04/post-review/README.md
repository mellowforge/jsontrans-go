# Strict resource-oracle review correction

The independent whole-branch review found one Important validation issue: the
new expected-error branches could accept otherwise expected failures joined
with unrelated cancellation/closure, and could accept an unrelated InternalError
message as the allocation fixture's OOM. This was reproduced with constructed
Go errors, without any new native trigger. The regression failed on all six
joined causes and the unrelated internal message before correction.

Source `9eadb55dbd4e297fb99915167bbfbd5542cf1868` extracts and tightens the oracle.
It rejects context cancellation, deadlines and ErrClosed before classifying
outcomes. The fixed over-cap allocation fixture must retain the known native
InternalError / out-of-memory diagnostic; because it does not consume available
heap before requesting an oversized buffer, no broad fallback is allowed here.
This is a fixture assertion, never text-based classification in the library.

Only test files changed in the correction. The soak harness, runtime, guest and
dependencies remain unchanged. All earlier campaign evidence stays labelled
with its actual `dc29b60` source and is not relabelled as the stricter run.
The strict resource campaign and full verification are recorded separately.
The reviewer was not asked to repeat the review; the single fix pass is checked
with RED→GREEN regressions and renewed suites.

One Minor coverage suggestion is deferred: the observer-error test currently
fails at baseline, so it does not directly assert nonzero-count preservation
and engine draining after a late progress-write error. Cleanup implementation
was reviewed; the separate cleanup-cancellation and process interruption checks
are retained. No observed runtime failure is claimed for the missing test.

The stricter resource rerun completed 100,000 executions (190.058 seconds) on
Linux. All three modules pass final normal/race/vet checks on Mac and Linux.
Native Windows public vet/tests/race/module verification and the CGO-free
example also pass on this oracle revision. See the reports and compressed logs.
