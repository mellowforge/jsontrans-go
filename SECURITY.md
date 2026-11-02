# Execution safety and vulnerability reports

Expressions may be supplied by untrusted users. Compilation, input conversion,
evaluation, and output conversion use cooperative work and size budgets.
Defaults are a 1-second timeout, 64 KiB expressions, depth 256, one million
steps, 100,000 items per collection, 16 MiB input/output, and a 50 ms regex
timeout. Configure larger positive limits for trusted workloads.

These controls are not OS memory isolation or guaranteed immediate termination.
Some library operations can only check cancellation before and after a call.
Applications requiring hard isolation should run the library in supervised
processes with appropriate operating-system resource limits.

Registered Go functions are trusted application code. They must cooperate with
context cancellation, account for their own allocations, and be concurrent-safe.
Expression input alone does not grant filesystem or network access; do not
register functions exposing capabilities that expression authors should not have.

Do not disclose exploitable vulnerabilities in public issues. Before public
release, maintainers must enable private vulnerability reporting on the GitHub
repository and identify a monitored contact route there. This project currently
has no supported public releases.
