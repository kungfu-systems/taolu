# Taolu

Taolu is a Site-owned distribution compiler and cross-platform installer runtime for products published as ordinary GitHub Releases.

Product repositories do not depend on Taolu. They keep publishing their own release assets. A Site pins Taolu plus a declarative product catalog, compiles immutable installation bundles, and serves thin POSIX and PowerShell bootstraps. Updating Taolu therefore requires a Site change only, even when a product release takes days to produce.

## What 1.0 provides

- Deterministic `taolu.catalog/v1` + pinned GitHub Release metadata compilation.
- A rooted `taolu.bundle/v1` binding release ID, asset ID, URL, digest, platform, extraction contract, and runtime compatibility.
- A finite `exact-asset/v1` adapter; no catalog-provided executable hooks.
- HTTPS/file download, resumable partial HTTPS downloads, content-addressed cache, SHA-256 verification, traversal/symlink-safe ZIP and tar.gz extraction, owned install roots, staged activation, current/previous state, rollback, status, and JSON receipts.
- Checksummed `curl | sh` and PowerShell bootstraps that install the released Taolu CLI with no sudo, shell-profile edit, or implicit PATH mutation.
- `taolu installer`, which turns one Site-owned product configuration and pinned GitHub Release snapshot into a rooted `bundle.json`, `install.sh`, and `install.ps1`.
- Buildchain v4 as the sole build, verification, evidence, and release control plane.

Release publication is a promote-only transaction over the PR-stage candidate.
Buildchain's declarative Provider Plane creates and reads back the immutable
GitHub Release, emits the Release Passport, and retains the rooted transaction
state. Taolu does not own a release-side shell publisher.

## Quick start

```bash
# Use an exact alpha or stable Release URL in production.
curl -fsSL https://github.com/kungfu-systems/taolu/releases/download/v1.0.0/taolu-install.sh | sh

taolu installer \
  --config testdata/catalog.json \
  --releases testdata/releases.json \
  --bundle-url https://install.example/product/bundle.json \
  --out /tmp/product-installer

go test ./...
```

The command emits the complete Site publication unit. A product repository does not import Taolu, add a Taolu workflow, or rebuild when Taolu changes; it continues to publish ordinary GitHub Release assets. The Site owns the configuration and regenerates the installer with whichever released Taolu version it has admitted.

Taolu releases are dogfooded in order. An alpha release must first pass public-URL self-consumption on macOS ARM64, Linux x64, and Windows x64. The tested released binary generates and executes Taolu's own product installer. Stable publication is fail-closed unless that exact alpha evidence is supplied to the Buildchain-controlled release gate.

The examples under `examples/` show how the current single-product Kungfu Site and multi-product libkungfu.dev Site shapes map to Site-owned catalogs. Placeholder digests must be replaced by official release digests before compilation.

Read [the architecture](docs/architecture.md) and [Site adoption contract](docs/adoption.md) before integrating it. Taolu 1.0 does not migrate either live Site repository; it provides the independent contract needed for that later Site-only change.

## Trust boundary

Taolu consumes exact public release facts. It does not create product tags, mutate product releases, infer missing checksums, run remote hooks, overwrite foreign install roots, or bypass platform and archive validation. The GitHub source repository is not itself a distribution channel; use the checksummed assets from a protected Taolu release.

Licensed under Apache-2.0.
