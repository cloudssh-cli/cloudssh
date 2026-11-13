---
name: release
description: Plan and cut a cloudssh release with semantic versioning. Publishing is tag-driven via GoReleaser; use to pick the next version / draft notes and print the tag commands.
---

Status: pushing a `v*` tag runs `.github/workflows/release.yml` (GoReleaser, `.goreleaser.yaml`) and publishes a GitHub Release plus the Homebrew formula. Never push the tag yourself; print the commands.

1. `git fetch --tags`; find the last tag (`git describe --tags --abbrev=0`; none -> `v0.1.0` would be first).
2. Read commits since it. Bump: `!`/`BREAKING CHANGE` -> major (pre-1.0: minor); `feat` -> minor; else patch.
3. Ensure `make check` passes (Docker) and CI is green.
4. Print (don't run) the tag command for the user.
