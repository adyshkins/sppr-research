$ErrorActionPreference = "Stop"
Set-Location $PSScriptRoot
if (Get-Command py -ErrorAction SilentlyContinue) {
    & py -3 tools/run.py --install
} elseif (Get-Command python -ErrorAction SilentlyContinue) {
    & python tools/run.py --install
} else { throw "Python 3.10+ is required. Install Python and restart the terminal." }
if ($LASTEXITCODE -ne 0) { throw "Startup failed. Read the message above." }
