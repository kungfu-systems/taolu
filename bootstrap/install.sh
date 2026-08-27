#!/bin/bash
# SPDX-License-Identifier: Apache-2.0
# This file is packaged into each release with @TAOLU_VERSION@ replaced.
set -eu

version=${TAOLU_VERSION:-@TAOLU_VERSION@}
case "$(uname -s)-$(uname -m)" in
  Linux-x86_64) platform=linux-x64 ;;
  Linux-aarch64|Linux-arm64) platform=linux-arm64 ;;
  Darwin-arm64) platform=darwin-arm64 ;;
  Darwin-x86_64) platform=darwin-x64 ;;
  *) echo "taolu installer: unsupported platform" >&2; exit 1 ;;
esac

base_url=${TAOLU_RELEASE_BASE_URL:-"https://github.com/kungfu-systems/taolu/releases/download/v$version"}
install_dir=${TAOLU_INSTALL_DIR:-"$HOME/.local/bin"}
asset="taolu-$platform"
tmp_dir=$(mktemp -d "${TMPDIR:-/tmp}/taolu-runtime.XXXXXX")
trap 'rm -rf "$tmp_dir"' EXIT HUP INT TERM
mkdir -p "$install_dir"

curl --fail --silent --show-error --location --proto '=https' --tlsv1.2 "$base_url/$asset.sha256" --output "$tmp_dir/checksum"
curl --fail --silent --show-error --location --proto '=https' --tlsv1.2 "$base_url/$asset" --output "$tmp_dir/$asset"
expected=$(awk -v asset="$asset" 'NF == 2 && $2 == asset { print $1 }' "$tmp_dir/checksum")
[ ${#expected} -eq 64 ] || { echo "taolu installer: invalid checksum receipt" >&2; exit 1; }
actual=$(shasum -a 256 "$tmp_dir/$asset" | awk '{print $1}')
[ "$actual" = "$expected" ] || { echo "taolu installer: runtime digest mismatch" >&2; exit 1; }
chmod 0755 "$tmp_dir/$asset"
mv "$tmp_dir/$asset" "$install_dir/taolu"
installed_version=$("$install_dir/taolu" version)
[ "$installed_version" = "$version" ] || { echo "taolu installer: runtime version mismatch" >&2; exit 1; }
echo "Taolu $version installed at $install_dir/taolu" >&2
