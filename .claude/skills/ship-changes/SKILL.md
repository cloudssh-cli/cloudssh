---
name: ship-changes
description: Commit cloudssh work as small Conventional Commits straight on main and push it. Use whenever the user asks to commit/push, or after finishing a feature when they said to ship as you go.
---

Rules come from CLAUDE.md: small logical commits, Conventional Commits, **no Co-Authored-By trailer**, never push unless asked (the user may grant "push as you go").

1. `make check` first (fmt, vet, lint, tests). Also `docker compose run --rm dev go vet -tags integration ./...` after touching `internal/provider/aws` (that build tag is not covered by `make check`).
2. Commit directly on `main`; do not create feature branches (user preference, solo repo).
3. Group by concern, in dependency order: provider contract → provider impl (+ its tests) → config/cli → ui → build → docs. Tests ship in the same commit as the behaviour. Scope examples: `feat(provider)`, `feat(aws)`, `feat(ui)`, `fix(ui)`, `build(make)`, `docs`.
4. Run `git add <paths>` and `git commit -m ... -m ...` as short separate commands. Very long chained `&&` one-liners with embedded newlines get denied by the permission layer; plain commands pass.
5. Never commit throwaway files (`internal/ui/zz_preview_test.go`), credentials or `.pem` files.
6. `git push origin main` may be blocked by a permission rule even when the user asked for it; do not work around it or edit permissions. Report it and give the user `! git push origin main`.
