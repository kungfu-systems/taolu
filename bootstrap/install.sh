#!/bin/bash
# SPDX-License-Identifier: Apache-2.0
set -eu

: "${TAOLU_VERSION:?set TAOLU_VERSION to an exact released version}"
: "${TAOLU_SHA256:?set TAOLU_SHA256 to the published binary digest}"
: "${TAOLU_BUNDLE_URL:?set TAOLU_BUNDLE_URL to the site-owned bundle URL}"
: "${TAOLU_BUNDLE_ROOT:?set TAOLU_BUNDLE_ROOT to the published sha256 bundle root}"
: "${TAOLU_PRODUCT:?set TAOLU_PRODUCT to the site-owned product id}"

case "$(uname -s)-$(uname -m)" in
  Linux-x86_64) platform=linux-x64 ;;
  Linux-aarch64|Linux-arm64) platform=linux-arm64 ;;
  Darwin-arm64) platform=darwin-arm64 ;;
  Darwin-x86_64) platform=darwin-x64 ;;
  *) echo "taolu bootstrap: unsupported platform" >&2; exit 1 ;;
esac

root=${TAOLU_HOME:-"$HOME/.taolu"}
cache="$root/bootstrap/$TAOLU_VERSION"
binary="$cache/taolu"
bundle="$cache/bundle.json"
base_url=${TAOLU_RELEASE_BASE_URL:-"https://github.com/kungfu-systems/taolu/releases/download/v$TAOLU_VERSION"}
mkdir -p "$cache"

if [ -f "$binary" ]; then
  actual=$(shasum -a 256 "$binary" | awk '{print $1}')
  [ "$actual" = "$TAOLU_SHA256" ] || { echo "taolu bootstrap: cached runtime digest mismatch" >&2; exit 1; }
else
  curl --fail --location --proto '=https' --tlsv1.2 "$base_url/taolu-$platform" --output "$binary.tmp"
  actual=$(shasum -a 256 "$binary.tmp" | awk '{print $1}')
  [ "$actual" = "$TAOLU_SHA256" ] || { echo "taolu bootstrap: runtime digest mismatch" >&2; exit 1; }
  chmod 0755 "$binary.tmp"
  mv "$binary.tmp" "$binary"
fi
curl --fail --location --proto '=https' --tlsv1.2 "$TAOLU_BUNDLE_URL" --output "$bundle.tmp"
mv "$bundle.tmp" "$bundle"
exec "$binary" install --bundle "$bundle" --bundle-root "$TAOLU_BUNDLE_ROOT" --product "$TAOLU_PRODUCT" --platform "$platform" --root "$root"
