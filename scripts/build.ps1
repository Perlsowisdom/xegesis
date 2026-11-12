# Builds the desktop app for Windows into build\bin\Xegesis.exe with the version stamped in.
#   scripts\build.ps1                  direct-download build (has the updater)
#   scripts\build.ps1 -Store           Microsoft Store build (no updater: build tag "store")
# The frontend must already be built (cd frontend; npm ci; npm run build).
param([string]$Version = (& "$PSScriptRoot\version.ps1"), [switch]$Store, [string]$Out = 'build\bin\Xegesis.exe')
$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot
Push-Location $root
try {
  $tags = 'desktop,production'
  if ($Store) { $tags += ',store' }
  $ld = "-s -w -H windowsgui -X github.com/Perlsowisdom/xegesis/internal/command/core.Version=$Version"
  New-Item -ItemType Directory -Force (Split-Path $Out) | Out-Null
  go build -trimpath -tags $tags -ldflags $ld -o $Out ./app
  if ($LASTEXITCODE -ne 0) { throw 'go build failed' }
  Write-Host "Built $Out version $Version tags $tags"
} finally { Pop-Location }
