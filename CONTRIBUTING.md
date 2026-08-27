# Contributing

Use an isolated worktree and a feature branch based on `dev/v1/v1.0`. Commits must follow Conventional Commits and include a DCO sign-off.

All changes must preserve these invariants:

- product repositories remain unaware of Taolu;
- adapters remain finite, declarative, and version-bounded;
- release and artifact identities are exact and content-bound;
- extraction cannot escape the staged destination or materialize links;
- activation is transactional and foreign ownership is never overwritten;
- `.buildchain/buildchain.toml` is the single real lifecycle declaration.

Before opening a pull request, run `go test ./...`, `go vet ./...`, `bash -n bootstrap/install.sh`, and `git diff --check`. The protected Buildchain v4 workflows remain authoritative.
