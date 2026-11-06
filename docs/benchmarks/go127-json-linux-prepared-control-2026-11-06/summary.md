# Local benchmark summary

6 samples per configuration; median microseconds per operation. Ranges and allocation counts are in summary.csv/summary.json. Descriptive results; no statistical significance claim.

| Workload | Go 1.26 | Go 1.27, old JSON | Go 1.27 | Go 1.27 + prototype |
|---|---:|---:|---:|---:|
| 64KiB/prepare_batch/N16 | 52.93 | 53.75 | 56.30 | 55.86 |

## Whole-process peak RSS

MiB minimum–maximum across benchmark processes. These processes run entire suites and warm the runtime; values are neither per-evaluation nor retained-document memory.

| Configuration | Native suite | JavaScript suite |
|---|---:|---:|
| go126 | 11.8–12.2 | unavailable |
| go127-nojsonv2 | 12.0–12.0 | unavailable |
| go127 | 12.5–12.9 | unavailable |
| go127-jsontext | 12.5–12.8 | unavailable |
