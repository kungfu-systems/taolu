# SPDX-License-Identifier: Apache-2.0
# This file is packaged into each release with @TAOLU_VERSION@ replaced.
$ErrorActionPreference = "Stop"
$version = if ($env:TAOLU_VERSION) { $env:TAOLU_VERSION } else { "@TAOLU_VERSION@" }
if (-not [Environment]::Is64BitOperatingSystem) { throw "Taolu requires a 64-bit operating system" }
$platform = if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") { "windows-arm64" } else { "windows-x64" }
$baseUrl = if ($env:TAOLU_RELEASE_BASE_URL) { $env:TAOLU_RELEASE_BASE_URL } else { "https://github.com/kungfu-systems/taolu/releases/download/v$version" }
$installDir = if ($env:TAOLU_INSTALL_DIR) { $env:TAOLU_INSTALL_DIR } else { Join-Path $HOME ".local/bin" }
$asset = "taolu-$platform.exe"
$temp = Join-Path ([IO.Path]::GetTempPath()) ("taolu-runtime-" + [guid]::NewGuid())
New-Item -ItemType Directory -Force -Path $temp | Out-Null
New-Item -ItemType Directory -Force -Path $installDir | Out-Null
try {
  Invoke-WebRequest -UseBasicParsing -Uri "$baseUrl/$asset.sha256" -OutFile (Join-Path $temp "checksum")
  Invoke-WebRequest -UseBasicParsing -Uri "$baseUrl/$asset" -OutFile (Join-Path $temp $asset)
  $line = (Get-Content (Join-Path $temp "checksum") -Raw).Trim() -split '\s+'
  if ($line.Count -ne 2 -or $line[1] -ne $asset -or $line[0] -notmatch '^[0-9a-fA-F]{64}$') { throw "Invalid Taolu checksum receipt" }
  $actual = (Get-FileHash -Algorithm SHA256 (Join-Path $temp $asset)).Hash.ToLowerInvariant()
  if ($actual -ne $line[0].ToLowerInvariant()) { throw "Taolu runtime digest mismatch" }
  Move-Item -Force (Join-Path $temp $asset) (Join-Path $installDir "taolu.exe")
  $installedVersion = & (Join-Path $installDir "taolu.exe") version
  if ($installedVersion -ne $version) { throw "Taolu runtime version mismatch" }
  Write-Host "Taolu $version installed at $(Join-Path $installDir 'taolu.exe')"
} finally { Remove-Item -Recurse -Force $temp -ErrorAction SilentlyContinue }
