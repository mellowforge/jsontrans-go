#!/usr/bin/env python3
"""Validate and compare three serially paired full-matrix NATS load runs."""
import argparse
import csv
from datetime import datetime
import itertools
import json
import math
import re
from pathlib import Path
import statistics

from summarize import summarize


STRATEGIES = ['direct', 'prepared', 'batch']
CONFIG_FIELDS = ['engine', 'kind', 'fanout', 'workers', 'target_rate', 'count', 'queue_capacity', 'target_bytes', 'strategy']


def require(condition, message):
    if not condition:
        raise ValueError(message)


def configurations(profile='full', seconds=1):
    if profile == 'object-one-worker':
        return [dict(zip(CONFIG_FIELDS[:-1], ['jsonata', 'object', n, 1, 5000, int(seconds*5000), 64, 1024])) for n in [1, 4]]
    require(profile == 'full', 'unknown comparison profile')
    return [dict(zip(CONFIG_FIELDS[:-1], [engine, kind, n, w, r, r, w*64, 1024]))
            for engine, kind in [('jsonata', 'scalar'), ('jsonata', 'object'), ('mixed', 'mixed')]
            for n, w, r in itertools.product([1, 4, 16], [1, 4, 8], [1000, 5000])]


def key(config):
    return tuple(config[name] for name in CONFIG_FIELDS)


def read_run(directory, pair, manifest):
    # Existing validation rejects missing/duplicate cases, bad quantiles, loss
    # misclassification and broken input/output/transport accounting.
    summarize(directory, write_output=False)
    meta = json.loads((directory / 'metadata.json').read_text())
    profile = manifest.get('profile', 'full')
    configs = configurations(profile, 10 if profile == 'object-one-worker' else 1)
    require(meta.get('workload_profile', 'full') == profile, 'mismatched workload profile')
    expected = {'rounds': 1, 'round_offset': pair-1, 'cases_per_round': len(configs)*3, 'duration': manifest.get('duration', '1s'),
                'go': manifest['go'], 'image': manifest['image'], 'broker_cpus': '16,17',
                'broker_memory_bytes': 512 << 20, 'api_profile': 'public-prepared-json-v1',
                'entry_points': {'direct': 'jsontrans.EvalJSON', 'prepared': 'jsontrans.EvalPreparedJSON', 'batch': 'Batch.EvalPreparedJSON'}}
    require(all(meta.get(k) == v for k, v in expected.items()), f'mismatched metadata: {directory}')
    host = meta['container_inspect']['HostConfig']
    require(host['CpusetCpus'] == '16,17' and host['Memory'] == 512 << 20, 'broker resource settings changed')
    require('GOMAXPROCS=2' in meta['container_inspect']['Config']['Env'], 'broker GOMAXPROCS changed')
    rows = [json.loads(line) for line in (directory / 'runs.jsonl').read_text().splitlines() if line.strip()]
    offset = (pair-1) % len(configs)
    configs = configs[offset:] + configs[:offset]
    order = [(index, {**c, 'strategy': STRATEGIES[(si+pair-1) % 3]})
             for index, c in enumerate(configs) for si in range(3)]
    require(len(rows) == len(order), 'wrong number of cases')
    result = {}
    for row, (index, config) in zip(rows, order):
        r = row['report']
        require(row['round'] == 1 and row['case_index'] == index and r['config'] == config, 'case/strategy order changed')
        require(r['server_version'] == '2.15.2' and r['gomaxprocs'] == config['workers'], 'server/client settings changed')
        counters = ['sent', 'received', 'processed', 'completed', 'outputs', 'queue_rejected', 'transform_errors',
                    'publish_errors', 'invalid_outputs', 'async_errors', 'input_pending_dropped', 'output_pending_dropped',
                    'unaccounted_inputs', 'incomplete_fanout', 'allocated_bytes_during_run', 'mallocs_during_run',
                    'gcs_during_run', 'max_pacing_lateness_ns']
        require(all(type(r[name]) is int and r[name] >= 0 for name in counters), 'invalid nonnegative integer counter')
        require(all(type(r['config'][name]) is int for name in CONFIG_FIELDS if name not in ['engine', 'kind', 'strategy']), 'invalid configuration counter')
        require(type(r['valid']) is bool and type(r['lossless']) is bool, 'invalid result classification')
        require(r['received'] == r['sent'] and r['processed'] == r['completed']+r['transform_errors']
                and r['completed'] <= r['sent'], 'inconsistent completion accounting')
        require(r['transform_errors'] == 0, 'transformation errors are not acceptable load results')
        for name in ['publish_seconds', 'total_seconds', 'actual_offered_per_second', 'completed_per_second',
                     'allocated_bytes_during_run', 'mallocs_during_run', 'gcs_during_run', 'max_pacing_lateness_ns']:
            require(math.isfinite(r[name]) and r[name] >= 0, f'invalid metric {name}')
        for label in ['processing', 'queue_wait', 'end_to_end']:
            require(all(type(r[label][f'{q}_ns']) is int and 0 <= r[label][f'{q}_ns'] < 1 << 63 for q in ['p50', 'p95', 'p99']), 'invalid integer percentile')
        require(r['publish_seconds'] > 0 and r['total_seconds'] >= r['publish_seconds'], 'invalid elapsed times')
        require(math.isclose(r['actual_offered_per_second'], r['sent']/r['publish_seconds'], rel_tol=1e-9), 'incorrect actual offered rate')
        require(math.isclose(r['completed_per_second'], r['completed']/r['total_seconds'], rel_tol=1e-9), 'incorrect completed throughput')
        result[key(config)] = r
    return result


def metrics(rows):
    median = lambda f: statistics.median(f(r) for r in rows)
    out = {'lossless_rounds': sum(r['lossless'] for r in rows),
           'completion_percent': median(lambda r: 100*r['completed']/r['sent']),
           'completed_total': sum(r['completed'] for r in rows),
           'sent_total': sum(r['sent'] for r in rows),
           'queue_rejected_total': sum(r['queue_rejected'] for r in rows),
           'actual_offered_per_second': median(lambda r: r['actual_offered_per_second']),
           'completed_per_second': median(lambda r: r['completed_per_second']),
           'pacing_lateness_max_us': max(r['max_pacing_lateness_ns']/1000 for r in rows),
           'allocated_bytes': median(lambda r: r['allocated_bytes_during_run']),
           'allocated_bytes_per_sent': median(lambda r: r['allocated_bytes_during_run']/r['sent']),
           'mallocs_per_sent': median(lambda r: r['mallocs_during_run']/r['sent']),
           'gcs': median(lambda r: r['gcs_during_run'])}
    for numerator, output in [('allocated_bytes_during_run', 'allocated_bytes_per_completed'), ('mallocs_during_run', 'mallocs_per_completed')]:
        out[output] = median(lambda r: r[numerator]/r['completed']) if all(r['completed'] for r in rows) else None
    for label in ['processing', 'queue_wait', 'end_to_end']:
        prefix = {'queue_wait': 'queue', 'end_to_end': 'e2e'}.get(label, label)
        for p in ['p50', 'p95', 'p99']:
            out[f'{prefix}_{p}_us'] = median(lambda r: r[label][f'{p}_ns']/1000)
        out[f'{prefix}_p99_min_us'] = min(r[label]['p99_ns']/1000 for r in rows)
        out[f'{prefix}_p99_max_us'] = max(r[label]['p99_ns']/1000 for r in rows)
    return out


def change(before, after):
    return 100*(after/before-1) if before else None



def validate_execution(directory, pairs):
    with (directory / 'execution.tsv').open() as stream:
        rows = list(csv.DictReader(stream, delimiter='\t'))
    expected = [(pair, version) for pair in range(1, pairs+1)
                for version in (['baseline', 'main'] if pair % 2 else ['main', 'baseline'])]
    require(len(rows) == len(expected), 'incomplete paired execution log')
    previous_end = None
    for row, (pair, version) in zip(rows, expected):
        require(int(row['pair']) == pair and row['version'] == version
                and int(row['offset']) == pair-1 and int(row['exit_status']) == 0,
                'wrong paired version order, rotation or exit status')
        start, end = [datetime.fromisoformat(row[k].replace('Z', '+00:00')) for k in ['start_utc', 'end_utc']]
        require(start.tzinfo is not None and end.tzinfo is not None and start < end, 'invalid execution timestamps')
        require(previous_end is None or start >= previous_end, 'overlapping load runs')
        previous_end = end



def load_matches(before, after, target):
    b, a = before['actual_offered_per_second'], after['actual_offered_per_second']
    return abs(b-a) <= .05*min(b, a) and abs(b-target) <= .05*target and abs(a-target) <= .05*target


def compare(directory):
    manifest = json.loads((directory / 'manifest.json').read_text())
    profile = manifest.get('profile', 'full')
    require(profile in ['full', 'object-one-worker'], 'unknown comparison profile')
    pairs = 6 if profile == 'object-one-worker' else 3
    duration = '10s' if profile == 'object-one-worker' else '1s'
    require(manifest['pairs'] == pairs and manifest.get('duration', '1s') == duration and manifest['baseline'] != manifest['main'], 'invalid source/pair/duration declaration')
    require(all(re.fullmatch('[0-9a-f]{40}', manifest[v]) for v in ['baseline', 'main']), 'invalid source revisions')
    require(re.fullmatch('nats@sha256:[0-9a-f]{64}', manifest['image']), 'unpinned broker image')
    validate_execution(directory, pairs)
    require(manifest['go'] == 'go1.27.3', 'unexpected comparison toolchain')
    versions = {v: [read_run(directory / v / f'round-{pair}', pair, manifest) for pair in range(1, pairs+1)]
                for v in ['baseline', 'main']}
    result = {'manifest': manifest, 'overview': {'cases_per_version': len(configurations(profile))*3*pairs}, 'comparisons': []}
    for v, runs in versions.items():
        rows = [r for run in runs for r in run.values()]
        result['overview'][v] = {name: sum(r[name] for r in rows)
                                for name in ['sent', 'completed', 'outputs', 'queue_rejected', 'transform_errors', 'publish_errors', 'invalid_outputs', 'async_errors', 'input_pending_dropped', 'output_pending_dropped', 'unaccounted_inputs', 'incomplete_fanout', 'allocated_bytes_during_run', 'mallocs_during_run', 'gcs_during_run']}
        result['overview'][v]['lossless_cases'] = sum(r['lossless'] for r in rows)
    for k in sorted(versions['baseline'][0]):
        rows = {v: [run[k] for run in runs] for v, runs in versions.items()}
        # A conservative descriptive filter, not a significance/SLO test. All
        # rounds must be lossless and both actual loads within 5% of the target
        # and of one another. Retain non-comparable observations without a delta.
        target = rows['baseline'][0]['config']['target_rate']
        matched = all(load_matches(b,a,target) for b,a in zip(rows['baseline'],rows['main']))
        lossless = all(r['lossless'] for values in rows.values() for r in values)
        before, after = metrics(rows['baseline']), metrics(rows['main'])
        comparable = matched and lossless
        result['comparisons'].append({'config': rows['baseline'][0]['config'],
                                     'baseline': before, 'main': after,
                                     'matched_load': matched, 'latency_comparable': comparable,
                                     'processing_p50_change_pct': change(before['processing_p50_us'], after['processing_p50_us']),
                                     'e2e_p99_change_pct': change(before['e2e_p99_us'], after['e2e_p99_us']) if comparable else None})
        if profile == 'object-one-worker':
            pairs_out = []
            for pair, (b, a) in enumerate(zip(rows['baseline'], rows['main']), 1):
                valid_pair = b['lossless'] and a['lossless'] and load_matches(b, a, target)
                pairs_out.append({'pair': pair, 'latency_comparable': valid_pair,
                                  'baseline': metrics([b]), 'main': metrics([a]),
                                  'e2e_p99_change_pct': change(b['end_to_end']['p99_ns'], a['end_to_end']['p99_ns']) if valid_pair else None})
            result['comparisons'][-1]['paired_observations'] = pairs_out
            result['comparisons'][-1]['slower_tail_pairs'] = sum(p['e2e_p99_change_pct'] > 0 for p in pairs_out) if comparable else None

    return result


def write(directory, result):
    (directory / 'comparison.json').write_text(json.dumps(result, indent=2)+'\n')
    flat = []
    for row in result['comparisons']:
        flat.append({**row['config'], 'matched_load': row['matched_load'], 'latency_comparable': row['latency_comparable'],
                     'processing_p50_change_pct': row['processing_p50_change_pct'], 'e2e_p99_change_pct': row['e2e_p99_change_pct'],
                     **{f'{v}_{k}': value for v in ['baseline', 'main'] for k, value in row[v].items()}})
    with (directory / 'comparison.csv').open('w', newline='') as stream:
        writer = csv.DictWriter(stream, fieldnames=list(flat[0]), lineterminator='\n')
        writer.writeheader(); writer.writerows(flat)
    pairs_out = []
    for row in result['comparisons']:
        for pair in row.get('paired_observations', []):
            pairs_out.append({**row['config'], 'pair': pair['pair'], 'latency_comparable': pair['latency_comparable'],
                              'e2e_p99_change_pct': pair['e2e_p99_change_pct'],
                              **{f'{v}_{k}': value for v in ['baseline', 'main'] for k, value in pair[v].items()}})
    if pairs_out:
        with (directory / 'paired-observations.csv').open('w', newline='') as stream:
            writer = csv.DictWriter(stream, fieldnames=list(pairs_out[0]), lineterminator='\n')
            writer.writeheader(); writer.writerows(pairs_out)



if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('directory', type=Path)
    root = parser.parse_args().directory
    report = compare(root)
    write(root, report)
    print(json.dumps(report['overview'], indent=2))
