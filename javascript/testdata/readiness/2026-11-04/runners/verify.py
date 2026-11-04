import concurrent.futures, datetime, hashlib, json, os, platform, subprocess, sys, threading, time
from pathlib import Path

mode=sys.argv[1]
base=Path('/data/ci/jsontrans-readiness-20261104') if mode=='linux' else Path('/tmp/jsontrans-readiness-20261104')
root=base/'repo' if mode=='linux' else Path('/Users/konstantinpenzin/Developer/jsontrans-go')
cache=Path('/data/ci/jsontrans-go-20261104-07de350')
out=base/'verification';out.mkdir(exist_ok=False)
env=dict(os.environ,GOTOOLCHAIN='go1.26.10',GOMAXPROCS='4',GOWORK='off')
if mode=='linux':
 env.update(GOPATH=str(cache/'go-path'),GOMODCACHE=str(cache/'go-modules'),GOCACHE=str(cache/'go-build'),GOTMPDIR=str(cache/'tmp'),TMPDIR=str(cache/'tmp'),npm_config_cache=str(cache/'npm-cache'))
 env['PATH']=str(cache/'tools/node-v22.22.3-linux-x64/bin')+':'+str(cache/'tools')+':'+env['PATH']
else:env['GOCACHE']='/tmp/jsontrans-go-build'
version=subprocess.check_output(['go','version'],env=env,text=True).strip()
goroot=subprocess.check_output(['go','env','GOROOT'],env=env,text=True).strip()
env['PATH']=goroot+'/bin:'+env['PATH']
source=subprocess.check_output(['git','rev-parse','HEAD'],cwd=root,text=True).strip()
assert source=='acbf01cb0271089c08de2f63ece849067a2137fb'
assert not subprocess.check_output(['git','status','--porcelain'],cwd=root)

def step(name,args,changes=None):return name,args,changes or {}
def modules(toolchain):
 result=[]
 for directory in ['.','javascript','experiments/javascript']:
  prefix=directory.replace('/','-').replace('.','core')
  for label,args in [('vet',['vet','./...']),('test',['test','-count=1','./...']),('race',['test','-race','-count=1','./...']),('modules',['mod','verify'])]:result.append(step(prefix+'-'+label,['go','-C',directory]+args,{'GOTOOLCHAIN':toolchain}))
 return result

groups={'native-126':[step('format',[goroot+'/bin/gofmt','-l','.'])]+modules('go1.26.10'), 'native-127':modules('go1.27.3')}
if mode=='linux':
 groups['reference-packaging']=[step('oracle-install',['npm','ci','--ignore-scripts','--prefix','conformance/oracle']),step('oracle',['go','test','-tags','oracle','-count=1','./conformance','./jsonata']),step('packaging-regressions',['python3','javascript/scripts/test_check_module.py']),step('outside-consumer',['python3','javascript/scripts/check_module.py'])]
 groups['probes-soak']=[step('assessment-build',['go','-C','experiments/javascript','build','-o',str(out/'assess'),'./cmd/assess'],{'CGO_ENABLED':'0'}),step('owned-probes',['python3','experiments/javascript/run_probes.py','--binary',str(out/'assess'),'--backend','owned-quickjs','--strict','--output',str(out/'owned-probes.json')]),step('recovery-soak',['go','-C','javascript','run','./cmd/hardening','-iterations','100','-workers','4','-cycles','3'])]
 groups['guest']=[step('rebuild',['python3','javascript/internal/quickjswasm/guest/build.py','--cache',str(cache/'tools/guest-build-cache'),'--output',str(out/'quickjs.wasm')]),step('compare',['cmp','javascript/internal/quickjswasm/quickjs.wasm',str(out/'quickjs.wasm')])]
 groups['fuzz']=[]
 for directory,package,target,count in [('.', './jsonpath','FuzzCompileAndEval',40000),('.', './jsonata','FuzzCompileAndEval',14000),('.', './internal/document','FuzzPrepareAndMaterialize',27000),('javascript','.', 'FuzzOwnedTerminalPhases',200),('javascript','.', 'FuzzOwnedStringConversion',200),('javascript','./internal/quickjswasm','FuzzOwnedCompile',200),('javascript','./internal/quickjswasm','FuzzDiagnosticRecord',1000),('javascript','.', 'FuzzRuleLocation',1000),('javascript','.', 'FuzzOwnedJSONConversion',500),('javascript','.', 'FuzzOwnedResourceLimits',1000),('experiments/javascript','.', 'FuzzStringConversion',1000)]:
  groups['fuzz'].append(step(directory.replace('/','-')+'-'+package.replace('/','-')+'-'+target,['go','-C',directory,'test',package,'-run','^$','-fuzz','^'+target+'$','-fuzztime='+str(count)+'x','-parallel=2','-timeout=20m']))
 groups['security']=[step('gate-regressions',['python3','-m','unittest','discover','-s','scripts','-p','test_check_go_security.py'])]
 for target in ['linux-amd64','windows-amd64','darwin-arm64']:
  goos,goarch=target.split('-');groups['security'].append(step(target,['python3','scripts/check_go_security.py','--govulncheck',str(cache/'tools/govulncheck'),'--output',str(out/('security-'+target))],{'GOOS':goos,'GOARCH':goarch,'CGO_ENABLED':'0'}))

report={'source_commit':source,'platform':platform.platform(),'go':version,'node':subprocess.check_output(['node','--version'],env=env,text=True).strip(),'guest_sha256':hashlib.sha256((root/'javascript/internal/quickjswasm/quickjs.wasm').read_bytes()).hexdigest(),'started_utc':datetime.datetime.now(datetime.timezone.utc).isoformat(),'groups':{},'complete':False,'passed':False}
lock=threading.Lock()
def save():
 p=out/'report.tmp';p.write_text(json.dumps(report,indent=2)+'\n');p.replace(out/'report.json')
def run_group(name,steps):
 with lock:report['groups'][name]={'state':'running','steps':[]};save();print(name+': started',flush=True)
 for label,command,changes in steps:
  log=out/(name+'-'+label+'.log');started=time.monotonic()
  try:
   with log.open('w') as stream:result=subprocess.run(command,cwd=root,env=env|changes,stdout=stream,stderr=subprocess.STDOUT,timeout=1800)
   code=result.returncode
   if label=='format' and log.read_text().strip():code=1
   row={'name':label,'command':command,'environment':changes,'exit_code':code,'seconds':round(time.monotonic()-started,3),'log':log.name}
  except Exception as exc:code=1;row={'name':label,'command':command,'environment':changes,'exit_code':code,'error':str(exc),'log':log.name}
  with lock:
   report['groups'][name]['steps'].append(row)
   if code:report['groups'][name]['state']='failed';save();print(name+'/'+label+': FAILED',flush=True);return
   save();print(name+'/'+label+': passed',flush=True)
 with lock:report['groups'][name]['state']='passed';save()
with concurrent.futures.ThreadPoolExecutor(max_workers=3) as pool:
 for f in [pool.submit(run_group,n,steps) for n,steps in groups.items()]:f.result()
report['complete']=True;report['passed']=all(g['state']=='passed' for g in report['groups'].values());report['finished_utc']=datetime.datetime.now(datetime.timezone.utc).isoformat();save()
print('OVERALL '+('PASS' if report['passed'] else 'FAIL')+' '+str(out/'report.json'),flush=True)
raise SystemExit(0 if report['passed'] else 1)
