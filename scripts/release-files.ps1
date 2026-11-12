# Makes the files of a release in build\release: the setup, the MSIX, SHA256SUMS and latest.json.
#   scripts\release-files.ps1 -Version 1.2.3 -Setup <setup.exe> -Msix <x.msix>
# latest.json (what the app's updater reads) carries the setup's SHA-256 and our Ed25519 signature.
# The private key is read from the XEGESIS_UPDATE_KEY environment variable (a GitHub secret in CI).
# Without it, signing is skipped with a warning and NO latest.json is written, so no update is
# offered. The signature is checked against the public key committed in the app before it is used,
# which catches a secret that does not belong to that key.
param(
  [Parameter(Mandatory)][string]$Version,
  [Parameter(Mandatory)][string]$Setup,
  [string]$Msix = '',
  [string]$OutDir = '',
  [string]$Repo = $(if ($env:GITHUB_REPOSITORY) { $env:GITHUB_REPOSITORY } else { 'Perlsowisdom/xegesis' }),
  [string]$PubKeyFile = '',
  [string]$Notes = $env:RELEASE_NOTES
)
$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot
if (-not $OutDir) { $OutDir = "$root\build\release" }
if (-not $PubKeyFile) { $PubKeyFile = "$root\internal\update\pubkey.txt" }
New-Item -ItemType Directory -Force $OutDir | Out-Null
$files = @(Copy-Item $Setup $OutDir -PassThru)
if ($Msix) { $files += Copy-Item $Msix $OutDir -PassThru }

$lines = foreach ($f in $files) { '{0}  {1}' -f (Get-FileHash $f.FullName -Algorithm SHA256).Hash.ToLower(), $f.Name }
[IO.File]::WriteAllText("$OutDir\SHA256SUMS", (($lines -join "`n") + "`n"), (New-Object Text.UTF8Encoding $false))

$setupFile = $files[0]
if (-not $env:XEGESIS_UPDATE_KEY) {
  Write-Host "::warning::XEGESIS_UPDATE_KEY is not set: the release is not signed and latest.json was not written, so the app will not offer this version as an update."
  return
}
Push-Location $root
try {
  $tool = "$root\build\bin\xegesis-sign.exe"
  go build -o $tool ./cmd/xegesis-sign
  if ($LASTEXITCODE) { throw 'building xegesis-sign failed' }
} finally { Pop-Location }
$sha = (& $tool hash $setupFile.FullName).Trim()
$sig = (& $tool sign -version $Version $setupFile.FullName).Trim()
if ($LASTEXITCODE) { throw 'signing failed' }
& $tool verify -version $Version -pub $PubKeyFile $setupFile.FullName $sig
if ($LASTEXITCODE) { throw "The signature does not verify against $PubKeyFile. XEGESIS_UPDATE_KEY is not the key whose public half is committed there (docs/releasing.md)." }

$manifest = [ordered]@{
  version   = $Version
  url       = "https://github.com/$Repo/releases/download/v$Version/$($setupFile.Name)"
  sha256    = $sha
  signature = $sig
  notes     = if ($Notes) { $Notes } else { '' }
}
[IO.File]::WriteAllText("$OutDir\latest.json", (($manifest | ConvertTo-Json) + "`n"), (New-Object Text.UTF8Encoding $false))
Write-Host "Wrote $OutDir\latest.json for $Version"
