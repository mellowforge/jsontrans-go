# Default-branch rules and hosted verification

The public repository protects its default branch (`main`) with a repository
ruleset. The checked-in definition is `.github/rulesets/main.json`; GitHub's
repository Rules page is the authority for currently enforced settings.

## Merge policy

- Changes reach `main` through pull requests.
- All CI and Go security checks must pass, and the PR branch must be up to date.
- Review conversations must be resolved.
- Force pushes and deletion of the default branch are blocked.
- There are no bypass actors, including an administrator bypass entry.
- Zero external approving reviews are required while the project has one
  maintainer. The maintainer can merge their own green PR; CI remains required.
  Revisit the review threshold when another maintainer joins.

The ruleset does not change allowed merge methods, require a merge queue, or
restrict feature branches and release tags. Existing release tags remain intact.

## Required checks

The required contexts are captured from actual GitHub Actions check runs, with
the GitHub Actions application ID recorded in the ruleset. All four native Go/OS
matrix entries, fuzzing, reference comparisons, JavaScript assessment/module/
Windows checks, guest reproducibility and all three Go security targets are
required. New or renamed jobs need a coordinated ruleset update; a nonexistent
required context will block merges.

The CI matrix executes native Linux and macOS core checks with Go 1.26.x and
stable, plus native Windows public-JavaScript tests/race detection. Security
matrix targets are source/binary cross-target scans on Linux; they do not
replace native execution. Full local verification remains documented in
[local-verification.md](local-verification.md).

## Workflow operation

Both workflows support pull requests, pushes to `main`, and manual dispatch.
Feature-branch pushes do not start duplicate push and PR matrices. Required jobs
have no path filters or skip conditions that would silently omit their status.
Do not use `[skip ci]` for a PR that must merge through these rules.

Standard hosted runner labels are `ubuntu-latest`, `macos-latest` and
`windows-latest`. They use public-repository compute eligibility. A queued macOS
job may simply be waiting for capacity; missing or failed jobs are never treated
as successful verification. The jobs keep read-only contents permissions.

Public visibility also enables private vulnerability reporting through the
Security tab. The email route in [SECURITY.md](../SECURITY.md) remains available.
No self-hosted runner service or paid/larger runner is added by this setup.
