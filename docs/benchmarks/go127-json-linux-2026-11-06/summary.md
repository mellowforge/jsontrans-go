# Local benchmark summary

6 samples per configuration; median microseconds per operation. Ranges and allocation counts are in summary.csv/summary.json. Descriptive results; no statistical significance claim.

| Workload | Go 1.26 | Go 1.27, old JSON | Go 1.27 | Go 1.27 + prototype |
|---|---:|---:|---:|---:|
| 1KiB/decode | 26.24 | 25.22 | 15.64 | 13.76 |
| 64KiB/decode | 451.53 | 437.81 | 290.88 | 269.54 |
| array1MiB/decode | 51,774.28 | 47,454.18 | 27,237.35 | 24,353.97 |
| 64KiB/encode | 216.23 | 215.20 | 211.22 | 215.96 |
| 64KiB/eval_json/jsonata | 635.55 | 621.25 | 468.77 | 449.05 |
| 64KiB/eval_json/jsonpath | 632.45 | 616.58 | 467.10 | 447.87 |
| array1MiB/eval_json/jsonata | 87,055.67 | 83,321.41 | 64,992.78 | 60,734.83 |
| array1MiB/large_output | 138,606.99 | 132,377.24 | 108,675.57 | 100,401.42 |
| 64KiB/prepare | 44.84 | 46.30 | 47.56 | 46.76 |
| 64KiB/reuse/N1 | 0.57 | 0.58 | 0.62 | 0.62 |
| 64KiB/reuse/N16 | 7.72 | 7.50 | 7.81 | 7.72 |
| 64KiB/reuse/N64 | 30.34 | 28.93 | 30.86 | 30.82 |
| 64KiB/prepare_batch/N16 | 53.23 | 53.96 | 56.16 | 56.37 |
| 64KiB/reuse_mixed/N16 | 36.19 | 36.24 | 36.78 | 36.83 |
| JS/Eval | 1,052.37 | 1,056.90 | 1,063.47 | 1,048.41 |
| JS/EvalJSON | 1,049.40 | 1,048.91 | 1,051.20 | 1,056.00 |
| JS/PreparedFallback | 1,061.05 | 1,045.57 | 1,066.50 | 1,052.92 |

## Decode allocations

| Workload | Go 1.26 B/op; allocs/op | Go 1.27 B/op; allocs/op | Prototype B/op; allocs/op |
|---|---:|---:|---:|
| 1KiB | 19,024; 485 | 14,008; 247 | 12,368; 167 |
| 64KiB | 471,785; 491 | 337,737; 253 | 271,198; 173 |
| array1MiB | 32,495,800; 1,029,890 | 21,412,866; 486,888 | 19,168,262; 318,364 |

## Whole-process peak RSS

MiB minimum–maximum across benchmark processes. These processes run entire suites and warm the runtime; values are neither per-evaluation nor retained-document memory.

| Configuration | Native suite | JavaScript suite |
|---|---:|---:|
| go126 | 132.8–146.0 | 107.7–125.6 |
| go127-nojsonv2 | 131.2–151.2 | 119.2–142.1 |
| go127 | 115.0–127.9 | 115.2–139.7 |
| go127-jsontext | 127.0–131.0 | 121.3–135.0 |
