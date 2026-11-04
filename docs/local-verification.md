# Verification without hosted Actions

Run the workflow commands directly on machines you control. No Actions runner,
paid plan or CI server is required. Keep reports locally rather than uploading
artifacts. The existing workflows remain the reference for test coverage.

Use a separate checkout with complete history **and release tags**. Packaging
validates the minimum core pseudo-version against its preceding tag; a Git
bundle containing only a branch does not include that tag automatically.
Keep all machines on the same commit, or retain the exact tested source patch
and its hash. Record each platform and toolchain alongside results.

## Machines and prerequisites

| Machine | Checks |
|---|---|
| Linux amd64 | Full workflow, security target matrix, Linux guest rebuild |
| macOS arm64 | Native tests on both Go versions; other checks can also run here |
| Windows amd64 VM | Native public JavaScript vet, tests and race detection |

The current CI versions are Go **1.26.10** for the main checks and **1.27.3**
for the native `stable` matrix, Node **22** for the reference oracle, Python 3,
Git and a working C compiler for race detection. Update these pins together
with CI when versions change. The owned guest rebuild downloads the pinned
WASI SDK; ordinary Go tests require neither Node nor a C compiler for the guest.
Go's race detector itself requires CGO and a supported C toolchain.

Toolchains and caches can live in a private workspace. Set GOPATH, GOMODCACHE,
GOCACHE, GOTMPDIR, TMPDIR and npm_config_cache there to avoid changing global
installations or filling a small system disk. Download prerequisites first;
packaging checks themselves use local Git and cached modules without network
access. Do not register a runner service just to execute these commands.

## Linux and macOS

From the checkout root, with Node 22 on PATH:

```sh
set -eu
export GOTOOLCHAIN=go1.26.10
export GOWORK=off

gofmt_tool="$(go env GOROOT)/bin/gofmt"
formatting=$("$gofmt_tool" -l .)
test -z "$formatting"

for module in . javascript experiments/javascript; do
  go -C "$module" vet ./...
  go -C "$module" test -count=1 ./...
  go -C "$module" test -race -count=1 ./...
  go -C "$module" mod verify
done

GOTOOLCHAIN=go1.27.3 go vet ./...
GOTOOLCHAIN=go1.27.3 go test -count=1 ./...
GOTOOLCHAIN=go1.27.3 go test -race -count=1 ./...

npm ci --ignore-scripts --prefix conformance/oracle
go test -tags oracle -count=1 ./conformance ./jsonata
python3 javascript/scripts/test_check_module.py
python3 javascript/scripts/check_module.py

CGO_ENABLED=0 go -C experiments/javascript build -o /tmp/jsontrans-owned-assess ./cmd/assess
python3 experiments/javascript/run_probes.py --binary /tmp/jsontrans-owned-assess --backend owned-quickjs --strict --output /tmp/jsontrans-owned-probes.json
go -C javascript run ./cmd/hardening -iterations 50 -workers 4 -cycles 2

python3 javascript/internal/quickjswasm/guest/build.py --output /tmp/jsontrans-guest.wasm
cmp javascript/internal/quickjswasm/quickjs.wasm /tmp/jsontrans-guest.wasm
```

The `-count=1` runs execute tests even if earlier results are cached. Do not set
CGO_ENABLED=0 globally: it would disable race detection. Use a task-specific
temporary directory instead of `/tmp` above where appropriate.

Run the same bounded fuzz targets as CI; counts avoid timed coordinator expiry:

```sh
go test ./jsonpath -run '^$' -fuzz '^FuzzCompileAndEval$' -fuzztime=40000x -parallel=2
go test ./jsonata -run '^$' -fuzz '^FuzzCompileAndEval$' -fuzztime=14000x -parallel=2
go test ./internal/document -run '^$' -fuzz '^FuzzPrepareAndMaterialize$' -fuzztime=27000x -parallel=2
go -C javascript test -run '^$' -fuzz '^FuzzOwnedStringConversion$' -fuzztime=200x -parallel=2
go -C javascript test ./internal/quickjswasm -run '^$' -fuzz '^FuzzOwnedCompile$' -fuzztime=200x -parallel=2
go -C javascript test ./internal/quickjswasm -run '^$' -fuzz '^FuzzDiagnosticRecord$' -fuzztime=1000x -parallel=2
go -C javascript test -run '^$' -fuzz '^FuzzRuleLocation$' -fuzztime=1000x -parallel=2
go -C javascript test -run '^$' -fuzz '^FuzzOwnedJSONConversion$' -fuzztime=500x -parallel=2
go -C experiments/javascript test -run '^$' -fuzz '^FuzzStringConversion$' -fuzztime=1000x -parallel=2
```

Install the host-native scanner before setting cross-target variables:

```sh
GOBIN=/tmp/jsontrans-security-tools go install golang.org/x/vuln/cmd/govulncheck@v1.9.0
python3 -m unittest discover -s scripts -p 'test_check_go_security.py'
for target in linux-amd64 windows-amd64 darwin-arm64; do
  GOOS=${target%-*} GOARCH=${target#*-} CGO_ENABLED=0 \
    python3 scripts/check_go_security.py \
    --govulncheck /tmp/jsontrans-security-tools/govulncheck \
    --output "/tmp/jsontrans-security-$target"
done
```

These cross-target scans analyze source and binaries; they do not execute
Windows or macOS code on Linux. A clean Go scan does not clear the embedded
QuickJS release blocker. See the [security update record](javascript-go-security-updates.md).

## Windows

Use an actual x64 Windows machine or VM, with Go 1.26 and a supported Windows
C toolchain on PATH. A Windows VM on the Linux host is sufficient; Linux
containers and cross-compilation do not replace Windows execution.

From the repository root in PowerShell:

```powershell
$env:GOTOOLCHAIN = 'go1.26.10'
$env:CGO_ENABLED = '1'
function Invoke-Go {
    & go @args
    if ($LASTEXITCODE -ne 0) { throw "go failed with exit code $LASTEXITCODE" }
}
Invoke-Go -C javascript vet ./...
Invoke-Go -C javascript test -count=1 ./...
Invoke-Go -C javascript test -race -count=1 ./...
```

This matches the existing Windows workflow. Preserve stdout/stderr and exit
codes; missing tools or unexecuted platforms remain pending, never a pass.

## GitHub status and later automation

Local execution does not turn an existing red GitHub check green. Retain the
commit, source patch, toolchain versions, commands, logs and exit codes as local
evidence. A push normally triggers the existing hosted workflows; coordinate
how to publish a verified fix before pushing while hosted minutes are exhausted.
Do not disable required checks or change billing settings as part of local runs.

Later, the same commands can run on self-hosted Linux/Windows/macOS runners.
GitHub's current [billing documentation](https://docs.github.com/en/billing/concepts/product-billing/github-actions)
lists self-hosted compute as free; cloud artifact/cache storage has separate
allowances. The current workflows also trigger on both push and pull_request,
so a PR branch update can execute two complete matrices. Adjusting those
triggers and runner labels is a separate automation change.
