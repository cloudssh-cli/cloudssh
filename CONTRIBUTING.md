# Contributing

- Develop through Docker: `make check` must pass (fmt, vet, golangci-lint, `go test -race`).
- Every change ships with tests in the same commit.
- Use small, logical commits following [Conventional Commits](https://www.conventionalcommits.org) (`feat(ec2): …`, `fix(ui): …`). Conventional Commits will drive the changelog and semver once releases are set up.
- Adding a resource or cloud: see `.claude/skills/add-resource/SKILL.md` (also a good human checklist).
- Integration tests (`-tags integration`) hit a real AWS account; CI runs them only on trusted branches via OIDC.
