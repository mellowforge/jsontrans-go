# Native message results

6-sample warm medians. All paths include fresh input handling and encoded outputs. Values are per input message, including its complete fan-out. ns/op for parallel profiles is aggregate wall time divided by completed messages, not individual latency.

| JSONata, 1 KiB target | Fan-out | Strategy | us/message | B/message | Allocations |
|---|---:|---|---:|---:|---:|
| scalar | 1 | direct | 61.74 | 49638 | 910 |
| scalar | 1 | prepared | 9.31 | 15379 | 49 |
| scalar | 1 | batch | 9.39 | 15539 | 51 |
| scalar | 4 | direct | 246.97 | 196227 | 3619 |
| scalar | 4 | prepared | 12.15 | 16315 | 88 |
| scalar | 4 | batch | 11.58 | 15947 | 84 |
| scalar | 16 | direct | 982.43 | 782551 | 14453 |
| scalar | 16 | prepared | 23.07 | 20060 | 244 |
| scalar | 16 | batch | 19.76 | 17659 | 216 |
| object | 1 | direct | 124.64 | 104778 | 1966 |
| object | 1 | prepared | 35.03 | 50138 | 454 |
| object | 1 | batch | 35.90 | 50449 | 458 |
| object | 4 | direct | 498.05 | 416700 | 7843 |
| object | 4 | prepared | 93.25 | 114150 | 1686 |
| object | 4 | batch | 95.55 | 114462 | 1690 |
| object | 16 | direct | 2010.57 | 1667665 | 31386 |
| object | 16 | prepared | 327.30 | 370126 | 6615 |
| object | 16 | batch | 336.72 | 370410 | 6618 |
| array | 1 | direct | 68.46 | 51869 | 1023 |
| array | 1 | prepared | 22.98 | 26169 | 327 |
| array | 1 | batch | 23.67 | 26480 | 331 |
| array | 4 | direct | 271.72 | 204280 | 4021 |
| array | 4 | prepared | 62.98 | 58702 | 1158 |
| array | 4 | batch | 65.49 | 59009 | 1162 |
| array | 16 | direct | 1042.00 | 798130 | 15169 |
| array | 16 | prepared | 190.99 | 175364 | 3828 |
| array | 16 | batch | 197.53 | 175656 | 3832 |
| bindings | 1 | direct | 64.60 | 50769 | 916 |
| bindings | 1 | prepared | 18.16 | 24673 | 237 |
| bindings | 1 | batch | 18.77 | 24984 | 241 |
| bindings | 4 | direct | 256.72 | 200728 | 3639 |
| bindings | 4 | prepared | 46.32 | 53480 | 839 |
| bindings | 4 | batch | 47.42 | 53790 | 842 |
| bindings | 16 | direct | 1025.29 | 800608 | 14534 |
| bindings | 16 | prepared | 161.00 | 168706 | 3243 |
| bindings | 16 | batch | 163.22 | 169017 | 3247 |

Payload sizes vary; inventory.json records actual sizes and exact expression/corpus identities. Parallel results, controls, JavaScript baseline and all ranges are in summary.csv/summary.json. No NATS transport or tail latency is measured here.
