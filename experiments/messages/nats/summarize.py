#!/usr/bin/env python3
"""Validate a complete NATS load run; retain loss alongside latency."""
import argparse
import csv
import itertools
import json
from pathlib import Path
import statistics
import re


def expected_keys(meta):
    if meta['cases_per_round']==6:
        cases=[('mixed','mixed',n,2,500,128,128,1024) for n in [1,4]]
    elif meta['cases_per_round']==162:
        match=re.fullmatch(r"([0-9.]+)(ns|us|µs|ms|s)",meta['duration'])
        if not match:raise ValueError('unsupported duration')
        seconds=float(match.group(1))*{'ns':1e-9,'us':1e-6,'µs':1e-6,'ms':1e-3,'s':1}[match.group(2)]
        count=lambda rate:max(1,int(seconds*rate))
        cases=[(engine,kind,n,w,r,count(r),w*64,1024)
               for engine,kind in [('jsonata','scalar'),('jsonata','object'),('mixed','mixed')]
               for n,w,r in itertools.product([1,4,16],[1,4,8],[1000,5000])]
    else:raise ValueError('unknown case matrix')
    return {(round_id,*case,strategy) for round_id in range(1,meta['rounds']+1) for case in cases for strategy in ['direct','prepared','batch']}


def summarize(directory):
    meta=json.loads((directory/'metadata.json').read_text())
    if not meta.get('completed') or not meta.get('container_removed') or meta.get('cleanup_error'):
        raise ValueError('run or cleanup incomplete')
    lines=[json.loads(line) for line in (directory/'runs.jsonl').read_text().splitlines() if line.strip()]
    expected=expected_keys(meta);seen=set();groups={}
    for row in lines:
        if row.get('error'):raise ValueError('run error')
        r=row['report'];c=r['config']
        fields=tuple(c[k] for k in ['engine','kind','fanout','workers','target_rate','count','queue_capacity','target_bytes'])
        key=(row['round'],*fields,c['strategy'])
        if key in seen or key not in expected:raise ValueError('duplicate/unknown case')
        seen.add(key)
        if not r['valid'] or r['sent']!=c['count'] or r['received']!=r['processed']+r['queue_rejected']:
            raise ValueError('invalid input accounting')
        if r['completed']+r['queue_rejected']+r['transform_errors']!=r['sent'] or r['outputs']!=r['completed']*c['fanout']:
            raise ValueError('invalid output accounting')
        for error in ['publish_errors','invalid_outputs','async_errors','input_pending_dropped','output_pending_dropped','unaccounted_inputs','incomplete_fanout']:
            if r[error]!=0:raise ValueError('transport/correctness error '+error)
        if r['lossless']!=(r['queue_rejected']==0 and r['transform_errors']==0 and r['completed']==r['sent']):
            raise ValueError('incorrect lossless classification')
        for label in ['processing','queue_wait','end_to_end']:
            q=r[label]
            if not 0<=q['p50_ns']<=q['p95_ns']<=q['p99_ns']:raise ValueError('invalid quantiles')
        groups.setdefault((*fields,c['strategy']),[]).append(r)
    if seen!=expected:raise ValueError('missing cases/strategies/rounds')
    out=[]
    for key,values in groups.items():
        c=values[0]['config'];median=lambda f:statistics.median(f(r) for r in values)
        out.append({**c,'rounds':len(values),'lossless_rounds':sum(r['lossless'] for r in values),
                    'completion_percent':median(lambda r:100*r['completed']/r['sent']),
                    'queue_rejected_median':median(lambda r:r['queue_rejected']),
                    'actual_offered_median':median(lambda r:r['actual_offered_per_second']),
                    'completed_per_second_median':median(lambda r:r['completed_per_second']),
                    'processing_p50_us':median(lambda r:r['processing']['p50_ns']/1000),
                    'processing_p99_us':median(lambda r:r['processing']['p99_ns']/1000),
                    'queue_p99_us':median(lambda r:r['queue_wait']['p99_ns']/1000),
                    'e2e_p50_us':median(lambda r:r['end_to_end']['p50_ns']/1000),
                    'e2e_p99_us':median(lambda r:r['end_to_end']['p99_ns']/1000),
                    'pacing_late_max_us':max(r['max_pacing_lateness_ns']/1000 for r in values),
                    'allocated_bytes_median':median(lambda r:r['allocated_bytes_during_run'])})
    (directory/'summary.json').write_text(json.dumps(out,indent=2)+'\n')
    with (directory/'summary.csv').open('w',newline='') as f:
        writer=csv.DictWriter(f,fieldnames=list(out[0]),lineterminator='\n');writer.writeheader();writer.writerows(out)
    selected=[r for r in out if r['target_rate']==5000 and r['fanout'] in [1,16] and r['workers'] in [1,8]]
    text=['# NATS instrumented load results','','Median per-run metrics. p99 values below are medians of individual-run p99s, not a pooled p99. End-to-end percentiles include completed inputs only. Always read completion/loss alongside latency. Brief deterministic paced local tests do not establish production capacity.','','| Route family | Fan-out | Workers | Strategy | Completion % | Lossless rounds | Processing p50 us | Processing p99 us | E2E p99 us |','|---|---:|---:|---|---:|---:|---:|---:|---:|']
    for r in selected:text.append(f'| {r["kind"]} | {r["fanout"]} | {r["workers"]} | {r["strategy"]} | {r["completion_percent"]:.1f} | {r["lossless_rounds"]}/{r["rounds"]} | {r["processing_p50_us"]:.1f} | {r["processing_p99_us"]:.1f} | {r["e2e_p99_us"]:.1f} |')
    text+=['','Selected rows target5000 messages/sec; summary.csv records achieved offered rate, queue rejection/wait and all1000/sec cases as well. Lossy runs are not SLO improvements. Core NATS, loopback broker, no TLS/persistence/acks.']
    (directory/'summary.md').write_text('\n'.join(text)+'\n')


if __name__=='__main__':
    parser=argparse.ArgumentParser();parser.add_argument('directory',type=Path);summarize(parser.parse_args().directory)
