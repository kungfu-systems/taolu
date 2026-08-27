# Taolu

Taolu is a Site-owned distribution compiler and cross-platform installer runtime for products published as ordinary GitHub Releases.

Product repositories do not depend on Taolu. They keep publishing their own release assets. A Site pins Taolu plus a declarative product catalog, compiles immutable installation bundles, and serves thin POSIX and PowerShell bootstraps. Updating Taolu therefore requires a Site change only, even when a product release takes days to produce.

## What 1.0 provides

- Deterministic `taolu.catalog/v1` + pinned GitHub Release metadata compilation.
- A rooted `taolu.bundle/v1` binding release ID, asset ID, URL, digest, platform, extraction contract, and runtime compatibility.
- A finite `exact-asset/v1` adapter; no catalog-provided executable hooks.
- HTTPS/file download, resumable partial HTTPS downloads, content-addressed cache, SHA-256 verification, traversal/symlink-safe ZIP and tar.gz extraction, owned install roots, staged activation, current/previous state, rollback, status, and JSON receipts.
- Thin `install.sh` and `install.ps1` bootstraps with no sudo, shell-profile edit, or implicit PATH mutation.
- Buildchain v4 as the sole build, verification, evidence, and release control plane.

Release publication is a promote-only transaction over the PR-stage candidate.
Buildchain's declarative Provider Plane creates and reads back the immutable
GitHub Release, emits the Release Passport, and retains the rooted transaction
state. Taolu does not own a release-side shell publisher.

## Quick start

```bash
go run ./cmd/taolu compile \
  --catalog testdata/catalog.json \
  --releases testdata/releases.json \
  --out /tmp/taolu-bundle.json

go test ./...
```

The examples under `examples/` show how the current single-product Kungfu Site and multi-product libkungfu.dev Site shapes map to Site-owned catalogs. Placeholder digests must be replaced by official release digests before compilation.

Read [the architecture](docs/architecture.md) and [Site adoption contract](docs/adoption.md) before integrating it. Taolu 1.0 does not migrate either live Site repository; it provides the independent contract needed for that later Site-only change.

## Trust boundary

Taolu consumes exact public release facts. It does not create product tags, mutate product releases, infer missing checksums, run remote hooks, overwrite foreign install roots, or bypass platform and archive validation. The GitHub source repository is not itself a distribution channel; use the checksummed assets from a protected Taolu release.

Licensed under Apache-2.0.
