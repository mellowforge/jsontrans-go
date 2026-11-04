$ErrorActionPreference='Stop'
$ProgressPreference='SilentlyContinue'
[Console]::OutputEncoding=New-Object System.Text.UTF8Encoding($false)
$task=Join-Path $env:USERPROFILE 'jsontrans-ci\extended-20261104'
$repo=Join-Path $task 'repo'
$cache=Join-Path $env:USERPROFILE 'jsontrans-ci\pr-b-20261104'
$env:PATH=(Join-Path $cache 'tools\go1.26.10\go\bin')+';'+(Join-Path $cache 'tools\mingw64\bin')+';'+$env:PATH
$env:GOTOOLCHAIN='local'; $env:CGO_ENABLED='1'; $env:GOMAXPROCS='4'; $env:GOWORK='off'
$env:GOPATH=Join-Path $cache 'go-path'; $env:GOMODCACHE=Join-Path $cache 'go-modules'; $env:GOCACHE=Join-Path $cache 'go-build'
$env:GOTMPDIR=Join-Path $cache 'tmp'; $env:TMP=$env:GOTMPDIR; $env:TEMP=$env:GOTMPDIR
New-Item -ItemType Directory -Force -Path $env:GOTMPDIR | Out-Null
if (!(Test-Path $repo)) {
& git -c core.autocrlf=false clone --branch codex/javascript-extended-soak (Join-Path $task 'source-final.bundle') $repo
if($LASTEXITCODE -ne 0){ throw 'Clone failed' }
}
Set-Location $repo
if ((& git status --porcelain)) { throw 'Verification checkout has changes' }
& git fetch (Join-Path $task 'source-final.bundle') codex/javascript-extended-soak
if($LASTEXITCODE -ne 0){throw 'Bundle fetch failed'}
& git merge --ff-only FETCH_HEAD
if($LASTEXITCODE -ne 0){throw 'Bundle fast-forward failed'}
$commit=(& git rev-parse HEAD).Trim()
if($commit -ne '9eadb55dbd4e297fb99915167bbfbd5542cf1868'){ throw 'Unexpected source commit' }
$stamp=(Get-Date).ToUniversalTime().ToString('yyyyMMddTHHmmssZ')
$out=Join-Path $task ('post-review\'+$stamp)
New-Item -ItemType Directory -Force -Path $out | Out-Null
$report=[ordered]@{commit=$commit;go=(& go version);platform=[Environment]::OSVersion.VersionString;processor=$env:PROCESSOR_IDENTIFIER;started_utc=$stamp;steps=@();complete=$false}
function Save-Report { $report | ConvertTo-Json -Depth 8 | Set-Content -Encoding UTF8 (Join-Path $out 'report.json') }
function Invoke-Check([string]$name,[string[]]$commandArgs) {
 $log=Join-Path $out ($name+'.log'); $clock=[Diagnostics.Stopwatch]::StartNew()
 $process=Start-Process -FilePath (Get-Command go).Source -ArgumentList $commandArgs -Wait -NoNewWindow -PassThru -RedirectStandardOutput ($log+'.out') -RedirectStandardError ($log+'.err')
 $code=$process.ExitCode
 Get-Content ($log+'.out'),($log+'.err') | Set-Content -Encoding UTF8 $log
 $clock.Stop()
 $report.steps+=@{name=$name;command=@('go')+$commandArgs;exit_code=$code;seconds=$clock.Elapsed.TotalSeconds;log=$log}
 Save-Report
 Write-Output ($name+': exit '+$code)
 if($code -ne 0){Get-Content -Tail 60 $log;throw ($name+' failed')}
}
Save-Report
Invoke-Check 'vet' @('-C','javascript','vet','./...')
Invoke-Check 'test' @('-C','javascript','test','-count=1','./...')
Invoke-Check 'race' @('-C','javascript','test','-race','-count=1','./...')
Invoke-Check 'module-verify' @('-C','javascript','mod','verify')
$env:CGO_ENABLED='0'
Invoke-Check 'cgo-free-build' @('-C','javascript','build','-o',(Join-Path $out 'example.exe'),'./examples/basic')
& (Join-Path $out 'example.exe') > (Join-Path $out 'example.log')
if($LASTEXITCODE -ne 0){throw 'CGO-free example failed'}
$report.complete=$true; $report.passed=$true; $report.finished_utc=(Get-Date).ToUniversalTime().ToString('o');Save-Report
Write-Output ('OVERALL PASS: '+(Join-Path $out 'report.json'))
