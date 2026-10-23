---
name: release
description: Plan a cloudssh release with semantic versioning. Release publishing is NOT set up yet - use only to pick the next version / draft notes.
---

Status: the project is not being distributed yet. There is no release workflow or GoReleaser config, so tags publish nothing. Do not add them unless the user asks.

1. `git fetch --tags`; find the last tag (`git describe --tags --abbrev=0`; none -> `v0.1.0` would be first).
2. Read commits since it. Bump: `!`/`BREAKING CHANGE` -> major (pre-1.0: minor); `feat` -> minor; else patch.
3. Ensure `make check` passes (Docker) and CI is green.
4. Print (don't run) the tag command for the user.
