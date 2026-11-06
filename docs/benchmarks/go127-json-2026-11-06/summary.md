# Local benchmark summary

6 samples per configuration; median microseconds per operation. Ranges and allocation counts are in summary.csv/summary.json. Descriptive results; no statistical significance claim.

| Workload | Go 1.26 | Go 1.27, old JSON | Go 1.27 | Go 1.27 + prototype |
|---|---:|---:|---:|---:|
| 1KiB/decode | 20.95 | 19.84 | 12.76 | 11.03 |
| 64KiB/decode | 451.53 | 425.75 | 307.68 | 270.54 |
| array1MiB/decode | 38,397.70 | 37,231.00 | 21,628.85 | 18,181.52 |
| 64KiB/encode | 245.18 | 228.32 | 225.51 | 223.34 |
| 64KiB/eval_json/jsonata | 671.01 | 616.91 | 500.09 | 471.55 |
| 64KiB/eval_json/jsonpath | 661.04 | 615.93 | 495.24 | 478.45 |
| array1MiB/eval_json/jsonata | 69,460.48 | 67,350.06 | 52,353.32 | 48,832.69 |
| array1MiB/large_output | 109,058.87 | 106,389.53 | 88,260.21 | 83,431.92 |
| 64KiB/prepare | 40.99 | 40.46 | 41.96 | 43.84 |
| 64KiB/reuse/N1 | 0.47 | 0.45 | 0.50 | 0.49 |
| 64KiB/reuse/N16 | 6.05 | 5.78 | 6.14 | 6.06 |
| 64KiB/reuse/N64 | 23.60 | 22.46 | 23.95 | 23.78 |
| 64KiB/prepare_batch/N16 | 47.09 | 46.64 | 47.57 | 53.01 |
| 64KiB/reuse_mixed/N16 | 28.79 | 29.71 | 29.63 | 29.51 |
| JS/Eval | 780.74 | 768.58 | 764.48 | 752.50 |
| JS/EvalJSON | 774.82 | 758.60 | 755.89 | 738.01 |
| JS/PreparedFallback | 761.40 | 739.09 | 740.83 | 738.53 |

## Decode allocations

| Workload | Go 1.26 B/op; allocs/op | Go 1.27 B/op; allocs/op | Prototype B/op; allocs/op |
|---|---:|---:|---:|
| 1KiB | 19,024; 485 | 14,008; 247 | 12,368; 167 |
| 64KiB | 471,784; 491 | 337,738; 253 | 271,198; 173 |
| array1MiB | 32,495,792; 1,029,890 | 21,412,851; 486,888 | 19,168,254; 318,364 |

## Whole-process peak RSS

MiB minimum–maximum across benchmark processes. These processes run entire suites and warm the runtime; values are neither per-evaluation nor retained-document memory.

| Configuration | Native suite | JavaScript suite |
|---|---:|---:|
| go126 | 126.0–190.7 | 170.9–209.0 |
| go127-nojsonv2 | 120.5–148.2 | 173.1–203.8 |
| go127 | 122.3–138.2 | 162.8–190.8 |
| go127-jsontext | 119.5–189.9 | 166.6–202.4 |
