# Packs the Microsoft Store package: build\bin\Xegesis-<version>-windows-x64.msix.
#   scripts\package-msix.ps1                       build (Store build tag: no updater) and pack
#   scripts\package-msix.ps1 -ManifestOnly         only write and check the manifest (no SDK needed)
# The package is NOT signed. The Store signs it at certification. For a local test install, sign a
# copy with scripts\test-sign-msix.ps1 (throwaway certificate).
# Needs the Windows SDK's makeappx.exe (present on GitHub's windows-latest runner).
# Partner Center values come from the environment (or parameters); until they are set, marked
# placeholders go in and the package is only good for a test signing:
#   XEGESIS_MSIX_IDENTITY, XEGESIS_MSIX_PUBLISHER, XEGESIS_MSIX_PUBLISHER_DISPLAY
param(
  [string]$Version = (& "$PSScriptRoot\version.ps1"),
  [string]$Identity = $(if ($env:XEGESIS_MSIX_IDENTITY) { $env:XEGESIS_MSIX_IDENTITY } else { 'PLACEHOLDER.Xegesis' }),
  [string]$Publisher = $(if ($env:XEGESIS_MSIX_PUBLISHER) { $env:XEGESIS_MSIX_PUBLISHER } else { 'CN=PLACEHOLDER-PUBLISHER-ID' }),
  [string]$PublisherDisplay = $(if ($env:XEGESIS_MSIX_PUBLISHER_DISPLAY) { $env:XEGESIS_MSIX_PUBLISHER_DISPLAY } else { 'PLACEHOLDER Publisher' }),
  [switch]$ManifestOnly,
  [string]$Layout = ''
)
$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot

# msixVersion turns 1.2.3 (or 1.2.3-beta.1, or 0.0.0-dev+abc) into 1.2.3.0. The Store reserves the
# fourth part, and 0.0.0.0 is not allowed, so a dev build becomes 0.0.1.0.
function ConvertTo-MsixVersion([string]$v) {
  $n = @([regex]::Matches(($v -replace '[-+].*$', ''), '\d+') | Select-Object -First 3 | ForEach-Object { [int]$_.Value })
  while ($n.Count -lt 3) { $n += 0 }
  if (($n[0] + $n[1] + $n[2]) -eq 0) { $n[2] = 1 }
  ($n -join '.') + '.0'
}

# New-MsixManifest fills the template and checks the result: it is well-formed XML, no field is
# left unfilled, and every logo it names exists.
function New-MsixManifest([string]$Dir) {
  $text = Get-Content "$root\build\msix\AppxManifest.xml.template" -Raw
  $text = $text.Replace('{{IDENTITY_NAME}}', [Security.SecurityElement]::Escape($Identity)).
    Replace('{{PUBLISHER}}', [Security.SecurityElement]::Escape($Publisher)).
    Replace('{{PUBLISHER_DISPLAY}}', [Security.SecurityElement]::Escape($PublisherDisplay)).
    Replace('{{VERSION}}', (ConvertTo-MsixVersion $Version))
  if ($text -match '\{\{') { throw 'The manifest still has unfilled {{fields}}.' }
  $xml = [xml]$text   # throws if not well-formed
  foreach ($node in $xml.SelectNodes('//@*[contains(., "Assets\")]')) {
    if (-not (Test-Path (Join-Path $Dir $node.Value))) { throw "The manifest names $($node.Value), which is missing." }
  }
  [IO.File]::WriteAllText((Join-Path $Dir 'AppxManifest.xml'), $text, (New-Object Text.UTF8Encoding $false))
}

function Find-MakeAppx {
  $c = Get-Command makeappx.exe -ErrorAction SilentlyContinue
  if ($c) { return $c.Source }
  $kits = "${env:ProgramFiles(x86)}\Windows Kits\10\bin"
  $f = if (Test-Path $kits) { Get-ChildItem $kits -Recurse -Filter makeappx.exe | Where-Object { $_.FullName -match '\\x64\\' } | Sort-Object FullName -Descending | Select-Object -First 1 }
  if ($f) { return $f.FullName }
  throw 'makeappx.exe not found. Install the Windows SDK (it is on GitHub windows-latest runners).'
}

if (-not $Layout) { $Layout = "$root\build\msix\layout" }
if (Test-Path $Layout) { Remove-Item $Layout -Recurse -Force }
New-Item -ItemType Directory -Force "$Layout\Assets" | Out-Null
Copy-Item "$root\build\msix\Assets\*.png" "$Layout\Assets"
New-MsixManifest $Layout
if ($ManifestOnly) { Write-Host "Manifest OK: $Layout\AppxManifest.xml (version $(ConvertTo-MsixVersion $Version))"; return }

Push-Location $root
try {
  & "$PSScriptRoot\build.ps1" -Version $Version -Store -Out "$Layout\Xegesis.exe"
  Copy-Item "$root\LICENSE-MIT", "$root\LICENSE-APACHE" $Layout
  $out = "$root\build\bin\Xegesis-$Version-windows-x64.msix"
  New-Item -ItemType Directory -Force (Split-Path $out) | Out-Null
  & (Find-MakeAppx) pack /d $Layout /p $out /o
  if ($LASTEXITCODE -ne 0) { throw 'makeappx pack failed' }
  Write-Host "Built $out (unsigned: the Store signs it)"
} finally { Pop-Location }
