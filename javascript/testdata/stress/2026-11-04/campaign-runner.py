import argparse,datetime,json,os,platform,subprocess,time
from pathlib import Path
ap=argparse.ArgumentParser()
ap.add_argument('--root',type=Path,required=True);ap.add_argument('--out',type=Path,required=True);ap.add_argument('--binary',type=Path,required=True)
ap.add_argument('--profile',required=True);ap.add_argument('--duration',type=int,required=True);ap.add_argument('--guest',type=int,required=True);ap.add_argument('--recycle',type=int,default=0);ap.add_argument('--workers',type=int,default=4)
a=ap.parse_args();a.out.mkdir(parents=True,exist_ok=False)
source=subprocess.check_output(['git','rev-parse','HEAD'],cwd=a.root,text=True).strip()
command=[str(a.binary),'-duration',str(a.duration)+'s','-recycle',str(a.recycle)+'s','-sample-interval','30s','-workers',str(a.workers),'-guest-mib',str(a.guest),'-progress',str(a.out/'progress.jsonl')]
report={'source_commit':source,'platform':platform.platform(),'profile':a.profile,'command':command,'gomaxprocs':os.environ.get('GOMAXPROCS'),'started_utc':datetime.datetime.now(datetime.timezone.utc).isoformat(),'complete':False,'passed':False}
(a.out/'runner.json').write_text(json.dumps(report,indent=2)+'\n')
started=time.monotonic();observations=0;peak=0;rss_errors=[];timed_out=False
with (a.out/'result.json').open('wb') as output,(a.out/'stderr.log').open('wb') as errors,(a.out/'rss.jsonl').open('w') as samples:
 p=subprocess.Popen(command,cwd=a.root,stdout=output,stderr=errors)
 report['pid']=p.pid
 while p.poll() is None:
  try:
   if platform.system()=='Linux':
    fields={l.split(':',1)[0]:l.split(':',1)[1].strip() for l in Path('/proc',str(p.pid),'status').read_text().splitlines() if ':' in l}
    rss=int(fields['VmRSS'].split()[0])*1024;hwm=int(fields['VmHWM'].split()[0])*1024
   else:
    rss=int(subprocess.check_output(['/bin/ps','-o','rss=','-p',str(p.pid)],text=True).strip())*1024;hwm=None
   peak=max(peak,rss,hwm or 0);observations+=1
   samples.write(json.dumps({'elapsed_ns':int((time.monotonic()-started)*1e9),'rss_bytes':rss,'sampled_hwm_bytes':hwm})+'\n');samples.flush()
  except (OSError,ValueError,KeyError,subprocess.CalledProcessError) as exc:
   if p.poll() is None:rss_errors.append(str(exc))
  if time.monotonic()-started>a.duration+45:
   timed_out=True;p.terminate()
   try:p.wait(timeout=5)
   except subprocess.TimeoutExpired:p.kill();p.wait()
   break
  time.sleep(1)
 code=p.wait()
report.update(exit_code=code,seconds=round(time.monotonic()-started,3),rss_samples=observations,sampled_peak_rss_bytes=peak,rss_errors=rss_errors,timed_out=timed_out,complete=True,finished_utc=datetime.datetime.now(datetime.timezone.utc).isoformat())
try:
 r=json.loads((a.out/'result.json').read_text());total=sum(x['calls'] for x in r['outcomes'].values())
 stages={m['stage'] for m in r['post_gc_memory']}
 report['passed']=(code==0 and not timed_out and not rss_errors and observations>0 and r['status']=='passed' and r['complete'] is True and r['elapsed_ns']>=a.duration*1_000_000_000 and total==r['verified_recoveries']==r['completed_batches']*25*a.workers and len(r['close_ns'])==len(r['construction_ns'])>0 and {'baseline','live','closed'}<=stages)
 if a.recycle==0:report['passed']=report['passed'] and len(r['close_ns'])==1
 report['verified_recoveries']=r['verified_recoveries'];report['closed_lifecycles']=len(r['close_ns'])
except Exception as exc:report['validation_error']=str(exc)
(a.out/'runner.json').write_text(json.dumps(report,indent=2)+'\n')
print(a.profile+': '+('PASS' if report['passed'] else 'FAIL')+' '+str(a.out/'runner.json'),flush=True)
raise SystemExit(0 if report['passed'] else 1)
