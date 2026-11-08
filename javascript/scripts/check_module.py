#!/usr/bin/env python3
"""Verify module-zip consumption outside the checkout with no replace/workspace.

Run module tests first so normal dependencies exist in the local Go cache. The
minimum core is fetched by Go from local Git history (full history required), not
from current working-tree files. Neither stage needs network or credentials.
"""
import argparse
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile
import zipfile

ROOT = Path(__file__).resolve().parents[2]
MODULE = 'github.com/mellowforge/jsontrans-go/javascript'
CORE = 'github.com/mellowforge/jsontrans-go'
VERSION = 'v0.0.0-extraction'


def run(command, *, cwd, env, capture=False):
    p = subprocess.run(command, cwd=cwd, env=env, text=True, capture_output=capture)
    if p.returncode:
        if capture:
            print(p.stdout, end='')
            print(p.stderr, end='')
        raise SystemExit(p.returncode)
    return p.stdout if capture else ''


def download_core(go, version, work, env, repository):
    # Go discovers short pseudo-version revisions through advertised branches.
    # PR checkouts are detached and have no local heads. Advertise the immutable
    # minimum commit in a private mirror, leaving the checkout untouched.
    mirror = work / 'core-source.git'
    run(['git', 'clone', '--mirror', '--no-hardlinks', str(repository), str(mirror)], cwd=work, env=env, capture=True)
    pseudo = re.search(r'-([0-9a-f]{12})$', version)
    ref = pseudo.group(1) if pseudo else 'refs/tags/' + version
    commit = run(['git', '--git-dir', str(mirror), 'rev-parse', ref + '^{commit}'], cwd=work, env=env, capture=True).strip()
    run(['git', '--git-dir', str(mirror), 'update-ref', 'refs/heads/module-minimum', commit], cwd=work, env=env)
    # Environment-only Git rewrite; never modify caller Git configuration.
    fetch = env | dict(GOPROXY='direct', GONOPROXY=CORE, GIT_ALLOW_PROTOCOL='file',
                       GIT_CONFIG_COUNT='1', GIT_CONFIG_KEY_0='url.' + mirror.as_uri() + '.insteadOf',
                       GIT_CONFIG_VALUE_0='https://github.com/mellowforge/jsontrans-go')
    run([go, 'mod', 'download', CORE + '@' + version], cwd=work, env=fetch)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--module-dir', type=Path, default=ROOT / 'javascript')
    args = parser.parse_args()
    source = args.module_dir.resolve()
    manifest = (source / 'go.mod').read_text()
    core_version = re.search(r'github.com/mellowforge/jsontrans-go\s+(\S+)', manifest).group(1)
    original = os.environ.copy()
    goroot = run(['go', 'env', 'GOROOT'], cwd=ROOT, env=original, capture=True).strip()
    old_cache = run(['go', 'env', 'GOMODCACHE'], cwd=ROOT, env=original, capture=True).strip()
    go = str(Path(goroot) / 'bin' / ('go.exe' if os.name == 'nt' else 'go'))
    with tempfile.TemporaryDirectory(prefix='jsontrans-module-consumer-') as tmp:
        work = Path(tmp)
        cache = work / 'cache'
        env = original | dict(GOWORK='off', GOTOOLCHAIN='local', GOMODCACHE=str(cache),
                              GOSUMDB='off', GOPRIVATE=CORE, CGO_ENABLED='0')
        # Canonical archive and version validation by Go itself.
        download_core(go, core_version, work, env, ROOT)
        proxy = work / 'proxy'
        versions = proxy / MODULE / '@v'
        versions.mkdir(parents=True)
        (versions / (VERSION + '.mod')).write_text(manifest)
        (versions / (VERSION + '.info')).write_text(json.dumps({'Version': VERSION, 'Time': '2026-11-03T00:00:00Z'}))
        (versions / 'list').write_text(VERSION + '\n')
        prefix = MODULE + '@' + VERSION + '/'
        with zipfile.ZipFile(versions / (VERSION + '.zip'), 'w', zipfile.ZIP_DEFLATED) as archive:
            for path in sorted(source.rglob('*')):
                relative = path.relative_to(source)
                if any(part.startswith('.') or part == '__pycache__' for part in relative.parts):
                    continue
                if path.is_symlink():
                    raise SystemExit('module fixture must not follow symlinks: ' + str(relative))
                if path.is_file():
                    archive.writestr(prefix + relative.as_posix(), path.read_bytes())
        env |= dict(GOPROXY=proxy.as_uri() + ',' + (Path(old_cache) / 'cache/download').as_uri() + ',off', GONOPROXY='none')
        consumer = work / 'consumer'
        consumer.mkdir()
        (consumer / 'go.mod').write_text('module example.com/consumer\n\ngo 1.26.0\n\nrequire ' + MODULE + ' ' + VERSION + '\n')
        (consumer / 'main.go').write_text('''package main
import("context";"errors";"fmt";jt "github.com/mellowforge/jsontrans-go";js "github.com/mellowforge/jsontrans-go/javascript")
func main(){ctx:=context.Background();e,err:=js.New(ctx,js.Config{});if err!=nil{panic(err)};defer e.Close()
x,err:=jt.New(map[string]jt.Processor{"js":e});if err!=nil{panic(err)}
r,err:=x.ProcessJSON(ctx,"js",[]byte(`{"x":3}`),`(x,b)=>x.x+b.n`,jt.EvalOptions{Bindings:map[string]any{"n":2}});if err!=nil||string(r.Value)!="5"{panic(fmt.Sprint(r,err))}
prepared,err:=x.Compile(ctx,"js",`(x,b)=>x.x+b.n`);if err!=nil{panic(err)}
doc,err:=jt.PrepareJSON(ctx,[]byte(`{"x":3}`),jt.PrepareOptions{});if err!=nil{panic(err)}
options:=jt.EvalOptions{Bindings:map[string]any{"n":2}}
encoded,err:=jt.EvalPreparedJSON(ctx,prepared,doc,options);if err!=nil||!encoded.Defined||string(encoded.Value)!="5"{panic(fmt.Sprint(encoded,err))}
batch,err:=jt.NewBatch([]jt.Program{prepared});if err!=nil{panic(err)}
results,err:=batch.EvalPreparedJSON(ctx,doc,nil,options);if err!=nil||len(results)!=1||!results[0].Defined||string(results[0].Value)!="5"{panic(fmt.Sprint(results,err))}
p,err:=e.Compile(ctx,`()=>{throw new TypeError('sentinel')}`);if err!=nil{panic(err)};_,err=p.Eval(ctx,nil,jt.EvalOptions{});var ex *js.Exception;if !errors.As(err,&ex)||ex.Name!="TypeError"{panic(fmt.Sprint(err))}
if err=e.Close();err!=nil{panic(err)};_,err=p.Eval(ctx,nil,jt.EvalOptions{});if !errors.Is(err,js.ErrClosed){panic(fmt.Sprint(err))};fmt.Println("outside consumer: PASS")}
''')
        run([go, 'mod', 'tidy'], cwd=consumer, env=env)
        modules = run([go, 'list', '-m', '-json', CORE], cwd=consumer, env=env, capture=True)
        resolved = json.loads(modules)
        if resolved.get('Replace') or resolved['Version'] != core_version:
            raise SystemExit('consumer did not resolve the pinned core without replaces')
        deps = run([go, 'list', '-deps', '.'], cwd=consumer, env=env, capture=True)
        forbidden = ('github.com/dop251/goja', 'github.com/fastschema/qjs', CORE + '/experiments/', CORE + '/jsonata', CORE + '/jsonpath')
        if any(item in deps for item in forbidden):
            raise SystemExit('optional processor dependency boundary violated')
        binary = work / ('consumer.exe' if os.name == 'nt' else 'consumer-bin')
        run([go, 'build', '-o', str(binary), '.'], cwd=consumer, env=env)
        empty = work / 'empty'
        empty.mkdir()
        run([str(binary)], cwd=empty, env=env)
        native = work / 'native'
        native.mkdir()
        (native / 'go.mod').write_text('module example.com/native\n\ngo 1.26.0\n\nrequire ' + CORE + ' ' + core_version + '\n')
        (native / 'main.go').write_text('package main\nimport jt "github.com/mellowforge/jsontrans-go"\nfunc main(){_,_=jt.New(nil)}\n')
        # Inspect this consumer's production imports. tidy also loads tests of
        # dependency packages, pulling historical JSONata-only requirements
        # unrelated to the root-contract isolation check into this fixture.
        deps = run([go, 'list', '-mod=mod', '-deps', '.'], cwd=native, env=env, capture=True)
        if MODULE in deps or 'github.com/tetratelabs/wazero' in deps:
            raise SystemExit('native root acquired optional runtime dependency')
        native_binary = work / ('native.exe' if os.name == 'nt' else 'native-bin')
        run([go, 'build', '-mod=readonly', '-o', str(native_binary), '.'], cwd=native, env=env)
        run([str(native_binary)], cwd=empty, env=env)
        print('module zip, minimum core, embedded assets, dependency boundaries: PASS')


if __name__ == '__main__':
    main()
