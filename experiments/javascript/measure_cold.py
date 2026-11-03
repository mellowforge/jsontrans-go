#!/usr/bin/env python3
import argparse, json, pathlib, subprocess
parser = argparse.ArgumentParser()
parser.add_argument('--binary', default='/tmp/jsontrans-js-assess')
args = parser.parse_args()
rows = []
for backend in ['quickjs', 'goja']:
    for _ in range(5):
        rows.append(json.loads(subprocess.check_output(
            [args.binary, '-backend', backend, '-probe', 'cold-start'], timeout=4)))
path = pathlib.Path(__file__).parent / 'testdata/results/cold.json'
path.parent.mkdir(parents=True, exist_ok=True)
path.write_text(json.dumps(rows, indent=2) + '\n')
