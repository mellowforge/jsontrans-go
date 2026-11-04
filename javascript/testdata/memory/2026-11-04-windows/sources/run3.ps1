param([string]$name='probe3',[string]$yieldMode='0')
$ErrorActionPreference='Stop'
$task=Join-Path $env:USERPROFILE 'jsontrans-ci\winmem-20261104'
$out=Join-Path $task $name
New-Item -ItemType Directory -Path $out | Out-Null
$env:GOMAXPROCS='2'
$env:JSONTRANS_MEMORY_YIELD=$yieldMode
$env:JSONTRANS_MEMORY_PROFILE='0'
$env:JSONTRANS_MEMORY_DIAGNOSTICS=$out
$binary=Join-Path $task 'diagnostic2.test.exe'
[ordered]@{started_utc=(Get-Date).ToUniversalTime().ToString('o');platform=[Environment]::OSVersion.VersionString;gomaxprocs=2;binary_sha256=(Get-FileHash $binary -Algorithm SHA256).Hash;command=@($binary,'-test.run','^TestMemoryDiagnostic$','-test.v','-test.timeout','6m')} | ConvertTo-Json -Depth 6 | Set-Content -Encoding UTF8 (Join-Path $out 'run.json')
& $binary '-test.run=^TestMemoryDiagnostic$' '-test.v' '-test.timeout=6m' *> (Join-Path $out 'test.log')
$code=$LASTEXITCODE
@{exit_code=$code;finished_utc=(Get-Date).ToUniversalTime().ToString('o')} | ConvertTo-Json | Set-Content -Encoding UTF8 (Join-Path $out 'exit.json')
Get-Content (Join-Path $out 'test.log')
exit $code
