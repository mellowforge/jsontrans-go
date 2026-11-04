$ErrorActionPreference='Stop'
$ProgressPreference='SilentlyContinue'
[Console]::OutputEncoding=New-Object System.Text.UTF8Encoding($false)
$task=Join-Path $env:USERPROFILE 'jsontrans-ci\extended-20261104'
$cache=Join-Path $env:USERPROFILE 'jsontrans-ci\pr-b-20261104'
$repo=Join-Path $task 'repo'
$env:PATH=(Join-Path $cache 'tools\go1.26.10\go\bin')+';'+(Join-Path $cache 'tools\mingw64\bin')+';'+$env:PATH
$env:GOTOOLCHAIN='local';$env:CGO_ENABLED='1';$env:GOMAXPROCS='2';$env:GOWORK='off'
$env:GOPATH=Join-Path $cache 'go-path';$env:GOMODCACHE=Join-Path $cache 'go-modules';$env:GOCACHE=Join-Path $cache 'go-build';$env:GOTMPDIR=Join-Path $cache 'tmp';$env:TEMP=$env:GOTMPDIR;$env:TMP=$env:GOTMPDIR
if (!(Test-Path $repo)) {
 & git -c core.autocrlf=false clone --branch codex/javascript-extended-soak (Join-Path $task 'source.bundle') $repo
 if($LASTEXITCODE -ne 0){throw 'Clone failed'}
}
Set-Location $repo
$commit=(& git rev-parse HEAD).Trim()
if($commit -ne 'dc29b608edb42111221fa3e173e889b9a7f61583'){throw 'Wrong source commit'}
if((& git status --porcelain)){throw 'Verification checkout has changes'}
$stamp=(Get-Date).ToUniversalTime().ToString('yyyyMMddTHHmmssZ')
$out=Join-Path $task ('runs\'+$stamp);New-Item -ItemType Directory -Path $out | Out-Null
$report=[ordered]@{source_commit=$commit;go=(& go version);platform=[Environment]::OSVersion.VersionString;started_utc=$stamp;guest_sha256=(Get-FileHash (Join-Path $repo 'javascript\internal\quickjswasm\quickjs.wasm') -Algorithm SHA256).Hash.ToLowerInvariant();steps=@();complete=$false;passed=$false}
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
$original=Join-Path $task 'runs\20261104T112722Z'
$prior=Get-Content -Raw (Join-Path $original 'report.json') | ConvertFrom-Json
$checks=@($prior.steps | Where-Object {$_.name -in @('vet','test','race','module-verify','build')})
if($prior.source_commit -ne $commit -or $checks.Count -ne 5 -or @($checks | Where-Object {$_.exit_code -ne 0}).Count -ne 0){throw 'Prior native verification cannot be reused'}
$binary=Join-Path $original 'hardening.exe'
$report.verification_reused_from=Join-Path $original 'report.json'
$report.binary_sha256=(Get-FileHash -Algorithm SHA256 $binary).Hash.ToLowerInvariant()
$report.wrapper_correction='Cache the asynchronous child process Handle before polling; retain the original failed report.'
Save-Report
$campaign=Join-Path $out 'campaign';New-Item -ItemType Directory -Path $campaign | Out-Null
$arguments=@('-duration','900s','-recycle','60s','-sample-interval','30s','-workers','2','-guest-mib','64','-progress',(Join-Path $campaign 'progress.jsonl'))
$clock=[Diagnostics.Stopwatch]::StartNew()
$process=Start-Process -FilePath $binary -ArgumentList $arguments -NoNewWindow -PassThru -RedirectStandardOutput (Join-Path $campaign 'result.json') -RedirectStandardError (Join-Path $campaign 'stderr.log')
$heldHandle=$process.Handle # Preserve a real handle so ExitCode remains available.
$peak=[int64]0;$observations=0;$timedOut=$false
$rssPath=Join-Path $campaign 'rss.jsonl'
while (!$process.HasExited) {
 $process.Refresh()
 if(!$process.HasExited){
  $rss=$process.WorkingSet64;$hwm=$process.PeakWorkingSet64;$peak=[Math]::Max($peak,$hwm);$observations++
  @{elapsed_ns=[int64]($clock.Elapsed.TotalMilliseconds*1000000);rss_bytes=$rss;sampled_hwm_bytes=$hwm} | ConvertTo-Json -Compress | Add-Content -Encoding UTF8 $rssPath
 }
 if($clock.Elapsed.TotalSeconds -gt 945){$timedOut=$true;Stop-Process -Id $process.Id -Force;break}
 Start-Sleep -Milliseconds 1000
}
$process.WaitForExit();$clock.Stop();$code=$process.ExitCode
$summary=[ordered]@{source_commit=$commit;profile='windows-recycled';command=@($binary)+$arguments;gomaxprocs=2;pid=$process.Id;exit_code=$code;seconds=$clock.Elapsed.TotalSeconds;rss_samples=$observations;sampled_peak_rss_bytes=$peak;timed_out=$timedOut;complete=$true;passed=$false}
try {
 $r=Get-Content -Raw (Join-Path $campaign 'result.json') | ConvertFrom-Json
 $total=[int64]0;foreach($p in $r.outcomes.PSObject.Properties){$total+=$p.Value.calls}
 $stages=@($r.post_gc_memory | ForEach-Object {$_.stage} | Select-Object -Unique)
 $summary.passed=($code -eq 0 -and !$timedOut -and $observations -gt 0 -and $r.status -eq 'passed' -and $r.complete -eq $true -and $r.elapsed_ns -ge 900000000000 -and $total -eq $r.verified_recoveries -and $total -eq ($r.completed_batches*25*2) -and $r.close_ns.Count -eq $r.construction_ns.Count -and $r.close_ns.Count -gt 0 -and $stages.Contains('baseline') -and $stages.Contains('live') -and $stages.Contains('closed'))
 $summary.verified_recoveries=$r.verified_recoveries;$summary.closed_lifecycles=$r.close_ns.Count
} catch {$summary.validation_error=$_.Exception.Message}
$summary | ConvertTo-Json -Depth 10 | Set-Content -Encoding UTF8 (Join-Path $campaign 'runner.json')
$report.steps+=@{name='campaign';exit_code=$code;seconds=$clock.Elapsed.TotalSeconds;passed=$summary.passed};$report.complete=$true;$report.passed=$summary.passed;$report.finished_utc=(Get-Date).ToUniversalTime().ToString('o');Save-Report
if(!$summary.passed){throw 'Windows campaign failed'}
Write-Output ('OVERALL PASS: '+(Join-Path $out 'report.json'))
