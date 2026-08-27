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

1. Record the exact public GitHub Release metadata and official SHA-256 values.
2. Update the Site-owned `taolu.catalog/v1` document.
3. Run `taolu compile --catalog catalog.json --releases releases.json --out bundle.json` twice and require identical bytes.
4. Publish `bundle.json` plus a configured copy of `bootstrap/install.sh` and `bootstrap/install.ps1` from the Site.
5. Pin `TAOLU_VERSION`, the platform-specific Taolu digest, `TAOLU_BUNDLE_URL`, the compiled `TAOLU_BUNDLE_ROOT`, and `TAOLU_PRODUCT` in the Site response.

Do not place those files or settings in a product repository. A Taolu bug fix changes the Site's Taolu version and runtime digest only; it does not require rebuilding the product release.
