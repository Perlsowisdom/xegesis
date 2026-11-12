# Builds the direct-download setup: build\bin\Xegesis-<version>-windows-x64-setup.exe.
# Needs NSIS (makensis) on PATH or in "C:\Program Files (x86)\NSIS". On GitHub's windows-latest runner:
#   choco install nsis -y
# Steps: make the icon/manifest resource, build the app, fetch the WebView2 bootstrapper, run NSIS.
# The frontend must already be built (cd frontend; npm ci; npm run build).
param([string]$Version = (& "$PSScriptRoot\version.ps1"), [switch]$SkipWebView2)
$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot
Push-Location $root
try {
  $make = (Get-Command makensis -ErrorAction SilentlyContinue).Source
  if (-not $make) { $make = 'C:\Program Files (x86)\NSIS\makensis.exe' }
  if (-not (Test-Path $make)) { throw 'makensis not found. Install NSIS (choco install nsis).' }
  Push-Location tools
  go run ./winres .. $Version
  if ($LASTEXITCODE) { throw 'winres failed' }
  Pop-Location
  try { & "$PSScriptRoot\build.ps1" -Version $Version } finally { Remove-Item app\rsrc_windows_amd64.syso -ErrorAction SilentlyContinue }
  $nums = @([regex]::Matches($Version, '\d+') | Select-Object -First 3 | ForEach-Object { $_.Value })
  while ($nums.Count -lt 3) { $nums += '0' }
  $versionNum = ($nums -join '.') + '.0'
  $setup = "$root\build\bin\Xegesis-$Version-windows-x64-setup.exe"
  $nsisArgs = @("/DVERSION=$Version", "/DVERSION_NUM=$versionNum", "/DEXE=$root\build\bin\Xegesis.exe", "/DOUTFILE=$setup")
  if (-not $SkipWebView2) {
    $wv = "$root\build\bin\MicrosoftEdgeWebview2Setup.exe"
    if (-not (Test-Path $wv)) { Invoke-WebRequest 'https://go.microsoft.com/fwlink/p/?LinkId=2124703' -OutFile $wv }
    $nsisArgs += "/DWEBVIEW2=$wv"
  }
  & $make @nsisArgs "$root\build\windows\installer\xegesis.nsi"
  if ($LASTEXITCODE -ne 0) { throw 'makensis failed' }
  Write-Host "Built $setup"
} finally { Pop-Location }
