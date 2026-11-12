---
name: add-cloud
description: Add support for a new cloud (GCP, Azure, Hetzner, ...) to cloudssh, with demo data and tests. Use when asked to support a cloud that is not under internal/provider yet.
---

A cloud is one package, `internal/provider/<name>`, that registers itself. The UI, CLI and config never import it directly; they only know `internal/provider` (contract) and `internal/cloud` (registry). Read `internal/provider/provider.go` first: its package doc lists what is required and what is optional.

## Steps

1. **Package layout** (copy the shapes from `internal/provider/aws`):
   - `<name>.go`: the `Provider` type, `Open(ctx, provider.Options)`, a client struct.
   - `register.go`: `func init() { cloud.Register(cloud.Cloud{Name: "<name>", Description: "...", Open: Open}) }` (lower-case name).
   - one file per resource kind, `filters.go`, `auth.go`, `demo*.go`.
2. **Required contract:** `provider.Provider` (`Name`, `Resources` in shortcut order 1..9) and `provider.Resource` for each resource. Follow `.claude/skills/add-resource` for each resource; drill-only children are built by their parent's `Drill`.
3. **Optional capabilities** (type-asserted by the UI, implement what the cloud has): `Drillable`, `HostResolver` (fill `Host.DefaultUser` from the image), `Authenticator` (profiles/sign-in), `Authorizer` (short-lived keys), `Demoer`.
4. **Never fail because the user is not signed in.** `Open` must return a provider whose `AuthError()` carries a friendly, non-blaming message (headline first line, guidance after) so the UI can start and offer the Profiles screen. Keep the real SDK error last as `details:`.
5. **Put SDK clients behind small interfaces** (see `aws/api.go`): list only the methods you call. This is what lets the demo fake the cloud without touching production code, and it makes new API calls impossible to add without updating the demo.
6. **Register:** add one blank import to `internal/cli/clouds.go`. Nothing else in cli/ui changes. If you find yourself editing `internal/ui`, `internal/cli` logic, `internal/config` or `internal/sshx` with cloud-specific names, stop: the generic layers must stay cloud-agnostic (the only cloud-specific code outside your package is the parser list in `sshx/tunnels.go`, if the cloud has a tunnel CLI).
7. **Demo data and conformance:** follow `.claude/skills/demo-data`. The cloud is not done until `providertest.Conformance(t, NewDemo(8), ...)` passes and `cloudssh --cloud <name> --demo` works.
8. **Docs:** a row in the README (what the cloud offers, credentials it uses), the layout line in `CLAUDE.md` if you add a package kind.
9. **Verify:** `make check`, `make build`, then run `--cloud <name> --demo`. Commit in small Conventional Commits (`feat(<name>): ...`), see `.claude/skills/ship-changes`.

## Self-check before you finish

```sh
grep -rniE "<your-cloud-words>" internal/ui internal/cli internal/config internal/provider/provider.go   # expect nothing but comments/examples
```
