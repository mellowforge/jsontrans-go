import concurrent.futures,datetime,hashlib,json,os,subprocess,threading,time
from pathlib import Path
workspace=Path('/data/ci/jsontrans-go-extended-20261104');root=workspace/'repo';cache=Path('/data/ci/jsontrans-go-20261104-07de350')
out=workspace/'runs'/datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%SZ');out.mkdir(parents=True,exist_ok=False)
env=dict(os.environ,GOTOOLCHAIN='go1.26.10',GOMAXPROCS='4',GOWORK='off',GOPATH=str(cache/'go-path'),GOMODCACHE=str(cache/'go-modules'),GOCACHE=str(cache/'go-build'),GOTMPDIR=str(cache/'tmp'),TMPDIR=str(cache/'tmp'),npm_config_cache=str(cache/'npm-cache'))
env['PATH']=str(cache/'tools/node-v22.22.3-linux-x64/bin')+':'+str(cache/'tools')+':'+env['PATH']
goroot=subprocess.check_output(['go','env','GOROOT'],env=env,text=True).strip();env['PATH']=goroot+'/bin:'+env['PATH']
source=subprocess.check_output(['git','rev-parse','HEAD'],cwd=root,text=True).strip();assert source=='dc29b608edb42111221fa3e173e889b9a7f61583'
subprocess.run(['go','-C','javascript','build','-o',str(out/'hardening'),'./cmd/hardening'],cwd=root,env=env|{'CGO_ENABLED':'0'},check=True)
report={'source_commit':source,'go':subprocess.check_output(['go','version'],env=env,text=True).strip(),'guest_sha256':hashlib.sha256((root/'javascript/internal/quickjswasm/quickjs.wasm').read_bytes()).hexdigest(),'out':str(out),'started_utc':datetime.datetime.now(datetime.timezone.utc).isoformat(),'groups':{},'complete':False}
lock=threading.Lock()
def save():
 p=out/'report.tmp';p.write_text(json.dumps(report,indent=2)+'\n');p.replace(out/'report.json')
def step(label,args,changes=None):return label,args,changes or {}
def modules():
 result=[]
 for directory in ['.','javascript','experiments/javascript']:
  prefix=directory.replace('/','-').replace('.','core')
  for label,args in [('test',['test','-count=1','./...']),('race',['test','-race','-count=1','./...']),('vet',['vet','./...']),('mod',['mod','verify'])]:result.append(step(prefix+'-'+label,['go','-C',directory]+args))
 return result
verification=[step('format',[goroot+'/bin/gofmt','-l','.'])]+modules()+[
 step('oracle-install',['npm','ci','--ignore-scripts','--prefix','conformance/oracle']),step('oracle',['go','test','-tags','oracle','-count=1','./conformance','./jsonata']),
 step('packaging-regressions',['python3','javascript/scripts/test_check_module.py']),step('consumer',['python3','javascript/scripts/check_module.py']),
 step('assessment-build',['go','-C','experiments/javascript','build','-o',str(out/'assess'),'./cmd/assess'],{'CGO_ENABLED':'0'}),
 step('owned-probes',['python3','experiments/javascript/run_probes.py','--binary',str(out/'assess'),'--backend','owned-quickjs','--strict','--output',str(out/'owned-probes.json')]),
 step('scanner-regressions',['python3','-m','unittest','discover','-s','scripts','-p','test_check_go_security.py'])]
for target in ['linux-amd64','windows-amd64','darwin-arm64']:
 goos,goarch=target.split('-');verification.append(step('security-'+target,['python3','scripts/check_go_security.py','--govulncheck',str(cache/'tools/govulncheck'),'--output',str(out/('security-'+target))],{'GOOS':goos,'GOARCH':goarch,'CGO_ENABLED':'0'}))
fuzz=[]
for package,target,count in [('./internal/quickjswasm','FuzzOwnedCompile',20000),('./internal/quickjswasm','FuzzDiagnosticRecord',1000000),('.','FuzzRuleLocation',1000000),('.','FuzzOwnedStringConversion',100000),('.','FuzzOwnedJSONConversion',100000),('.','FuzzOwnedTerminalPhases',10000),('.','FuzzOwnedResourceLimits',100000)]:
 fuzz.append(step(target,['go','-C','javascript','test',package,'-run','^$','-fuzz','^'+target+'$','-fuzztime='+str(count)+'x','-parallel=2','-timeout=30m']))
def run_group(name,steps):
 with lock:report['groups'][name]={'state':'running','steps':[]};save();print(name+': started',flush=True)
 for label,command,changes in steps:
  started=time.monotonic();log=out/(name+'-'+label+'.log')
  try:
   with log.open('w') as stream:p=subprocess.run(command,cwd=root,env=env|changes,stdout=stream,stderr=subprocess.STDOUT,timeout=3700 if name.startswith('soak') else 1900)
   code=p.returncode
   if label=='format' and log.read_text().strip():code=1
   row={'name':label,'command':command,'environment':changes,'exit_code':code,'seconds':round(time.monotonic()-started,3),'log':log.name}
  except Exception as exc:code=1;row={'name':label,'command':command,'error':str(exc),'exit_code':1,'log':log.name}
  with lock:
   report['groups'][name]['steps'].append(row)
   if code:report['groups'][name]['state']='failed';save();print(name+'/'+label+': FAILED',flush=True);return
   save();print(name+'/'+label+': passed',flush=True)
 with lock:report['groups'][name]['state']='passed';save()
runner=workspace/'jsontrans-extended-run-campaign.py'
def campaign(name,guest,recycle):
 return [step('campaign',['python3',str(runner),'--root',str(root),'--out',str(out/name),'--binary',str(out/'hardening'),'--profile',name,'--duration','3600','--guest',str(guest),'--recycle',str(recycle),'--workers','4'])]
def validation():run_group('fuzz',fuzz);run_group('verification',verification)
with concurrent.futures.ThreadPoolExecutor(max_workers=3) as pool:
 futures=[pool.submit(run_group,'soak-persistent',campaign('soak-persistent',8,0)),pool.submit(run_group,'soak-recycled',campaign('soak-recycled',64,60)),pool.submit(validation)]
 for f in futures:f.result()
report['complete']=True;report['passed']=all(g['state']=='passed' for g in report['groups'].values());report['finished_utc']=datetime.datetime.now(datetime.timezone.utc).isoformat();save()
print('OVERALL '+('PASS' if report['passed'] else 'FAIL')+' '+str(out/'report.json'),flush=True)
raise SystemExit(0 if report['passed'] else 1)
