import datetime,json,os,subprocess,time
from pathlib import Path
root=Path('/data/ci/jsontrans-go-extended-20261104/repo');cache=Path('/data/ci/jsontrans-go-20261104-07de350');out=root.parent/'post-review'/datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%SZ');out.mkdir(parents=True)
env=dict(os.environ,GOTOOLCHAIN='go1.26.10',GOMAXPROCS='4',GOWORK='off',GOPATH=str(cache/'go-path'),GOMODCACHE=str(cache/'go-modules'),GOCACHE=str(cache/'go-build'),GOTMPDIR=str(cache/'tmp'),TMPDIR=str(cache/'tmp'))
report={'source_commit':subprocess.check_output(['git','rev-parse','HEAD'],cwd=root,text=True).strip(),'steps':[],'complete':False,'passed':False};assert report['source_commit']=='9eadb55dbd4e297fb99915167bbfbd5542cf1868'
steps=[('oracle',['go','-C','javascript','test','-run','^TestResourceFailureOracleRejectsUnrelatedCauses$','-count=1']),('strict-resource-fuzz',['go','-C','javascript','test','-run','^$','-fuzz','^FuzzOwnedResourceLimits$','-fuzztime=100000x','-parallel=2','-timeout=30m'])]
for directory in ['.','javascript','experiments/javascript']:
 for label,args in [('test',['test','./...']),('race',['test','-race','./...']),('vet',['vet','./...'])]:steps.append((directory.replace('/','-').replace('.','core')+'-'+label,['go','-C',directory]+args))
for name,args in steps:
 start=time.monotonic()
 with (out/(name+'.log')).open('w') as f:p=subprocess.run(args,cwd=root,env=env,stdout=f,stderr=subprocess.STDOUT,timeout=1900)
 report['steps'].append({'name':name,'command':args,'exit_code':p.returncode,'seconds':round(time.monotonic()-start,3),'log':name+'.log'})
 (out/'report.json').write_text(json.dumps(report,indent=2)+'\n');print(name+': '+str(p.returncode),flush=True)
 if p.returncode:raise SystemExit(p.returncode)
report['complete']=True;report['passed']=True;(out/'report.json').write_text(json.dumps(report,indent=2)+'\n');print('OVERALL PASS '+str(out/'report.json'),flush=True)
