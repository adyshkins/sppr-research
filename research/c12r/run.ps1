param([ValidateSet('smoke','full','replay')][string]$Mode='smoke')
$ErrorActionPreference='Stop'
$previous=Get-Location
$previousWork=$env:GOWORK
try {
    Set-Location -LiteralPath $PSScriptRoot
    $env:GOWORK='off'
    if(-not (Get-Command go -ErrorAction SilentlyContinue)){throw 'Go is not on PATH'}
    $stamp=Get-Date -Format 'yyyyMMdd-HHmmss-fff'
    $out=Join-Path 'results' ('local-'+$Mode+'-'+$stamp)
    if(Test-Path -LiteralPath $out){throw 'Output exists; no files overwritten'}
    $repeats=1; $days=12; $seed=900001
    if($Mode -eq 'full'){$repeats=32; $days=60; $seed=300001}
    if($Mode -eq 'replay'){$repeats=2; $days=60; $seed=300001}
    Write-Host ('OUTPUT: '+(Join-Path $PSScriptRoot $out))
    & go run ./cmd/study -repeats $repeats -seed $seed -days $days -scenarios 64 -horizon 7 -out $out
    if($LASTEXITCODE -ne 0){throw ('Experiment failed: '+$LASTEXITCODE)}
    Write-Host 'Completed; historical evidence unchanged.'
} finally {
    $env:GOWORK=$previousWork
    Set-Location -LiteralPath $previous.Path
}
