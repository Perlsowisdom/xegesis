# Signs a COPY of the MSIX with a throwaway self-signed certificate so it can be installed on a test
# PC. This is only for trying the package; the Store signs the real one at certification, and the
# unsigned .msix is what you upload to Partner Center.
#   scripts\test-sign-msix.ps1 -Package build\bin\Xegesis-1.2.3-windows-x64.msix
# Then, in an administrator PowerShell, trust the certificate and install:
#   Import-Certificate build\bin\xegesis-test.cer -CertStoreLocation Cert:\LocalMachine\TrustedPeople
#   Add-AppxPackage build\bin\Xegesis-test-signed.msix
# Remove afterwards: Get-AppxPackage *Xegesis* | Remove-AppxPackage, and delete the certificate from
# TrustedPeople. Needs signtool.exe (Windows SDK).
# The certificate's subject must equal the manifest's Publisher: package with the same
# XEGESIS_MSIX_PUBLISHER (default CN=PLACEHOLDER-PUBLISHER-ID).
param(
  [Parameter(Mandatory)][string]$Package,
  [string]$Publisher = $(if ($env:XEGESIS_MSIX_PUBLISHER) { $env:XEGESIS_MSIX_PUBLISHER } else { 'CN=PLACEHOLDER-PUBLISHER-ID' })
)
$ErrorActionPreference = 'Stop'
$dir = Split-Path (Resolve-Path $Package)
$signed = Join-Path $dir 'Xegesis-test-signed.msix'
$pfx = Join-Path $dir 'xegesis-test.pfx'
$cer = Join-Path $dir 'xegesis-test.cer'

function Find-SignTool {
  $c = Get-Command signtool.exe -ErrorAction SilentlyContinue
  if ($c) { return $c.Source }
  $kits = "${env:ProgramFiles(x86)}\Windows Kits\10\bin"
  $f = if (Test-Path $kits) { Get-ChildItem $kits -Recurse -Filter signtool.exe | Where-Object { $_.FullName -match '\\x64\\' } | Sort-Object FullName -Descending | Select-Object -First 1 }
  if ($f) { return $f.FullName }
  throw 'signtool.exe not found. Install the Windows SDK.'
}

$tool = Find-SignTool
# A one-off key: the password only protects the .pfx file lying next to the package.
$pass = [Guid]::NewGuid().ToString('N')
$cert = New-SelfSignedCertificate -Type Custom -Subject $Publisher -KeyUsage DigitalSignature -FriendlyName 'Xegesis test signing' `
  -CertStoreLocation 'Cert:\CurrentUser\My' -TextExtension @('2.5.29.37={text}1.3.6.1.5.5.7.3.3', '2.5.29.19={text}')
try {
  Export-PfxCertificate -Cert $cert -FilePath $pfx -Password (ConvertTo-SecureString $pass -AsPlainText -Force) | Out-Null
  Export-Certificate -Cert $cert -FilePath $cer | Out-Null
  Copy-Item $Package $signed -Force
  & $tool sign /fd SHA256 /a /f $pfx /p $pass $signed
  if ($LASTEXITCODE -ne 0) { throw 'signtool sign failed' }
} finally {
  Remove-Item "Cert:\CurrentUser\My\$($cert.Thumbprint)" -ErrorAction SilentlyContinue
  Remove-Item $pfx -ErrorAction SilentlyContinue
}
Write-Host "Signed copy: $signed`nCertificate to trust: $cer"
