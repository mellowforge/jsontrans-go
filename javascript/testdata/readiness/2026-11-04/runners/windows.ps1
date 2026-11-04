$ErrorActionPreference='Stop'
$ProgressPreference='SilentlyContinue'
[Console]::OutputEncoding=New-Object System.Text.UTF8Encoding($false)
$task=Join-Path $env:USERPROFILE 'jsontrans-ci\readiness-20261104'
$cache=Join-Path $env:USERPROFILE 'jsontrans-ci\pr-b-20261104'
$repo=Join-Path $task 'repo'
$env:PATH=(Join-Path $cache 'tools\go1.26.10\go\bin')+';'+(Join-Path $cache 'tools\mingw64\bin')+';'+$env:PATH
$env:GOTOOLCHAIN='local';$env:CGO_ENABLED='1';$env:GOMAXPROCS='2';$env:GOWORK='off'
$env:GOPATH=Join-Path $cache 'go-path';$env:GOMODCACHE=Join-Path $cache 'go-modules';$env:GOCACHE=Join-Path $cache 'go-build';$env:GOTMPDIR=Join-Path $cache 'tmp';$env:TEMP=$env:GOTMPDIR;$env:TMP=$env:GOTMPDIR
& git -c core.autocrlf=false clone --branch codex/javascript-production-readiness (Join-Path $task 'source.bundle') $repo
if($LASTEXITCODE -ne 0){throw 'Clone failed'}
Set-Location $repo
$commit=(& git rev-parse HEAD).Trim()
if($commit -ne 'acbf01cb0271089c08de2f63ece849067a2137fb'){throw 'Wrong source commit'}
if((& git status --porcelain)){throw 'Verification checkout has changes'}
$out=Join-Path $task 'verification';New-Item -ItemType Directory -Path $out | Out-Null
$report=[ordered]@{source_commit=$commit;go=(& go version);platform=[Environment]::OSVersion.VersionString;started_utc=(Get-Date).ToUniversalTime().ToString('o');guest_sha256=(Get-FileHash (Join-Path $repo 'javascript\internal\quickjswasm\quickjs.wasm') -Algorithm SHA256).Hash.ToLowerInvariant();steps=@();complete=$false;passed=$false}
function Save-Report {$report | ConvertTo-Json -Depth 10 | Set-Content -Encoding UTF8 (Join-Path $out 'report.json')}
function Invoke-Check([string]$name,[string[]]$commandArgs) {
 $log=Join-Path $out ($name+'.log');$clock=[Diagnostics.Stopwatch]::StartNew()
 $process=Start-Process -FilePath (Get-Command go).Source -ArgumentList $commandArgs -Wait -NoNewWindow -PassThru -RedirectStandardOutput ($log+'.out') -RedirectStandardError ($log+'.err')
 $clock.Stop();$code=$process.ExitCode
 $report.steps+=@{name=$name;command=@('go')+$commandArgs;exit_code=$code;seconds=$clock.Elapsed.TotalSeconds;stdout=($log+'.out');stderr=($log+'.err')};Save-Report
 Write-Output ($name+': exit '+$code)
 if($code -ne 0){Get-Content -Tail 50 ($log+'.out'),($log+'.err');throw ($name+' failed')}
}
Save-Report
Invoke-Check 'vet' @('-C','javascript','vet','./...')
Invoke-Check 'test' @('-C','javascript','test','-count=1','./...')
Invoke-Check 'race' @('-C','javascript','test','-race','-count=1','./...')
Invoke-Check 'module-verify' @('-C','javascript','mod','verify')
$env:CGO_ENABLED='0'
$binary=Join-Path $out 'javascript-example.exe'
Invoke-Check 'example-build' @('-C','javascript','build','-o',$binary,'./examples/basic')
$process=Start-Process -FilePath $binary -Wait -NoNewWindow -PassThru -RedirectStandardOutput (Join-Path $out 'example.out') -RedirectStandardError (Join-Path $out 'example.err')
$code=$process.ExitCode
$report.steps+=@{name='example-run';command=@($binary);exit_code=$code};Save-Report
if($code -ne 0){throw 'Example failed'}
$report.complete=$true;$report.passed=$true;$report.finished_utc=(Get-Date).ToUniversalTime().ToString('o');Save-Report
Write-Output ('OVERALL PASS: '+(Join-Path $out 'report.json'))
