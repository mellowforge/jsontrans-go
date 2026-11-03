#!/usr/bin/env python3
"""Record every gate, including failures; --strict fails for any failed gate."""
import argparse, json, os, pathlib, platform, subprocess, time
parser = argparse.ArgumentParser()
parser.add_argument('--binary', default='/tmp/jsontrans-js-assess')
parser.add_argument('--output', default='testdata/results/probes.json')
parser.add_argument('--strict', action='store_true')
args = parser.parse_args()
cases = ['cold-start', 'compile-cancel', 'cancel-loop', 'cancel-regex', 'cancel-conversion',
         'recursion', 'allocation-cap', 'guest-memory', 'cleanup', 'host-capabilities',
         'cache-cancellation', 'unicode', 'numbers', 'exceptions', 'conversion-limits',
         'limits-forwarding', 'prepared-fallback', 'exception-coercion']
rows = []
for backend in ['quickjs', 'goja']:
    for case in cases:
        try:
            child = subprocess.run([args.binary, '-backend', backend, '-probe', case],
                                   capture_output=True, text=True, timeout=4)
            if child.returncode:
                row = dict(backend=backend, case=case, status='FAIL', detail=child.stderr[-4000:])
            else:
                row = json.loads(child.stdout)
        except subprocess.TimeoutExpired:
            row = dict(backend=backend, case=case, status='FAIL',
                       detail='external watchdog killed child at 4 seconds; 75 ms evaluation deadline ineffective')
        rows.append(row)
        print(f'{backend}/{case}: {row["status"]}', flush=True)
output = pathlib.Path(args.output); output.parent.mkdir(parents=True, exist_ok=True)
output.write_text(json.dumps(dict(platform=platform.platform(), cases=rows), indent=2) + '\n')
failed = sum(r['status'] == 'FAIL' for r in rows)
print(f'{failed} failed gates; report: {output}')
if args.strict and failed:
    raise SystemExit(1)
