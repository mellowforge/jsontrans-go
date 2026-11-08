import copy
from datetime import datetime, timedelta, timezone
import json
from pathlib import Path
import tempfile
import unittest

from compare import compare, configurations


class MatchedReports(unittest.TestCase):
    def fixture(self, root, focused=False):
        image = 'nats@sha256:' + 'a' * 64
        manifest = {'baseline': 'b' * 40, 'main': 'c' * 40, 'go': 'go1.27.3', 'image': image, 'pairs': 6 if focused else 3}
        if focused: manifest.update(profile='object-one-worker', duration='10s')
        (root / 'manifest.json').write_text(json.dumps(manifest))
        execution = ['pair\tversion\toffset\tstart_utc\tend_utc\texit_status']
        for pair in range(1, manifest['pairs']+1):
            for version in (['baseline', 'main'] if pair % 2 else ['main', 'baseline']):
                start = datetime(2026, 11, 8, tzinfo=timezone.utc) + timedelta(minutes=4*(len(execution)-1))
                end = start + timedelta(minutes=3)
                execution.append(f'{pair}\t{version}\t{pair-1}\t{start.isoformat()}\t{end.isoformat()}\t0')
        (root / 'execution.tsv').write_text('\n'.join(execution)+'\n')
        for pair in range(1, manifest['pairs']+1):
            for version in ['baseline', 'main']:
                d = root / version / f'round-{pair}'
                d.mkdir(parents=True)
                meta = {'completed': True, 'container_removed': True, 'rounds': 1,
                        'round_offset': pair-1, 'cases_per_round': 6 if focused else 162, 'duration': '10s' if focused else '1s',
                        'go': manifest['go'], 'image': image, 'broker_cpus': '16,17',
                        'broker_memory_bytes': 512 << 20, 'api_profile': 'public-prepared-json-v1',
                        'entry_points': {'direct': 'jsontrans.EvalJSON', 'prepared': 'jsontrans.EvalPreparedJSON', 'batch': 'Batch.EvalPreparedJSON'},
                        'container_inspect': {'HostConfig': {'CpusetCpus': '16,17', 'Memory': 512 << 20}, 'Config': {'Env': ['GOMAXPROCS=2']}}}
                if focused: meta['workload_profile'] = 'object-one-worker'
                (d / 'metadata.json').write_text(json.dumps(meta))
                configs = configurations('object-one-worker' if focused else 'full', 10 if focused else 1)
                offset = (pair-1) % len(configs)
                configs = configs[offset:] + configs[:offset]
                rows = []
                for index, c in enumerate(configs):
                    for si in range(3):
                        strategy = ['direct', 'prepared', 'batch'][(si+pair-1) % 3]
                        c = {**c, 'strategy': strategy}
                        count = c['count']
                        p50 = pair * (10000 if version == 'main' else 20000)
                        tail = pair * (400000 if version == 'main' else 100000)
                        r = {'config': c, 'valid': True, 'lossless': True, 'sent': count, 'received': count,
                             'processed': count, 'completed': count, 'outputs': count*c['fanout'],
                             'server_version': '2.15.2', 'gomaxprocs': c['workers'],
                             'publish_seconds': count/c['target_rate'], 'total_seconds': count/c['target_rate']+.1,
                             'actual_offered_per_second': c['target_rate'], 'completed_per_second': count/(count/c['target_rate']+.1),
                             'max_pacing_lateness_ns': 100, 'allocated_bytes_during_run': count*100,
                             'mallocs_during_run': count*2, 'gcs_during_run': pair}
                        for error in ['queue_rejected', 'transform_errors', 'publish_errors', 'invalid_outputs', 'async_errors', 'input_pending_dropped', 'output_pending_dropped', 'unaccounted_inputs', 'incomplete_fanout']:
                            r[error] = 0
                        r['processing'] = {'p50_ns': p50, 'p95_ns': p50*2, 'p99_ns': p50*3}
                        for label in ['end_to_end', 'queue_wait']:
                            r[label] = {'p50_ns': tail//3, 'p95_ns': tail//2, 'p99_ns': tail}
                        rows.append({'round': 1, 'case_index': index, 'report': r})
                self.write(d, rows)

    def write(self, d, rows):
        (d / 'runs.jsonl').write_text(''.join(json.dumps(r)+'\n' for r in rows))

    def first(self, root):
        d = root / 'baseline/round-1'
        return d, [json.loads(s) for s in (d / 'runs.jsonl').read_text().splitlines()]

    def test_six_pair_long_focused_protocol(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp); self.fixture(root, focused=True)
            result = compare(root)
            self.assertEqual(result['overview']['cases_per_version'], 36)
            self.assertEqual(len(result['comparisons']), 6)
            self.assertTrue(all(r['config']['count']==50000 and r['config']['workers']==1 for r in result['comparisons']))
            self.assertTrue(all(r['baseline']['lossless_rounds']==6 and r['main']['lossless_rounds']==6 for r in result['comparisons']))
            self.assertTrue(all(len(r['paired_observations'])==6 for r in result['comparisons']))
            self.assertTrue(all(r['slower_tail_pairs']==6 for r in result['comparisons']))

    def test_processing_gain_does_not_imply_latency_gain(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp); self.fixture(root)
            result = compare(root)
            self.assertEqual(result['overview']['cases_per_version'], 486)
            self.assertEqual(len(result['comparisons']), 162)
            self.assertFalse((root / 'baseline/round-1/summary.json').exists(), 'comparison must not create per-run artifacts')
            c = result['comparisons'][0]
            self.assertTrue(c['latency_comparable'])
            self.assertEqual(c['baseline']['processing_p50_us'], 40)
            self.assertEqual(c['main']['processing_p50_us'], 20)
            self.assertEqual(c['processing_p50_change_pct'], -50)
            self.assertEqual(c['e2e_p99_change_pct'], 300)
            self.assertEqual(c['main']['allocated_bytes_per_sent'], 100)
            self.assertEqual(c['main']['mallocs_per_completed'], 2)
            self.assertEqual(c['main']['gcs'], 2)

    def test_any_loss_suppresses_latency_comparison(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp); self.fixture(root)
            d, rows = self.first(root); r = rows[0]['report']
            r.update(completed=r['sent']-1, processed=r['sent']-1, queue_rejected=1,
                     outputs=(r['sent']-1)*r['config']['fanout'], lossless=False)
            r['completed_per_second'] = r['completed']/r['total_seconds']
            self.write(d, rows)
            result = compare(root)
            cases = [c for c in result['comparisons'] if not c['latency_comparable']]
            self.assertEqual(len(cases), 1)
            self.assertEqual(cases[0]['baseline']['lossless_rounds'], 2)
            self.assertIsNone(cases[0]['e2e_p99_change_pct'])

    def test_different_delivered_load_is_flagged(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp); self.fixture(root)
            d, rows = self.first(root); r = rows[0]['report']
            r['publish_seconds'] = 1.25; r['total_seconds'] = 1.3
            r['actual_offered_per_second'] = r['sent']/1.25
            r['completed_per_second'] = r['completed']/1.3
            self.write(d, rows)
            result = compare(root)
            cases = [c for c in result['comparisons'] if not c['latency_comparable']]
            self.assertEqual(len(cases), 1)
            self.assertFalse(cases[0]['matched_load'])
            self.assertIsNone(cases[0]['e2e_p99_change_pct'])

    def test_load_match_is_relative_to_actual_rates(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp); self.fixture(root)
            d, rows = self.first(root); r = rows[0]['report']
            r['publish_seconds'] = r['sent']/(r['config']['target_rate']*.95)
            r['actual_offered_per_second'] = r['sent']/r['publish_seconds']
            self.write(d, rows)
            result = compare(root)
            cases = [c for c in result['comparisons'] if not c['matched_load']]
            self.assertEqual(len(cases), 1)
            self.assertIsNone(cases[0]['e2e_p99_change_pct'])

    def test_invalid_or_incomplete_measurements_are_rejected(self):
        for defect in ['missing', 'duplicate', 'order', 'offset', 'go', 'cleanup', 'transform', 'metric', 'quantile', 'fractional-quantile', 'counter', 'boolean-counter', 'fractional-counter', 'execution', 'overlap']:
            with self.subTest(defect=defect), tempfile.TemporaryDirectory() as tmp:
                root = Path(tmp); self.fixture(root)
                d, rows = self.first(root)
                if defect == 'missing': rows.pop()
                if defect == 'duplicate': rows.append(copy.deepcopy(rows[0]))
                if defect == 'order': rows[0], rows[1] = rows[1], rows[0]
                if defect in ['offset', 'go', 'cleanup']:
                    m = json.loads((d/'metadata.json').read_text())
                    m[{'offset': 'round_offset', 'go': 'go', 'cleanup': 'container_removed'}[defect]] = {'offset': 2, 'go': 'go1.26.10', 'cleanup': False}[defect]
                    (d/'metadata.json').write_text(json.dumps(m))
                if defect == 'transform':
                    r = rows[0]['report']; r.update(transform_errors=1, completed=r['sent']-1, outputs=(r['sent']-1)*r['config']['fanout'], lossless=False)
                if defect == 'metric': rows[0]['report']['actual_offered_per_second'] = -1
                if defect == 'quantile': rows[0]['report']['end_to_end']['p99_ns'] = float('inf')
                if defect == 'fractional-quantile': rows[0]['report']['end_to_end']['p99_ns'] += .5
                if defect == 'counter':
                    r = rows[0]['report']; r.update(queue_rejected=-1, processed=r['sent']+1, completed=r['sent']+1, outputs=(r['sent']+1)*r['config']['fanout'], lossless=False)
                    r['completed_per_second'] = r['completed']/r['total_seconds']
                if defect == 'boolean-counter': rows[0]['report']['mallocs_during_run'] = True
                if defect == 'fractional-counter': rows[0]['report']['gcs_during_run'] = 1.5
                if defect in ['execution', 'overlap']:
                    lines = (root/'execution.tsv').read_text().splitlines()
                    fields = lines[1 if defect == 'execution' else 2].split('\t')
                    if defect == 'execution': fields[1] = 'main'
                    else: fields[3] = lines[1].split('\t')[3]
                    lines[1 if defect == 'execution' else 2] = '\t'.join(fields)
                    (root/'execution.tsv').write_text('\n'.join(lines)+'\n')
                self.write(d, rows)
                with self.assertRaises(ValueError): compare(root)


if __name__ == '__main__':
    unittest.main()
