# SPDX-License-Identifier: Apache-2.0
$ErrorActionPreference = "Stop"
foreach ($name in @("TAOLU_DOGFOOD_TAG", "TAOLU_DOGFOOD_PLATFORM", "TAOLU_DOGFOOD_WORK")) {
  if (-not [Environment]::GetEnvironmentVariable($name)) { throw "$name is required" }
}
$tag = $env:TAOLU_DOGFOOD_TAG
$version = $tag.TrimStart("v")
$platform = $env:TAOLU_DOGFOOD_PLATFORM
$work = $env:TAOLU_DOGFOOD_WORK
$base = "https://github.com/kungfu-systems/taolu/releases/download/$tag"
$binDir = Join-Path $work "bin"
New-Item -ItemType Directory -Force -Path $binDir | Out-Null
curl.exe -fsSL "$base/taolu-install.ps1" -o (Join-Path $work "install-taolu.ps1")
$env:TAOLU_INSTALL_DIR = $binDir
& (Join-Path $work "install-taolu.ps1")
$taolu = Join-Path $binDir "taolu.exe"
if ((& $taolu version) -ne $version) { throw "Installed Taolu version mismatch" }

$asset = "taolu-$platform.exe"
$assetPath = Join-Path $work $asset
curl.exe -fsSL "$base/$asset" -o $assetPath
$digest = (Get-FileHash -Algorithm SHA256 $assetPath).Hash.ToLowerInvariant()
$size = (Get-Item $assetPath).Length
$catalog = @{schema="taolu.catalog/v1";taoluVersion=$version;products=@(@{id="taolu";repository="kungfu-systems/taolu";releaseTag=$tag;adapter=@{kind="exact-asset";version=1};platforms=@(@{id=$platform;assetName=$asset;sha256=$digest;archive="file";stripComponents=0;entrypoint=$asset})})}
$releases = @{schema="taolu.github-releases/v1";releases=@(@{repository="kungfu-systems/taolu";tag=$tag;id=1;assets=@(@{id=1;name=$asset;url="$base/$asset";size=$size})})}
$catalog | ConvertTo-Json -Depth 10 | Set-Content -Encoding utf8 (Join-Path $work "catalog.json")
$releases | ConvertTo-Json -Depth 10 | Set-Content -Encoding utf8 (Join-Path $work "releases.json")
$out = Join-Path $work "installer"
$bundlePath = (Join-Path $out "bundle.json").Replace("\", "/")
& $taolu installer --config (Join-Path $work "catalog.json") --releases (Join-Path $work "releases.json") --bundle-url "file:///$bundlePath" --out $out
$env:TAOLU_BIN = $taolu
$env:TAOLU_HOME = Join-Path $work "product-home"
& (Join-Path $out "install.ps1")
& $taolu status --product taolu --root $env:TAOLU_HOME
