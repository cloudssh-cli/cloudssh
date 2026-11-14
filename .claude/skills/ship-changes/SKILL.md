---
name: ship-changes
description: Commit cloudssh work as small Conventional Commits on a branch and open a pull request. Use whenever the user asks to commit/push, or after finishing a feature when they said to ship as you go. Never commits to main.
---

Rules come from CLAUDE.md: small logical commits, Conventional Commits, **no Co-Authored-By trailer**, never push unless asked (the user may grant "push as you go").

1. `make check` first (fmt, vet, lint, tests), then `make build` so `./bin/cloudssh` matches what you are about to commit (the Stop hook also does this). Also `docker compose run --rm dev go vet -tags integration ./...` after touching `internal/provider/aws` (that build tag is not covered by `make check`).
2. Never commit on `main`. If you are on it, create a branch first (`git switch -c feat/<topic>`; prefixes `feat/ fix/ chore/ docs/`). The PR title must be a Conventional Commit: it becomes the squash commit and the changelog line.
3. Group by concern, in dependency order: provider contract → provider impl (+ its tests) → config/cli → ui → build → docs. Tests ship in the same commit as the behaviour. Scope examples: `feat(provider)`, `feat(aws)`, `feat(ui)`, `fix(ui)`, `build(make)`, `docs`.
4. Run `git add <paths>` and `git commit -m ... -m ...` as short separate commands. Very long chained `&&` one-liners with embedded newlines get denied by the permission layer; plain commands pass.
5. Never commit throwaway files (`internal/ui/zz_preview_test.go`), credentials or `.pem` files.
6. Push the branch (`git push -u origin <branch>`) and open the PR with `gh pr create` only when the user asks. If a push is blocked by a permission rule, do not work around it or edit permissions: report it and give the user the `! git push -u origin <branch>` command. Merging is the user's call; release-please batches merged PRs into the Release PR.
