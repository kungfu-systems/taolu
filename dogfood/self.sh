#!/bin/bash
# SPDX-License-Identifier: Apache-2.0
set -eu

: "${TAOLU_DOGFOOD_TAG:?exact alpha tag is required}"
: "${TAOLU_DOGFOOD_PLATFORM:?exact platform is required}"
: "${TAOLU_DOGFOOD_WORK:?work directory is required}"

base="https://github.com/kungfu-systems/taolu/releases/download/$TAOLU_DOGFOOD_TAG"
bin_dir="$TAOLU_DOGFOOD_WORK/bin"
mkdir -p "$bin_dir"
curl --fail --silent --show-error --location "$base/taolu-install.sh" |
  TAOLU_INSTALL_DIR="$bin_dir" /bin/bash

version=${TAOLU_DOGFOOD_TAG#v}
[ "$("$bin_dir/taolu" version)" = "$version" ]
asset="taolu-$TAOLU_DOGFOOD_PLATFORM"
curl --fail --silent --show-error --location "$base/$asset" --output "$TAOLU_DOGFOOD_WORK/$asset"
digest=$(shasum -a 256 "$TAOLU_DOGFOOD_WORK/$asset" | awk '{print $1}')
size=$(wc -c < "$TAOLU_DOGFOOD_WORK/$asset" | tr -d ' ')

jq -n --arg version "$version" --arg tag "$TAOLU_DOGFOOD_TAG" --arg platform "$TAOLU_DOGFOOD_PLATFORM" --arg asset "$asset" --arg digest "$digest" '{schema:"taolu.catalog/v1",taoluVersion:$version,products:[{id:"taolu",repository:"kungfu-systems/taolu",releaseTag:$tag,adapter:{kind:"exact-asset",version:1},platforms:[{id:$platform,assetName:$asset,sha256:$digest,archive:"file",stripComponents:0,entrypoint:$asset}]}]}' > "$TAOLU_DOGFOOD_WORK/catalog.json"
jq -n --arg tag "$TAOLU_DOGFOOD_TAG" --arg asset "$asset" --arg url "$base/$asset" --argjson size "$size" '{schema:"taolu.github-releases/v1",releases:[{repository:"kungfu-systems/taolu",tag:$tag,id:1,assets:[{id:1,name:$asset,url:$url,size:$size}]}]}' > "$TAOLU_DOGFOOD_WORK/releases.json"

out="$TAOLU_DOGFOOD_WORK/installer"
"$bin_dir/taolu" installer --config "$TAOLU_DOGFOOD_WORK/catalog.json" --releases "$TAOLU_DOGFOOD_WORK/releases.json" --bundle-url "file://$out/bundle.json" --out "$out"
TAOLU_BIN="$bin_dir/taolu" TAOLU_HOME="$TAOLU_DOGFOOD_WORK/product-home" "$out/install.sh"
"$bin_dir/taolu" status --product taolu --root "$TAOLU_DOGFOOD_WORK/product-home"
