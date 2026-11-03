#!/usr/bin/env python3
"""Build pinned QuickJS core + owned ABI. No upstream libc/module/host wrappers."""
import argparse, hashlib, json, os, pathlib, platform, shutil, subprocess, tarfile, tempfile, urllib.request
HERE = pathlib.Path(__file__).resolve().parent
LOCK = json.loads((HERE / 'lock.json').read_text())
parser = argparse.ArgumentParser()
parser.add_argument('--cache', type=pathlib.Path, default=pathlib.Path('/tmp/jsontrans-quickjs-toolchain'))
parser.add_argument('--output', type=pathlib.Path, default=HERE.parent / 'quickjs.wasm')
args = parser.parse_args()
args.cache.mkdir(parents=True, exist_ok=True)
arch = {'aarch64': 'arm64', 'arm64': 'arm64', 'x86_64': 'x86_64'}.get(platform.machine())
os_name = {'Darwin':'macos','Linux':'linux'}.get(platform.system())
asset = f'{arch}-{os_name}'
if asset not in LOCK['sdk_assets']:
    raise SystemExit(f'no checksum-pinned SDK asset for {asset}')

def archive(name, url, checksum):
    path = args.cache / name
    if not path.exists():
        urllib.request.urlretrieve(url, path)
    if hashlib.sha256(path.read_bytes()).hexdigest() != checksum:
        raise SystemExit(f'checksum mismatch: {path}')
    return path

source = archive('quickjs.tar.gz', LOCK['source_url'], LOCK['source_sha256'])
sdk_version = LOCK['sdk_version']
sdk_release = 'wasi-sdk-' + sdk_version.split('.')[0]
sdk_name = f'wasi-sdk-{sdk_version}-{asset}'
sdk = archive(sdk_name + '.tar.gz', f'https://github.com/WebAssembly/wasi-sdk/releases/download/{sdk_release}/{sdk_name}.tar.gz', LOCK['sdk_assets'][asset])
with tempfile.TemporaryDirectory(prefix='jsontrans-guest-build-') as directory:
    build = pathlib.Path(directory)
    for path in [source, sdk]:
        with tarfile.open(path) as tar:
            tar.extractall(build, filter='data')
    src = build / ('quickjs-' + LOCK['quickjs_commit'])
    compiler = build / sdk_name / 'bin/clang'
    out = build / 'guest.wasm'
    command = [str(compiler), '-O2', '-mexec-model=reactor', '-DNDEBUG', '-D_GNU_SOURCE', '-fvisibility=hidden', '-fno-ident',
               f'-ffile-prefix-map={build}=/build', f'-I{src}', '-Wl,--no-entry',
               '-Wl,-z,stack-size=1048576', '-Wl,--initial-memory=2097152',
               '-Wl,--max-memory=67108864', '-Wl,--strip-all']
    exports = ['abi_version','abi_init','abi_alloc','abi_free','abi_compile','abi_eval',
               'abi_output_ptr','abi_output_len','abi_status']
    command += ['-Wl,--export=' + name for name in exports]
    command += ['-Wl,--wrap=__wasi_' + name for name in ['clock_time_get','fd_close','fd_seek','fd_write','proc_exit']]
    command += [str(src / name) for name in ['cutils.c','dtoa.c','libregexp.c','libunicode.c','quickjs.c']]
    # Copy ABI to a stable path so file/debug metadata cannot encode the checkout path.
    shutil.copyfile(HERE / 'abi.c', build / 'abi.c')
    command += [str(build / 'abi.c'), '-lm', '-o', str(out)]
    subprocess.run(command, check=True, env={**os.environ, 'SOURCE_DATE_EPOCH':'1760511657', 'LC_ALL':'C', 'TZ':'UTC'})
    digest = hashlib.sha256(out.read_bytes()).hexdigest()
    args.output.parent.mkdir(parents=True, exist_ok=True)
    shutil.copyfile(out, args.output)
    print(json.dumps(dict(sha256=digest, bytes=args.output.stat().st_size, output=str(args.output))))
