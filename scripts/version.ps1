# Prints the version to stamp into a build: a tag "vX.Y.Z" gives "X.Y.Z"; anything else gives
# "0.0.0-dev+<short commit>". CI passes the tag in $env:GITHUB_REF_NAME.
param([string]$Ref = $env:GITHUB_REF_NAME)
$ErrorActionPreference = 'Stop'
if ($Ref -match '^v(\d+\.\d+\.\d+(?:-[0-9A-Za-z.]+)?)$') { $Matches[1]; exit 0 }
$ErrorActionPreference = 'Continue'
$t = (git describe --tags --exact-match --match 'v*' 2>$null)
if ($LASTEXITCODE -eq 0 -and $t -match '^v(\d+\.\d+\.\d+(?:-[0-9A-Za-z.]+)?)$') { $Matches[1]; exit 0 }
$sha = (git rev-parse --short HEAD 2>$null)
if (-not $sha) { $sha = 'unknown' }
"0.0.0-dev+$sha"
