#!/usr/bin/env python3
"""Validate separate runtime-slot controls; do not merge them with primary runs."""
import csv
import json
import math
from pathlib import Path
import statistics
import sys

root = Path(__file__).resolve().parent
observations = []
for version in ['baseline', 'main']:
    for procs in [1, 2]:
        for round_ in range(1, 4):
            meta = json.loads((root/'controls'/version/f'round-{round_}-procs-{procs}'/'metadata.json').read_text())
            r = meta['report']; c = r['config']
            assert meta['completed'] and meta['container_removed'] and meta['diagnostic_trace'] is False
            assert meta['go'] == 'go1.27.3' and meta['duration'] == '5s'
            assert meta['image'] == 'nats@sha256:afae2fdbe2e326c65e1246cb83b9d05d319188734f3bd577ac8072daae78a10f'
            assert c == {'client_procs': procs, 'engine': 'jsonata', 'kind': 'object', 'strategy': 'prepared', 'fanout': 4,
                         'workers': 1, 'target_rate': 5000, 'count': 25000, 'queue_capacity': 64, 'target_bytes': 1024}
            assert r['gomaxprocs'] == procs and r['server_version'] == '2.15.2' and r['valid']
            assert r['sent'] == r['received'] == 25000
            assert r['processed'] == r['completed'] and r['completed']+r['queue_rejected'] == r['sent']
            assert r['outputs'] == r['completed']*4
            assert r['lossless'] == (r['queue_rejected'] == 0)
            for error in ['transform_errors', 'publish_errors', 'invalid_outputs', 'async_errors', 'input_pending_dropped', 'output_pending_dropped', 'unaccounted_inputs', 'incomplete_fanout']:
                assert r[error] == 0, error
            assert math.isclose(r['actual_offered_per_second'], r['sent']/r['publish_seconds'], rel_tol=1e-9)
            observations.append({'version': version, 'procs': procs, 'round': round_, 'lossless': r['lossless'],
                                 'completed': r['completed'], 'queue_rejected': r['queue_rejected'],
                                 'actual_offered': r['actual_offered_per_second'],
                                 'processing_p50_us': r['processing']['p50_ns']/1000,
                                 'queue_p99_ms': r['queue_wait']['p99_ns']/1e6, 'e2e_p99_ms': r['end_to_end']['p99_ns']/1e6})
summary = []
for version in ['baseline', 'main']:
    for procs in [1, 2]:
        rows = [r for r in observations if r['version'] == version and r['procs'] == procs]
        assert len(rows) == 3
        summary.append({'version': version, 'procs': procs, 'lossless_runs': sum(r['lossless'] for r in rows),
                        'queue_rejected': sum(r['queue_rejected'] for r in rows),
                        **{k: statistics.median(r[k] for r in rows) for k in ['actual_offered', 'processing_p50_us', 'queue_p99_ms', 'e2e_p99_ms']},
                        'e2e_p99_min_ms': min(r['e2e_p99_ms'] for r in rows), 'e2e_p99_max_ms': max(r['e2e_p99_ms'] for r in rows)})
for round_ in range(1, 4):
    pair = [r for r in observations if r['version'] == 'main' and r['round'] == round_]
    rates = [r['actual_offered'] for r in pair]
    assert all(abs(r-5000) <= 250 for r in rates) and max(rates)-min(rates) <= .05*min(rates)
(root/'controls'/'summary.json').write_text(json.dumps({'observations': observations, 'summary': summary}, indent=2)+'\n')
with (root/'controls'/'observations.csv').open('w', newline='') as stream:
    writer = csv.DictWriter(stream, fieldnames=list(observations[0]), lineterminator='\n'); writer.writeheader(); writer.writerows(observations)
print(json.dumps(summary, indent=2))
