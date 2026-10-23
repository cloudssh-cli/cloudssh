# cloudssh

k9s-style terminal UI (Go, Bubble Tea) to browse cloud resources (AWS first) and SSH into them.
The previous Python/Textual implementation lives on the `main-python` branch.

## Layout
- `cmd/cloudssh` – entrypoint only
- `internal/cli` – cobra root command, flag/config wiring
- `internal/config` – YAML config + defaults
- `internal/provider` – cloud-agnostic interfaces (`Provider`, `Resource`, `HostResolver`); `provider/aws` is the AWS impl
- `internal/ui` – Bubble Tea model; depends only on provider interfaces
- `internal/sshx` – builds `ssh` invocations; the TUI suspends (`tea.ExecProcess`) and execs system `ssh`. Never embed a terminal emulator.

## Rules
- **All tooling runs in Docker** (`make test|lint|fmt|vet|check|run`). Don't install toolchains on the host.
- Every package ships tests (`*_test.go`); new behaviour needs a test in the same commit. UI tests use a fake provider, AWS tests test pure mapping funcs; real-AWS tests carry `//go:build integration`.
- **Small logical commits**, Conventional Commits (`feat(ec2): …`, `fix(ui): …`, `ci: …`). No `Co-Authored-By` trailers. Never push unless asked.
- Not distributing yet: no release/publish pipeline (no GoReleaser, no release workflow). CI only runs checks. Conventional Commits keep semver/changelog possible later.
- Secrets used in CI: `AWS_ROLE_ARN` (OIDC, integration tests). Never commit credentials or `.pem` files.
- Adding a cloud/resource: see `.claude/skills/add-resource`.
