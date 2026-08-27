# SPDX-License-Identifier: Apache-2.0
$ErrorActionPreference = "Stop"
foreach ($name in @("TAOLU_VERSION", "TAOLU_SHA256", "TAOLU_BUNDLE_URL", "TAOLU_BUNDLE_ROOT", "TAOLU_PRODUCT")) {
  if (-not [Environment]::GetEnvironmentVariable($name)) { throw "Set $name to an exact site-owned value" }
}
if (-not [Environment]::Is64BitOperatingSystem) { throw "Taolu requires a 64-bit operating system" }
$platform = if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") { "windows-arm64" } else { "windows-x64" }
$root = if ($env:TAOLU_HOME) { $env:TAOLU_HOME } else { Join-Path $HOME ".taolu" }
$cache = Join-Path $root "bootstrap/$env:TAOLU_VERSION"
$binary = Join-Path $cache "taolu.exe"
$bundle = Join-Path $cache "bundle.json"
$baseUrl = if ($env:TAOLU_RELEASE_BASE_URL) { $env:TAOLU_RELEASE_BASE_URL } else { "https://github.com/kungfu-systems/taolu/releases/download/v$env:TAOLU_VERSION" }
New-Item -ItemType Directory -Force -Path $cache | Out-Null
if (Test-Path $binary) {
  $actual = (Get-FileHash -Algorithm SHA256 $binary).Hash.ToLowerInvariant()
  if ($actual -ne $env:TAOLU_SHA256.ToLowerInvariant()) { throw "Taolu cached runtime digest mismatch" }
} else {
  Invoke-WebRequest -UseBasicParsing -Uri "$baseUrl/taolu-$platform.exe" -OutFile "$binary.tmp"
  $actual = (Get-FileHash -Algorithm SHA256 "$binary.tmp").Hash.ToLowerInvariant()
  if ($actual -ne $env:TAOLU_SHA256.ToLowerInvariant()) { throw "Taolu runtime digest mismatch" }
  Move-Item -Force "$binary.tmp" $binary
}
Invoke-WebRequest -UseBasicParsing -Uri $env:TAOLU_BUNDLE_URL -OutFile "$bundle.tmp"
Move-Item -Force "$bundle.tmp" $bundle
& $binary install --bundle $bundle --bundle-root $env:TAOLU_BUNDLE_ROOT --product $env:TAOLU_PRODUCT --platform $platform --root $root
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
