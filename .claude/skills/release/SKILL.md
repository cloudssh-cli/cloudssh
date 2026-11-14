---
name: release
description: Explain or sanity-check the next cloudssh release (version, notes). Releases are automated by release-please and done by maintainers; use this to understand or preview, not to tag.
---

How releases work: every merge to `main` updates a bot-opened **Release PR** (next semver + changelog from Conventional Commits). A maintainer merges it when ready; that tags the release and `.github/workflows/release.yml` runs GoReleaser (`.goreleaser.yaml`) to publish archives, the cosign-signed `checksums.txt`, SBOMs and the Homebrew formula. Nobody tags or edits `CHANGELOG.md` by hand.

To preview or check a release:

1. `git fetch --tags`; last tag: `git describe --tags --abbrev=0` (none yet means the Release PR proposes the first version).
2. Read the commits since it. Bump: `!`/`BREAKING CHANGE` -> major (pre-1.0: minor); `feat` -> minor; everything else -> patch. Commit types `docs`, `test`, `ci`, `build`, `chore`, `refactor` alone do not trigger a release.
3. Check `make check` passes and CI is green.
4. Open the Release PR and compare its version/notes with step 2. Report differences; never tag, push, or edit the PR's version yourself.

To force a version, a maintainer adds an empty commit with the footer `Release-As: X.Y.Z`.
