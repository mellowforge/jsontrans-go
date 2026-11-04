import concurrent.futures
import datetime
import hashlib
import json
import os
from pathlib import Path
import platform
import subprocess
import sys
import threading
import time

workspace = Path('/data/ci/jsontrans-go-pr-b-20261104')
cache = Path('/data/ci/jsontrans-go-20261104-07de350')
root = workspace / 'repo'
out = workspace / 'runs' / datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%SZ')
out.mkdir(parents=True, exist_ok=False)
env = dict(os.environ, GOTOOLCHAIN='go1.26.10', GOMAXPROCS='8', GOWORK='off',
           GOPATH=str(cache/'go-path'), GOMODCACHE=str(cache/'go-modules'),
           GOCACHE=str(cache/'go-build'), GOTMPDIR=str(cache/'tmp'),
           TMPDIR=str(cache/'tmp'), npm_config_cache=str(cache/'npm-cache'))
env['PATH'] = str(cache/'tools/node-v22.22.3-linux-x64/bin') + ':' + str(cache/'tools') + ':' + env['PATH']
version = subprocess.check_output(['go','version'], env=env, text=True).strip()
goroot = subprocess.check_output(['go','env','GOROOT'], env=env, text=True).strip()
env['PATH'] = goroot + '/bin:' + env['PATH']

def step(name, args, changes=None):
    return (name, args, changes or {})

def module_steps(prefix):
    args=['go'] + (['-C',prefix] if prefix else [])
    return [step('vet', args+['vet','./...']), step('test', args+['test','-count=1','./...']),
            step('race', args+['test','-race','-count=1','./...']), step('verify-modules', args+['mod','verify'])]

groups = {
 'native-126': [step('format', [goroot+'/bin/gofmt','-l','.'])]+module_steps(''),
 'native-stable': [step(n,a,{'GOTOOLCHAIN':'go1.27.3'}) for n,a,_ in module_steps('')],
 'public': module_steps('javascript') + [step('packaging-regressions',['python3','javascript/scripts/test_check_module.py']),step('packaging',['python3','javascript/scripts/check_module.py'])],
 'assessment': module_steps('experiments/javascript') + [step('build',['go','-C','experiments/javascript','build','-o',str(out/'assess'),'./cmd/assess'],{'CGO_ENABLED':'0'}),step('owned-probes',['python3','experiments/javascript/run_probes.py','--binary',str(out/'assess'),'--backend','owned-quickjs','--strict','--output',str(out/'owned-probes.json')])],
 'reference': [step('install-oracle',['npm','ci','--ignore-scripts','--prefix','conformance/oracle']),step('oracle',['go','test','-tags','oracle','-count=1','./conformance','./jsonata'])],
 'fuzz': [],
 'soak': [step('recovery',['go','-C','javascript','run','./cmd/hardening','-iterations','5000','-workers','4','-cycles','5'])],
 'guest': [step('rebuild',['python3','javascript/internal/quickjswasm/guest/build.py','--cache',str(cache/'tools/guest-build-cache'),'--output',str(out/'quickjs.wasm')]), step('compare',['cmp','javascript/internal/quickjswasm/quickjs.wasm',str(out/'quickjs.wasm')])],
 'security': [step('gate-tests',['python3','-m','unittest','discover','-s','scripts','-p','test_check_go_security.py'])],
}
for directory,package,target,count in [('', './jsonpath','FuzzCompileAndEval',40000),('', './jsonata','FuzzCompileAndEval',14000),('', './internal/document','FuzzPrepareAndMaterialize',27000),('javascript','.', 'FuzzOwnedTerminalPhases',200),('javascript','.', 'FuzzOwnedStringConversion',200),('javascript','./internal/quickjswasm','FuzzOwnedCompile',200),('javascript','./internal/quickjswasm','FuzzDiagnosticRecord',1000),('javascript','.', 'FuzzRuleLocation',1000),('javascript','.', 'FuzzOwnedJSONConversion',500),('experiments/javascript','.', 'FuzzStringConversion',1000)]:
    args=['go']+(['-C',directory] if directory else [])+['test',package,'-run','^$','-fuzz','^'+target+'$','-fuzztime='+str(count)+'x','-parallel=2']
    groups['fuzz'].append(step((directory.replace('/','-') or package.replace('./','').replace('/','-'))+'-'+target,args))
for target,goos,goarch in [('linux-amd64','linux','amd64'),('windows-amd64','windows','amd64'),('darwin-arm64','darwin','arm64')]:
    groups['security'].append(step(target,['python3','scripts/check_go_security.py','--govulncheck',str(cache/'tools/govulncheck'),'--output',str(out/('security-'+target))],{'GOOS':goos,'GOARCH':goarch,'CGO_ENABLED':'0'}))
patch = subprocess.check_output(['git','diff','--binary'],cwd=root)
(out/'source.patch').write_bytes(patch)
report={'commit':subprocess.check_output(['git','rev-parse','HEAD'],cwd=root,text=True).strip(), 'source_patch_sha256':hashlib.sha256(patch).hexdigest(), 'platform':platform.platform(), 'go':version, 'node':subprocess.check_output(['node','--version'],env=env,text=True).strip(), 'started_utc':datetime.datetime.now(datetime.timezone.utc).isoformat(), 'groups':{}, 'complete':False}
lock=threading.Lock()
def save():
    temp=out/'report.tmp'
    temp.write_text(json.dumps(report,indent=2)+'\n')
    temp.replace(out/'report.json')

def run_group(name, steps):
    with lock:
        report['groups'][name]={'state':'running','steps':[]}
        save()
        print(name+': started',flush=True)
    for label,command,changes in steps:
        log=out/(name+'-'+label+'.log')
        started=time.monotonic()
        try:
            with log.open('w') as stream:
                result=subprocess.run(command,cwd=root,env=env|changes,stdout=stream,stderr=subprocess.STDOUT,timeout=1800)
            code=result.returncode
            if label=='format' and log.read_text().strip():
                code=1
            item={'name':label,'command':command,'environment':changes,'exit_code':code,'seconds':round(time.monotonic()-started,3),'log':log.name}
        except Exception as exc:
            code=1
            item={'name':label,'command':command,'exit_code':code,'error':str(exc),'log':log.name}
        with lock:
            report['groups'][name]['steps'].append(item)
            if code:
                report['groups'][name]['state']='failed'
                save()
                print(name+'/'+label+': FAILED; see '+str(log),flush=True)
                return
            save()
            print(name+'/'+label+': passed',flush=True)
    with lock:
        report['groups'][name]['state']='passed'
        save()

with concurrent.futures.ThreadPoolExecutor(max_workers=3) as executor:
    futures=[executor.submit(run_group,name,steps) for name,steps in groups.items()]
    for f in futures:
        f.result()
report['complete']=True
report['passed']=all(v['state']=='passed' for v in report['groups'].values())
report['finished_utc']=datetime.datetime.now(datetime.timezone.utc).isoformat()
save()
print('OVERALL: '+('PASS' if report['passed'] else 'FAIL')+' '+str(out/'report.json'),flush=True)
sys.exit(0 if report['passed'] else 1)
