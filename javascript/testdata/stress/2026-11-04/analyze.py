import argparse,json,statistics
from pathlib import Path
p=argparse.ArgumentParser();p.add_argument('directory',type=Path);p.add_argument('--output',type=Path,required=True);a=p.parse_args()
r=json.loads((a.directory/'result.json').read_text(encoding='utf-8-sig'));runner=json.loads((a.directory/'runner.json').read_text(encoding='utf-8-sig'))
if not runner.get('passed') or r.get('status')!='passed' or r.get('complete') is not True:raise SystemExit('Cannot assess an incomplete or failed campaign as a completed run')
workers=r['campaign']['workers'];batches=r['completed_batches'];expected=batches*25*workers
if r['verified_recoveries']!=expected or set(r['outcomes'])!={'success','exception','conversion','cancellation','allocation'} or any(x['calls']!=batches*5*workers for x in r['outcomes'].values()):raise SystemExit('Outcome/recovery accounting mismatch')
duration=r['campaign']['duration_ns'];warmup=duration//10
samples=r['post_gc_memory'];answer={'profile':runner['profile'],'source_commit':runner['source_commit'],'go':r['go_version'],'platform':r['platform'],'actual_seconds':r['elapsed_ns']/1e9,'recoveries':r['verified_recoveries'],'closed_lifecycles':len(r['close_ns']),'guest_cap_bytes':r['guest_cap_bytes'],'warmup_seconds':warmup/1e9,'baseline':samples[0],'timing_values_payload_bytes':8*sum(len(r[k]) for k in ['construction_ns','compilation_ns','first_eval_ns','close_ns']),'stages':{}}
for stage in ['live','closed']:
 rows=[m for m in samples if m['stage']==stage and m['elapsed_ns']>=warmup]
 v={'samples':len(rows),'trend_available':len(rows)>=2}
 if rows:
  window=min(5,max(1,len(rows)//4));first=rows[:window];last=rows[-window:]
  v.update(start_seconds=rows[0]['elapsed_ns']/1e9,end_seconds=rows[-1]['elapsed_ns']/1e9,window_samples=window,first_heap_median=statistics.median(m['heap_alloc_bytes'] for m in first),last_heap_median=statistics.median(m['heap_alloc_bytes'] for m in last),heap_max=max(m['heap_alloc_bytes'] for m in rows),go_sys_max=max(m['go_sys_bytes'] for m in rows),first_goroutine_median=statistics.median(m['goroutines'] for m in first),last_goroutine_median=statistics.median(m['goroutines'] for m in last),goroutine_max=max(m['goroutines'] for m in rows))
  v['heap_median_delta']=v['last_heap_median']-v['first_heap_median']
 answer['stages'][stage]=v
rss=[json.loads(line) for line in (a.directory/'rss.jsonl').read_text(encoding='utf-8-sig').splitlines() if line.strip()]
steady=[s for s in rss if s['elapsed_ns']>=warmup]
answer['rss']={'samples':len(rss),'sampled_peak_bytes':runner['sampled_peak_rss_bytes'],'first_postwarmup_30_median':statistics.median(s['rss_bytes'] for s in steady[:30]),'last_30_median':statistics.median(s['rss_bytes'] for s in steady[-30:])}
answer['note']='Diagnostic trends, not a memory-safety proof or hard process-memory cap. Timing/sample arrays are retained by the measurement harness. Live and closed samples must be interpreted separately.'
a.output.write_text(json.dumps(answer,indent=2)+'\n');print(json.dumps(answer,indent=2))
