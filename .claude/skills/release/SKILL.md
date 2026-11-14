---
name: release
description: Plan and cut a cloudssh release with semantic versioning. Releases are automated by release-please; use to explain or sanity-check the next version / notes.
---

Status: release-please opens/updates a Release PR on every merge to `main`; merging it tags and `.github/workflows/release.yml` runs GoReleaser (`.goreleaser.yaml`) to publish binaries and the Homebrew formula. Never tag or push yourself; tell the user to merge the Release PR.

1. `git fetch --tags`; find the last tag (`git describe --tags --abbrev=0`; none -> `v0.1.0` would be first).
2. Read commits since it. Bump: `!`/`BREAKING CHANGE` -> major (pre-1.0: minor); `feat` -> minor; else patch.
3. Ensure `make check` passes (Docker) and CI is green.
4. Print (don't run) the tag command for the user.
