#!/usr/bin/env python3
"""Record every gate, including failures; --strict fails for any failed gate."""
import argparse, json, os, pathlib, platform, subprocess, time
parser = argparse.ArgumentParser()
parser.add_argument('--binary', default='/tmp/jsontrans-js-assess')
parser.add_argument('--output', default='testdata/results/probes.json')
parser.add_argument('--strict', action='store_true')
parser.add_argument('--backend', action='append', choices=['quickjs','goja','owned-quickjs'])
parser.add_argument('--case', dest='selected_cases', action='append')
args = parser.parse_args()
cases = ['cold-start', 'compile-cancel', 'cancel-loop', 'cancel-regex', 'cancel-conversion',
         'recursion', 'allocation-cap', 'guest-memory', 'cleanup', 'host-capabilities',
         'cache-cancellation', 'unicode', 'numbers', 'exceptions', 'conversion-limits',
         'limits-forwarding', 'prepared-fallback', 'exception-coercion']
owned_cases = ['lifecycle-contract', 'diagnostic-classification', 'diagnostic-locations']
rows = []
if args.selected_cases:
    unknown = set(args.selected_cases) - set(cases + owned_cases)
    if unknown:
        parser.error('unknown cases: ' + ', '.join(sorted(unknown)))
    if set(args.selected_cases) & set(owned_cases) and args.backend != ['owned-quickjs']:
        parser.error('owned contract cases require --backend owned-quickjs')
for backend in args.backend or ['quickjs', 'goja']:
    for case in args.selected_cases or (cases + (owned_cases if backend == 'owned-quickjs' else [])):
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
failed = sum(r['status'] != 'PASS' if r['backend'] == 'owned-quickjs' else r['status'] == 'FAIL' for r in rows)
print(f'{failed} failed gates; report: {output}')
if args.strict and failed:
    raise SystemExit(1)
