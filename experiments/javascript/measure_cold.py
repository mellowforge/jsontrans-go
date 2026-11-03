#!/usr/bin/env python3
import argparse, json, pathlib, subprocess
parser = argparse.ArgumentParser()
parser.add_argument('--binary', default='/tmp/jsontrans-js-assess')
parser.add_argument('--backend', action='append', choices=['quickjs','goja','owned-quickjs'])
parser.add_argument('--output', type=pathlib.Path, default=pathlib.Path(__file__).parent / 'testdata/results/cold.json')
args = parser.parse_args()
rows = []
for backend in args.backend or ['quickjs', 'goja']:
    for _ in range(5):
        rows.append(json.loads(subprocess.check_output(
            [args.binary, '-backend', backend, '-probe', 'cold-start'], timeout=4)))
path = args.output
path.parent.mkdir(parents=True, exist_ok=True)
path.write_text(json.dumps(rows, indent=2) + '\n')
