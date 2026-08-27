# Repository instructions

Taolu is a public `kungfu-systems` repository. Keep all public repository content in English, use isolated worktrees and feature branches, preserve product-repository zero coupling, and use Buildchain v4 as the only build and delivery control plane.

Do not add arbitrary executable adapter hooks, checksum bypasses, unsafe archive extraction, sudo escalation, shell-profile edits, implicit PATH changes, or direct protected-branch writes. Run the lifecycle declared in `.buildchain/buildchain.toml`, sign commits off for DCO, and deliver through protected pull requests.
