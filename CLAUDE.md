# cloudssh

k9s-style terminal UI (Go, Bubble Tea) to browse cloud resources (AWS first) and SSH into them.

## Contributing in one minute
- `make check` runs everything CI runs, in Docker (nothing to install). `./bin/cloudssh --demo` runs the app on sample data with no cloud account.
- Ask your assistant in plain words ("add GCP", "filter instances by launch template"); the skills below hold the step-by-step guides. Humans: see CONTRIBUTING.md.

## Layout
- `cmd/cloudssh` – entrypoint only
- `internal/cli` – cobra root command, flag/config wiring
- `internal/config` – YAML config + defaults
- `internal/provider` – cloud-agnostic contract (`Provider`, `Resource`, optional `Drillable`/`HostResolver`/`Authenticator`/`Authorizer`/`Demoer`); `provider/aws` is the AWS impl; `provider/providertest` is the conformance suite every cloud must pass
- `internal/cloud` – registry of cloud factories; each `provider/<cloud>` registers itself from `init()`, `internal/cli/clouds.go` imports them
- `internal/e2e` – drives the real UI over the demo data (`cloudssh --demo`). Its `TestMain` swaps the UI's timers (`ui.SetTick`) for deterministic ones; new UI timers must go through `tick(...)` in `internal/ui`, never `tea.Tick`, or e2e tests turn flaky on slow CI runners
- `internal/ui` – Bubble Tea model; depends only on provider interfaces
- `internal/sshx` – builds `ssh` invocations; the TUI suspends (`tea.ExecProcess`) and execs system `ssh`. Never embed a terminal emulator.

## Rules
- **Generic layers stay cloud-agnostic.** `internal/ui`, `internal/cli`, `internal/config`, `internal/sshx` and the provider contract must not name a cloud (comments aside). Cloud-specific text, defaults and behaviour live in the cloud's package; clouds are picked through `internal/cloud`.
- **Demo data is part of the feature.** A resource/column/filter/drill/API-call change updates the demo data and keeps `providertest.Conformance` and `internal/e2e` green (`.claude/skills/demo-data`). Demo mode never runs real ssh or touches the network.
- **No ctrl-key chords in the UI;** dialogs are tab/arrow driven (`.claude/skills/ui-keybindings`).
- **All tooling runs in Docker** (`make test|lint|fmt|vet|check|run`). Don't install toolchains on the host.
- Every package ships tests (`*_test.go`); new behaviour needs a test in the same commit. UI tests use a fake provider, AWS tests test pure mapping funcs; real-AWS tests carry `//go:build integration`.
- **Verify before you hand over.** `make check` must pass (fmt, vet, lint, race tests, conformance and e2e). Run `make build` and try `./bin/cloudssh --demo` when behaviour or UI changed. The target platform comes from `local.mk`/env (README "Build target"), never from tracked files. An optional Stop hook that keeps `./bin/cloudssh` fresh lives in `.claude/hooks/` (opt in via your own `.claude/settings.local.json`; see CONTRIBUTING).
- **Never commit or push to `main`.** Work on a branch (`feat/…`, `fix/…`, `chore/…`), push the branch and open a pull request; `main` only changes by merging a PR (squash, PR title = Conventional Commit). Never push unless asked, and never force-push shared branches.
- **Small logical commits**, Conventional Commits (`feat(ec2): …`, `fix(ui): …`, `ci: …`). Never a `Claude-Session:` line or session link in commits or PRs.
- Releases are maintainer-only and automated by release-please: merged PRs accumulate in a Release PR (next semver + changelog from Conventional Commits `feat` minor, `fix` patch, `!`/`BREAKING CHANGE` major); merging it tags and `.github/workflows/release.yml` publishes binaries, the signed `checksums.txt` and the Homebrew formula via GoReleaser. Contributors never tag or edit versions or `CHANGELOG.md`.
- **Pin the current latest.** Before writing or editing a `uses:` line, a tool version in a Dockerfile or a `go.mod` requirement, look up the latest release (`gh api repos/<owner>/<repo>/releases/latest`, `go list -m -u`) instead of recalling a version; pin actions by commit SHA with a `# vX.Y.Z` comment and keep related steps (all `github/codeql-action/*`) on one version. Stale pins cause waves of Dependabot PRs.
- Never commit credentials, `.pem` or `.env` files. CI secrets (`AWS_ROLE_ARN` for integration tests, `HOMEBREW_TAP_TOKEN` for releases) are maintainer-managed and never needed to contribute.
- Adding a cloud/resource: see `.claude/skills/add-resource`. Other skills: `add-cloud` (new cloud), `demo-data` (sample data, no drift), `add-filter`, `ui-keybindings`, `tui-preview` (render screens as text), `update-demo-gif` (re-record the README GIF), `ship-changes` (commit/push flow).
