#!/usr/bin/env python3
"""Summarize GC stop-the-world ranges from Go 1.27 parsed trace events.

Raw execution traces stay outside Git. This deliberately ignores concurrent GC
ranges and trace-start pauses; it cannot attribute an end-to-end tail to GC.
"""
import argparse
import json
import re
import subprocess

EVENT = re.compile(r'\b(RangeBegin|RangeEnd) Time=(\d+) Name="([^"]+)" Scope=(\S+)')


def summarize(lines):
    active, durations = {}, []
    for line in lines:
        match = EVENT.search(line)
        if not match:
            continue
        kind, stamp, name, scope = match.groups()
        if not name.startswith('stop-the-world (GC '):
            continue
        key = (name, scope)
        stamp = int(stamp)
        if kind == 'RangeBegin':
            if key in active:
                raise ValueError('duplicate GC range start')
            active[key] = stamp
        else:
            if key not in active:
                raise ValueError('GC range ended without start')
            elapsed = stamp-active.pop(key)
            if elapsed < 0:
                raise ValueError('negative GC range')
            durations.append(elapsed)
    if active:
        raise ValueError('incomplete GC stop-the-world ranges')
    durations.sort()
    def percentile(p):
        return durations[(p*len(durations)+99)//100-1]/1000 if durations else None
    return {'gc_stw_ranges': len(durations), 'total_us': sum(durations)/1000,
            'p50_us': percentile(50), 'p95_us': percentile(95), 'p99_us': percentile(99),
            'max_us': durations[-1]/1000 if durations else None}


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('trace_tool')
    parser.add_argument('trace_file')
    args = parser.parse_args()
    command = subprocess.Popen([args.trace_tool, '-d=parsed', args.trace_file], stdout=subprocess.PIPE, text=True)
    try:
        result = summarize(command.stdout)
    except BaseException:
        command.terminate(); command.wait()
        raise
    if command.wait() != 0:
        raise SystemExit('trace decoder failed')
    print(json.dumps(result, indent=2))
