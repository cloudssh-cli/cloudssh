# Contributing

cloudssh is meant to be extended by everyone, including with an AI assistant. The repo ships the instructions an assistant needs: `CLAUDE.md` (rules) and `.claude/skills/` (step-by-step guides). Open the repo in Claude Code and ask in plain words, for example:

- "Add GCP support" -> `add-cloud`
- "Show Cloud SQL instances for AWS" -> `add-resource`
- "Filter instances by launch template" -> `add-filter`
- "Add a key to do X" -> `ui-keybindings`

Every guide ends the same way, so a change is safe to merge without a maintainer reading every line:

1. `make check` passes (fmt, vet, golangci-lint, race tests). This includes the **conformance suite** (`internal/provider/providertest`) that checks any cloud against the contract, the demo-data consistency tests, and the end-to-end tests in `internal/e2e` that drive the real UI over `--demo` data.
2. `make build`, then try it: `./bin/cloudssh --demo` (no cloud account needed; ssh opens a harmless local shell).
3. Work on a branch and open a pull request; `main` is only changed by merged PRs. Use small logical commits in [Conventional Commits](https://www.conventionalcommits.org) style (`feat(gcp): ...`, `fix(ui): ...`); tests ship in the same commit as the behaviour.

## Working by hand

- Everything runs through Docker (`make check|test|lint|fmt|vet|build|run`), nothing to install on your machine.
- A cloud is one package under `internal/provider/<name>` that registers itself with `internal/cloud`; the UI and CLI never import it. The generic layers (`ui`, `cli`, `config`, `sshx`, the provider contract) must not mention a cloud.
- Demo data is built from the cloud SDK's own types and read by the real provider code through small client interfaces, so it cannot drift. Names must look real (the demo is recorded for the README GIF).
- Integration tests (`-tags integration`) hit a real account and are run only manually or on trusted branches.
- Never commit credentials, `.pem`/`.key` files or `.env` files.
