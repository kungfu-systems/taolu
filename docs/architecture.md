---
status: draft
period: 2026-08-27
theme: taolu-architecture
doc_type: architecture
source_level: local-files
confidence: high
sensitivity: public
evidence_grade: A
review_state: self-reviewed
last_reviewed: 2026-08-27
ai_provenance:
  model_family: GPT-5
  product: Codex
  generated_at: 2026-08-27
  invisible_context_boundary: Did not inspect credentials, private release assets, or provider secrets.
---

# Architecture

Taolu separates product publication from product installation.

1. Product repositories publish ordinary immutable GitHub Release assets.
2. A Site-owned catalog pins the repository, tag, exact asset names, digests, extraction contract, and Taolu runtime version.
3. `taolu compile` joins that catalog with a pinned GitHub Release metadata snapshot and emits one rooted, deterministic bundle.
4. The Site serves the bundle and thin bootstrap. The bootstrap obtains a digest-pinned Taolu runtime; the runtime downloads and verifies the selected product asset.
5. Installation is owned, staged, atomically activated, receipted, and rollback-capable under one user-writable Taolu root.

There is no product-side Taolu dependency, workflow, configuration, tag, hook, or release action. Adapters are a finite compiled registry. Version 1 supports only `exact-asset/v1`; catalogs cannot execute shell, PowerShell, JavaScript, or remote hooks.

Buildchain v4 is the repository's only build and delivery control plane. The repository declares the real lifecycle once in `.buildchain/buildchain.toml`; GitHub workflows are thin consumers of the public floating v4 workflows and carry exact stable and alpha contract locks.
