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

To force a version, put the footer `Release-As: X.Y.Z` in the commit message that lands on `main`. A squash merge keeps only the PR title plus the merge body, so a footer in the PR *description* is lost: merge with `gh pr merge <n> --squash --subject "<title> (#<n>)" --body "...\n\nRelease-As: X.Y.Z"` (or add an empty commit with the footer through a PR).

Things that bit us before:
- Tags are plain `vX.Y.Z` (`include-component-in-tag: false` in `release-please-config.json`). A component-prefixed tag such as `cloudssh-v0.1.0` is rejected by GoReleaser; if one appears, delete the release and tag and fix the config.
- Hand-written release notes go in the Release PR body and its branch's `CHANGELOG.md`. Any later commit on `main` regenerates the PR and overwrites them, so merge nothing else in between.
- `build(deps)`/`ci(deps)`/`chore` commits never open a Release PR; `fix(deps)` (Go module bumps) does.
- After the release run: check the 4 archives, 4 SBOMs, `checksums.txt` + `checksums.txt.sigstore.json`, `Formula/cloudssh.rb` in `cloudssh-cli/homebrew-tap`, and `cosign verify-blob` on `checksums.txt` (certificate identity matches `github.com/cloudssh-cli/cloudssh`).
- Merging a PR that edits `.github/workflows/*` needs a token with the `workflow` scope (`gh auth refresh -s workflow`) or the GitHub web UI.
