---
status: draft
period: 2026-08-27
theme: taolu-site-adoption
doc_type: runbook
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
  invisible_context_boundary: Did not inspect credentials, private release assets, or live Site configuration.
---

# Site adoption

A Site adopts Taolu without changing a product repository:

1. Install an exact admitted Taolu release. On POSIX systems the public entrypoint is `curl -fsSL <exact-release>/taolu-install.sh | sh`; Windows downloads and executes `taolu-install.ps1`.
2. Record the product's exact public GitHub Release metadata and official SHA-256 values in the Site.
3. Update the Site-owned single-product `taolu.catalog/v1` document.
4. Run `taolu installer --config catalog.json --releases releases.json --bundle-url <final-bundle-url> --out installer`.
5. Publish the generated `bundle.json`, `install.sh`, and `install.ps1` from the Site. The scripts already bind the product ID and bundle root.

`taolu compile` remains the lower-level deterministic bundle command. Normal consumers use `taolu installer`.

Do not place Taolu, its workflow, or its generated files in a product repository. A Taolu bug fix changes the Site's admitted Taolu release and regenerates the Site publication unit only; it does not require rebuilding the product release.
