# cloudssh

k9s-style terminal UI (Go, Bubble Tea) to browse cloud resources (AWS first) and SSH into them.

## Layout
- `cmd/cloudssh` – entrypoint only
- `internal/cli` – cobra root command, flag/config wiring
- `internal/config` – YAML config + defaults
- `internal/provider` – cloud-agnostic contract (`Provider`, `Resource`, optional `Drillable`/`HostResolver`/`Authenticator`/`Authorizer`/`Demoer`); `provider/aws` is the AWS impl; `provider/providertest` is the conformance suite every cloud must pass
- `internal/cloud` – registry of cloud factories; each `provider/<cloud>` registers itself from `init()`, `internal/cli/clouds.go` imports them
- `internal/e2e` – drives the real UI over the demo data (`cloudssh --demo`)
- `internal/ui` – Bubble Tea model; depends only on provider interfaces
- `internal/sshx` – builds `ssh` invocations; the TUI suspends (`tea.ExecProcess`) and execs system `ssh`. Never embed a terminal emulator.

## Rules
- **Generic layers stay cloud-agnostic.** `internal/ui`, `internal/cli`, `internal/config`, `internal/sshx` and the provider contract must not name a cloud (comments aside). Cloud-specific text, defaults and behaviour live in the cloud's package; clouds are picked through `internal/cloud`.
- **Demo data is part of the feature.** A resource/column/filter/drill/API-call change updates the demo data and keeps `providertest.Conformance` and `internal/e2e` green (`.claude/skills/demo-data`). Demo mode never runs real ssh or touches the network.
- **No ctrl-key chords in the UI;** dialogs are tab/arrow driven (`.claude/skills/ui-keybindings`).
- **All tooling runs in Docker** (`make test|lint|fmt|vet|check|run`). Don't install toolchains on the host.
- Every package ships tests (`*_test.go`); new behaviour needs a test in the same commit. UI tests use a fake provider, AWS tests test pure mapping funcs; real-AWS tests carry `//go:build integration`.
- **Always leave a fresh build.** After finishing any code change, run `make check` and then `make build`, so `./bin/cloudssh` is built from the latest code whenever the user tries it. A Stop hook (`.claude/hooks/ensure-build.sh`) enforces this: it rebuilds when sources are newer than the binary and blocks finishing if the build fails. The target platform comes from `local.mk`/env (see README "Build target"), never from tracked files.
- **Never commit or push to `main`.** Work on a branch (`feat/…`, `fix/…`, `chore/…`), push the branch and open a pull request; `main` only changes by merging a PR (squash, PR title = Conventional Commit). Never push unless asked, and never force-push shared branches.
- **Small logical commits**, Conventional Commits (`feat(ec2): …`, `fix(ui): …`, `ci: …`). No `Co-Authored-By` trailers.
- Releases: automated by release-please. Merges to `main` keep a Release PR (next semver + changelog from Conventional Commits) up to date; merging it tags and runs `.github/workflows/release.yml`, where GoReleaser attaches linux/darwin x amd64/arm64 archives + checksums to the GitHub Release and updates the Homebrew tap (`cloudssh-cli/homebrew-tap`, token `HOMEBREW_TAP_TOKEN`). Commit types decide the bump (`feat` minor, `fix` patch, `!`/BREAKING major). Never tag by hand; `scripts/install.sh` is the curl installer. CI (`ci.yml`) is still parked (manual workflow only).
- Secrets used in CI: `AWS_ROLE_ARN` (OIDC, integration tests), `HOMEBREW_TAP_TOKEN` (release; PAT with contents:write on the tap repo). Never commit credentials or `.pem` files.
- Adding a cloud/resource: see `.claude/skills/add-resource`. Other skills: `add-cloud` (new cloud), `demo-data` (sample data, no drift), `add-filter`, `ui-keybindings`, `tui-preview` (render screens as text), `ship-changes` (commit/push flow).
