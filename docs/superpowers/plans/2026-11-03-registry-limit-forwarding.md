# Registry limit-forwarding fix

Scope: fix the unsupported-limit failure in the experimental JS processor without
changing `Processor`, `Program` or `EvalOptions`, or weakening common/native budgets.

Implementation runs inline with regression-first tests and one final independent review.

- [x] Reproduce timeout-only and configured-default failures through registry,
      JSON, prepared, fallback and batch paths using the actual JS adapter.
- [x] Add optional `EvalLimitSupport.UnsupportedEvalLimits() LimitSet`; immutable
      bit flags cover the eight existing limit fields. Programs without it keep
      legacy behavior. Unknown flags fail closed.
- [x] Snapshot support metadata at registry/batch construction. Reject explicit
      nonzero unsupported overrides, including values equal to defaults, before
      validation/evaluation work. Negative limits still report ErrLimit.
- [x] Forward resolved supported fields only, retaining fully resolved common
      budgets and configured program/wrapper intersections. No caller pointers
      or stored configuration may be mutated. Preserve native fast dispatch.
- [x] Validate direct JSON/prepared helpers and mixed batches. Batch unsupported
      requests identify the first unsupported entry before any expression runs.
- [x] Declare JS unsupported steps/regex controls; keep direct rejection intact.
      Exercise cancellation, input/result bounds, metadata snapshots, nested
      wrappers, zero/nil overrides, all flag fields and typed diagnostics.
- [x] Update adapter/compatibility and assessment docs; retain historical probe
      results and add current owned-probe evidence. Promote full owned CI probes
      to strict acceptance for the recorded cases, without claiming production JS.
- [x] Run formatting, root/nested tests, race detection, vet, relevant bounded
      fuzzing and native prepared benchmark smoke checks.
- [x] Complete independent review (no blocking findings).
- [ ] Commit, push and open the PR; verify CI.
