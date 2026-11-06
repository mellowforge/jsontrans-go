# Local benchmark summary

6 samples per configuration; median microseconds per operation. Ranges and allocation counts are in summary.csv/summary.json. Descriptive results; no statistical significance claim.

| Workload | Go 1.26 | Go 1.27, old JSON | Go 1.27 | Go 1.27 + prototype |
|---|---:|---:|---:|---:|
| 64KiB/prepare_batch/N16 | 46.80 | 46.78 | 46.53 | 46.21 |

## Whole-process peak RSS

MiB minimum–maximum across benchmark processes. These processes run entire suites and warm the runtime; values are neither per-evaluation nor retained-document memory.

| Configuration | Native suite | JavaScript suite |
|---|---:|---:|
| go126 | 13.5–13.8 | unavailable |
| go127-nojsonv2 | 13.6–13.7 | unavailable |
| go127 | 13.9–14.2 | unavailable |
| go127-jsontext | 14.1–14.4 | unavailable |
